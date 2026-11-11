package model

import (
	"fmt"
	"github.com/toblu96/taktwin/st"
	"go/token"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/toblu96/taktwin/internal/plcio"
)

// Point is a resolved position in station coordinates (mm, z up).
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// BranchPlan sends workpieces to another segment while a state is high, or always
// if State is -1.
type BranchPlan struct {
	At    float64
	State int
	To    int
}

// SegmentPlan is a transport. Arc is set for curves, otherwise it is a line from
// From to To.
type SegmentPlan struct {
	Name      string
	Count     int  // state: how many workpieces are on it
	Before    int  // state: on while another transport takes workpieces over at its start
	HandsOver bool // its end should lead onto another module's transport
	From, To  Point
	Arc       bool
	Center    Point
	Radius    float64
	Start     float64 // radians
	Sweep     float64 // radians
	Length    float64
	Open      bool // workpieces fall off its ends
	Drive     int  // state index, or -1 if gravity driven
	Reverse   int  // state index that runs it backwards, or -1
	Factor    int  // state index that scales its speed, or -1
	Speed     float64
	FreeFrom  float64 // distance from which it moves without its drive, or -1
	Branches  []BranchPlan
	Next      int // segment after the end, or -1
	// NextAt is where on Next a workpiece arrives: below 0 where it crosses a gap, such as
	// from the belt of one station onto the next one's.
	NextAt float64
	// Across is set where Next runs across the end of this one, which hands its workpieces
	// over the side of Next, as a belt that ends beside another one in a corner.
	Across bool
	// Prev is the segment a workpiece running backwards goes onto past PrevAt, at or below 0,
	// as when a belt hands workpieces back to the one before it, or -1.
	Prev   int
	PrevAt float64
	Travel int // state index of the distance travelled
	// Rides is the chain of the node it moves with, and Local its ends in that node's frame.
	// From and To are where they are at rest. Its handovers are worked out as it moves.
	Rides *Chain
	Local [2]Point
}

// CylinderPlan is a pneumatic cylinder.
// DistancePlan is a sensor that measures straight down.
type DistancePlan struct {
	Origin    Point
	Min, Max  float64
	Surface   float64 // distance when nothing is below it
	Threshold float64
	Detected  int
	Value     int    // volts
	Distance  int    // mm
	Rides     *Chain // the node it moves with, and Local its origin in that node's frame
	Local     Point
}

// GripperPlan holds a workpiece at the end of a chain of joints.
type GripperPlan struct {
	Name     string
	Tip      Chain
	Position int // of the cylinder that opens the jaws, or -1
	Open     int // that cylinder is commanded open, or -1
	Hold     int // a state that holds while on, as a vacuum, or -1
	Metal    bool
	Segment  int // where a held workpiece is, so everything treats it like any other
	Holding  int
	Reach    float64
}

// ProcessPlan marks the workpiece under Origin once On has been on for Time seconds.
type ProcessPlan struct {
	Name           string
	Origin         Point
	Reach          float64
	Mark           string
	Amount         float64
	Adds           bool
	Time           float64
	On, Busy, Done int
}

// LogicPlan works out a state from others: Expr, or a latch that Set turns on and Reset off.
// A change of On waits OnDelay or OffDelay seconds. Value is Expr itself, or On for a latch.
// ProgramPlan is a module's own controller in Structured Text: its program, how often it
// runs, and which states its variables take their values from and give theirs to.
type ProgramPlan struct {
	Name    string
	Program *st.Program
	POU     string
	Cycle   float64
	In, Out []VarBinding
}

// VarBinding joins a variable of a program, by its path, to a state.
type VarBinding struct {
	Path  string
	State int
}

// SafetyPlan is a safety relay: it opens while Reset holds, and closes while Set does.
type SafetyPlan struct {
	Name       string
	Set, Reset Expr
	OK         int
}

type LogicPlan struct {
	Name              string
	Expr              Expr
	Set, Reset        Expr
	OnDelay, OffDelay float64
	On, Value         int
	Drives            int // a device input it sets, or -1
}

// PusherPlan moves workpieces with a cylinder, from the bottom of a stack along a path. From
// is -1 for one that takes whatever stands at the start of To.
type PusherPlan struct {
	Name     string
	Position int
	From, To int // segments
}

// ObstaclePlan is the box around a fixed part of the station, in station coordinates. What
// moves stops against it. Its edges start at Origin and do not have to be upright.
type ObstaclePlan struct {
	Root    string // the top-level module it belongs to, whose own moving parts ignore it
	Name    string
	Origin  Point
	U, V, W Point
}

// SolidPlan is a box that workpieces cannot enter, and that cannot move into them.
type SolidPlan struct {
	Name     string
	Root     string // the top-level module it belongs to
	Label    string
	Shoves   bool // pushes a workpiece along its transport instead of stopping
	Chain    Chain
	Min, Max Point
	Outline  []Point // its shape from above, in the same frame, if a box does not do
}

type CylinderPlan struct {
	Name      string
	Bistable  bool
	RestOut   bool
	Extend    int // state index, or -1
	Retract   int // state index, or -1
	Travel    float64
	Position  int
	Extended  int
	Retracted int
	Commanded int
	Midway    int
	Limited   bool    // stops on what it presses on without that being a collision
	Stop      int     // a state that holds it at StopAt on its way out, or -1
	StopAt    float64 // 0 to 1
}

// AxisPlan is a motor that moves to any position: Position runs from 0 to 1 over Stroke
// in Travel seconds, and Value is how far that is. With Targets, Go drives it to the one that
// the Records inputs select in binary, and Done says it is there.
type AxisPlan struct {
	Name               string
	Stroke, Travel     float64
	Wrap               bool
	Forward, Backward  int // inputs
	Position, Value    int
	Start, End, Moving int
	Targets            []float64 // mm or degrees from the start
	Records            []int     // inputs, the lowest bit first
	Go, Done           int       // -1 without targets or a setpoint
	Setpoint           int       // input with the value to drive to, or -1
	Enable             int       // input without which it does not move, or -1
	Ready, Homed       int       // states of its controller, or -1
	Error              int
}

// maxRecordBits is how many record inputs a positioning axis has at most.
const maxRecordBits = 3

// recordBits is how many record inputs select one of n targets.
func recordBits(n int) int {
	bits := 0
	for 1<<bits < n {
		bits++
	}
	return bits
}

// MotorControllerPlan is a DC motor controller.
type MotorControllerPlan struct {
	Clockwise, Counterclockwise, Creep int // inputs
	Forward, Backward, Running, Factor int // outputs
	Ready                              int
}

// BeamPlan is a straight piece of a beam.
type BeamPlan struct {
	Origin, Dir Point
	Reach       float64
}

// SensorPlan is a beam sensor.
type SensorPlan struct {
	Free   int // the opposite of Detected, as a light-on output
	Name   string
	Origin Point
	Dir    Point
	Reach  float64
	Mirror *BeamPlan // the rest of a beam that is folded by a mirror
	Detect string
	// Threshold is in percent of the light, for reflection and beam sensors.
	Threshold float64
	OffDelay  float64
	Detected  int
	Fault     bool
	Has       string // a mark or joined part a workpiece needs to be seen
	// Rides is the chain of the node it moves with, as on a lift's platform, and Local its
	// origin and a point 1 mm along its beam in that node's frame.
	Rides *Chain
	Local [2]Point
}

// CopyPlan drives a net from a state or a constant.
type CopyPlan struct {
	From     int // state index, or -1 for a constant
	Invert   bool
	Constant bool
	Analog   bool // keeps the value, instead of on or off
	To       int
}

// PLCAnalogPlan connects an analogue input of the PLC to a net.
type PLCAnalogPlan struct {
	Channel int
	State   int
	PLC     int // index in Plan.PLCs
}

// PLCBitPlan connects one bit of a PLC's image to a net.
type PLCBitPlan struct {
	Addr  plcio.Addr
	State int
	PLC   int // index in Plan.PLCs
	// Switched is an output that reaches a switched connector, which a safety relay cuts.
	Switched bool
}

// ActionPlan is a resolved function of the model.
type ActionPlan struct {
	Type      string
	Label     string
	Dock      bool
	Segment   int // -1 with Over
	At        float64
	BlockedBy int // state index or -1
	Family    string
	Over      *Point // in station coordinates: the action works on what is under it
	// Heading is the way a workpiece with a size points as it is placed, in station
	// coordinates, or zero to follow the transport.
	Heading Point
}

// Plan is a station with every reference and expression resolved.
type Plan struct {
	States    []string
	Segments  []SegmentPlan
	Cylinders []CylinderPlan
	Axes      []AxisPlan
	Sensors   []SensorPlan
	Motors    []MotorControllerPlan
	Distances []DistancePlan
	Grippers  []GripperPlan
	Solids    []SolidPlan
	Obstacles []ObstaclePlan
	Pushers   []PusherPlan
	Logic     []LogicPlan
	Safety    []SafetyPlan
	Programs  []ProgramPlan
	// Naming is how the program of each PLC, in the order of PLCs, names its variables.
	Naming    []plcio.Naming
	Processes []ProcessPlan
	Controls  map[string]int // device name to state index
	Copies    []CopyPlan
	PLCIn     []PLCBitPlan
	PLCOut    []PLCBitPlan
	PLCAnalog []PLCAnalogPlan
	// PLCs are the instances of the station's PLCs, the first one first: each has an image
	// of its own, and a program of its own runs it.
	PLCs    []string
	Actions map[string]ActionPlan
	Bins    []int // segment index of each entry in Snapshot.Bins
	Scene   Scene
	Program string
	// Sites are the places where modules can be screwed on, as the station is built now.
	Sites []SitePlan
	// Mounted names, for each top-level module screwed onto another, that other one.
	Mounted map[string]string
	// Mounts holds, for each top-level module, how it can be screwed on.
	Mounts map[string][]MountPlan
	// Placed holds where each module with a place is screwed on, contained ones too.
	Placed map[string]PlacedPlan
	// Missing are the parts that modules need and lack.
	Missing []MissingPlan
	// InUse holds every pin, as instance.connector.pin, that leads to a device, so that a
	// free terminal pin can be told from a taken one.
	InUse map[string]bool

	stateIndex map[string]int
	alias      map[string]string
	maps       []PLCMapDef
}

// State returns the index of a named state or pin, or -1. Every name on an
// electrical net gives the same state.
func (p *Plan) State(name string) int {
	for {
		if i, ok := p.stateIndex[name]; ok {
			return i
		}
		next, ok := p.alias[name]
		if !ok {
			return -1
		}
		name = next
	}
}

// Bin returns the index in Snapshot.Bins of a named transport, or -1.
func (p *Plan) Bin(transport string) int {
	for i, seg := range p.Bins {
		if p.Segments[seg].Name == transport {
			return i
		}
	}
	return -1
}

// PLCInput returns the address of a named PLC input.
func (p *Plan) PLCInput(name string) (plcio.Addr, bool) {
	return findSignal(p.Scene.PLC.Inputs, name)
}

// PLCOutput returns the address of a named PLC output.
func (p *Plan) PLCOutput(name string) (plcio.Addr, bool) {
	return findSignal(p.Scene.PLC.Outputs, name)
}

func findSignal(list []SceneSignal, name string) (plcio.Addr, bool) {
	for _, s := range list {
		if s.Name == name {
			return plcio.Addr{Byte: s.Byte, Bit: s.Bit}, true
		}
	}
	return plcio.Addr{}, false
}

// parseAddress reads the name of a variable of the image of PLC k, as its module names them.
func (r *resolver) parseAddress(k int, s string) (input bool, a plcio.Addr, err error) {
	if k >= len(r.plan.Naming) {
		return false, a, fmt.Errorf("address %q is of no PLC", s)
	}
	a, input, err = r.plan.Naming[k].Parse(s)
	return input, a, err
}

// pin is one resolved connector pin.
type pin struct {
	key      string // instance.connector.pin
	inst     *instance
	conn     *ConnectorDef
	name     string
	def      PinDef
	declared bool
}

