// Package sim runs any resolved station. It knows the device and transport
// primitives but nothing about a specific station.
package sim

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// Errors returned by Place.
var (
	ErrBlocked      = errors.New("the belt must be stopped to place a workpiece")
	ErrOccupied     = errors.New("there is no room for another workpiece there")
	ErrUnknownKind  = errors.New("unknown workpiece kind")
	ErrWrongFamily  = errors.New("this is set up for another kind of workpiece")
	ErrNothingThere = errors.New("there is nothing to put a workpiece on there")
)

type part struct {
	id   int
	kind string
	seg  int
	s    float64
	off  model.Point // from a gripper's tip while it holds this, turned with the tip
	// dir is the way its length points, level and of unit length: in the station, or while a
	// gripper holds it, from the tip, turned with it. It matters for one with a size.
	dir  model.Point
	fall *fall // set once it has left the station
	// loads are what was set onto it, such as a cap, or boxes on a pallet, from the bottom up.
	loads []load
	// marks are what processes left on it, such as a hole or how many coins are in it.
	marks map[string]float64
	gone  bool // joined onto another one, and removed after this step
	// upsideDown is a workpiece that lies with its top down, as a cup with its opening.
	upsideDown bool
}

// load is a workpiece that was set onto another one and goes along with it: its kind, where
// its base is from the other's, along the other's heading, to its left and up, and the way
// it points in that frame.
type load struct {
	kind string
	off  model.Point
	dir  model.Point
}

// work is how far a process is with the workpiece under it.
type work struct {
	id    int // the workpiece, or 0
	timer float64
	done  bool
}

// fall is a workpiece on its way to the floor.
type fall struct {
	at, velocity model.Point
	dir          model.Point // horizontal, the way it tips over
	tumbled      float64     // radians
}

const (
	gravity = 9810.0 // mm/s²
	floorZ  = 0.0
	// Tipping over a belt's end roller throws a workpiece outwards, past the edge of the
	// plate a few mm further on, which a fall straight down would land on.
	tipSpeed   = 260.0 // mm/s
	tumbleRate = 7.0   // radians/s
)

type segment struct {
	*model.SegmentPlan
	up model.Point
}

// Plant is the state of a running station. It is not safe for concurrent use.
type Plant struct {
	plan     *model.Plan
	segments []segment
	v        []float64 // state values, indexed like plan.States
	parts    []part
	nextID   int
	hold     []float64 // remaining off-delay per sensor
	rides    []int     // segments that ride on moving nodes
	sensors  []model.SensorPlan
	dists    []model.DistancePlan // the plan's, moved along where they ride on nodes
	works    []work               // per process
	goals    []float64            // where each positioning axis is driving to, or NaN
	goes     []bool               // whether each axis's go input was on in the last step
	waits    []float64            // how long each logic device has wanted to change
	plcOut   map[int]bool
	failed   []bool // sensor fault per sensor
	forced   map[int]float64
	tips     []model.Point // where each gripper's tip is
	turns    []float64     // how far each gripper's tip has turned about the vertical, in radians
	carrier  map[int]int   // segment that carries a gripper's workpiece, to its gripper
	moves    map[int]moved // what each cylinder's position moves
	limited  map[int]bool  // positions of cylinders that stop on what they press on
	near     [][]int       // obstacles each solid can reach
	nearTip  [][]int       // obstacles a workpiece in each gripper can reach
	stalled  map[int]int   // cylinder position that a workpiece stopped this step, to the workpiece
	hit      []string      // what moved into a workpiece
	everHit  []string      // everything that ran into something since the reset
	cut      bool          // a safety relay is open: the switched outputs are off
	programs []program     // per program device
	struck   []struck      // every fixed part something ran into, for the station checks
	fell     int           // workpieces lost off the station since this was last acknowledged
	before   []float64     // room for the gaps of a move
	kinds    map[string]model.WorkpieceKind
	radius   float64
	height   float64
	diameter float64
	reach    float64 // the radius around the largest kind of workpiece
}

// New returns a plant in its power-on state.
func New(plan *model.Plan) *Plant {
	p := &Plant{
		plan:     plan,
		v:        make([]float64, len(plan.States)),
		hold:     make([]float64, len(plan.Sensors)),
		waits:    make([]float64, len(plan.Logic)),
		works:    make([]work, len(plan.Processes)),
		goals:    make([]float64, len(plan.Axes)),
		goes:     make([]bool, len(plan.Axes)),
		plcOut:   map[int]bool{},
		failed:   make([]bool, len(plan.Sensors)),
		forced:   map[int]float64{},
		tips:     make([]model.Point, len(plan.Grippers)),
		turns:    make([]float64, len(plan.Grippers)),
		carrier:  map[int]int{},
		moves:    map[int]moved{},
		stalled:  map[int]int{},
		kinds:    map[string]model.WorkpieceKind{},
		diameter: plan.Scene.Workpieces.Diameter,
		radius:   plan.Scene.Workpieces.Diameter / 2,
		height:   plan.Scene.Workpieces.Height,
	}
	p.segments = make([]segment, len(plan.Segments))
	p.takeSegments(plan)
	p.reach = p.radius
	for _, k := range plan.Scene.Workpieces.Kinds {
		p.kinds[k.ID] = k
		if len(k.Size) == 2 {
			p.reach = math.Max(p.reach, math.Hypot(k.Size[0], k.Size[1])/2)
		}
	}
	for i := range plan.Grippers {
		p.carrier[plan.Grippers[i].Segment] = i
	}
	for _, o := range plan.PLCOut {
		p.plcOut[o.State] = true
	}
	p.index()
	p.Reset()
	return p
}

// moved is what a cylinder takes along.
type moved struct {
	solids, grippers, pushers []int
}

// index works out what each cylinder moves and which obstacles that can ever reach, as
// nearly all of a station stays out of range of the few parts that move.
func (p *Plant) index() {
	clear(p.moves)
	p.limited = map[int]bool{}
	for _, c := range p.plan.Cylinders {
		if c.Limited {
			p.limited[c.Position] = true
		}
	}
	for s := range p.plan.Solids {
		for _, state := range jointsOf(&p.plan.Solids[s].Chain) {
			m := p.moves[state]
			m.solids = append(m.solids, s)
			p.moves[state] = m
		}
	}
	for g := range p.plan.Grippers {
		for _, state := range jointsOf(&p.plan.Grippers[g].Tip) {
			m := p.moves[state]
			m.grippers = append(m.grippers, g)
			p.moves[state] = m
		}
	}
	for i := range p.plan.Pushers {
		state := p.plan.Pushers[i].Position
		m := p.moves[state]
		m.pushers = append(m.pushers, i)
		p.moves[state] = m
	}
	p.near = make([][]int, len(p.plan.Solids))
	for s := range p.plan.Solids {
		solid := &p.plan.Solids[s]
		corners := []model.Point{}
		for _, x := range []float64{solid.Min.X, solid.Max.X} {
			for _, y := range []float64{solid.Min.Y, solid.Max.Y} {
				for _, z := range []float64{solid.Min.Z, solid.Max.Z} {
					corners = append(corners, model.Point{X: x, Y: y, Z: z})
				}
			}
		}
		p.near[s] = p.reachable(&solid.Chain, corners, 1, solid.Root)
	}
	p.nearTip = make([][]int, len(p.plan.Grippers))
	for g := range p.plan.Grippers {
		grip := &p.plan.Grippers[g]
		pad := 2*p.reach + grip.Reach + p.height
		root, _, _ := strings.Cut(grip.Name, "/")
		root, _, _ = strings.Cut(root, ".")
		p.nearTip[g] = p.reachable(&grip.Tip, []model.Point{{}}, pad, root)
	}
}

func jointsOf(c *model.Chain) []int {
	var states []int
	for i := range c.Links {
		if c.Links[i].Joint && !slices.Contains(states, c.Links[i].State) {
			states = append(states, c.Links[i].State)
		}
	}
	return states
}