// instance is a placed module during resolving.
type instance struct {
	def      *InstanceDef
	mod      *ModuleDef
	vars     env
	parent   *instance
	cos, sin float64
	cosX     float64
	sinX     float64
	cosY     float64
	sinY     float64
	at       Point
	rotZ     float64
	rotX     float64
	rotY     float64
	devices  map[string]*DeviceDef
	pins     []*pin
	conns    map[string]*ConnectorDef
}

// resolver turns a Bundle into a Plan. It keeps the first error, with the file
// location it happened in, so callers can resolve without checking every step.
type resolver struct {
	tests     map[string][]SceneTest
	bundle    *Bundle
	plan      *Plan
	where     string
	err       error
	vars      env
	texts     map[string]string
	inst      *instance
	instances map[string]*instance
	order     []*instance
	segs      map[string]int
	alias     map[string]string
	pins      map[string]*pin
	netPLC    map[string]string // net root to PLC address
	netPLCOf  map[string]int    // net root to the PLC that address belongs to
	netPLCOut map[string]bool   // net roots that a PLC output drives
	// cuts holds the gaps cut into each rail, by its full id, in mm along the rail.
	cuts map[string][]gap
	// removed holds the contained modules a station leaves out, by full id.
	removed map[string]bool

	partMaterial string // material a part's "inherit" nodes take
}

func (r *resolver) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("%s: %s", r.where, fmt.Sprintf(format, args...))
	}
}

func (r *resolver) num(n Num) float64 {
	if !n.set {
		return 0
	}
	v, err := n.eval(r.vars)
	if err != nil {
		r.fail("%v", err)
	}
	return v
}

func (r *resolver) numOr(n Num, fallback float64) float64 {
	if !n.set {
		return fallback
	}
	return r.num(n)
}

func (r *resolver) vec(v Vec3) Point { return Point{r.num(v[0]), r.num(v[1]), r.num(v[2])} }

func (r *resolver) enabled(n Num) bool { return r.numOr(n, 1) != 0 }

func (r *resolver) text(s string) string {
	for id, v := range r.vars {
		s = strings.ReplaceAll(s, "{"+id+"}", strconv.FormatFloat(v, 'f', -1, 64))
	}
	for id, v := range r.texts {
		s = strings.ReplaceAll(s, "{"+id+"}", v)
	}
	return s
}

// find returns the root of a net or state name.
func (r *resolver) find(name string) string {
	for {
		next, ok := r.alias[name]
		if !ok {
			return name
		}
		name = next
	}
}

func (r *resolver) union(a, b string) {
	a, b = r.find(a), r.find(b)
	if a != b {
		r.alias[a] = b
	}
}

func (r *resolver) state(name string) int {
	root := r.find(name)
	if i, ok := r.plan.stateIndex[root]; ok {
		r.plan.stateIndex[name] = i
		return i
	}
	i := len(r.plan.States)
	r.plan.States = append(r.plan.States, root)
	r.plan.stateIndex[root] = i
	r.plan.stateIndex[name] = i
	return i
}

func (in *instance) world(p Point) Point {
	d := in.direction(p)
	return Point{in.at.X + d.X, in.at.Y + d.Y, in.at.Z + d.Z}
}

// direction applies the instance's rotation: about x first, then y, then z.
func (in *instance) direction(p Point) Point {
	y, z := p.Y*in.cosX-p.Z*in.sinX, p.Y*in.sinX+p.Z*in.cosX
	x, z := p.X*in.cosY+z*in.sinY, -p.X*in.sinY+z*in.cosY
	return Point{x*in.cos - y*in.sin, x*in.sin + y*in.cos, z}
}

// scoped turns a reference inside a module into a full name. It names a contained
// module, such as "io.X1", or otherwise something of the module itself.
func (r *resolver) scoped(in *instance, name string) string {
	head, rest, ok := strings.Cut(name, ".")
	if child := r.instances[in.def.ID+"/"+head]; ok && child != nil {
		return child.def.ID + "." + rest
	}
	return in.def.ID + "." + name
}

// knownHere reports whether a state is one of a module's devices, or of one of its parts,
// as "lift.atRetracted.on" for the reed switch of its cylinder.
func (r *resolver) knownHere(in *instance, name string) bool {
	if in.knownState(name) {
		return true
	}
	head, rest, ok := strings.Cut(name, ".")
	child := r.instances[in.def.ID+"/"+head]
	return ok && child != nil && child.knownState(rest)
}

// knownState reports whether a local state name exists on a device of a module.
func (in *instance) knownState(name string) bool {
	device, state, ok := strings.Cut(name, ".")
	if !ok {
		return false
	}
	d := in.devices[device]
	if d != nil {
		return deviceHasState(d, state)
	}
	for i := range in.mod.Transport {
		if in.mod.Transport[i].ID == device && (state == "travel" || state == "count" || state == "before") {
			return true
		}
	}
	return false
}

func deviceHasState(d *DeviceDef, state string) bool {
	switch d.Type {
	case "cylinder", "rotarySolenoid", "rotaryDrive":
		switch state {
		case "position", "extended", "retracted", "commanded", "midway":
			return true
		case "extend":
			return d.Valve == "bistable" || d.Rest != "extended"
		case "retract":
			return d.Valve == "bistable" || d.Rest == "extended"
		}
	case "reedSwitch", "lamp":
		return state == "on"
	case "axis":
		switch state {
		case "position", "value", "start", "end", "moving", "forward", "backward":
			return true
		case "setpoint":
			return d.Setpoint
		case "go", "done":
			return len(d.Targets) > 0 || d.Setpoint
		case "enable", "ready", "homed", "error":
			return len(d.Targets) > 0 && d.Enable
		}
		for i := range recordBits(len(d.Targets)) {
			if state == fmt.Sprintf("record%d", i) {
				return true
			}
		}
	case "motorController":
		switch state {
		case "clockwise", "counterclockwise", "creep", "forward", "backward", "running", "factor", "ready":
			return true
		}
	case "sensor":
		return state == "detected" || state == "free"
	case "distanceSensor":
		return state == "detected" || state == "value" || state == "distance"
	case "gripper":
		return state == "holding" || state == "on" && d.Cylinder == ""
	case "logic":
		return state == "on" || state == "value"
	case "process":
		return state == "on" || state == "busy" || state == "done"
	case "button", "key":
		return state == "active"
	case "safetyRelay":
		return state == "ok"
	case "program":
		return slices.Contains(d.inputs, state) || slices.Contains(d.outputs, state)
	}
	return false
}

// local resolves a state of the current module, or one of its pins.
func (r *resolver) local(name string) int {
	full := r.scoped(r.inst, name)
	if p := r.pins[full]; p != nil {
		return r.state(p.key)
	}
	if head, rest, ok := strings.Cut(name, "."); ok {
		// A part the station leaves out never does anything, such as the motor controller
		// that a belt runs with.
		if r.leftOut(r.inst, name) {
			return r.state(r.inst.def.ID + "/" + head + "." + rest)
		}
		if child := r.instances[r.inst.def.ID+"/"+head]; child != nil {
			if !child.knownState(rest) {
				r.fail("unknown state %q", name)
				return 0
			}
			return r.state(full)
		}
	}
	if !r.inst.knownState(name) {
		r.fail("unknown state %q", name)
		return 0
	}
	return r.state(r.inst.def.ID + "." + name)
}

// global resolves a state as "instance.device.state" or "instance.connector.pin".
func (r *resolver) global(name string) int {
	id, rest, ok := strings.Cut(name, ".")
	in := r.instances[id]
	if ok && r.removed[id] {
		return r.state(name)
	}
	if !ok || in == nil {
		r.fail("reference %q does not name a module instance", name)
		return 0
	}
	if r.pins[name] != nil {
		return r.state(name)
	}
	if !in.knownState(rest) {
		r.fail("module %s has no state %q", id, rest)
		return 0
	}
	return r.state(name)
}

func (r *resolver) enter(in *instance) {
	r.inst, r.vars, r.texts = in, in.vars, in.def.Texts
	r.where = "module " + in.mod.ID + " (instance " + in.def.ID + ")"
}

// Resolve evaluates the bundle with the given station parameter values. Missing
// values use the station's defaults.
func (b *Bundle) Resolve(values map[string]float64) (*Plan, error) {
	plan, r, err := b.resolveWith(values, nil)
	if err != nil {
		return nil, err
	}
	// Rails are cut where modules that cut them stand, which is only known once all of
	// them are placed. Then everything is placed again, onto the cut rails.
	cuts, err := r.railCuts()
	if err != nil || len(cuts) == 0 {
		return plan, err
	}
	plan, _, err = b.resolveWith(values, cuts)
	return plan, err
}

func (b *Bundle) resolveWith(values map[string]float64, cuts map[string][]gap) (*Plan, *resolver, error) {
	plan := &Plan{Controls: map[string]int{}, Actions: map[string]ActionPlan{}, stateIndex: map[string]int{}, Program: b.Station.PLC.Program, Placed: map[string]PlacedPlan{}}
	alias := map[string]string{}
	plan.alias = alias
	r := &resolver{
		bundle: b, plan: plan, segs: map[string]int{}, alias: alias,
		pins: map[string]*pin{}, netPLC: map[string]string{}, netPLCOf: map[string]int{}, netPLCOut: map[string]bool{}, instances: map[string]*instance{},
		cuts: cuts, removed: map[string]bool{},
	}
	st := &b.Station
	r.where = "station " + st.ID

	stationVars := env{}
	params := make([]SceneParameter, 0, len(st.Parameters))
	for _, p := range st.Parameters {
		v, ok := values[p.ID]
		if !ok {
			v = p.Default
		}
		if v < p.Min || v > p.Max {
			return nil, nil, fmt.Errorf("%s: parameter %s must be between %g and %g", r.where, p.ID, p.Min, p.Max)
		}
		stationVars[p.ID] = v
		params = append(params, SceneParameter{ID: p.ID, Label: p.Label, Unit: p.Unit, Kind: p.Kind, Description: p.Description, Options: p.Options, Names: p.Names, Value: v, Min: p.Min, Max: p.Max, Step: p.Step, Module: p.Module, Advanced: p.Advanced, Boolean: p.Boolean})
	}

	for i := range st.Modules {
		def := &st.Modules[i]
		r.where = "station " + st.ID + ", module instance " + def.ID
		if !safeID.MatchString(def.ID) || r.instances[def.ID] != nil {
			r.fail("missing, invalid or duplicate instance id")
			return nil, nil, r.err
		}
		r.addInstance(def, stationVars, nil)
		if r.err != nil {
			return nil, nil, r.err
		}
	}

	// Connect everything first, so each electrical net gets a single state.
	for _, in := range r.order {
		r.enter(in)
		r.connectInstance(in)
	}
	for _, in := range r.order {
		r.enter(in)
		r.connectInner(in)
	}
	r.vars, r.texts = stationVars, nil
	r.connectStation(st)
	if r.err != nil {
		return nil, nil, r.err
	}

	scene := Scene{
		ID: st.ID, Name: st.Name, Description: st.Description,
		Materials: b.Materials, Workpieces: b.Workpieces, Parameters: params, Docks: []SceneDock{}, Checks: []SceneCheck{},
	}
	for _, in := range r.order {
		r.enter(in)
		r.devicesOf(in)
		r.obstaclesOf(in)
		r.transports(in)
	}
	r.vars, r.texts = stationVars, nil
	r.links(st)
	r.handovers()
	r.sitePlans()
	for _, in := range r.order {
		r.enter(in)
		r.pushers(in)
		r.logic(in)
	}
	r.vars, r.texts = stationVars, nil
	// On its own, a transport that shares a drive has nothing to share it with.
	if !b.Preview {
		r.checkShared()
	}
	r.needs()
	r.plcBits()
	r.pinDrivers()
	r.pinsInUse()
	if r.err != nil {
		return nil, nil, r.err
	}
	for _, in := range r.order {
		r.enter(in)
		r.actions(in.mod.Actions, in.def.ID+".", func(name string) int { return r.segmentRef(in.def.ID + "." + name) })
	}
	r.where = "station " + st.ID
	r.addPreviewTests()
	plan.maps = st.PLC.Maps
	r.vars, r.texts, r.inst = stationVars, nil, nil
	r.actions(st.Actions, "", r.segmentRef)
	if r.err != nil {
		return nil, nil, r.err
	}
	for _, in := range r.order {
		r.enter(in)
		scene.Instances = append(scene.Instances, r.sceneInstance(in))
	}
	r.where = "station " + st.ID
	r.vars, r.texts, r.inst = stationVars, nil, nil
	scene.Models = r.workpieceModels()
	for i := range st.Docks {
		d := &st.Docks[i]
		if d.Place != "bottom" && d.Place != "side" {
			r.fail("dock place must be bottom or side, not %q", d.Place)
		}
		scene.Docks = append(scene.Docks, SceneDock{Place: d.Place, Widgets: r.widgets(d.Widgets, "")})
	}
	for k := range plan.PLCs {
		def := StationPLCDef{Module: plan.PLCs[k]}
		if k == 0 {
			def = st.PLC
			def.Module = plan.PLCs[0]
		} else if i := slices.IndexFunc(st.PLCs, func(d StationPLCDef) bool { return d.Module == plan.PLCs[k] }); i >= 0 {
			def = st.PLCs[i]
		}
		scene.PLCs = append(scene.PLCs, r.scenePLC(st, def, k))
	}
	if len(scene.PLCs) > 0 {
		scene.PLC = scene.PLCs[0]
	} else {
		scene.PLC = r.scenePLC(st, st.PLC, 0)
		scene.PLCs = []ScenePLC{}
	}
	if r.err != nil {
		return nil, nil, r.err
	}
	scene.States = plan.States
	plan.Scene = scene
	return plan, r, nil
}

// addInstance places a module, and the modules it contains under "id/child".
func (r *resolver) addInstance(def *InstanceDef, outer env, parent *instance) {
	mod := r.bundle.Modules[def.Module]
	in := r.instantiate(def, mod, outer, parent)
	if r.err != nil {
		return
	}
	r.instances[def.ID] = in
	r.order = append(r.order, in)
	for id := range def.Parts {
		if !slices.ContainsFunc(mod.Modules, func(c InstanceDef) bool { return c.ID == id }) {
			r.fail("%s (%s) contains no %s to change", def.ID, mod.ID, id)
			return
		}
	}
	for i := range mod.Modules {
		child := mod.Modules[i]
		r.vars = in.vars
		if !r.enabled(child.When) {
			continue
		}
		if !safeID.MatchString(child.ID) {
			r.fail("module %s has an invalid contained module id %q", mod.ID, child.ID)
			return
		}
		if change, ok := def.Parts[child.ID]; ok {
			switch {
			case change.Removed && !child.Optional:
				r.fail("%s cannot leave out %s: %s needs it", def.ID, child.ID, mod.Name)
				return
			case change.Removed:
				r.removed[def.ID+"/"+child.ID] = true
				continue
			case change.Place != nil && child.Place == nil:
				r.fail("%s of %s is not screwed onto a site, so it cannot be placed elsewhere", child.ID, def.ID)
				return
			case change.Place != nil:
				place := *change.Place
				child.Place = &place
			}
			if len(change.Params) > 0 {
				params := maps.Clone(child.Params)
				if params == nil {
					params = map[string]Num{}
				}
				maps.Copy(params, change.Params)
				child.Params = params
			}
		}
		child.ID = def.ID + "/" + child.ID
		// A contained module can pass its clicks on to the module around it.
		if child.SelectAs == "parent" {
			child.SelectAs = def.ID
		} else if child.SelectAs != "" {
			child.SelectAs = def.ID + "/" + child.SelectAs
		}
		if r.instances[child.ID] != nil {
			r.fail("module %s contains %s twice", mod.ID, child.ID)
			return
		}
		if gaps := r.cuts[child.ID]; len(gaps) > 0 {
			for _, piece := range r.railPieces(&child, in, gaps) {
				r.addInstance(piece, in.vars, in)
			}
			continue
		}
		r.addInstance(&child, in.vars, in)
	}
}

// siteLine is one straight site of a placed module, in the module's own coordinates.
type siteLine struct {
	site     *SiteDef
	slot     int
	from     Point
	dir      Point // unit length
	up       Point // off the face a module stands on
	min, max float64
	gaps     []gap // where its rail is cut, in mm along the site
}

// gap is a stretch cut out of a rail, from and to in mm, for the module By.
type gap struct {
	From, To float64
	By       string
	name     string // how people know By
}

// within returns the gap a position along a site falls into, if any.
func (l *siteLine) within(t float64) *gap {
	for i := range l.gaps {
		if g := &l.gaps[i]; t > g.From+placeTolerance && t < g.To-placeTolerance {
			return g
		}
	}
	return nil
}

// sites lists the site lines of an instance, each repeat on its own.
func (r *resolver) sites(in *instance) []siteLine {
	r.vars = in.vars
	var out []siteLine
	for i := range in.mod.Sites {
		s := &in.mod.Sites[i]
		if !r.enabled(s.When) {
			continue
		}
		from, dir := r.vec(s.From), r.vec(s.Dir)
		if dir == (Point{}) {
			dir = Point{1, 0, 0}
		}
		dir = scale(dir, 1/length(dir))
		up := r.vec(s.Up)
		if up == (Point{}) {
			up = Point{0, 0, 1}
		}
		count, step := 1, Point{}
		if s.Repeat != nil {
			count, step = int(r.num(s.Repeat.Count)), r.vec(s.Repeat.Step)
		}
		gaps := r.siteGaps(in, s, from, dir)
		r.vars = in.vars
		for k := range count {
			f := float64(k)
			out = append(out, siteLine{site: s, slot: k, from: Point{from.X + step.X*f, from.Y + step.Y*f, from.Z + step.Z*f},
				dir: dir, up: up, min: r.num(s.Range[0]), max: r.num(s.Range[1]), gaps: gaps})
		}
	}
	return out
}

// place screws an instance onto a site of a module placed before it, and checks that
// every screw of its mount finds a site of the same type on that module.
func (r *resolver) place(in *instance, parent *instance, outer env) {
	p, def := in.def.Place, in.def
	if def.At[0].set || def.At[1].set || def.At[2].set || def.RotateX.set || def.RotateY.set || def.RotateZ.set {
		r.fail("%s has both a place and at or rotate", def.ID)
		return
	}
	ownerID, siteID, ok := strings.Cut(p.On, ".")
	if !ok {
		r.fail("%s is placed on %q, which names no site (module.site)", def.ID, p.On)
		return
	}
	// A contained module names a sibling, or with an empty name the module around it.
	switch {
	case parent != nil && ownerID == "":
		ownerID = parent.def.ID
	case parent != nil:
		ownerID = parent.def.ID + "/" + ownerID
	}
	owner := r.instances[ownerID]
	if owner == nil {
		r.fail("%s is placed on %s, which is not placed before it", def.ID, ownerID)
		return
	}
	lines := r.sites(owner)
	var line *siteLine
	for i := range lines {
		if lines[i].site.ID == siteID && lines[i].slot == p.Slot {
			line = &lines[i]
		}
	}
	if line == nil {
		r.fail("%s has no site %s with slot %d", ownerID, siteID, p.Slot)
		return
	}
	r.vars = in.vars
	// Of the mounts that fit the site, the first whose screws all find a slot is used, as a
	// module turned across the plate stands on the holes it has for that.
	var mounts []*MountDef
	for i := range in.mod.Mounts {
		m := &in.mod.Mounts[i]
		if r.enabled(m.When) && m.Type == line.site.Type && m.Fits(owner.direction(line.dir)) && (p.Mount == "" || p.Mount == m.ID) {
			mounts = append(mounts, m)
		}
	}
	if len(mounts) == 0 {
		r.fail("%s (%s) has no mount for a site of type %s", def.ID, in.mod.ID, line.site.Type)
		return
	}
	r.vars = outer
	along, turn := r.num(p.At), r.num(p.Turn)
	r.vars = in.vars
	if along < line.min-placeTolerance || along > line.max+placeTolerance {
		r.failPlace(def.ID, "", "%s does not fit there: %g mm is off %s.%s, which runs from %g to %g mm", r.nameOf(in), along, ownerID, siteID, line.min, line.max)
		return
	}
	if g := line.within(along); g != nil {
		r.failPlace(def.ID, g.By, "%s does not fit there: the rail of %s is cut from %.0f to %.0f mm for %s", r.nameOf(in), r.nameOf(owner), g.From, g.To, g.name)
		return
	}
	try := func(mount *MountDef) (by, why string, ok bool) {
		r.vars = owner.vars
		siteTurn := r.num(line.site.Turn)
		r.vars = in.vars
		// The module stands on the site's frame: x along it, z off its face. Then it turns on it,
		// and a mount can stand it up first.
		if length(cross(line.dir, line.up)) < 1e-6 {
			return "", fmt.Sprintf("%s.%s runs along its own up, so give it another up", ownerID, siteID), false
		}
		rot := owner.rotation().mul(frame(line.dir, line.up)).
			mul(rotation(0, 0, siteTurn+r.num(mount.Turn)+turn)).mul(rotation(r.num(mount.RotateX), r.num(mount.RotateY), 0))
		in.setRotation(rot.euler())
		at := owner.world(Point{line.from.X + line.dir.X*along, line.from.Y + line.dir.Y*along, line.from.Z + line.dir.Z*along})
		m := in.direction(r.vec(mount.At))
		in.at = Point{at.X - m.X, at.Y - m.Y, at.Z - m.Z}
		// A screw may go into any site of the same type of the module it is screwed onto, or of
		// a part of the same module, as the other rail of a slide.
		type onLine struct {
			in   *instance
			line siteLine
		}
		var all []onLine
		for _, o := range r.family(owner) {
			for _, l := range r.sites(o) {
				all = append(all, onLine{o, l})
			}
		}
		r.vars = in.vars
		for i, s := range mount.Screws {
			w := in.world(r.vec(s))
			found := false
			var cut *gap
			for _, ol := range all {
				l := ol.line
				if l.site.Type != mount.Type {
					continue
				}
				o, d := ol.in.world(l.from), ol.in.direction(l.dir)
				rel := Point{w.X - o.X, w.Y - o.Y, w.Z - o.Z}
				t := rel.X*d.X + rel.Y*d.Y + rel.Z*d.Z
				off := Point{rel.X - d.X*t, rel.Y - d.Y*t, rel.Z - d.Z*t}
				if length(off) > placeTolerance || t < l.min-placeTolerance || t > l.max+placeTolerance {
					continue
				}
				if g := l.within(t); g != nil {
					cut = g
					continue
				}
				found = true
			}
			if !found && cut != nil {
				return cut.By, fmt.Sprintf("%s does not fit there: the rail of %s is cut from %.0f to %.0f mm for %s", r.nameOf(in), r.nameOf(owner), cut.From, cut.To, cut.name), false
			}
			if !found {
				return "", fmt.Sprintf("%s does not fit there: screw %d of its mount %s, at (%.1f, %.1f, %.1f), finds no %s on %s", r.nameOf(in), i+1, mount.ID, w.X, w.Y, w.Z, mount.Type, ownerID), false
			}
		}
		return "", "", true
	}
	by, why := "", ""
	for k, mount := range mounts {
		b, w, ok := try(mount)
		if ok {
			r.plan.Placed[def.ID] = PlacedPlan{On: ownerID + "." + siteID, Slot: p.Slot, At: along, Turn: turn}
			return
		}
		if k == 0 {
			by, why = b, w
		}
	}
	r.failPlace(def.ID, by, "%s", why)
}

// pinsInUse marks every pin whose net has a device on it.
func (r *resolver) pinsInUse() {
	device := map[string]bool{}
	for _, in := range r.order {
		for _, p := range in.pins {
			if p.def.Reads != "" || p.def.Drives != "" {
				device[r.find(p.key)] = true
			}
		}
	}
	r.plan.InUse = map[string]bool{}
	for _, in := range r.order {
		for _, p := range in.pins {
			if device[r.find(p.key)] {
				r.plan.InUse[p.key] = true
			}
		}
	}
}