// reachable lists the obstacles of other modules within pad of where a chain can take some
// points, trying each of its joints at several positions.
func (p *Plant) reachable(c *model.Chain, points []model.Point, pad float64, root string) []int {
	states := append([]float64(nil), p.v...)
	lo := model.Point{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi := model.Point{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	joints := jointsOf(c)
	var try func(j int)
	try = func(j int) {
		if j < len(joints) {
			for _, f := range []float64{0, 0.25, 0.5, 0.75, 1} {
				states[joints[j]] = f
				try(j + 1)
			}
			return
		}
		for _, point := range points {
			q := c.Apply(point, states)
			lo = model.Point{X: min(lo.X, q.X), Y: min(lo.Y, q.Y), Z: min(lo.Z, q.Z)}
			hi = model.Point{X: max(hi.X, q.X), Y: max(hi.Y, q.Y), Z: max(hi.Z, q.Z)}
		}
	}
	try(0)
	var near []int
	for o := range p.plan.Obstacles {
		ob := &p.plan.Obstacles[o]
		if ob.Root == root {
			continue
		}
		b := obstacleBox(ob)
		a, z := b.bounds()
		if a.X <= hi.X+pad && z.X >= lo.X-pad && a.Y <= hi.Y+pad && z.Y >= lo.Y-pad && a.Z <= hi.Z+pad && z.Z >= lo.Z-pad {
			near = append(near, o)
		}
	}
	return near
}

// upOf returns the surface normal of a segment, for tilting workpieces on it.
func upOf(s *model.SegmentPlan) model.Point {
	if s.Arc || (s.To.X == s.From.X && s.To.Y == s.From.Y) {
		return model.Point{Z: 1}
	}
	d := model.Point{X: (s.To.X - s.From.X) / s.Length, Y: (s.To.Y - s.From.Y) / s.Length, Z: (s.To.Z - s.From.Z) / s.Length}
	up := model.Point{X: -d.Z * d.X, Y: -d.Z * d.Y, Z: 1 - d.Z*d.Z}
	n := math.Sqrt(up.X*up.X + up.Y*up.Y + up.Z*up.Z)
	return model.Point{X: up.X / n, Y: up.Y / n, Z: up.Z / n}
}

// ApplyPlan takes over parameter-dependent values from a plan resolved from the
// same bundle, without disturbing the running state.
// takeSegments takes over the transports and sensors of a plan. One that rides on a moving
// node gets a copy of its own, which follows the node.
func (p *Plant) takeSegments(plan *model.Plan) {
	p.sensors, p.dists = slices.Clone(plan.Sensors), slices.Clone(plan.Distances)
	p.rides = p.rides[:0]
	for i := range plan.Segments {
		s := &plan.Segments[i]
		if s.Rides != nil {
			own := *s
			s = &own
			p.rides = append(p.rides, i)
		}
		p.segments[i] = segment{SegmentPlan: s, up: upOf(s)}
	}
	if p.v != nil {
		p.refreshRides()
	}
}

// refreshRides moves the transports and sensors that ride on nodes to where those nodes are now.
func (p *Plant) refreshRides() {
	for i := range p.sensors {
		if s := &p.sensors[i]; s.Rides != nil {
			s.Origin = s.Rides.Apply(s.Local[0], p.v)
			along := s.Rides.Apply(s.Local[1], p.v)
			s.Dir = model.Point{X: along.X - s.Origin.X, Y: along.Y - s.Origin.Y, Z: along.Z - s.Origin.Z}
		}
	}
	for i := range p.dists {
		if d := &p.dists[i]; d.Rides != nil {
			d.Origin = d.Rides.Apply(d.Local, p.v)
		}
	}
	for _, i := range p.rides {
		s := &p.segments[i]
		s.From, s.To = s.Rides.Apply(s.Local[0], p.v), s.Rides.Apply(s.Local[1], p.v)
		s.up = upOf(s.SegmentPlan)
	}
}

// join is the transport that the end of segment i meets now, where one of the two rides on
// a node, or -1, and where on it a workpiece arrives: below 0 where it crosses a gap, as from
// the open end of a belt into the nest of a rotary indexing table on the next stand.
func (p *Plant) join(i int) (int, float64) {
	if len(p.rides) == 0 {
		return -1, 0
	}
	s := &p.segments[i]
	root := func(name string) string {
		name, _, _ = strings.Cut(name, ".")
		name, _, _ = strings.Cut(name, "/")
		return name
	}
	for j := range p.segments {
		o := &p.segments[j]
		if j == i || (s.Rides == nil && o.Rides == nil) || o.Arc || root(o.Name) == root(s.Name) {
			continue
		}
		if dist(s.To, o.From) <= model.LinkTolerance {
			return j, 0
		}
		if s.Open && !s.Arc && o.Length > 0 {
			dir, other := unitOf(s.From, s.To), unitOf(o.From, o.To)
			rel := model.Point{X: o.From.X - s.To.X, Y: o.From.Y - s.To.Y, Z: o.From.Z - s.To.Z}
			ahead := rel.X*dir.X + rel.Y*dir.Y + rel.Z*dir.Z
			side := math.Hypot(math.Hypot(rel.X-dir.X*ahead, rel.Y-dir.Y*ahead), rel.Z-dir.Z*ahead)
			if dir.X*other.X+dir.Y*other.Y+dir.Z*other.Z >= 0.999 && ahead > 0 && ahead <= model.BridgeGap && side <= model.LinkTolerance {
				return j, -ahead
			}
		}
	}
	return -1, 0
}

// unitOf is the direction from a to b.
func unitOf(a, b model.Point) model.Point {
	d := model.Point{X: b.X - a.X, Y: b.Y - a.Y, Z: b.Z - a.Z}
	l := math.Sqrt(d.X*d.X + d.Y*d.Y + d.Z*d.Z)
	if l == 0 {
		return d
	}
	return model.Point{X: d.X / l, Y: d.Y / l, Z: d.Z / l}
}

func (p *Plant) ApplyPlan(plan *model.Plan) {
	p.plan = plan
	p.takeSegments(plan)
	clear(p.plcOut)
	for _, o := range plan.PLCOut {
		p.plcOut[o.State] = true
	}
	p.index()
}

// Reset removes all workpieces and returns actuators and controls to power-on.
// Parameters and faults are kept.
func (p *Plant) Reset() {
	clear(p.forced)
	clear(p.stalled)
	p.hit = nil
	p.everHit = nil
	p.fell = 0
	p.parts = p.parts[:0]
	p.nextID = 0
	clear(p.v)
	clear(p.hold)
	clear(p.waits)
	clear(p.works)
	clear(p.goes)
	p.startPrograms()
	// The safety circuit is closed at power-on.
	for i := range p.plan.Safety {
		p.v[p.plan.Safety[i].OK] = 1
	}
	for i := range p.goals {
		p.goals[i] = math.NaN()
	}
	for i := range p.plan.Axes {
		if a := &p.plan.Axes[i]; a.Done >= 0 {
			p.v[a.Done] = 1
		}
	}
	for i := range p.plan.Cylinders {
		c := &p.plan.Cylinders[i]
		if c.RestOut {
			p.v[c.Position] = 1
		}
	}
	p.updateCylinders(0)
	p.updateMotors()
	p.updateAxes(0)
	p.refreshRides()
	p.refreshTips()
	p.updateSensors(0)
	p.updateLogic(0)
	p.copyNets()
}

// SetControl presses or releases a button, or turns a key switch.
func (p *Plant) SetControl(name string, on bool) error {
	i, ok := p.plan.Controls[name]
	if !ok {
		return fmt.Errorf("unknown control %q", name)
	}
	p.v[i] = b2f(on)
	p.copyNets()
	return nil
}

// SetPin holds a signal on or off, to try out a module without a PLC. Reset ends it.
func (p *Plant) SetPin(state int, on bool) error {
	if state < 0 || state >= len(p.v) {
		return fmt.Errorf("no signal %d", state)
	}
	p.forced[state] = b2f(on)
	p.v[state] = b2f(on)
	p.copyNets()
	return nil
}

// SetFault enables or clears the fault of a sensor.
func (p *Plant) SetFault(name string, on bool) error {
	for i := range p.plan.Sensors {
		s := &p.plan.Sensors[i]
		if s.Name == name && s.Fault {
			p.failed[i] = on
			p.updateSensors(0)
			p.copyNets()
			return nil
		}
	}
	return fmt.Errorf("unknown fault %q", name)
}

// Faults returns the names of the active faults.
func (p *Plant) Faults() []string {
	active := []string{}
	for i, f := range p.failed {
		if f {
			active = append(active, p.plan.Sensors[i].Name)
		}
	}
	return active
}

// RunAction triggers a function of the model. A placeWorkpiece action takes the
// workpiece kind as args["kind"].
func (p *Plant) RunAction(name string, args map[string]string) error {
	a, ok := p.plan.Actions[name]
	if !ok {
		return fmt.Errorf("unknown action %q", name)
	}
	switch a.Type {
	case "clearTransport":
		if a.Over != nil {
			p.clearAt(*a.Over)
		} else {
			p.clearSegment(a.Segment)
		}
	case "placeWorkpiece":
		if err := p.place(&a, args["kind"], args["upsideDown"] == "true"); err != nil {
			return err
		}
	}
	p.updateSensors(0)
	p.copyNets()
	return nil
}

// place puts a new workpiece on a transport. As on a real station, this is
// refused while it is blocked or the position is occupied.
func (p *Plant) place(a *model.ActionPlan, kind string, upsideDown bool) error {
	k, ok := p.kinds[kind]
	if !ok {
		return ErrUnknownKind
	}
	if a.Family != "" && k.Family != a.Family {
		return ErrWrongFamily
	}
	if a.BlockedBy >= 0 && p.v[a.BlockedBy] >= 0.5 {
		return ErrBlocked
	}
	w := part{kind: kind, seg: a.Segment, s: a.At, upsideDown: upsideDown}
	if a.Over != nil {
		seg, s, ok := p.surfaceBelow(*a.Over, p.radius/2, -1)
		if !ok {
			return ErrNothingThere
		}
		w.seg, w.s = seg, s
	}
	w.dir = level(a.Heading)
	if w.dir == (model.Point{}) {
		seg := p.segments[w.seg].SegmentPlan
		w.dir = level(model.Point{X: seg.To.X - seg.From.X, Y: seg.To.Y - seg.From.Y})
	}
	if w.dir == (model.Point{}) {
		w.dir = model.Point{X: 1}
	}
	for i := range p.parts {
		if p.parts[i].fall == nil && p.partsGap(&w, &p.parts[i]) < placeRoom {
			return ErrOccupied
		}
	}
	p.nextID++
	w.id = p.nextID
	p.parts = append(p.parts, w)
	return nil
}

// clearAt takes away what stands at a point.
func (p *Plant) clearAt(at model.Point) {
	p.parts = slices.DeleteFunc(p.parts, func(w part) bool {
		if _, held := p.carrier[w.seg]; held || w.fall != nil {
			return false
		}
		c := p.world(&w)
		return p.footOf(&w).contains(at.X, at.Y, 0) && math.Abs(c.Z-at.Z) <= 4
	})
}

func (p *Plant) clearSegment(seg int) {
	kept := p.parts[:0]
	for _, w := range p.parts {
		if w.seg != seg {
			kept = append(kept, w)
		}
	}
	p.parts = kept
}

// Step advances the plant by dt seconds with the given outputs of its first PLC, and none of
// any other.
func (p *Plant) Step(dt float64, out plcio.Image) { p.StepAll(dt, []plcio.Image{out}) }

// StepAll advances the plant by dt seconds with the outputs of each of its PLCs, in the
// order of Plan.PLCs. A PLC without an image has all its outputs off.
func (p *Plant) StepAll(dt float64, outs []plcio.Image) {
	p.updateCounts()
	p.updateSafety()
	for i := range p.plan.PLCOut {
		o := &p.plan.PLCOut[i]
		on := false
		if o.PLC < len(outs) {
			on = outs[o.PLC].Get(o.Addr)
		}
		p.v[o.State] = b2f(on && !(o.Switched && p.cut))
	}
	for state, value := range p.forced {
		p.v[state] = value
	}
	p.hit = p.hit[:0]
	clear(p.stalled)
	p.updatePrograms(dt)
	p.updateCylinders(dt)
	p.updateMotors()
	p.updateAxes(dt)
	p.refreshRides()
	p.updateGrippers()
	p.sweep()
	p.updatePushers()
	for i := range p.segments {
		s := &p.segments[i]
		if s.Drive >= 0 && p.v[s.Drive] >= 0.5 {
			p.v[s.Travel] += s.speed(p) * dt
		} else if s.Reverse >= 0 && p.v[s.Reverse] >= 0.5 {
			p.v[s.Travel] -= s.speed(p) * dt
		}
	}
	for i := range p.parts {
		p.move(i, dt)
	}
	// What reaches the floor is lost.
	p.parts = slices.DeleteFunc(p.parts, func(w part) bool {
		lost := w.fall != nil && w.fall.at.Z <= floorZ && !w.gone
		if lost {
			p.fell++
		}
		return lost
	})
	p.sweep()
	p.updateSensors(dt)
	p.updateProcesses(dt)
	p.updateLogic(dt)
	p.copyNets()
}

// updateLogic works out the states of logic devices, in the order the modules declare them,
// so that one can read another above it in the same step.
// updateSafety opens a safety relay while its reset holds, and closes it while its set does.
func (p *Plant) updateSafety() {
	p.cut = false
	for i := range p.plan.Safety {
		s := &p.plan.Safety[i]
		switch {
		case s.Reset(p.v) >= 0.5:
			p.v[s.OK] = 0
		case s.Set(p.v) >= 0.5:
			p.v[s.OK] = 1
		}
		p.cut = p.cut || p.v[s.OK] < 0.5
	}
}

func (p *Plant) updateLogic(dt float64) {
	for i := range p.plan.Logic {
		l := &p.plan.Logic[i]
		on := p.v[l.On] >= 0.5
		want := on
		if l.Expr != nil {
			p.v[l.Value] = l.Expr(p.v)
			want = p.v[l.Value] >= 0.5
		} else if l.Reset(p.v) >= 0.5 {
			want = false
		} else if l.Set(p.v) >= 0.5 {
			want = true
		}
		if want == on {
			p.waits[i] = 0
		} else {
			delay := l.OnDelay
			if !want {
				delay = l.OffDelay
			}
			if p.waits[i] += dt; p.waits[i] >= delay-1e-9 {
				on, p.waits[i] = want, 0
			}
		}
		p.v[l.On] = b2f(on)
		if l.Expr == nil {
			p.v[l.Value] = p.v[l.On]
		}
		// A device input that the PLC drives follows the PLC.
		if l.Drives >= 0 && !p.plcOut[l.Drives] {
			p.v[l.Drives] = p.v[l.On]
		}
	}
}

// copyNets feeds nets from the device states that drive them.
func (p *Plant) copyNets() {
	for i := range p.plan.Copies {
		c := &p.plan.Copies[i]
		if c.From < 0 {
			p.v[c.To] = b2f(c.Constant)
		} else if c.Analog {
			p.v[c.To] = p.v[c.From]
		} else {
			p.v[c.To] = b2f((p.v[c.From] >= 0.5) != c.Invert)
		}
	}
}

// updateMotors applies the function table of the DC motor controller: one
// direction input runs the motor, both brake it, none lets it stop. Creep speed
// is half of the set speed.
func (p *Plant) updateMotors() {
	for i := range p.plan.Motors {
		m := &p.plan.Motors[i]
		cw, ccw := p.v[m.Clockwise] >= 0.5, p.v[m.Counterclockwise] >= 0.5
		p.v[m.Forward] = b2f(cw && !ccw)
		p.v[m.Backward] = b2f(ccw && !cw)
		p.v[m.Running] = b2f(cw != ccw)
		p.v[m.Ready] = 1
		p.v[m.Factor] = 1
		if p.v[m.Creep] >= 0.5 {
			p.v[m.Factor] = 0.5
		}
	}
}

// updateAxes moves each axis while one of its inputs is on, as far as there is room. A
// positioning axis drives to the target of its record as its go input comes on.
func (p *Plant) updateAxes(dt float64) {
	for i := range p.plan.Axes {
		a := &p.plan.Axes[i]
		fwd, back := p.v[a.Forward] >= 0.5, p.v[a.Backward] >= 0.5
		pos := p.v[a.Position]
		if p.cut && a.Go >= 0 {
			// Without power its drive stops where it is, and forgets where it was going.
			p.goals[i] = math.NaN()
			p.v[a.Done] = 0
			p.goes[i] = p.v[a.Go] >= 0.5
			continue
		}
		if a.Go >= 0 {
			if goes := p.v[a.Go] >= 0.5; goes != p.goes[i] {
				p.goes[i] = goes
				if record := p.record(a); goes && a.Setpoint >= 0 {
					p.goals[i] = math.Max(0, math.Min(a.Stroke, p.v[a.Setpoint])) / a.Stroke
					p.v[a.Done] = 0
				} else if goes && record < len(a.Targets) {
					p.goals[i] = a.Targets[record] / a.Stroke
					p.v[a.Done] = 0
				}
			}
			if math.IsNaN(p.goals[i]) && fwd != back {
				p.v[a.Done] = 0
			}
			// Without enable its controller holds it where it is.
			if a.Enable >= 0 {
				enabled := p.v[a.Enable] >= 0.5
				p.v[a.Ready], p.v[a.Homed], p.v[a.Error] = b2f(enabled), 1, 0
				if !enabled {
					fwd, back = false, false
				}
			}
		}
		// A goal, from a record or from a controller that drives the axis, wins over its inputs.
		if !math.IsNaN(p.goals[i]) && (a.Enable < 0 || p.v[a.Enable] >= 0.5) {
			fwd, back = p.goals[i] > pos, p.goals[i] < pos
		}
		to := pos
		switch {
		case fwd && !back:
			to = pos + dt/a.Travel
		case back && !fwd:
			to = pos - dt/a.Travel
		}
		if goal := p.goals[i]; !math.IsNaN(goal) && (fwd && to >= goal || back && to <= goal || !fwd && !back && (a.Enable < 0 || p.v[a.Enable] >= 0.5)) {
			to = goal
		}
		if !a.Wrap {
			to = math.Max(0, math.Min(1, to))
		}
		reached := pos
		if dt > 0 {
			reached = p.travel(a.Position, a.Position, pos, to)
		}
		if a.Wrap {
			reached -= math.Floor(reached)
		}
		p.v[a.Position] = reached
		p.v[a.Value] = reached * a.Stroke
		p.v[a.Moving] = b2f(reached != pos)
		p.v[a.Start] = b2f(!a.Wrap && reached == 0)
		p.v[a.End] = b2f(!a.Wrap && reached == 1)
		if goal := p.goals[i]; !math.IsNaN(goal) && reached == goal {
			p.goals[i] = math.NaN()
			if a.Done >= 0 {
				p.v[a.Done] = 1
			}
		}
	}
}

// record is the target that a positioning axis's record inputs select.
func (p *Plant) record(a *model.AxisPlan) int {
	n := 0
	for bit, state := range a.Records {
		if p.v[state] >= 0.5 {
			n |= 1 << bit
		}
	}
	return n
}

func (s *segment) speed(p *Plant) float64 {
	if s.Factor >= 0 {
		return s.Speed * p.v[s.Factor]
	}
	return s.Speed
}

func (p *Plant) updateCylinders(dt float64) {
	for i := range p.plan.Cylinders {
		c := &p.plan.Cylinders[i]
		target := p.target(c, dt == 0)
		pos := p.v[c.Position]
		if dt > 0 {
			if target {
				to := math.Min(1, pos+dt/c.Travel)
				// A stop in its path holds it there on its way out.
				if c.Stop >= 0 && p.v[c.Stop] >= 0.5 && pos <= c.StopAt+1e-9 {
					to = math.Min(to, c.StopAt)
				}
				pos = p.travel(c.Position, c.Position, pos, to)
			} else {
				pos = p.travel(c.Position, c.Position, pos, math.Max(0, pos-dt/c.Travel))
			}
		}
		p.v[c.Extended] = b2f(pos == 1)
		p.v[c.Retracted] = b2f(pos == 0)
		p.v[c.Commanded] = b2f(target)
		p.v[c.Midway] = b2f(pos >= 0.5)
	}
}

// travel moves a cylinder or axis from one position towards the next, as far as there is
// room for what it moves, and returns where it got. state is what the joints follow, and
// position the cylinder's or axis's own position.
func (p *Plant) travel(state, position int, from, to float64) float64 {
	p.v[state] = to
	m, moves := p.moves[state]
	if from == to || !moves {
		return to
	}
	p.v[state] = from
	p.refreshTips()
	p.before = p.before[:0]
	p.pairs(&m, func(gap float64, _ contact) bool {
		p.before = append(p.before, gap)
		return false
	})
	p.v[state] = to
	p.refreshTips()
	first, pressed := p.pressed(&m)
	for tries := 0; pressed && tries < len(p.parts); tries++ {
		if first.solid < 0 || first.part < 0 || !p.plan.Solids[first.solid].Shoves || !p.shove(first.part, &p.plan.Solids[first.solid]) {
			break
		}
		first, pressed = p.pressed(&m)
	}
	if !pressed {
		return to
	}
	reachable := from
	for range contactSteps {
		mid := (reachable + to) / 2
		p.v[state] = mid
		p.refreshTips()
		if _, pressed := p.pressed(&m); pressed {
			to = mid
		} else {
			reachable = mid
		}
	}
	p.v[state] = reachable
	p.refreshTips()
	switch {
	case first.solid >= 0 && first.part >= 0 && p.isJaw(position):
		p.stalled[position] = first.part // gripping, or it will say so
	case p.limited[position]:
		// Its pressure regulator lets it stop gently on what it presses on.
	default:
		p.report(first)
	}
	return reachable
}

// shove pushes a workpiece back along its transport until it is clear of a solid, and takes
// along what it runs into. It goes back and not forward, as what comes down between two
// workpieces has to end up in front of the second. It reports whether that worked.
func (p *Plant) shove(i int, solid *model.SolidPlan) bool {
	w := p.parts[i]
	if w.fall != nil {
		return false
	}
	if _, held := p.carrier[w.seg]; held {
		return false
	}
	for d := shoveStep; d <= shoveLimit && d <= w.s; d += shoveStep {
		c := w
		c.s = w.s - d
		if p.gapOf(solid, &c) >= -touching {
			p.parts[i].s = c.s
			p.bump(i)
			return true
		}
	}
	return false
}

// bump moves workpieces back, out of the way of one that was just shoved.
func (p *Plant) bump(i int) {
	for j := range p.parts {
		if j == i || p.parts[j].seg != p.parts[i].seg || p.parts[j].fall != nil || p.parts[j].s > p.parts[i].s {
			continue
		}
		if gap := p.partsGap(&p.parts[i], &p.parts[j]); gap < -touching {
			p.parts[j].s = math.Max(0, p.parts[j].s+gap)
			p.bump(j)
		}
	}
}

// updatePushers takes the lowest workpiece of a stack along with a cylinder, and leaves it
// on what lies below at the end of the stroke. Without anything there, it stays out there.
func (p *Plant) updatePushers() {
	for k := range p.plan.Pushers {
		push := &p.plan.Pushers[k]
		to := &p.segments[push.To]
		at := p.v[push.Position] * to.Length
		for i := range p.parts {
			w := &p.parts[i]
			if w.fall != nil {
				continue
			}
			if push.From < 0 && w.seg != push.To && at > 0 && at <= pushTake && p.standsAt(w, to.From) {
				w.seg, w.s = push.To, 0
			}
			if push.From >= 0 && w.seg == push.From && at > 0 && w.s >= p.segments[push.From].Length-stackRest {
				w.seg, w.s = push.To, 0
			}
			if w.seg != push.To {
				continue
			}
			w.s = math.Max(w.s, at)
			if w.s >= to.Length-stackRest && to.Next >= 0 {
				// Over the gap onto the next station's belt, which takes it on from there.
				w.seg, w.s = to.Next, to.NextAt
				continue
			}
			if w.s >= to.Length-stackRest {
				c := p.world(w)
				if seg, s, ok := p.surfaceBelow(c, stackReach, push.To); ok {
					w.seg, w.s = seg, s
				}
			}
		}
	}
}

// standsAt tells whether a workpiece stands on a transport with its centre at a point, as
// in the nest that an ejector empties.
func (p *Plant) standsAt(w *part, at model.Point) bool {
	if _, held := p.carrier[w.seg]; held {
		return false
	}
	c := p.world(w)
	return math.Hypot(c.X-at.X, c.Y-at.Y) <= model.LinkTolerance && math.Abs(c.Z-at.Z) <= 2
}

// surfaceBelow finds the straight transport right below a point.
func (p *Plant) surfaceBelow(c model.Point, reach float64, except int) (int, float64, bool) {
	best, bestS, bestGap := -1, 0.0, math.Inf(1)
	for j := range p.segments {
		s := p.segments[j].SegmentPlan
		if _, held := p.carrier[j]; held || s.Arc || j == except {
			continue
		}
		dx, dy := s.To.X-s.From.X, s.To.Y-s.From.Y
		if dx == 0 && dy == 0 {
			continue // a stack
		}
		along := s.Length * ((c.X-s.From.X)*dx + (c.Y-s.From.Y)*dy) / (dx*dx + dy*dy)
		along = math.Max(0, math.Min(s.Length, along))
		at := model.PointOn(s, along)
		gap := c.Z - at.Z
		if math.Hypot(c.X-at.X, c.Y-at.Y) <= reach && gap > -4 && gap < 40 && gap < bestGap {
			best, bestS, bestGap = j, along, gap
		}
	}
	return best, bestS, best >= 0
}

func (p *Plant) isJaw(position int) bool {
	for g := range p.plan.Grippers {
		if p.plan.Grippers[g].Position == position {
			return true
		}
	}
	return false
}

func (p *Plant) refreshTips() {
	for g := range p.plan.Grippers {
		tip := &p.plan.Grippers[g].Tip
		p.tips[g] = tip.Point(p.v)
		ahead := tip.Apply(model.Point{X: 1}, p.v)
		p.turns[g] = math.Atan2(ahead.Y-p.tips[g].Y, ahead.X-p.tips[g].X)
	}
}

// target returns the end position a cylinder's valve drives it to. A monostable
// valve follows its coil and returns when it drops; a bistable one keeps the last
// position when neither or both coils are on.
func (p *Plant) target(c *model.CylinderPlan, atRest bool) bool {
	if c.Bistable {
		extend, retract := p.v[c.Extend] >= 0.5, p.v[c.Retract] >= 0.5
		if extend != retract {
			return extend
		}
		if atRest {
			return c.RestOut
		}
		return p.v[c.Commanded] >= 0.5
	}
	if c.RestOut {
		return p.v[c.Retract] < 0.5
	}
	return p.v[c.Extend] >= 0.5
}

func (p *Plant) move(i int, dt float64) {
	w := p.parts[i]
	if w.fall != nil {
		f := w.fall
		f.velocity.Z -= gravity * dt
		was := f.at
		f.at = model.Point{X: f.at.X + f.velocity.X*dt, Y: f.at.Y + f.velocity.Y*dt, Z: f.at.Z + f.velocity.Z*dt}
		f.tumbled += tumbleRate * dt
		p.land(i, was)
		return
	}
	if _, held := p.carrier[w.seg]; held {
		return
	}
	s := &p.segments[w.seg]
	if s.Reverse >= 0 && p.v[s.Reverse] >= 0.5 {
		p.moveBack(i, &w, s, dt)
		return
	}
	driven := s.Drive < 0 || p.v[s.Drive] >= 0.5
	// Past FreeFrom the leading edge is over the next segment and gravity
	// finishes the transfer.
	if !driven && !(s.FreeFrom >= 0 && w.s >= s.FreeFrom) {
		return
	}
	next := w
	next.s = w.s + s.speed(p)*dt
	moved := false
	for j := range s.Branches {
		b := &s.Branches[j]
		if (b.State < 0 || p.v[b.State] >= 0.5) && w.s < b.At && next.s >= b.At {
			next.seg, next.s = b.To, next.s-b.At
			moved = true
			break
		}
	}
	if !moved && next.s >= s.Length {
		joined, joinedAt := -1, 0.0
		if s.Next < 0 {
			joined, joinedAt = p.join(w.seg)
		}
		switch {
		case s.Next >= 0:
			next.seg, next.s = s.Next, next.s-s.Length+s.NextAt
		case joined >= 0:
			next.seg, next.s = joined, next.s-s.Length+joinedAt
		case s.Open:
			p.tipOff(i, s, 1, s.speed(p))
			return
		default:
			next.s = s.Length
		}
	}
	p.advance(i, &w, &next)
}

// land puts a falling workpiece onto a straight transport it falls onto, as from the end of
// a belt onto another one below it or beside it, or onto a slide. was is where it fell from
// in the last step.
func (p *Plant) land(i int, was model.Point) {
	w := &p.parts[i]
	at := w.fall.at
	if w.gone || p.joinOnto(i, at, 0.5) {
		return
	}
	for j := range p.segments {
		s := p.segments[j].SegmentPlan
		if _, held := p.carrier[j]; held || s.Arc || s.Length == 0 {
			continue
		}
		dx, dy := s.To.X-s.From.X, s.To.Y-s.From.Y
		if dx == 0 && dy == 0 {
			continue // a stack
		}
		along := s.Length * ((at.X-s.From.X)*dx + (at.Y-s.From.Y)*dy) / (dx*dx + dy*dy)
		if along < 0 || along > s.Length {
			continue
		}
		on := model.PointOn(s, along)
		if math.Hypot(at.X-on.X, at.Y-on.Y) > p.footOf(w).narrow() || was.Z < on.Z-0.5 || at.Z > on.Z {
			continue
		}
		next := *w
		next.fall, next.seg, next.s, next.off = nil, j, along, model.Point{}
		if p.occupied(i, &next) {
			continue
		}
		p.parts[i] = next
		return
	}
}

// sweep removes the workpieces that were joined onto others.
func (p *Plant) sweep() {
	p.parts = slices.DeleteFunc(p.parts, func(w part) bool { return w.gone })
}

// joinOnto joins workpiece i, set down or dropped with its base at c, onto the workpiece
// right below it, if it goes onto that one's family: a lid onto the middle of a body's
// top, no more than above mm below c, or a box anywhere onto a pallet or onto what was put on
// it before. It is then gone after this step, and the one below carries it.
func (p *Plant) joinOnto(i int, c model.Point, above float64) bool {
	w := &p.parts[i]
	onto := p.kinds[w.kind].Onto
	if onto == nil {
		return false
	}
	for j := range p.parts {
		o := &p.parts[j]
		if _, held := p.carrier[o.seg]; j == i || held || o.fall != nil || o.gone || p.kinds[o.kind].Family != onto.Family {
			continue
		}
		var l load
		if onto.Free {
			var ok bool
			if l, ok = p.restOn(o, w, c, above); !ok {
				continue
			}
		} else {
			if len(o.loads) > 0 && p.kinds[o.loads[len(o.loads)-1].kind].Onto != nil {
				continue
			}
			oc := p.world(o)
			top := oc.Z + p.heightOf(o)
			if !p.footOf(o).middle(c.X, c.Y) || c.Z < top-onto.Depth-2 || c.Z > top+above {
				continue
			}
			l = load{kind: w.kind, off: model.Point{Z: p.heightOf(o) - onto.Depth}, dir: model.Point{X: 1}}
		}
		o.loads = append(slices.Clone(o.loads), l)
		w.gone = true
		if w.fall == nil {
			w.fall = &fall{at: c}
		}
		return true
	}
	return false
}

// restOn tells where workpiece w, with its base at c, comes to rest on carrier o: on its top,
// or on the highest of what was put on it before that lies under w, if that is no more than
// above mm below c. It gives the load as o carries it.
func (p *Plant) restOn(o, w *part, c model.Point, above float64) (load, bool) {
	of := p.footOf(o)
	if !of.contains(c.X, c.Y, 0) {
		return load{}, false
	}
	wf := p.footOf(w)
	wf.x, wf.y = c.X, c.Y
	top := p.kindHeight(o.kind)
	for _, l := range o.loads {
		if p.loadFoot(o, l).gapTo(wf) < -1 {
			top = math.Max(top, l.off.Z+p.kindHeight(l.kind))
		}
	}
	base := p.world(o).Z
	if c.Z < base+top-2 || c.Z > base+top+above {
		return load{}, false
	}
	lx, ly := of.local(c.X, c.Y)
	h := p.headingOf(w)
	return load{kind: w.kind, off: model.Point{X: lx, Y: ly, Z: top}, dir: model.Point{X: h.X*of.ux + h.Y*of.uy, Y: -h.X*of.uy + h.Y*of.ux}}, true
}

// loadFoot is the footprint of a load in the station.
func (p *Plant) loadFoot(o *part, l load) foot {
	of := p.footOf(o)
	c := model.Point{X: of.x + l.off.X*of.ux - l.off.Y*of.uy, Y: of.y + l.off.X*of.uy + l.off.Y*of.ux}
	k := p.kinds[l.kind]
	if len(k.Size) != 2 {
		return round(c, p.radius)
	}
	return rect(c, k.Size[0], k.Size[1], model.Point{X: l.dir.X*of.ux - l.dir.Y*of.uy, Y: l.dir.X*of.uy + l.dir.Y*of.ux})
}

// has tells whether a workpiece has a mark, or a joined part of that family or kind.
func (p *Plant) has(w *part, what string) bool {
	if what == "upsideDown" {
		return w.upsideDown
	}
	if w.marks[what] > 0 {
		return true
	}
	for _, l := range w.loads {
		if l.kind == what || p.kinds[l.kind].Family == what {
			return true
		}
	}
	return false
}

// updateProcesses works on the workpieces under processes that are on. A process works on a
// workpiece once each time it is switched on, and is done with it until it is switched off.
func (p *Plant) updateProcesses(dt float64) {
	for k := range p.plan.Processes {
		pr, st := &p.plan.Processes[k], &p.works[k]
		under := -1
		for i := range p.parts {
			w := &p.parts[i]
			if w.fall != nil {
				continue
			}
			c := p.world(w)
			if top := c.Z + p.heightOf(w); p.footOf(w).middle(pr.Origin.X, pr.Origin.Y) && top <= pr.Origin.Z+0.5 && top >= pr.Origin.Z-pr.Reach {
				under = i
				break
			}
		}
		id := 0
		if under >= 0 {
			id = p.parts[under].id
		}
		on := p.v[pr.On] >= 0.5
		if id != st.id || !on {
			*st = work{id: id}
		}
		if on && under >= 0 && !st.done {
			if st.timer += dt; st.timer >= pr.Time-1e-9 {
				w := &p.parts[under]
				marks := maps.Clone(w.marks)
				if marks == nil {
					marks = map[string]float64{}
				}
				if pr.Adds {
					marks[pr.Mark] += pr.Amount
				} else {
					marks[pr.Mark] = pr.Amount
				}
				w.marks, st.done = marks, true
			}
		}
		p.v[pr.Busy] = b2f(on && under >= 0 && !st.done)
		p.v[pr.Done] = b2f(under >= 0 && st.done)
	}
}

// tipOff starts a workpiece falling from the end of a transport.
func (p *Plant) tipOff(i int, s *segment, sign, speed float64) {
	w := &p.parts[i]
	dx, dy := (s.To.X-s.From.X)/s.Length*sign, (s.To.Y-s.From.Y)/s.Length*sign
	w.fall = &fall{
		at:       p.world(w),
		dir:      model.Point{X: dx, Y: dy},
		velocity: model.Point{X: dx * (speed + tipSpeed), Y: dy * (speed + tipSpeed)},
	}
}

// moveBack runs a workpiece towards the start of a reversed transport. It stops there
// unless that end is open. Deflectors only act in the forward direction.
func (p *Plant) moveBack(i int, w *part, s *segment, dt float64) {
	next := *w
	next.s = w.s - s.speed(p)*dt
	switch {
	case s.Prev >= 0 && next.s <= s.PrevAt:
		// Back onto the transport before it, as onto the belt of the station next to it.
		next.seg, next.s = s.Prev, p.segments[s.Prev].Length+next.s-s.PrevAt
	case s.Prev >= 0:
		// Still crossing the gap to it.
	case next.s <= 0 && s.Open:
		p.tipOff(i, s, -1, s.speed(p))
		return
	case next.s <= 0:
		next.s = 0
	}
	p.advance(i, w, &next)
}

// advance moves a workpiece as far as the solids let it, and not into other workpieces.
// Running into something only stops it: a collision takes something moving into it.
func (p *Plant) advance(i int, w, next *part) {
	if next.seg == w.seg && next.s == w.s {
		return
	}
	if p.intoSolid(w, next) {
		if next.seg != w.seg {
			return
		}
		from, to := w.s, next.s
		for range contactSteps {
			next.s = (from + to) / 2
			if p.intoSolid(w, next) {
				to = next.s
			} else {
				from = next.s
			}
		}
		next.s = from
	}
	if !p.blocked(i, w, next) {
		p.parts[i] = *next
		return
	}
	if next.seg != w.seg {
		return
	}
	// Right up to it, so that a stack adds up to its height.
	from, to := w.s, next.s
	for range contactSteps {
		next.s = (from + to) / 2
		if p.blocked(i, w, next) {
			to = next.s
		} else {
			from = next.s
		}
	}
	p.parts[i].s = from
}

// intoSolid reports whether moving from a to b would push a workpiece further into a solid.
func (p *Plant) intoSolid(a, b *part) bool {
	from, to, height := p.footOf(a), p.footOf(b), p.heightOf(a)
	za, zb := p.world(a).Z, p.world(b).Z
	for s := range p.plan.Solids {
		solid := &p.plan.Solids[s]
		if after := p.gap(solid, to, zb, height); after < -touching && after < p.gap(solid, from, za, height)-deeper {
			return true
		}
	}
	return false
}

// Two things touch at a gap of zero. These keep rounding errors from counting as contact.
const (
	touching     = 1e-6 // mm
	deeper       = 1e-9 // mm
	contactSteps = 7    // halvings of one step of the simulation, to find where contact is
	shoveStep    = 0.25 // mm
	shoveLimit   = 30.0 // mm, more than a blade's tip can reach into a workpiece
	stackRest    = 0.01 // mm from the end of a stack or a pusher's path that counts as there
	stackReach   = 8.0  // mm from a pusher's path to the surface it leaves a workpiece on
	pushTake     = 5.0  // mm a pusher without a stack may have come out and still take a workpiece along
	placeRoom    = 0.5  // mm that must be left around a workpiece put on a transport
)

// box is a box in station coordinates, as a corner and the three edges from it.
type box struct {
	o, u, v, w model.Point
}

func solidBox(s *model.SolidPlan, states []float64) box {
	o := s.Chain.Apply(s.Min, states)
	edge := func(p model.Point) model.Point {
		q := s.Chain.Apply(p, states)
		return model.Point{X: q.X - o.X, Y: q.Y - o.Y, Z: q.Z - o.Z}
	}
	return box{o,
		edge(model.Point{X: s.Max.X, Y: s.Min.Y, Z: s.Min.Z}),
		edge(model.Point{X: s.Min.X, Y: s.Max.Y, Z: s.Min.Z}),
		edge(model.Point{X: s.Min.X, Y: s.Min.Y, Z: s.Max.Z})}
}

func obstacleBox(o *model.ObstaclePlan) box { return box{o.Origin, o.U, o.V, o.W} }

func (b box) center() model.Point {
	return model.Point{
		X: b.o.X + (b.u.X+b.v.X+b.w.X)/2,
		Y: b.o.Y + (b.u.Y+b.v.Y+b.w.Y)/2,
		Z: b.o.Z + (b.u.Z+b.v.Z+b.w.Z)/2,
	}
}

// bounds returns the corners of the axis-aligned box around it.
func (b box) bounds() (lo, hi model.Point) {
	lo = model.Point{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi = model.Point{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for i := range 8 {
		q := b.o
		for bit, e := range []model.Point{b.u, b.v, b.w} {
			if i>>bit&1 == 1 {
				q = model.Point{X: q.X + e.X, Y: q.Y + e.Y, Z: q.Z + e.Z}
			}
		}
		lo = model.Point{X: min(lo.X, q.X), Y: min(lo.Y, q.Y), Z: min(lo.Z, q.Z)}
		hi = model.Point{X: max(hi.X, q.X), Y: max(hi.Y, q.Y), Z: max(hi.Z, q.Z)}
	}
	return lo, hi
}

func dot(a, b model.Point) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

func cross(a, b model.Point) model.Point {
	return model.Point{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}

// boxGap returns how far apart two boxes are along the axis that separates them best, by
// the separating axis theorem. It is negative while they overlap.
func boxGap(a, b box) float64 {
	d := model.Point{}
	ca, cb := a.center(), b.center()
	d = model.Point{X: cb.X - ca.X, Y: cb.Y - ca.Y, Z: cb.Z - ca.Z}
	ea, eb := [3]model.Point{a.u, a.v, a.w}, [3]model.Point{b.u, b.v, b.w}
	gap := math.Inf(-1)
	along := func(axis model.Point) {
		n := math.Sqrt(dot(axis, axis))
		if n < 1e-9 {
			return // parallel edges give no new axis
		}
		reach := 0.0
		for i := range 3 {
			reach += (math.Abs(dot(ea[i], axis)) + math.Abs(dot(eb[i], axis))) / 2
		}
		gap = max(gap, (math.Abs(dot(d, axis))-reach)/n)
	}
	for i := range 3 {
		along(ea[i])
		along(eb[i])
		for j := range 3 {
			along(cross(ea[i], eb[j]))
		}
	}
	return gap
}

// partGap returns how far an upright workpiece with the footprint f and its base at height z
// is from a box. It is negative while they overlap.
func (p *Plant) partGap(b box, f foot, z, height float64) float64 {
	c := model.Point{X: f.x, Y: f.y, Z: z}
	upright := func(e model.Point) bool { return math.Abs(e.X)+math.Abs(e.Y) < 1e-6 }
	var u, v, w model.Point
	switch {
	case upright(b.w):
		u, v, w = b.u, b.v, b.w
	case upright(b.v):
		u, v, w = b.u, b.w, b.v
	case upright(b.u):
		u, v, w = b.v, b.w, b.u
	default:
		// A tilted box, such as a slide: the box around the workpiece is close enough.
		r := f.extent()
		return boxGap(b, box{
			model.Point{X: c.X - r, Y: c.Y - r, Z: c.Z},
			model.Point{X: 2 * r}, model.Point{Y: 2 * r}, model.Point{Z: height},
		})
	}
	var side float64
	if f.round() {
		// The point of the footprint nearest to the workpiece's axis.
		along := func(e model.Point) (float64, float64) {
			length := math.Hypot(e.X, e.Y)
			t := math.Max(0, math.Min(length, ((c.X-b.o.X)*e.X+(c.Y-b.o.Y)*e.Y)/length))
			return e.X / length * t, e.Y / length * t
		}
		ux, uy := along(u)
		vx, vy := along(v)
		side = math.Hypot(c.X-(b.o.X+ux+vx), c.Y-(b.o.Y+uy+vy)) - f.r
	} else {
		side = f.gapTo(boxFoot(b.o, u, v))
	}
	low, high := math.Min(b.o.Z, b.o.Z+w.Z), math.Max(b.o.Z, b.o.Z+w.Z)
	return math.Max(side, math.Max(low-(c.Z+height), c.Z-high))
}

// gap returns how far an upright workpiece with the footprint f and its base at height z is
// from a solid.
func (p *Plant) gap(s *model.SolidPlan, f foot, z, height float64) float64 {
	if len(s.Outline) == 0 {
		return p.partGap(solidBox(s, p.v), f, z, height)
	}
	low := s.Chain.Apply(model.Point{Z: s.Min.Z}, p.v).Z
	high := s.Chain.Apply(model.Point{Z: s.Max.Z}, p.v).Z
	return math.Max(p.outlineGap(s, f), math.Max(low-(z+height), z-high))
}

// gapOf returns how far a workpiece is from a solid.
func (p *Plant) gapOf(s *model.SolidPlan, w *part) float64 {
	return p.gap(s, p.footOf(w), p.world(w).Z, p.heightOf(w))
}

// outlineGap returns how far a workpiece with the footprint f is from a solid's outline. It is
// negative while they overlap. A rectangle reaches towards the outline's nearest point as far
// as it reaches in that direction.
func (p *Plant) outlineGap(s *model.SolidPlan, f foot) float64 {
	c := model.Point{X: f.x, Y: f.y}
	inside, nearest := false, math.Inf(1)
	var nx, ny float64
	a := s.Chain.Apply(s.Outline[len(s.Outline)-1], p.v)
	for _, q := range s.Outline {
		b := s.Chain.Apply(q, p.v)
		dx, dy := b.X-a.X, b.Y-a.Y
		t := math.Max(0, math.Min(1, ((c.X-a.X)*dx+(c.Y-a.Y)*dy)/(dx*dx+dy*dy)))
		if d := math.Hypot(c.X-(a.X+t*dx), c.Y-(a.Y+t*dy)); d < nearest {
			nearest, nx, ny = d, a.X+t*dx-c.X, a.Y+t*dy-c.Y
		}
		if (a.Y > c.Y) != (b.Y > c.Y) && c.X < a.X+(c.Y-a.Y)/dy*dx {
			inside = !inside
		}
		a = b
	}
	if inside {
		nearest = -nearest
	}
	if f.round() {
		return nearest - f.r
	}
	if n := math.Hypot(nx, ny); n > 1e-9 {
		return nearest - f.support(nx/n, ny/n)
	}
	return nearest - f.narrow()
}

// skin is how much of a fixed part may be touched before it counts. It keeps a surface
// modelled a hair too high, such as a shallow recess, from stopping a workpiece set on it.
const skin = 0.25 // mm

// contact is what stopped a move: which solid or carried workpiece met what. Unused
// fields are -1.
type contact struct {
	solid    int // a solid that moved
	part     int // the workpiece it met
	other    int // another solid it met
	obstacle int // a fixed part it met
	carried  int // a workpiece that was carried along
}

func met() contact { return contact{solid: -1, part: -1, other: -1, obstacle: -1, carried: -1} }

// pairs calls visit with the gap of everything a cylinder moves and what that can meet, until
// visit returns true. It always goes through them in the same order.
func (p *Plant) pairs(m *moved, visit func(float64, contact) bool) {
	for _, s := range m.solids {
		solid := &p.plan.Solids[s]
		b := solidBox(solid, p.v)
		for i := range p.parts {
			w := &p.parts[i]
			if p.pushed(m, w) {
				continue
			}
			c := met()
			c.solid, c.part = s, i
			if visit(p.gapOf(solid, w), c) {
				return
			}
		}
		for _, o := range p.near[s] {
			c := met()
			c.solid, c.obstacle = s, o
			if visit(boxGap(b, obstacleBox(&p.plan.Obstacles[o]))+skin, c) {
				return
			}
		}
		for t := range p.plan.Solids {
			if other := &p.plan.Solids[t]; other.Root != solid.Root {
				c := met()
				c.solid, c.other = s, t
				if visit(boxGap(b, solidBox(other, p.v)), c) {
					return
				}
			}
		}
	}
	for _, k := range m.pushers {
		for i := range p.parts {
			w := &p.parts[i]
			push := &p.plan.Pushers[k]
			if w.seg != push.To || w.fall != nil {
				continue
			}
			// Where the cylinder is about to take it.
			ahead := *w
			ahead.s = math.Max(w.s, p.v[push.Position]*p.segments[push.To].Length)
			for j := range p.parts {
				if j != i {
					c := met()
					c.carried, c.part = i, j
					if visit(p.partsGap(&ahead, &p.parts[j]), c) {
						return
					}
				}
			}
		}
	}
	for _, g := range m.grippers {
		for i := range p.parts {
			w := &p.parts[i]
			if w.seg != p.plan.Grippers[g].Segment || w.fall != nil {
				continue
			}
			f, z, height := p.footOf(w), p.world(w).Z, p.heightOf(w)
			for j := range p.parts {
				if j != i {
					c := met()
					c.carried, c.part = i, j
					if visit(p.partsGap(w, &p.parts[j]), c) {
						return
					}
				}
			}
			for _, o := range p.nearTip[g] {
				c := met()
				c.carried, c.obstacle = i, o
				if visit(p.partGap(obstacleBox(&p.plan.Obstacles[o]), f, z, height)+skin, c) {
					return
				}
			}
			for t := range p.plan.Solids {
				if !slices.Contains(m.solids, t) {
					c := met()
					c.carried, c.other = i, t
					if visit(p.gap(&p.plan.Solids[t], f, z, height), c) {
						return
					}
				}
			}
		}
	}
}

// pushed tells whether a cylinder has a workpiece in front of its pusher, which it takes
// along instead of running into.
func (p *Plant) pushed(m *moved, w *part) bool {
	for _, k := range m.pushers {
		push := &p.plan.Pushers[k]
		if w.seg == push.To || (push.From >= 0 && w.seg == push.From && w.s >= p.segments[push.From].Length-stackRest) {
			return true
		}
	}
	return false
}

// partsGap returns how far apart two upright workpieces are: the nearest of what each is
// made of, itself and what it carries, so that a carton set down beside another one on a
// pallet only meets the pallet where it stands on it.
func (p *Plant) partsGap(a, b *part) float64 {
	gap := math.Inf(1)
	for _, x := range p.bodies(a) {
		for _, y := range p.bodies(b) {
			side := x.foot.gapTo(y.foot)
			gap = math.Min(gap, math.Max(side, math.Max(y.z-(x.z+x.height), x.z-(y.z+y.height))))
		}
	}
	return gap
}

// body is an upright block of a workpiece: its footprint, the height of its base and how high
// it is.
type body struct {
	foot      foot
	z, height float64
}

// bodies are what a workpiece is made of: itself, and each load it carries.
func (p *Plant) bodies(w *part) []body {
	c := p.world(w)
	out := []body{{p.footOf(w), c.Z, p.kindHeight(w.kind)}}
	for _, l := range w.loads {
		out = append(out, body{p.loadFoot(w, l), c.Z + l.off.Z, p.kindHeight(l.kind)})
	}
	return out
}

// pressed compares the gaps with those saved in p.before, and returns the first that went
// further below zero.
func (p *Plant) pressed(m *moved) (contact, bool) {
	k := 0
	var hit contact
	found := false
	p.pairs(m, func(gap float64, c contact) bool {
		was := p.before[k]
		k++
		if gap < -touching && gap < was-deeper {
			hit, found = c, true
		}
		return found
	})
	return hit, found
}

// report notes a collision.
func (p *Plant) report(c contact) {
	who := "A carried workpiece"
	if c.solid >= 0 {
		who = p.plan.Solids[c.solid].Label
	}
	what := "another workpiece"
	switch {
	case c.obstacle >= 0:
		what = p.plan.Obstacles[c.obstacle].Name
	case c.other >= 0:
		what = p.plan.Solids[c.other].Label
	case c.solid >= 0:
		what = "a workpiece"
	}
	text := who + " hits " + what
	if !slices.Contains(p.hit, text) {
		p.hit = append(p.hit, text)
	}
	if !slices.Contains(p.everHit, text) {
		p.everHit = append(p.everHit, text)
	}
	if c.obstacle >= 0 && !slices.ContainsFunc(p.struck, func(s struck) bool { return s.text == text }) {
		roots := []string{p.plan.Obstacles[c.obstacle].Root}
		if c.solid >= 0 {
			roots = append([]string{p.plan.Solids[c.solid].Root}, roots...)
		}
		p.struck = append(p.struck, struck{text, roots})
	}
}

// struck is a fixed part something ran into, and the top-level modules of both.
type struck struct {
	text  string
	roots []string
}

// FellOff returns how many workpieces left the station without being taken off by hand,
// since this was last acknowledged. Only that and Reset clear it, as a fall is over in a moment.
func (p *Plant) FellOff() int { return p.fell }

// AcknowledgeFellOff clears FellOff.
func (p *Plant) AcknowledgeFellOff() { p.fell = 0 }

// Collisions returns what moved into a workpiece in this step.
func (p *Plant) Collisions() []string { return append([]string{}, p.hit...) }

// Hits returns everything that ran into something since the reset, each once.
func (p *Plant) Hits() []string { return append([]string{}, p.everHit...) }

// blocked reports whether moving from a to b would push into another workpiece.
func (p *Plant) blocked(i int, a, b *part) bool {
	from, to := p.world(a), p.world(b)
	for j := range p.parts {
		if j == i || p.parts[j].fall != nil {
			continue
		}
		o := p.world(&p.parts[j])
		after := dist(to, o)
		// One on top of another, in a stack.
		if math.Abs(to.Z-o.Z) > math.Hypot(to.X-o.X, to.Y-o.Y) {
			lower := p.heightOf(&p.parts[j])
			if to.Z < o.Z {
				lower = p.heightOf(a)
			}
			if math.Abs(to.Z-o.Z) < lower && after < dist(from, o) {
				return true
			}
			continue
		}
		fb, fo := p.footOf(b), p.footOf(&p.parts[j])
		if fb.round() && fo.round() {
			if after < p.diameter && after < dist(from, o) {
				return true
			}
			continue
		}
		if g := fb.gapTo(fo); g < 0 && g < p.footOf(a).gapTo(fo) && to.Z < o.Z+p.heightOf(&p.parts[j])-0.5 && o.Z < to.Z+p.heightOf(b)-0.5 {
			return true
		}
	}
	return false
}

// occupied reports whether another workpiece is where w would be.
func (p *Plant) occupied(i int, w *part) bool {
	c, h := p.world(w), p.heightOf(w)
	for j := range p.parts {
		if j == i || p.parts[j].fall != nil {
			continue
		}
		o := p.world(&p.parts[j])
		if p.footOf(w).gapTo(p.footOf(&p.parts[j])) < -0.5 && c.Z < o.Z+p.heightOf(&p.parts[j]) && o.Z < c.Z+h {
			return true
		}
	}
	return false
}

func (p *Plant) world(w *part) model.Point {
	if w.fall != nil {
		return w.fall.at
	}
	if g, held := p.carrier[w.seg]; held {
		off := turned(w.off, p.turns[g])
		return model.Point{X: p.tips[g].X + off.X, Y: p.tips[g].Y + off.Y, Z: p.tips[g].Z + off.Z}
	}
	return model.PointOn(p.segments[w.seg].SegmentPlan, w.s)
}

// headingOf is the way a workpiece's length points in the station, level and of unit length.
func (p *Plant) headingOf(w *part) model.Point {
	if g, held := p.carrier[w.seg]; held && w.fall == nil {
		return turned(w.dir, p.turns[g])
	}
	if w.dir == (model.Point{}) {
		return model.Point{X: 1}
	}
	return w.dir
}

// footOf is the footprint of a workpiece where it is.
func (p *Plant) footOf(w *part) foot {
	c := p.world(w)
	if k := p.kinds[w.kind]; len(k.Size) == 2 {
		return rect(c, k.Size[0], k.Size[1], p.headingOf(w))
	}
	return round(c, p.radius)
}

// level is the level part of a direction, of unit length, or zero.
func level(d model.Point) model.Point {
	n := math.Hypot(d.X, d.Y)
	if n < 1e-9 {
		return model.Point{}
	}
	return model.Point{X: d.X / n, Y: d.Y / n}
}

func (p *Plant) updateSensors(dt float64) {
	for i := range p.sensors {
		s := &p.sensors[i]
		seen := false
		if !p.failed[i] {
			for j := range p.parts {
				w := &p.parts[j]
				if p.sees(s, w) && p.hits(s, p.footOf(w), p.world(w).Z, p.heightOf(w)) {
					seen = true
					break
				}
			}
		}
		p.hold[i] = math.Max(0, p.hold[i]-dt)
		if seen {
			p.hold[i] = s.OffDelay
		}
		p.v[s.Detected] = b2f(seen || p.hold[i] > 0)
		p.v[s.Free] = b2f(!seen && p.hold[i] <= 0)
	}
	p.updateDistances()
}

// updateDistances measures down to whatever is below each distance sensor.
func (p *Plant) updateDistances() {
	for i := range p.dists {
		d := &p.dists[i]
		distance, seen := d.Surface, false
		for j := range p.parts {
			w := &p.parts[j]
			c := p.world(w)
			if f := p.footOf(w); f.round() && math.Hypot(c.X-d.Origin.X, c.Y-d.Origin.Y) < f.r || !f.round() && f.contains(d.Origin.X, d.Origin.Y, 0) {
				distance = d.Origin.Z - (c.Z + p.heightOf(w))
				seen = true
				break
			}
		}
		p.v[d.Distance] = distance
		p.v[d.Value] = math.Min(10, math.Max(0, (distance-d.Min)/(d.Max-d.Min)*10))
		p.v[d.Detected] = b2f(seen && distance <= d.Threshold)
	}
}

func (p *Plant) heightOf(w *part) float64 {
	h := p.kindHeight(w.kind)
	for _, l := range w.loads {
		h = math.Max(h, l.off.Z+p.kindHeight(l.kind))
	}
	return h
}

func (p *Plant) kindHeight(kind string) float64 {
	if h := p.kinds[kind].Height; h > 0 {
		return h
	}
	return p.height
}

// updateGrippers picks up what the jaws close on and sets it down as they open. Jaws that
// close on a workpiece they cannot hold have hit it.
func (p *Plant) updateGrippers() {
	p.refreshTips()
	for g := range p.plan.Grippers {
		grip := &p.plan.Grippers[g]
		holding := false
		for i := range p.parts {
			holding = holding || (p.parts[i].seg == grip.Segment && p.parts[i].fall == nil)
		}
		if grip.Hold >= 0 {
			p.v[grip.Holding] = b2f(p.suck(g, holding))
			continue
		}
		open := p.v[grip.Open] >= 0.5
		switch {
		case holding && open:
			p.release(g)
			holding = false
		case !holding && !open:
			if i, ok := p.stalled[grip.Position]; ok {
				if p.canHold(g, i) {
					w := &p.parts[i]
					c, tip := p.world(w), p.tips[g]
					w.off = turned(model.Point{X: c.X - tip.X, Y: c.Y - tip.Y, Z: c.Z - tip.Z}, -p.turns[g])
					w.dir = turned(p.headingOf(w), -p.turns[g])
					w.seg, w.s = grip.Segment, 0
					holding = true
				} else {
					c := met()
					c.part, c.solid = i, p.solidOf(grip)
					p.report(c)
				}
			}
		}
		p.v[grip.Holding] = b2f(holding)
	}
}

// suck runs a gripper that holds while its state is on, such as a suction cup, and reports
// whether it holds a workpiece: it takes one its tip touches from above as the state comes
// on, or while it is on, and lets go when the state drops.
func (p *Plant) suck(g int, holding bool) bool {
	grip := &p.plan.Grippers[g]
	on := p.v[grip.Hold] >= 0.5
	switch {
	case holding && !on:
		p.release(g)
		return false
	case holding || !on:
		return holding
	}
	tip := p.tips[g]
	for i := range p.parts {
		w := &p.parts[i]
		if _, carried := p.carrier[w.seg]; carried || w.fall != nil || grip.Metal && !p.kinds[w.kind].Metal {
			continue
		}
		c := p.world(w)
		top := c.Z + p.heightOf(w)
		if p.reaches(w, tip, grip.Reach) && tip.Z >= top-1 && tip.Z <= top+grip.Reach {
			w.off = turned(model.Point{X: c.X - tip.X, Y: c.Y - tip.Y, Z: c.Z - tip.Z}, -p.turns[g])
			w.dir = turned(p.headingOf(w), -p.turns[g])
			w.seg, w.s = grip.Segment, 0
			return true
		}
	}
	return false
}

// canHold tells whether a workpiece is between a gripper's jaws.
func (p *Plant) canHold(g, i int) bool {
	grip := &p.plan.Grippers[g]
	w, tip := &p.parts[i], p.tips[g]
	c := p.world(w)
	return p.reaches(w, tip, grip.Reach) && tip.Z >= c.Z && tip.Z <= c.Z+p.heightOf(w)
}

// reaches tells whether a gripper's tip is over a workpiece: within reach of the axis of a
// round one, or over a block.
func (p *Plant) reaches(w *part, tip model.Point, reach float64) bool {
	f := p.footOf(w)
	if f.round() {
		return math.Hypot(f.x-tip.X, f.y-tip.Y) <= reach
	}
	return f.contains(tip.X, tip.Y, 0)
}

// solidOf returns a solid moved by a gripper's jaws, to name it in a report.
func (p *Plant) solidOf(grip *model.GripperPlan) int {
	for s := range p.plan.Solids {
		for _, l := range p.plan.Solids[s].Chain.Links {
			if l.Joint && l.State == grip.Position {
				return s
			}
		}
	}
	return 0
}

// release puts a held workpiece on the straight transport or surface right below it. With
// nothing there it falls, which counts as lost.
func (p *Plant) release(g int) {
	grip := &p.plan.Grippers[g]
	for i := range p.parts {
		w := &p.parts[i]
		if w.seg != grip.Segment || w.fall != nil {
			continue
		}
		c := p.world(w)
		if p.joinOnto(i, c, grip.Reach+2) {
			return
		}
		heading := p.headingOf(w)
		best, bestS, ok := p.surfaceBelow(c, grip.Reach, -1)
		if !ok {
			w.fall, w.dir = &fall{at: c, dir: model.Point{X: 1}}, heading
			return
		}
		w.seg, w.s, w.off, w.dir = best, bestS, model.Point{}, heading
		return
	}
}

// sees tells whether a workpiece in range makes a sensor switch.
func (p *Plant) sees(s *model.SensorPlan, w *part) bool {
	k := p.kinds[w.kind]
	if s.Has != "" && !p.has(w, s.Has) {
		return false
	}
	switch s.Detect {
	case "metal":
		return k.Metal
	case "reflection":
		return k.Reflectivity >= s.Threshold
	default:
		return k.Transparency < s.Threshold
	}
}

func (p *Plant) hits(s *model.SensorPlan, f foot, z, height float64) bool {
	if p.beamHits(s.Origin, s.Dir, s.Reach, f, z, height) {
		return true
	}
	return s.Mirror != nil && p.beamHits(s.Mirror.Origin, s.Mirror.Dir, s.Mirror.Reach, f, z, height)
}

// beamHits reports whether a beam meets an upright workpiece with the footprint f and its
// base at height z.
func (p *Plant) beamHits(origin, dir model.Point, reach float64, f foot, z, height float64) bool {
	flat := dir.X*dir.X + dir.Y*dir.Y
	c := model.Point{X: f.x, Y: f.y, Z: z}
	if flat < 1e-9 {
		// Straight up or down, as a sensor in the floor of a magazine.
		far := origin.Z + dir.Z*reach
		over := math.Hypot(c.X-origin.X, c.Y-origin.Y) < f.r
		if !f.round() {
			over = f.contains(origin.X, origin.Y, 0)
		}
		return over && math.Max(origin.Z, far) >= c.Z && math.Min(origin.Z, far) <= c.Z+height
	}
	if !f.round() {
		t, ok := f.enter(origin.X, origin.Y, dir.X, dir.Y, reach)
		at := origin.Z + t*dir.Z
		return ok && at >= c.Z && at <= c.Z+height
	}
	radius := f.r
	vx, vy := c.X-origin.X, c.Y-origin.Y
	t := (vx*dir.X + vy*dir.Y) / flat
	px, py := origin.X+t*dir.X-c.X, origin.Y+t*dir.Y-c.Y
	offset := px*px + py*py
	if offset >= radius*radius {
		return false
	}
	// Distance along the beam to where it enters the workpiece.
	enter := t - math.Sqrt((radius*radius-offset)/flat)
	if enter > reach || t+radius < 0 {
		return false
	}
	at := origin.Z + t*dir.Z
	return at >= c.Z && at <= c.Z+height
}

// Inputs returns the input image of the first PLC for the current state.
func (p *Plant) Inputs() plcio.Image { return p.InputsOf(0) }

// InputsOf returns the input image of PLC k, in the order of Plan.PLCs.
func (p *Plant) InputsOf(k int) plcio.Image {
	var in plcio.Image
	for i := range p.plan.PLCIn {
		if s := &p.plan.PLCIn[i]; s.PLC == k {
			in.Set(s.Addr, p.v[s.State] >= 0.5)
		}
	}
	return in
}

// Analog returns the first PLC's analogue inputs, in volts.
func (p *Plant) Analog() plcio.Analog { return p.AnalogOf(0) }

// AnalogOf returns the analogue inputs of PLC k, in volts.
func (p *Plant) AnalogOf(k int) plcio.Analog {
	var in plcio.Analog
	for _, a := range p.plan.PLCAnalog {
		if a.PLC == k {
			in[a.Channel] = p.v[a.State]
		}
	}
	return in
}

// States returns a copy of all state values, indexed like Scene.States.
func (p *Plant) States() []float64 { return append([]float64(nil), p.v...) }

// WorkpieceView is a workpiece as seen by a renderer.
type WorkpieceView struct {
	ID   int         `json:"id"`
	Kind string      `json:"kind"`
	At   model.Point `json:"at"`
	Up   model.Point `json:"up"`
	// Heading is the way its length points, level, for one with a size.
	Heading model.Point `json:"heading"`
	// Loads are what was set onto it, from the bottom up, and Marks what processes left.
	Loads []LoadView         `json:"loads,omitempty"`
	Marks map[string]float64 `json:"marks,omitempty"`
	// On is the transport it is on, and S how far along it, unless it is held or falling.
	On         string  `json:"on,omitempty"`
	S          float64 `json:"s"`
	UpsideDown bool    `json:"upsideDown,omitempty"`
}

// LoadView is a workpiece set onto another one: its kind, where its base is from the other's,
// along the other's heading, to its left and up, and the way it points in that frame.
type LoadView struct {
	Kind    string      `json:"kind"`
	At      model.Point `json:"at"`
	Heading model.Point `json:"heading"`
}

// Workpieces returns all workpieces for rendering.
func (p *Plant) Workpieces() []WorkpieceView {
	out := make([]WorkpieceView, 0, len(p.parts))
	for i := range p.parts {
		w := &p.parts[i]
		up := p.segments[w.seg].up
		if f := w.fall; f != nil {
			sin, cos := math.Sincos(f.tumbled)
			up = model.Point{X: f.dir.X * sin, Y: f.dir.Y * sin, Z: cos}
		}
		if w.gone {
			continue
		}
		view := WorkpieceView{ID: w.id, Kind: w.kind, At: p.world(w), Up: up, Heading: p.headingOf(w), Marks: w.marks, UpsideDown: w.upsideDown}
		for _, l := range w.loads {
			view.Loads = append(view.Loads, LoadView{Kind: l.kind, At: l.off, Heading: l.dir})
		}
		if _, held := p.carrier[w.seg]; !held && w.fall == nil {
			view.On, view.S = p.segments[w.seg].Name, w.s
		}
		if w.upsideDown {
			// Turned over about its middle: the base it is drawn from is on top.
			h := p.heightOf(w)
			view.At = model.Point{X: view.At.X + up.X*h, Y: view.At.Y + up.Y*h, Z: view.At.Z + up.Z*h}
			view.Up = model.Point{X: -up.X, Y: -up.Y, Z: -up.Z}
		}
		out = append(out, view)
	}
	return out
}

// Take takes a workpiece away by hand, by its id.
func (p *Plant) Take(id int) error {
	i := slices.IndexFunc(p.parts, func(w part) bool { return w.id == id && !w.gone })
	if i < 0 {
		return fmt.Errorf("there is no workpiece %d", id)
	}
	p.parts = slices.Delete(p.parts, i, i+1)
	p.updateSensors(0)
	p.copyNets()
	return nil
}

// Bins returns the workpiece kinds on each transport that shows a fill level.
func (p *Plant) Bins() [][]string {
	out := make([][]string, len(p.plan.Bins))
	for i, seg := range p.plan.Bins {
		out[i] = []string{}
		for j := range p.parts {
			if p.parts[j].seg == seg {
				out[i] = append(out[i], p.parts[j].kind)
			}
		}
	}
	return out
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// turned returns a point turned about the vertical through the origin.
func turned(a model.Point, by float64) model.Point {
	sin, cos := math.Sincos(by)
	return model.Point{X: a.X*cos - a.Y*sin, Y: a.X*sin + a.Y*cos, Z: a.Z}
}

func dist(a, b model.Point) float64 {
	dx, dy, dz := a.X-b.X, a.Y-b.Y, a.Z-b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