// SitePlan is one straight site of a placed module, in station coordinates: a position on
// it is From plus that many mm along Dir, from Min to Max.
type SitePlan struct {
	Owner string  `json:"owner"`
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Slot  int     `json:"slot"`
	From  Point   `json:"from"`
	Dir   Point   `json:"dir"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	// Turn is how much further a module on it is turned, as on a conveyor's rear rail.
	Turn float64 `json:"turn"`
	// Gaps are where its rail is cut, for a module that needs the gap.
	Gaps []SiteGap `json:"gaps"`
}

// SiteGap is a stretch of a site cut out of its rail, from and to in mm along the site, for
// the module By.
type SiteGap struct {
	From float64 `json:"from"`
	To   float64 `json:"to"`
	By   string  `json:"by"`
}

// MissingPlan is a module that lacks parts it needs: Have of the Min modules of Module.
type MissingPlan struct {
	Instance string
	Module   string
	Name     string // of the module that is missing
	Min      int
	Have     int
	Why      string
}

// needs counts, for each module that needs parts, those it contains and those screwed onto
// it or onto what it contains.
func (r *resolver) needs() {
	for _, in := range r.order {
		if len(in.mod.Needs) == 0 {
			continue
		}
		within := func(id string) bool { return id == in.def.ID || strings.HasPrefix(id, in.def.ID+"/") }
		for _, n := range in.mod.Needs {
			have := 0
			for _, other := range r.order {
				if other.mod.ID != n.Module {
					continue
				}
				owner, _, _ := strings.Cut(r.plan.Placed[other.def.ID].On, ".")
				if within(other.def.ID) || owner != "" && within(owner) {
					have++
				}
			}
			if min := max(n.Min, 1); have < min {
				name := n.Module
				if m := r.bundle.Modules[n.Module]; m != nil {
					name = m.Name
				}
				r.plan.Missing = append(r.plan.Missing, MissingPlan{Instance: in.def.ID, Module: n.Module, Name: name, Min: min, Have: have, Why: n.Why})
			}
		}
	}
}

// PlacedPlan is where a module is screwed on: On names the module and its site, as in
// "belt.rear", and At, Slot and Turn are evaluated.
type PlacedPlan struct {
	On   string  `json:"on"`
	Slot int     `json:"slot"`
	At   float64 `json:"at"`
	Turn float64 `json:"turn"`
}

// MountPlan is how a placed module can be screwed on: the kind of site, its point that sits
// on the site, in the module's own coordinates, and how it faces there.
type MountPlan struct {
	Type string  `json:"type"`
	At   Point   `json:"at"`
	Turn float64 `json:"turn"`
}

// sitePlans lists every site of the station in station coordinates, and which top-level
// module is screwed onto which.
func (r *resolver) sitePlans() {
	r.plan.Mounted = map[string]string{}
	r.plan.Mounts = map[string][]MountPlan{}
	for _, in := range r.order {
		if r.coveredRail(in) {
			continue
		}
		for _, l := range r.sites(in) {
			r.vars = in.vars
			gaps := []SiteGap{}
			for _, g := range l.gaps {
				gaps = append(gaps, SiteGap{From: g.From, To: g.To, By: g.By})
			}
			r.plan.Sites = append(r.plan.Sites, SitePlan{
				Owner: in.def.ID, ID: l.site.ID, Type: l.site.Type, Slot: l.slot,
				From: in.world(l.from), Dir: in.direction(l.dir), Min: l.min, Max: l.max, Turn: r.num(l.site.Turn), Gaps: gaps,
			})
		}
		if in.parent == nil {
			r.vars = in.vars
			for i := range in.mod.Mounts {
				if m := &in.mod.Mounts[i]; r.enabled(m.When) {
					r.plan.Mounts[in.def.ID] = append(r.plan.Mounts[in.def.ID], MountPlan{Type: m.Type, At: r.vec(m.At), Turn: r.num(m.Turn)})
				}
			}
		}
		if p := in.def.Place; p != nil && in.parent == nil {
			owner, _, _ := strings.Cut(p.On, ".")
			r.plan.Mounted[in.def.ID], _, _ = strings.Cut(owner, "/")
		}
	}
}

// coveredRail is a rail, or a piece of one, whose groove the module around it offers as a
// site of its own, so that positions on it count the way that module does.
func (r *resolver) coveredRail(in *instance) bool {
	if in.parent == nil {
		return false
	}
	local := strings.TrimPrefix(in.def.ID, in.parent.def.ID+"/")
	for _, s := range in.parent.mod.Sites {
		if s.Rail != "" && (local == s.Rail || strings.HasPrefix(local, s.Rail+"-")) {
			return true
		}
	}
	return false
}

// family is a placed module and the other parts of the same top-level module, without the
// rails that the module covers with sites of its own.
func (r *resolver) family(owner *instance) []*instance {
	root := rootOf(owner)
	out := []*instance{owner}
	for _, o := range r.order {
		if o != owner && rootOf(o) == root && !r.coveredRail(o) {
			out = append(out, o)
		}
	}
	return out
}

// span is how far, in mm, the site an instance is placed on lies from the nearest other
// site of the same type that runs alongside it, on the module or a part of it, as the rail
// across a belt or a slide from the one a sensor stands on. It is false where there is none.
func (r *resolver) span(def *InstanceDef, parent *instance, outer env) (float64, bool) {
	p := def.Place
	ownerID, siteID, ok := strings.Cut(p.On, ".")
	if !ok {
		return 0, false
	}
	switch {
	case parent != nil && ownerID == "":
		ownerID = parent.def.ID
	case parent != nil:
		ownerID = parent.def.ID + "/" + ownerID
	}
	owner := r.instances[ownerID]
	if owner == nil {
		return 0, false
	}
	var here *siteLine
	lines := r.sites(owner)
	for i := range lines {
		if lines[i].site.ID == siteID && lines[i].slot == p.Slot {
			here = &lines[i]
		}
	}
	if here == nil {
		return 0, false
	}
	saved := r.vars
	r.vars = outer
	along := r.num(p.At)
	r.vars = saved
	at := owner.world(Point{here.from.X + here.dir.X*along, here.from.Y + here.dir.Y*along, here.from.Z + here.dir.Z*along})
	dir := owner.direction(here.dir)
	best, found := math.Inf(1), false
	for _, o := range r.family(owner) {
		for _, l := range r.sites(o) {
			if l.site.Type != here.site.Type || o == owner && l.site == here.site && l.slot == here.slot {
				continue
			}
			d := o.direction(l.dir)
			if math.Abs(dot(d, dir)) < 0.999 {
				continue
			}
			from := o.world(l.from)
			rel := Point{at.X - from.X, at.Y - from.Y, at.Z - from.Z}
			t := dot(rel, d)
			off := length(Point{rel.X - d.X*t, rel.Y - d.Y*t, rel.Z - d.Z*t})
			if off > 1 && off < best {
				best, found = off, true
			}
		}
	}
	return best, found
}

// placeTolerance is how far in mm a screw may be off its slot.
const placeTolerance = 0.1

func length(p Point) float64 { return math.Sqrt(p.X*p.X + p.Y*p.Y + p.Z*p.Z) }

func scale(p Point, f float64) Point { return Point{p.X * f, p.Y * f, p.Z * f} }

func (r *resolver) instantiate(def *InstanceDef, mod *ModuleDef, stationVars env, parent *instance) *instance {
	in := &instance{def: def, mod: mod, parent: parent, vars: env{}, devices: map[string]*DeviceDef{}, conns: map[string]*ConnectorDef{}}
	// A mount that spans to the next site, such as the second head of a through-beam sensor
	// to the other rail, gets that distance unless the station gives it.
	spans := map[string]float64{}
	if def.Place != nil {
		for _, m := range mod.Mounts {
			if _, given := def.Params[m.Span]; m.Span != "" && !given {
				if d, ok := r.span(def, parent, stationVars); ok {
					spans[m.Span] = d
				}
			}
		}
	}
	for _, p := range mod.Parameters {
		if v, ok := spans[p.ID]; ok {
			in.vars[p.ID] = v
			continue
		}
		// Expressions are parsed as Go, where keywords cannot be identifiers.
		if !token.IsIdentifier(p.ID) {
			r.fail("parameter name %q is reserved or not a valid name", p.ID)
		}
		r.vars = stationVars
		if o, ok := def.Params[p.ID]; ok {
			in.vars[p.ID] = r.num(o)
		} else {
			r.vars = in.vars
			if !p.Default.set {
				r.fail("parameter %s has no value", p.ID)
			}
			in.vars[p.ID] = r.num(p.Default)
		}
		if err := p.Allows(in.vars[p.ID]); err != nil {
			r.fail("%v", err)
		}
	}
	for id := range def.Params {
		known := false
		for _, p := range mod.Parameters {
			known = known || p.ID == id
		}
		if !known {
			r.fail("module %s has no parameter %q", mod.ID, id)
		}
	}
	r.vars = stationVars
	in.at = r.vec(def.At)
	in.setRotation(r.num(def.RotateX), r.num(def.RotateY), r.num(def.RotateZ))
	if parent != nil {
		in.at = parent.world(in.at)
		in.setRotation(parent.rotation().mul(in.rotation()).euler())
	}
	if def.Place != nil {
		r.place(in, parent, stationVars)
	}
	r.vars = in.vars
	for i := range mod.Devices {
		d := &mod.Devices[i]
		if r.enabled(d.When) {
			in.devices[d.ID] = d
		}
	}
	return in
}

// connectInstance registers the pins of a module and joins its inner nets.
func (r *resolver) connectInstance(in *instance) {
	for i := range in.mod.Terminal.Connectors {
		c := &in.mod.Terminal.Connectors[i]
		if !r.enabled(c.When) {
			continue
		}
		ct, ok := r.bundle.Connectors[c.Type]
		if !ok {
			r.fail("connector %s has unknown type %q", c.ID, c.Type)
			continue
		}
		in.conns[c.ID] = c
		add := func(name string, def PinDef, declared bool) {
			key := in.def.ID + "." + c.ID + "." + name
			p := &pin{key: key, inst: in, conn: c, name: name, def: def, declared: declared}
			r.pins[key] = p
			in.pins = append(in.pins, p)
		}
		declared := map[string]PinDef{}
		for _, d := range c.Pins {
			// A pin can depend on a part, as that of a reed switch a cylinder may come without.
			if d.When.set && !r.enabled(d.When) {
				continue
			}
			declared[d.Pin] = d
		}
		for n := 0; n < ct.Inputs; n++ {
			name := fmt.Sprintf("I%d", n)
			d, ok := declared[name]
			add(name, d, ok)
			delete(declared, name)
		}
		for n := 0; n < ct.Outputs; n++ {
			name := fmt.Sprintf("Q%d", n)
			d, ok := declared[name]
			add(name, d, ok)
			delete(declared, name)
		}
		for _, name := range ct.Analog {
			d, ok := declared[name]
			add(name, d, ok)
			delete(declared, name)
		}
		for _, d := range c.Pins {
			if _, extra := declared[d.Pin]; !extra || d.When.set && !r.enabled(d.When) {
				continue
			}
			if ct.Inputs+ct.Outputs+len(ct.Analog) > 0 {
				r.fail("connector %s (%s) has no pin %q", c.ID, c.Type, d.Pin)
				continue
			}
			add(d.Pin, d, true)
		}
	}
	for _, p := range in.pins {
		if p.def.Drives != "" {
			// An input of the module's own device, or of a part's, such as its cylinder's.
			if !r.knownHere(in, p.def.Drives) {
				r.fail("pin %s drives unknown input %q", p.key, p.def.Drives)
				continue
			}
			r.union(p.key, r.scoped(in, p.def.Drives))
		}
	}
	for _, b := range in.mod.Terminal.Bridges {
		a, c := in.conns[b.A], in.conns[b.B]
		if a == nil || c == nil {
			r.fail("bridge between unknown connectors %s and %s", b.A, b.B)
			continue
		}
		if len(b.Pins) > 0 {
			for from, to := range b.Pins {
				fromKey, toKey := in.def.ID+"."+b.A+"."+from, in.def.ID+"."+b.B+"."+to
				if r.pins[fromKey] == nil || r.pins[toKey] == nil {
					r.fail("bridge from %s to %s does not fit", fromKey, toKey)
					continue
				}
				r.union(fromKey, toKey)
			}
			continue
		}
		ct := r.bundle.Connectors[a.Type]
		for _, kind := range []struct {
			prefix string
			count  int
		}{{"I", ct.Inputs}, {"Q", ct.Outputs}} {
			for n := 0; n < kind.count; n++ {
				from := fmt.Sprintf("%s.%s.%s%d", in.def.ID, b.A, kind.prefix, n)
				to := fmt.Sprintf("%s.%s.%s%d", in.def.ID, b.B, kind.prefix, n+b.Offset)
				if r.pins[from] == nil || r.pins[to] == nil {
					r.fail("bridge from %s to %s does not fit", from, to)
					continue
				}
				r.union(from, to)
			}
		}
	}
	// Devices that are aliases of other states have no logic of their own.
	for _, d := range in.mod.Devices {
		if !r.enabled(d.When) {
			continue
		}
		name := in.def.ID + "." + d.ID
		switch d.Type {
		case "axis":
			if d.Forward != "" {
				r.union(name+".forward", r.scoped(in, d.Forward))
			}
			if d.Backward != "" {
				r.union(name+".backward", r.scoped(in, d.Backward))
			}
		case "reedSwitch":
			c := in.devices[d.Cylinder]
			if c == nil || (c.Type != "cylinder" && c.Type != "rotarySolenoid" && c.Type != "rotaryDrive") || (d.At != "extended" && d.At != "retracted") {
				r.fail("reed switch %s needs a cylinder and an end position", d.ID)
				continue
			}
			r.union(name+".on", in.def.ID+"."+d.Cylinder+"."+d.At)
		case "lamp":
			if d.State != "" {
				full := in.def.ID + "." + d.State
				if r.pins[full] == nil && !in.knownState(d.State) {
					r.fail("lamp %s shows unknown state %q", d.ID, d.State)
					continue
				}
				r.union(name+".on", full)
			}
		}
	}
}

func (r *resolver) connectStation(st *StationDef) {
	for i := range st.Cables {
		c := &st.Cables[i]
		r.where = fmt.Sprintf("station %s, cables[%d]", st.ID, i)
		if r.removedRef(c.From) || r.removedRef(c.To) {
			continue
		}
		a, b := r.connectorRef(c.From), r.connectorRef(c.To)
		if a == nil || b == nil {
			return
		}
		if a.Type != b.Type {
			r.fail("%s is a %s connector and %s a %s connector", c.From, a.Type, c.To, b.Type)
			return
		}
		ct := r.bundle.Connectors[a.Type]
		for n := 0; n < ct.Inputs; n++ {
			r.union(fmt.Sprintf("%s.I%d", c.From, n), fmt.Sprintf("%s.I%d", c.To, n))
		}
		for n := 0; n < ct.Outputs; n++ {
			r.union(fmt.Sprintf("%s.Q%d", c.From, n), fmt.Sprintf("%s.Q%d", c.To, n))
		}
		for _, name := range ct.Analog {
			r.union(c.From+"."+name, c.To+"."+name)
		}
	}
	for i := range st.Wires {
		w := &st.Wires[i]
		r.where = fmt.Sprintf("station %s, wires[%d]", st.ID, i)
		if r.removedRef(w.From) || r.removedRef(w.To) {
			continue
		}
		r.wire(w.From, w.To)
	}
	r.where = "station " + st.ID
}

func (r *resolver) wire(from, to string) {
	if r.pins[from] == nil || r.pins[to] == nil {
		missing := from
		if r.pins[from] != nil {
			missing = to
		}
		r.fail("%q is not a pin (instance.connector.pin)", missing)
		return
	}
	r.union(from, to)
}

// connectInner applies the cables and wires inside a module.
func (r *resolver) connectInner(in *instance) {
	for i := range in.mod.Cables {
		c := &in.mod.Cables[i]
		if !r.enabled(c.When) || r.leftOut(in, c.From) || r.leftOut(in, c.To) {
			continue
		}
		from, to := r.scoped(in, c.From), r.scoped(in, c.To)
		a, b := r.connectorRef(from), r.connectorRef(to)
		if a == nil || b == nil {
			return
		}
		if a.Type != b.Type {
			r.fail("%s is a %s connector and %s a %s connector", from, a.Type, to, b.Type)
			return
		}
		ct := r.bundle.Connectors[a.Type]
		for n := 0; n < ct.Inputs; n++ {
			r.union(fmt.Sprintf("%s.I%d", from, n), fmt.Sprintf("%s.I%d", to, n))
		}
		for n := 0; n < ct.Outputs; n++ {
			r.union(fmt.Sprintf("%s.Q%d", from, n), fmt.Sprintf("%s.Q%d", to, n))
		}
		for _, name := range ct.Analog {
			r.union(from+"."+name, to+"."+name)
		}
	}
	moved := func(end string) string {
		if to, ok := in.def.Rewire[end]; ok {
			return to
		}
		return end
	}
	for i := range in.mod.Wires {
		w := &in.mod.Wires[i]
		if r.enabled(w.When) && !r.leftOut(in, w.From) && !r.leftOut(in, w.To) {
			r.wire(r.scoped(in, moved(w.From)), r.scoped(in, moved(w.To)))
		}
	}
}

// leftOut reports whether a reference inside a module names a contained module that the
// station leaves out.
func (r *resolver) leftOut(in *instance, name string) bool {
	head, _, _ := strings.Cut(name, ".")
	return r.removed[in.def.ID+"/"+head]
}

// removedRef is a reference of the station, such as "belt/io.T.I3", to a part it leaves out.
func (r *resolver) removedRef(name string) bool {
	id, _, _ := strings.Cut(name, ".")
	return r.removed[id]
}

func (r *resolver) connectorRef(name string) *ConnectorDef {
	id, conn, ok := strings.Cut(name, ".")
	in := r.instances[id]
	if !ok || in == nil || in.conns[conn] == nil {
		r.fail("%q is not a connector (instance.connector)", name)
		return nil
	}
	return in.conns[conn]
}

func (r *resolver) devicesOf(in *instance) {
	inst := in.def.ID
	for i := range in.mod.Devices {
		d := &in.mod.Devices[i]
		if !r.enabled(d.When) {
			continue
		}
		name := inst + "." + d.ID
		switch d.Type {
		case "cylinder", "rotarySolenoid", "rotaryDrive":
			r.cylinder(d, name)
		case "axis":
			stroke, speed := r.num(d.Stroke), r.num(d.Speed)
			if stroke <= 0 || speed <= 0 {
				r.fail("axis %s needs a positive stroke and speed", d.ID)
				continue
			}
			a := AxisPlan{
				Name: name, Stroke: stroke, Travel: stroke / speed, Wrap: d.Wrap,
				Forward: r.state(name + ".forward"), Backward: r.state(name + ".backward"), Position: r.state(name + ".position"),
				Value: r.state(name + ".value"), Start: r.state(name + ".start"), End: r.state(name + ".end"), Moving: r.state(name + ".moving"),
				Go: -1, Done: -1, Enable: -1, Ready: -1, Homed: -1, Error: -1, Setpoint: -1,
			}
			if d.Setpoint {
				if len(d.Targets) > 0 || d.Wrap {
					r.fail("axis %s drives either to a setpoint or to targets, and not round", d.ID)
					continue
				}
				a.Setpoint, a.Go, a.Done = r.state(name+".setpoint"), r.state(name+".go"), r.state(name+".done")
			}
			if len(d.Targets) > 0 {
				if d.Wrap || len(d.Targets) > 1<<maxRecordBits {
					r.fail("axis %s can only have targets without wrap, and up to %d", d.ID, 1<<maxRecordBits)
					continue
				}
				for _, t := range d.Targets {
					v := r.num(t)
					if v < 0 || v > stroke {
						r.fail("axis %s has a target of %g, beyond its stroke of %g", d.ID, v, stroke)
					}
					a.Targets = append(a.Targets, math.Max(0, math.Min(stroke, v)))
				}
				for i := range recordBits(len(d.Targets)) {
					a.Records = append(a.Records, r.state(fmt.Sprintf("%s.record%d", name, i)))
				}
				a.Go, a.Done = r.state(name+".go"), r.state(name+".done")
				if d.Enable {
					a.Enable, a.Ready, a.Homed, a.Error = r.state(name+".enable"), r.state(name+".ready"), r.state(name+".homed"), r.state(name+".error")
				}
			}
			r.plan.Axes = append(r.plan.Axes, a)
		case "sensor":
			if d.Detect != "metal" && d.Detect != "reflection" && d.Detect != "beam" {
				r.fail("sensor %s cannot detect %q", d.ID, d.Detect)
			}
			s := SensorPlan{
				Name: name, Free: r.state(name + ".free"), Origin: in.world(r.vec(d.Origin)), Dir: in.direction(r.vec(d.Direction)), Reach: r.num(d.Reach),
				Detect: d.Detect, Threshold: r.num(d.Threshold), OffDelay: r.num(d.OffDelay), Detected: r.state(name + ".detected"), Fault: d.Fault != "", Has: d.Has,
				Mirror: r.mirror(in, d),
			}
			if c := r.rider(in); c != nil {
				if s.Mirror != nil {
					r.fail("sensor %s cannot fold its beam with a mirror while it rides on %s", d.ID, in.def.MountedOn)
				}
				s.Rides, s.Local = c, [2]Point{c.Local(s.Origin), c.Local(Point{s.Origin.X + s.Dir.X, s.Origin.Y + s.Dir.Y, s.Origin.Z + s.Dir.Z})}
			}
			r.plan.Sensors = append(r.plan.Sensors, s)
		case "button", "key":
			r.plan.Controls[name] = r.state(name + ".active")
		case "motorController":
			r.plan.Motors = append(r.plan.Motors, MotorControllerPlan{
				Clockwise: r.state(name + ".clockwise"), Counterclockwise: r.state(name + ".counterclockwise"), Creep: r.state(name + ".creep"),
				Forward: r.state(name + ".forward"), Backward: r.state(name + ".backward"), Running: r.state(name + ".running"),
				Factor: r.state(name + ".factor"), Ready: r.state(name + ".ready"),
			})
		case "distanceSensor":
			dp := DistancePlan{
				Origin: in.world(r.vec(d.Origin)), Min: r.num(d.Min), Max: r.num(d.Reach), Surface: r.num(d.Surface), Threshold: r.num(d.Threshold),
				Detected: r.state(name + ".detected"), Value: r.state(name + ".value"), Distance: r.state(name + ".distance"),
			}
			if c := r.rider(in); c != nil {
				dp.Rides, dp.Local = c, c.Local(dp.Origin)
			}
			r.plan.Distances = append(r.plan.Distances, dp)
		case "gripper":
			tip, ok := r.chainTo(in, d.Tip)
			if !ok {
				r.fail("gripper %s has no node %q", d.ID, d.Tip)
				continue
			}
			r.plan.Segments = append(r.plan.Segments, SegmentPlan{
				Name: name + ".held", Length: 1, Drive: -1, Reverse: -1, Factor: -1, FreeFrom: -1, Next: -1, Prev: -1, Travel: r.state(name + ".held.travel"), Count: r.state(name + ".held.count"), Before: r.state(name + ".held.before"),
			})
			g := GripperPlan{Name: name, Tip: *tip, Position: -1, Open: -1, Hold: -1, Segment: len(r.plan.Segments) - 1, Holding: r.state(name + ".holding"), Reach: r.num(d.Reach)}
			switch {
			case d.Cylinder != "" && d.State == "":
				g.Position, g.Open = r.local(d.Cylinder+".position"), r.local(d.Cylinder+".commanded")
			case d.State != "" && d.Cylinder == "":
				g.Hold = r.local(d.State)
			case d.Cylinder == "":
				g.Hold = r.state(name + ".on")
			default:
				r.fail("gripper %s has jaws, so it holds with them and not by a state", d.ID)
			}
			if d.Detect != "" && d.Detect != "metal" {
				r.fail("gripper %s can only be limited to metal, not %q", d.ID, d.Detect)
			}
			g.Metal = d.Detect == "metal"
			r.plan.Grippers = append(r.plan.Grippers, g)
		case "solid":
			chain, ok := r.chainTo(in, d.Node)
			if !ok || d.Box == nil || d.Label == "" {
				r.fail("solid %s needs a box, a label and an existing node, not %q", d.ID, d.Node)
				continue
			}
			from, to := r.vec(d.Box.From), r.vec(d.Box.To)
			var outline []Point
			for _, q := range d.Outline {
				outline = append(outline, Point{X: r.num(q[0]), Y: r.num(q[1])})
			}
			r.plan.Solids = append(r.plan.Solids, SolidPlan{
				Name: name, Root: rootOf(in), Label: d.Label, Shoves: d.Shoves, Chain: *chain, Outline: outline,
				Min: Point{math.Min(from.X, to.X), math.Min(from.Y, to.Y), math.Min(from.Z, to.Z)},
				Max: Point{math.Max(from.X, to.X), math.Max(from.Y, to.Y), math.Max(from.Z, to.Z)},
			})
		case "process":
			if d.Mark == "" || !slices.ContainsFunc(r.bundle.Workpieces.Marks, func(m MarkDef) bool { return m.ID == d.Mark }) {
				r.fail("process %s needs a mark that the workpieces know, not %q", d.ID, d.Mark)
				continue
			}
			p := ProcessPlan{
				Name: name, Origin: in.world(r.vec(d.Origin)), Reach: r.num(d.Reach), Mark: d.Mark, Amount: r.numOr(d.Amount, 1), Adds: d.Adds, Time: r.num(d.Time),
				On: r.state(name + ".on"), Busy: r.state(name + ".busy"), Done: r.state(name + ".done"),
			}
			if p.Reach <= 0 || p.Time < 0 {
				r.fail("process %s needs a reach and a time that is not negative", d.ID)
			}
			r.plan.Processes = append(r.plan.Processes, p)
		case "lamp", "reedSwitch", "plc", "pusher", "logic", "safetyRelay", "program":
		default:
			r.fail("unknown device type %q", d.Type)
		}
	}
}

// logic resolves the logic devices of a module once all its states are known.
func (r *resolver) logic(in *instance) {
	for i := range in.mod.Devices {
		d := &in.mod.Devices[i]
		if d.Type == "program" && r.enabled(d.When) {
			r.program(in, d)
			continue
		}
		if d.Type == "safetyRelay" && r.enabled(d.When) {
			name := in.def.ID + "." + d.ID
			if d.Set == "" || d.Reset == "" {
				r.fail("safety relay %s needs a set and a reset", d.ID)
				continue
			}
			r.plan.Safety = append(r.plan.Safety, SafetyPlan{Name: name, Set: r.expr(d.Set), Reset: r.expr(d.Reset), OK: r.state(name + ".ok")})
			continue
		}
		if d.Type != "logic" || !r.enabled(d.When) {
			continue
		}
		name := in.def.ID + "." + d.ID
		l := LogicPlan{Name: name, On: r.state(name + ".on"), Value: r.state(name + ".value"), OnDelay: r.num(d.OnDelay), OffDelay: r.num(d.OffDelay), Drives: -1}
		switch {
		case d.Expr != "" && d.Set == "" && d.Reset == "":
			l.Expr = r.expr(d.Expr)
		case d.Expr == "" && d.Set != "" && d.Reset != "":
			l.Set, l.Reset = r.expr(d.Set), r.expr(d.Reset)
		default:
			r.fail("logic %s needs either an expr, or a set and a reset", d.ID)
			continue
		}
		if d.Drives != "" {
			l.Drives = r.local(d.Drives)
		}
		if l.OnDelay < 0 || l.OffDelay < 0 {
			r.fail("logic %s cannot be delayed by less than nothing", d.ID)
		}
		r.plan.Logic = append(r.plan.Logic, l)
	}
}

// program resolves a module's own controller: its inputs and outputs, and what it reads
// and writes of the module.
func (r *resolver) program(in *instance, d *DeviceDef) {
	name := in.def.ID + "." + d.ID
	if d.compiled == nil {
		r.fail("program %s has no source", d.ID)
		return
	}
	cycle := 0.01
	if d.Cycle.Set() {
		cycle = r.num(d.Cycle)
	}
	if cycle <= 0 {
		r.fail("program %s needs a cycle longer than nothing", d.ID)
		return
	}
	p := ProgramPlan{Name: name, Program: d.compiled, POU: programPOU(d), Cycle: cycle}
	bound := map[string]bool{}
	bind := func(list *[]VarBinding, from map[string]string, input bool) {
		for _, v := range slices.Sorted(maps.Keys(from)) {
			names, states, err := expandBinding(v, from[v])
			if err != nil {
				r.fail("program %s: %v", d.ID, err)
				return
			}
			base, _, _ := strings.Cut(v, "[")
			vars, section := d.outputs, "VAR_OUTPUT"
			if input {
				vars, section = d.inputs, "VAR_INPUT"
			}
			if !slices.ContainsFunc(vars, func(n string) bool { return strings.EqualFold(n, base) }) {
				r.fail("program %s has no %s variable %s", d.ID, section, base)
				continue
			}
			for k := range names {
				bound[strings.ToUpper(strings.Split(names[k], "[")[0])] = true
				ref := -1
				if station, ok := strings.CutPrefix(r.text(states[k]), "/"); ok {
					// A state of the station, by the name the station gives it in its texts.
					if !input {
						r.fail("program %s can only read %s, which is not of its module", d.ID, states[k])
						continue
					}
					if unresolvedText.MatchString(station) {
						// A station that names none, and the module tried out on its own, leave
						// the variable at its initial value.
						continue
					}
					ref = r.global(station)
				} else {
					ref = r.local(states[k])
				}
				if ref < 0 {
					continue
				}
				*list = append(*list, VarBinding{Path: p.POU + "." + names[k], State: ref})
			}
		}
	}
	bind(&p.In, d.Reads, true)
	bind(&p.Out, d.Writes, false)
	for _, v := range d.inputs {
		if !bound[strings.ToUpper(v)] {
			p.In = append(p.In, VarBinding{Path: p.POU + "." + v, State: r.state(name + "." + v)})
		}
	}
	for _, v := range d.outputs {
		if !bound[strings.ToUpper(v)] {
			p.Out = append(p.Out, VarBinding{Path: p.POU + "." + v, State: r.state(name + "." + v)})
		}
	}
	r.plan.Programs = append(r.plan.Programs, p)
}

// expandBinding turns a variable name ending in [a..b] into one per element, with {} in the
// state standing for the index.
func expandBinding(name, state string) ([]string, []string, error) {
	base, rng, ok := strings.Cut(name, "[")
	if !ok {
		return []string{name}, []string{state}, nil
	}
	var lo, hi int
	if _, err := fmt.Sscanf(rng, "%d..%d]", &lo, &hi); err != nil || hi < lo {
		return nil, nil, fmt.Errorf("%s is no range of an array, such as taken[0..47]", name)
	}
	var names, states []string
	for k := lo; k <= hi; k++ {
		names = append(names, fmt.Sprintf("%s[%d]", base, k))
		states = append(states, strings.ReplaceAll(state, "{}", strconv.Itoa(k)))
	}
	return names, states, nil
}

// expr compiles an expression over the states and parameters of the current module.
func (r *resolver) expr(src string) Expr {
	f, err := compileExpr(src, func(name string) (Expr, error) {
		if !strings.Contains(name, ".") {
			v, ok := r.vars[name]
			if !ok {
				return nil, fmt.Errorf("unknown parameter %q", name)
			}
			return func([]float64) float64 { return v }, nil
		}
		i := r.local(name)
		return func(states []float64) float64 { return states[i] }, nil
	})
	if err != nil {
		r.fail("%v", err)
		return func([]float64) float64 { return 0 }
	}
	return f
}

// pushers runs once all transports are known.
func (r *resolver) pushers(in *instance) {
	for i := range in.mod.Devices {
		d := &in.mod.Devices[i]
		if d.Type != "pusher" || !r.enabled(d.When) {
			continue
		}
		from, to := -1, r.segmentRef(in.def.ID+"."+d.To)
		if d.From != "" {
			from = r.segmentRef(in.def.ID + "." + d.From)
		}
		r.plan.Pushers = append(r.plan.Pushers, PusherPlan{Name: in.def.ID + "." + d.ID, Position: r.local(d.Cylinder + ".position"), From: from, To: to})
		// What it pushes out goes on onto the transport of another module that starts a short
		// way straight ahead, as an ejector pushes a workpiece onto the belt of the next station.
		if to >= 0 && to < len(r.plan.Segments) {
			if seg := &r.plan.Segments[to]; seg.Next < 0 && !seg.Arc && seg.Length > 0 {
				seg.Next, seg.NextAt = r.bridge(to)
			}
		}
	}
}

func rootOf(in *instance) string {
	root, _, _ := strings.Cut(in.def.ID, "/")
	return root
}

// obstaclesOf adds a box around each fixed part of a module. Whatever moves has its solid
// instead, and what cannot be touched, such as a beam of light, is not picked either.
func (r *resolver) obstaclesOf(in *instance) {
	if riding(in) != nil {
		return
	}
	name := in.def.Name
	if name == "" {
		name = in.mod.Name
	}
	moving := map[string]bool{}
	for i := range in.mod.Joints {
		if j := &in.mod.Joints[i]; r.enabled(j.When) {
			moving[j.Node] = true
		}
	}
	var walk func(nodes []NodeDef, links []ChainLink)
	walk = func(nodes []NodeDef, links []ChainLink) {
		for i := range nodes {
			n := &nodes[i]
			if !r.enabled(n.When) || n.NoPick || moving[n.Name] {
				continue
			}
			link := ChainLink{Position: r.vec(n.Position), Rotation: r.vec(n.Rotation)}
			copies := 1
			var step Point
			if n.Repeat != nil {
				copies, step = int(r.num(n.Repeat.Count)), r.vec(n.Repeat.Step)
			}
			for c := range copies {
				f := float64(c)
				at := append(links[:len(links):len(links)], ChainLink{Position: Point{step.X * f, step.Y * f, step.Z * f}}, link)
				if lo, hi, ok := r.bounds(n.Geometry); ok {
					chain := Chain{At: in.at, RotX: in.rotX, RotY: in.rotY, RotZ: in.rotZ, Links: at}
					o := chain.Apply(lo, nil)
					edge := func(p Point) Point {
						q := chain.Apply(p, nil)
						return Point{q.X - o.X, q.Y - o.Y, q.Z - o.Z}
					}
					r.plan.Obstacles = append(r.plan.Obstacles, ObstaclePlan{
						Root: rootOf(in), Name: name, Origin: o,
						U: edge(Point{hi.X, lo.Y, lo.Z}), V: edge(Point{lo.X, hi.Y, lo.Z}), W: edge(Point{lo.X, lo.Y, hi.Z}),
					})
				}
				walk(n.Children, at)
			}
		}
	}
	walk(in.mod.Model, nil)
}

// bounds returns the corners of the box around a geometry, in the frame of its node. An
// extruded shape counts as its full outline, which is safe as long as nothing has to pass
// through its openings.
func (r *resolver) bounds(g *GeometryDef) (lo, hi Point, ok bool) {
	if g == nil {
		return lo, hi, false
	}
	switch g.Type {
	case "BoxGeometry":
		w, h, d := r.num(g.Width)/2, r.num(g.Height)/2, r.num(g.Depth)/2
		return Point{-w, -h, -d}, Point{w, h, d}, true
	case "CylinderGeometry":
		a, b, radius := r.vec(g.From), r.vec(g.To), r.num(g.Radius)
		pad := func(u, v float64) (float64, float64) {
			if u == v {
				return u, v // the axis
			}
			return min(u, v), max(u, v)
		}
		lx, hx := pad(a.X, b.X)
		ly, hy := pad(a.Y, b.Y)
		lz, hz := pad(a.Z, b.Z)
		// Along the axis it ends at its end points, and around it at its radius.
		grow := func(l, h, along float64) (float64, float64) {
			if along > 0 {
				return l, h
			}
			return l - radius, h + radius
		}
		lx, hx = grow(lx, hx, math.Abs(a.X-b.X))
		ly, hy = grow(ly, hy, math.Abs(a.Y-b.Y))
		lz, hz = grow(lz, hz, math.Abs(a.Z-b.Z))
		return Point{lx, ly, lz}, Point{hx, hy, hz}, true
	case "ExtrudeGeometry":
		if len(g.Points) == 0 {
			return lo, hi, false
		}
		lo.X, hi.X = math.Inf(1), math.Inf(-1)
		lo.Y, hi.Y = math.Inf(1), math.Inf(-1)
		for _, p := range g.Points {
			x, y := r.num(p[0]), r.num(p[1])
			lo.X, hi.X = min(lo.X, x), max(hi.X, x)
			lo.Y, hi.Y = min(lo.Y, y), max(hi.Y, y)
		}
		hi.Z = r.num(g.Depth)
		return lo, hi, true
	}
	return lo, hi, false
}

// mirror returns the part of a sensor's beam behind its mirror, if it has one.
func (r *resolver) mirror(in *instance, d *DeviceDef) *BeamPlan {
	if d.Mirror == nil || !r.enabled(d.Mirror.When) {
		return nil
	}
	origin, dir, reach := r.vec(d.Origin), r.vec(d.Direction), r.num(d.Reach)
	n := math.Sqrt(dir.X*dir.X + dir.Y*dir.Y + dir.Z*dir.Z)
	turned := rotateAbout(dir, Point{Z: 1}, r.num(d.Mirror.Angle))
	return &BeamPlan{
		Origin: in.world(Point{origin.X + dir.X/n*reach, origin.Y + dir.Y/n*reach, origin.Z + dir.Z/n*reach}),
		Dir:    in.direction(turned),
		Reach:  r.num(d.Mirror.Reach),
	}
}

func (r *resolver) cylinder(d *DeviceDef, name string) {
	if d.Valve != "monostable" && d.Valve != "bistable" {
		r.fail("cylinder %s needs a monostable or bistable valve", d.ID)
		return
	}
	if d.Rest != "retracted" && d.Rest != "extended" {
		r.fail("cylinder %s needs a rest position", d.ID)
		return
	}
	stroke, speed := r.num(d.Stroke), r.num(d.Speed)
	if stroke <= 0 || speed <= 0 {
		r.fail("cylinder %s needs a positive stroke and speed", d.ID)
		return
	}
	c := CylinderPlan{
		Name: name, Bistable: d.Valve == "bistable", RestOut: d.Rest == "extended", Travel: stroke / speed, Extend: -1, Retract: -1,
		Position: r.state(name + ".position"), Extended: r.state(name + ".extended"), Retracted: r.state(name + ".retracted"),
		Commanded: r.state(name + ".commanded"), Midway: r.state(name + ".midway"),
		Limited: r.num(d.Limited) >= 0.5, Stop: -1,
	}
	if d.Stop != "" {
		c.Stop, c.StopAt = r.local(d.Stop), r.num(d.StopAt)/stroke
		if c.StopAt <= 0 || c.StopAt >= 1 {
			r.fail("cylinder %s stops at %g mm, which is not within its stroke", d.ID, r.num(d.StopAt))
		}
	}
	if c.Bistable || !c.RestOut {
		c.Extend = r.state(name + ".extend")
	}
	if c.Bistable || c.RestOut {
		c.Retract = r.state(name + ".retract")
	}
	r.plan.Cylinders = append(r.plan.Cylinders, c)
}

func (r *resolver) transports(in *instance) {
	for i := range in.mod.Transport {
		t := &in.mod.Transport[i]
		if !r.enabled(t.When) {
			continue
		}
		name := in.def.ID + "." + t.ID
		s := SegmentPlan{Name: name, HandsOver: t.HandsOver, Open: t.Open, Drive: -1, Reverse: -1, Factor: -1, FreeFrom: -1, Next: -1, Prev: -1, Speed: r.num(t.Speed), Travel: r.state(name + ".travel"), Count: r.state(name + ".count"), Before: r.state(name + ".before")}
		switch {
		case (t.Path.Line == nil) == (t.Path.Arc == nil):
			r.fail("transport %s needs a line or an arc path", t.ID)
			return
		case t.Node != "" && t.Path.Line == nil:
			r.fail("transport %s rides on a node, so its path is a line", t.ID)
			return
		case t.Node != "":
			chain, ok := r.chainTo(in, t.Node)
			if !ok {
				r.fail("transport %s rides on node %q, which the module does not have", t.ID, t.Node)
				return
			}
			rest := make([]float64, len(r.plan.States))
			s.Rides, s.Local = chain, [2]Point{r.vec(t.Path.Line.From), r.vec(t.Path.Line.To)}
			s.From, s.To = chain.Apply(s.Local[0], rest), chain.Apply(s.Local[1], rest)
			s.Length = dist(s.From, s.To)
			if s.Length == 0 {
				r.fail("transport %s has no length", t.ID)
			}
		case t.Path.Line != nil:
			s.From, s.To = in.world(r.vec(t.Path.Line.From)), in.world(r.vec(t.Path.Line.To))
			s.Length = dist(s.From, s.To)
			if s.Length == 0 {
				r.fail("transport %s has no length", t.ID)
			}
		default:
			a := t.Path.Arc
			s.Arc = true
			s.Center = in.world(Point{r.num(a.Center[0]), r.num(a.Center[1]), r.num(a.Z)})
			s.Radius = r.num(a.Radius)
			s.Start = (r.num(a.Start) + in.rotZ) * math.Pi / 180
			s.Sweep = r.num(a.Sweep) * math.Pi / 180
			s.Length = s.Radius * math.Abs(s.Sweep)
			s.From = PointOn(&s, 0)
			s.To = PointOn(&s, s.Length)
		}
		if t.Drive != "" {
			s.Drive = r.local(t.Drive)
		}
		if t.Reverse != "" {
			s.Reverse = r.local(t.Reverse)
		}
		if t.Factor != "" {
			s.Factor = r.local(t.Factor)
		}
		if !t.Shared && !t.Static && s.Speed <= 0 {
			r.fail("transport %s needs a positive speed", t.ID)
		}
		if t.FreeFrom.set {
			s.FreeFrom = r.num(t.FreeFrom)
		}
		r.segs[name] = len(r.plan.Segments)
		r.plan.Segments = append(r.plan.Segments, s)
	}
}

// PointOn returns the point at distance s along a segment.
func PointOn(seg *SegmentPlan, s float64) Point {
	if seg.Arc {
		dir := 1.0
		if seg.Sweep < 0 {
			dir = -1
		}
		a := seg.Start + dir*s/seg.Radius
		return Point{seg.Center.X + seg.Radius*math.Cos(a), seg.Center.Y + seg.Radius*math.Sin(a), seg.Center.Z}
	}
	t := s / seg.Length
	return Point{seg.From.X + (seg.To.X-seg.From.X)*t, seg.From.Y + (seg.To.Y-seg.From.Y)*t, seg.From.Z + (seg.To.Z-seg.From.Z)*t}
}

func dist(a, b Point) float64 {
	return math.Sqrt((a.X-b.X)*(a.X-b.X) + (a.Y-b.Y)*(a.Y-b.Y) + (a.Z-b.Z)*(a.Z-b.Z))
}

func (r *resolver) segmentRef(name string) int {
	i, ok := r.segs[name]
	if !ok {
		r.fail("%q is not a transport", name)
		return 0
	}
	return i
}

// linkTolerance is how far two connected transports may be apart, in mm. It turns
// a misplaced module into a load error instead of a workpiece that teleports. A
// workpiece may drop a few mm, as from the belt onto a slide.
const linkTolerance = 8.0

// LinkTolerance is how far apart in mm the end of a transport and the start of the next may
// be for workpieces to go on from one to the other.
const LinkTolerance = linkTolerance

func (r *resolver) links(st *StationDef) {
	inst, vars, texts := r.inst, r.vars, r.texts
	for _, in := range r.order {
		if len(in.mod.Links) == 0 {
			continue
		}
		r.enter(in)
		id := in.def.ID
		r.linkAll(in.mod.Links, r.where, func(name string) string {
			if strings.Contains(name, ".") {
				return id + "/" + name
			}
			return id + "." + name
		}, r.local)
		if r.err != nil {
			return
		}
	}
	r.inst, r.vars, r.texts = inst, vars, texts
	r.linkAll(st.Links, "station "+st.ID, func(name string) string { return name }, r.global)
	r.where = "station " + st.ID
}

// linkAll applies links, with scope and state saying what their names mean where they are.
func (r *resolver) linkAll(links []LinkDef, where string, scope func(string) string, state func(string) int) {
	for i := range links {
		l := &links[i]
		if !r.enabled(l.If) {
			continue
		}
		r.where = fmt.Sprintf("%s, links[%d]", where, i)
		from := r.segmentRef(scope(l.Transport))
		if r.err != nil {
			return
		}
		seg := &r.plan.Segments[from]
		at := r.num(l.At)
		switch l.Type {
		case "continue":
			to := r.segmentRef(scope(l.To))
			if r.err != nil {
				return
			}
			if d := dist(seg.To, r.plan.Segments[to].From); d > linkTolerance {
				r.fail("%s ends %.1f mm away from the start of %s", l.Transport, d, l.To)
			}
			seg.Next = to
		case "branch":
			to := r.segmentRef(scope(l.To))
			if r.err != nil {
				return
			}
			if d := dist(PointOn(seg, at), r.plan.Segments[to].From); d > linkTolerance {
				r.fail("%s at %.1f mm is %.1f mm away from the start of %s", l.Transport, at, d, l.To)
			}
			when := -1 // always, for a fixed guide
			if l.When != "" {
				when = state(l.When)
			}
			seg.Branches = append(seg.Branches, BranchPlan{At: at, State: when, To: to})
		case "shareDrive":
			to := r.segmentRef(scope(l.To))
			if r.err != nil {
				return
			}
			// A curve only carries workpieces away from the belt, so it takes the
			// forward drive and speed but not the reverse.
			seg.Drive, seg.Speed, seg.Factor = r.plan.Segments[to].Drive, r.plan.Segments[to].Speed, r.plan.Segments[to].Factor
		default:
			r.fail("unknown link type %q", l.Type)
		}
	}
}

// handovers joins the transports of modules where they meet, so that a station needs no
// links of its own. A transport with a branch starts on another module's transport and
// takes workpieces off it there. A transport that ends where another module's starts,
// within linkTolerance, hands its workpieces on to the nearest one.
func (r *resolver) handovers() {
	root := func(name string) string {
		name, _, _ = strings.Cut(name, ".")
		name, _, _ = strings.Cut(name, "/")
		return name
	}
	for _, in := range r.order {
		r.enter(in)
		for i := range in.mod.Transport {
			t := &in.mod.Transport[i]
			if t.Branch == nil || !r.enabled(t.When) {
				continue
			}
			name := in.def.ID + "." + t.ID
			idx := r.segs[name]
			start, host, at := r.plan.Segments[idx].From, -1, 0.0
			for j := range r.plan.Segments {
				s := &r.plan.Segments[j]
				if s.Arc || s.Rides != nil || root(s.Name) == root(name) {
					continue
				}
				d := Point{s.To.X - s.From.X, s.To.Y - s.From.Y, s.To.Z - s.From.Z}
				along := ((start.X-s.From.X)*d.X + (start.Y-s.From.Y)*d.Y + (start.Z-s.From.Z)*d.Z) / s.Length
				if along >= 0 && along <= s.Length && dist(start, PointOn(s, along)) <= linkTolerance {
					host, at = j, along
				}
			}
			if host < 0 {
				if !r.bundle.Preview {
					r.fail("%s starts on no transport of another module", name)
				}
				continue
			}
			when := -1
			if t.Branch.When != "" {
				when = r.local(t.Branch.When)
			}
			h := &r.plan.Segments[host]
			h.Branches = append(h.Branches, BranchPlan{At: at, State: when, To: idx})
			if t.Shared {
				seg := &r.plan.Segments[idx]
				seg.Drive, seg.Speed, seg.Factor = h.Drive, h.Speed, h.Factor
			}
		}
	}
	for i := range r.plan.Segments {
		s := &r.plan.Segments[i]
		if s.Next >= 0 || s.Rides != nil {
			continue
		}
		best, gap := -1, linkTolerance
		for j := range r.plan.Segments {
			if d := dist(s.To, r.plan.Segments[j].From); root(r.plan.Segments[j].Name) != root(s.Name) && r.plan.Segments[j].Rides == nil && d <= gap {
				best, gap = j, d
			}
		}
		if best >= 0 {
			s.Next = best
			continue
		}
		if s.Open && !s.Arc {
			s.Next, s.NextAt = r.bridge(i)
			if s.Next < 0 {
				s.Next, s.NextAt = r.across(i)
				s.Across = s.Next >= 0
			}
		}
	}
	// What one transport hands on to the next, the next hands back when it runs backwards.
	for i := range r.plan.Segments {
		s := &r.plan.Segments[i]
		if s.Next < 0 || s.Arc || s.Across {
			continue
		}
		if n := &r.plan.Segments[s.Next]; n.Prev < 0 && !n.Arc {
			n.Prev, n.PrevAt = i, s.NextAt
		}
	}
}

// bridgeGap is how far apart in mm the end of a belt and the start of the next may be for a
// workpiece to cross onto it, as between the belts of two stations side by side.
const bridgeGap = 25.0

// BridgeGap is how far apart the end of a belt and the start of the next may be for a
// workpiece to cross onto it.
const BridgeGap = bridgeGap

// bridge finds the transport of another module that starts a short way past the open end
// of a segment, straight ahead and at the same height, and how far before its start a
// workpiece arrives on it.
func (r *resolver) bridge(i int) (int, float64) {
	s := &r.plan.Segments[i]
	unit := func(a, b Point) Point { d := Point{b.X - a.X, b.Y - a.Y, b.Z - a.Z}; return scale(d, 1/length(d)) }
	dir := unit(s.From, s.To)
	root := func(name string) string {
		name, _, _ = strings.Cut(name, ".")
		name, _, _ = strings.Cut(name, "/")
		return name
	}
	best, gap := -1, bridgeGap
	for j := range r.plan.Segments {
		o := &r.plan.Segments[j]
		if o.Arc || o.Length == 0 || o.Rides != nil || root(o.Name) == root(s.Name) || dot(dir, unit(o.From, o.To)) < 0.999 {
			continue
		}
		rel := Point{o.From.X - s.To.X, o.From.Y - s.To.Y, o.From.Z - s.To.Z}
		ahead := dot(rel, dir)
		side := Point{rel.X - dir.X*ahead, rel.Y - dir.Y*ahead, rel.Z - dir.Z*ahead}
		if ahead > 0 && ahead <= gap && length(side) <= linkTolerance {
			best, gap = j, ahead
		}
	}
	if best < 0 {
		return -1, 0
	}
	return best, -gap
}

// acrossGap is how far in mm from the open end of a belt the middle of a belt across it may
// run, for workpieces to cross over its side onto it: past its guide rail, which the belt
// that takes them leaves out there.
const acrossGap = 50.0

// across finds the transport of another module that runs across the open end of a segment,
// a short way ahead and at the same height, and where on it a workpiece arrives, at least a
// workpiece's radius from either end of it.
func (r *resolver) across(i int) (int, float64) {
	s := &r.plan.Segments[i]
	unit := func(a, b Point) Point { d := Point{b.X - a.X, b.Y - a.Y, b.Z - a.Z}; return scale(d, 1/length(d)) }
	dir := unit(s.From, s.To)
	radius := r.plan.Scene.Workpieces.Diameter / 2
	root := func(name string) string {
		name, _, _ = strings.Cut(name, ".")
		name, _, _ = strings.Cut(name, "/")
		return name
	}
	best, bestGap, bestAt := -1, acrossGap, 0.0
	for j := range r.plan.Segments {
		o := &r.plan.Segments[j]
		if o.Arc || o.Length == 0 || o.Rides != nil || root(o.Name) == root(s.Name) || math.Abs(o.From.Z-s.To.Z) > 1 || math.Abs(o.To.Z-s.To.Z) > 1 {
			continue
		}
		od := unit(o.From, o.To)
		if math.Abs(dot(dir, od)) > 0.01 {
			continue
		}
		rel := Point{o.From.X - s.To.X, o.From.Y - s.To.Y, o.From.Z - s.To.Z}
		ahead := dot(rel, dir)
		at := -dot(rel, od)
		if ahead > 0 && ahead <= bestGap && at >= radius && at <= o.Length-radius {
			best, bestGap, bestAt = j, ahead, at
		}
	}
	return best, bestAt
}

func (r *resolver) checkShared() {
	for _, in := range r.order {
		for i := range in.mod.Transport {
			t := &in.mod.Transport[i]
			if !t.Shared {
				continue
			}
			name := in.def.ID + "." + t.ID
			if idx, ok := r.segs[name]; ok && r.plan.Segments[idx].Speed <= 0 {
				r.fail("transport %s needs a shareDrive link", name)
			}
		}
	}
}

// plcBits maps the pins of PLC connectors to bits of the image.
func (r *resolver) plcBits() {
	// The station's PLCs, the one it names first, and every other module with a PLC
	// connector in the order the station has them.
	var plcs []string
	for _, in := range r.order {
		if in.parent != nil || slices.Contains(plcs, in.def.ID) {
			continue
		}
		if slices.ContainsFunc(in.mod.Terminal.Connectors, func(c ConnectorDef) bool { return c.Byte != nil || c.AnalogIn }) {
			plcs = append(plcs, in.def.ID)
		}
	}
	if first := slices.Index(plcs, r.bundle.Station.PLC.Module); first > 0 {
		plcs = append([]string{plcs[first]}, slices.Delete(plcs, first, first+1)...)
	}
	r.plan.PLCs = plcs
	r.plan.Naming = nil
	for _, id := range plcs {
		naming := plcio.Naming{}
		if in := r.instances[id]; in != nil && in.mod.Addresses != nil {
			naming = *in.mod.Addresses
		} else {
			r.fail("the PLC %s does not say how its program names its variables: its module needs addresses", id)
		}
		r.plan.Naming = append(r.plan.Naming, naming)
	}
	plcOf := func(in *instance) int { return max(0, slices.Index(plcs, rootOf(in))) }
	// The nets that pass through a switched connector, which a safety relay cuts.
	switched := map[string]bool{}
	for _, in := range r.order {
		for _, p := range in.pins {
			if p.conn.Switched {
				switched[r.find(p.key)] = true
			}
		}
	}
	for _, in := range r.order {
		k := plcOf(in)
		for _, p := range in.pins {
			if p.conn.AnalogIn {
				if number, ok := strings.CutPrefix(p.name, "UE"); ok {
					n, err := strconv.Atoi(number)
					if err != nil || n < 1 || n > plcio.AnalogInputs {
						r.fail("PLC pin %s is not an analogue input", p.key)
						continue
					}
					r.plan.PLCAnalog = append(r.plan.PLCAnalog, PLCAnalogPlan{Channel: n - 1, State: r.state(p.key), PLC: k})
					r.netPLC[r.find(p.key)] = r.plan.Naming[k].AnalogInput(n - 1)
					r.netPLCOf[r.find(p.key)] = k
				}
				continue
			}
			if p.conn.Byte == nil {
				continue
			}
			n, err := strconv.Atoi(p.name[1:])
			if err != nil {
				r.fail("PLC pin %s has no bit number", p.key)
				continue
			}
			if *p.conn.Byte < 0 || *p.conn.Byte >= plcio.ImageBytes {
				r.fail("connector %s.%s uses byte %d outside the image", in.def.ID, p.conn.ID, *p.conn.Byte)
				continue
			}
			a := plcio.Addr{Byte: uint8(*p.conn.Byte), Bit: uint8(n)}
			root := r.find(p.key)
			r.netPLCOf[root] = k
			if p.name[0] == 'I' {
				r.plan.PLCIn = append(r.plan.PLCIn, PLCBitPlan{Addr: a, State: r.state(p.key), PLC: k})
				r.netPLC[root] = r.plan.Naming[k].Input(a)
			} else {
				r.plan.PLCOut = append(r.plan.PLCOut, PLCBitPlan{Addr: a, State: r.state(p.key), PLC: k, Switched: switched[root]})
				r.netPLC[root] = r.plan.Naming[k].Output(a)
				r.netPLCOut[root] = true
			}
		}
	}
}

// pinDrivers checks every net has at most one source, and adds the copies that
// feed nets from device states.
func (r *resolver) pinDrivers() {
	sources := map[string]string{}
	source := func(root, who string) {
		if other, ok := sources[root]; ok {
			r.fail("%s and %s drive the same signal", other, who)
		}
		sources[root] = who
	}
	for _, in := range r.order {
		r.enter(in)
		for _, p := range in.pins {
			root := r.find(p.key)
			if p.conn.Byte != nil && p.name[0] == 'Q' {
				source(root, "PLC output "+p.key)
			}
			switch {
			case p.def.Constant != nil:
				source(root, p.key)
				r.plan.Copies = append(r.plan.Copies, CopyPlan{From: -1, Constant: *p.def.Constant, To: r.state(p.key)})
			case p.def.Reads != "":
				if !r.knownHere(in, p.def.Reads) {
					r.fail("pin %s reads unknown state %q", p.key, p.def.Reads)
					continue
				}
				source(root, p.key)
				r.plan.Copies = append(r.plan.Copies, CopyPlan{From: r.state(r.scoped(in, p.def.Reads)), Analog: p.def.Analog, Invert: p.def.Invert, To: r.state(p.key)})
			}
		}
	}
	// A device input on a net fed by a PLC input bit can never be driven.
	for _, in := range r.order {
		r.enter(in)
		for _, p := range in.pins {
			if p.def.Drives == "" {
				continue
			}
			if who, ok := sources[r.find(p.key)]; ok && !strings.HasPrefix(who, "PLC output") {
				r.fail("pin %s needs a signal from the PLC, but is wired to %s", p.key, who)
			}
		}
	}
}

func (r *resolver) actions(defs []ActionDef, prefix string, segment func(string) int) {
	for i := range defs {
		a := &defs[i]
		if !r.enabled(a.When) {
			continue
		}
		name := prefix + a.ID
		if a.Type != "clearTransport" && a.Type != "placeWorkpiece" {
			r.fail("action %s has unknown type %q", a.ID, a.Type)
			continue
		}
		p := ActionPlan{Type: a.Type, Label: a.Label, Dock: a.Dock, Segment: -1, At: r.num(a.At), BlockedBy: -1, Family: a.Family}
		if a.Over[0].set || a.Over[1].set || a.Over[2].set {
			if r.inst == nil || a.Transport != "" {
				r.fail("action %s needs either a transport or, on a module, a point it is over", a.ID)
				continue
			}
			over := r.inst.world(r.vec(a.Over))
			p.Over = &over
		} else {
			p.Segment = segment(a.Transport)
		}
		if a.Heading[0].set || a.Heading[1].set || a.Heading[2].set {
			h := r.vec(a.Heading)
			if r.inst != nil {
				h = r.inst.direction(h)
			}
			p.Heading = h
		}
		if a.BlockedBy != "" {
			if r.inst != nil {
				p.BlockedBy = r.local(a.BlockedBy)
			} else {
				p.BlockedBy = r.global(a.BlockedBy)
			}
		}
		r.plan.Actions[name] = p
	}
}
