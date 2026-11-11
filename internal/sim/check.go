package sim

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// sweepTime is how long each output stays on in the sweep, enough for any cylinder or
// swivel to reach its end.
const sweepTime = 2.5

// Check looks for what is wrong with how a station is built: what CheckLayout finds, and a
// moving part that runs into a fixed one with only one output on. It says so in words, and
// nothing when the station is fine.
func Check(plan *model.Plan) []model.SceneCheck {
	return append(CheckLayout(plan), sweep(plan)...)
}

// overlapTolerance is how far in mm the boxes of two modules may reach into each other at
// rest, as a box is a little bigger than the shape it stands for.
const overlapTolerance = 1.0

// CheckLayout is the quick part of Check, for each step of building a station: modules
// standing in each other, a transport that should hand workpieces on and leads nowhere, and
// a magazine or a gripper that puts them down over nothing.
func CheckLayout(plan *model.Plan) []model.SceneCheck {
	return append(append(append(Overlaps(plan), handovers(plan)...), missing(plan)...), unwired(plan)...)
}

// missing lists the parts that modules need and lack, such as a conveyor without its motor
// controller.
func missing(plan *model.Plan) []model.SceneCheck {
	out := []model.SceneCheck{}
	names := map[string]string{}
	for _, in := range plan.Scene.Instances {
		names[in.ID] = cmp.Or(in.Name, in.Module)
	}
	for _, m := range plan.Missing {
		text := fmt.Sprintf("%s has no %s", names[m.Instance], m.Name)
		if m.Min > 1 {
			text = fmt.Sprintf("%s has %d of the %d %s it needs", names[m.Instance], m.Have, m.Min, m.Name)
		}
		if m.Why != "" {
			text += ": " + m.Why
		}
		out = append(out, model.SceneCheck{Text: text, Instances: []string{root(m.Instance)}})
	}
	return out
}

// unwired finds what the PLC cannot reach: a pin that drives an actuator without a PLC
// output on its wire, and a sensor none of whose signals reaches a PLC input. A sensor's
// other signals, such as "nothing seen" beside "seen", need not be wired too.
func unwired(plan *model.Plan) []model.SceneCheck {
	out := []model.SceneCheck{}
	// A module with a controller of its own, a program device, runs its parts itself, and
	// what is cabled to its connectors, as the belt in front of it.
	var controlled []string
	for _, c := range plan.Programs {
		module, _, _ := strings.Cut(c.Name, ".")
		controlled = append(controlled, module+"/")
	}
	ownController := map[int]bool{}
	for _, in := range plan.Scene.Instances {
		if !slices.ContainsFunc(controlled, func(prefix string) bool { return strings.HasPrefix(in.ID+"/", prefix) }) {
			continue
		}
		for _, c := range in.Connectors {
			for _, p := range c.Pins {
				ownController[p.State] = true
			}
		}
	}
	for _, in := range plan.Scene.Instances {
		// A part shown as part of another, such as a sensor's logic box, only shows its signals.
		if in.SelectAs != "" || slices.ContainsFunc(controlled, func(prefix string) bool { return strings.HasPrefix(in.ID, prefix) }) {
			continue
		}
		name := cmp.Or(in.Name, in.Module)
		for _, c := range in.Connectors {
			if c.Type != "wires" {
				continue
			}
			sensing, reached := false, false
			for _, p := range c.Pins {
				switch {
				case p.Output && p.Function != "" && p.Address == "" && !ownController[p.State]:
					out = append(out, model.SceneCheck{Text: fmt.Sprintf("%s: %q is wired to no PLC output", name, p.Function), Instances: []string{root(in.ID)}})
				case !p.Output && p.Function != "":
					sensing = true
					reached = reached || p.Address != "" || ownController[p.State]
				}
			}
			if sensing && !reached && !slices.ContainsFunc(c.Pins, func(p model.ScenePin) bool { return p.Output }) {
				out = append(out, model.SceneCheck{Text: fmt.Sprintf("%s: none of its signals is wired to a PLC input", name), Instances: []string{root(in.ID)}})
			}
		}
	}
	return out
}

// Overlaps finds top-level modules that stand in each other at rest. One screwed onto the
// other is fine, as a gate's bracket sits in its rail.
func Overlaps(plan *model.Plan) []model.SceneCheck {
	out := []model.SceneCheck{}
	reported := map[[2]string]bool{}
	// The box around each box along the axes first, as most pairs are far apart.
	type bounds struct{ lo, hi model.Point }
	around := make([]bounds, len(plan.Obstacles))
	for i := range plan.Obstacles {
		o := &plan.Obstacles[i]
		lo, hi := o.Origin, o.Origin
		for _, e := range []model.Point{o.U, o.V, o.W} {
			lo = model.Point{X: lo.X + math.Min(e.X, 0), Y: lo.Y + math.Min(e.Y, 0), Z: lo.Z + math.Min(e.Z, 0)}
			hi = model.Point{X: hi.X + math.Max(e.X, 0), Y: hi.Y + math.Max(e.Y, 0), Z: hi.Z + math.Max(e.Z, 0)}
		}
		around[i] = bounds{lo, hi}
	}
	apart := func(a, b bounds) bool {
		t := overlapTolerance
		return a.hi.X-t < b.lo.X || b.hi.X-t < a.lo.X || a.hi.Y-t < b.lo.Y || b.hi.Y-t < a.lo.Y || a.hi.Z-t < b.lo.Z || b.hi.Z-t < a.lo.Z
	}
	for i := range plan.Obstacles {
		a := &plan.Obstacles[i]
		for j := i + 1; j < len(plan.Obstacles); j++ {
			b := &plan.Obstacles[j]
			pair := [2]string{a.Root, b.Root}
			if a.Root == b.Root || reported[pair] || plan.Mounted[a.Root] == b.Root || plan.Mounted[b.Root] == a.Root || apart(around[i], around[j]) {
				continue
			}
			if boxGap(obstacleBox(a), obstacleBox(b)) < -overlapTolerance {
				reported[pair] = true
				ab, bb := obstacleBox(a).center(), obstacleBox(b).center()
				at := model.Point{X: (ab.X + bb.X) / 2, Y: (ab.Y + bb.Y) / 2, Z: (ab.Z + bb.Z) / 2}
				out = append(out, model.SceneCheck{Text: fmt.Sprintf("%s and %s stand in each other", a.Name, b.Name), Instances: []string{a.Root, b.Root}, At: &at})
			}
		}
	}
	return out
}

// handovers finds a transport leading nowhere and a magazine or gripper putting workpieces
// down over nothing.
func handovers(plan *model.Plan) []model.SceneCheck {
	out := []model.SceneCheck{}
	names := map[string]string{}
	for _, in := range plan.Scene.Instances {
		names[in.ID] = cmp.Or(in.Name, in.Module)
	}
	// What people call the module a device or transport belongs to, and the device's own id.
	named := func(ref string) (string, string) {
		instance, id := ref, ""
		if i := strings.LastIndex(ref, "."); i >= 0 {
			instance, id = ref[:i], ref[i+1:]
		}
		return cmp.Or(names[instance], instance), id
	}
	for i := range plan.Segments {
		s := &plan.Segments[i]
		if s.HandsOver && s.Next < 0 {
			at := s.To
			name, id := named(s.Name)
			out = append(out, model.SceneCheck{Text: fmt.Sprintf("%s: its %s ends where no other module's transport starts", name, id), Instances: []string{root(s.Name)}, At: &at})
		}
	}
	p := New(plan)
	for _, push := range plan.Pushers {
		to := &plan.Segments[push.To]
		end := model.PointOn(to, to.Length)
		over := p.over(end, stackReach, push.To)
		// One that rides on a lift pushes out wherever the lift stops, at either end.
		if to.Rides != nil {
			for _, at := range ends(to.Rides, to.Local[1], len(plan.States)) {
				over = over || p.over(at, stackReach, push.To)
			}
		}
		if !over {
			name, _ := named(push.Name)
			out = append(out, model.SceneCheck{Text: fmt.Sprintf("%s pushes workpieces out where no transport takes them", name), Instances: []string{root(push.Name)}, At: &end})
		}
	}
	// A gripper on an axis that goes to taught positions comes down wherever the program
	// takes it, so only one moved by cylinders and plain motors is checked.
	// An axis that drives to positions, taught or set, goes down where it is sent.
	positioning := map[int]bool{}
	for _, a := range plan.Axes {
		if len(a.Targets) > 0 || a.Setpoint >= 0 {
			positioning[a.Position] = true
		}
	}
	for _, g := range plan.Grippers {
		if slices.ContainsFunc(g.Tip.Links, func(l model.ChainLink) bool { return l.Joint && positioning[l.State] }) {
			continue
		}
		for _, at := range lowered(&g.Tip, len(plan.States)) {
			if !p.over(at, gripReach, -1) {
				name, _ := named(g.Name)
				out = append(out, model.SceneCheck{Text: fmt.Sprintf("%s: its gripper comes down where no transport is", name), Instances: []string{root(g.Name)}, At: &at})
			}
		}
	}
	return out
}

// sweep runs each PLC output on its own and reports what then runs into a fixed part.
func sweep(plan *model.Plan) []model.SceneCheck {
	var out []model.SceneCheck
	names := map[plcio.Addr]string{}
	for _, s := range plan.Scene.PLC.Outputs {
		names[plcio.Addr{Byte: s.Byte, Bit: s.Bit}] = s.Name
	}
	for _, o := range plan.PLCOut {
		sweep := New(plan)
		var image plcio.Image
		image.Set(o.Addr, true)
		for range int(sweepTime / 0.002) {
			sweep.Step(0.002, image)
		}
		name := plan.Naming[o.PLC].Output(o.Addr)
		if n := names[o.Addr]; n != "" {
			name = n + " (" + name + ")"
		}
		for _, hit := range sweep.struck {
			out = append(out, model.SceneCheck{Text: fmt.Sprintf("With only %s on: %s", name, hit.text), Instances: hit.roots})
		}
	}
	return out
}

// root is the top-level module of a name such as belt/endGuide.curve.
func root(name string) string {
	name, _, _ = strings.Cut(name, ".")
	name, _, _ = strings.Cut(name, "/")
	return name
}

// gripReach is how far in mm a gripper may come down beside the transport it serves.
const gripReach = 3.0

// over reports whether a transport lies right below a point.
func (p *Plant) over(c model.Point, reach float64, except int) bool {
	_, _, ok := p.surfaceBelow(c, reach, except)
	return ok
}

// ends returns where a point of a moving node is with each of its joints at either end.
func ends(c *model.Chain, local model.Point, states int) []model.Point {
	var joints []int
	for _, l := range c.Links {
		if l.Joint {
			joints = append(joints, l.State)
		}
	}
	var at []model.Point
	v := make([]float64, states)
	for combo := range 1 << len(joints) {
		for k, state := range joints {
			v[state] = float64(combo >> k & 1)
		}
		at = append(at, c.Apply(local, v))
	}
	return at
}

// lowered returns each place a gripper's tip comes down to: of the positions its joints
// reach at their ends, the lowest one at each spot.
func lowered(tip *model.Chain, states int) []model.Point {
	var joints []int
	for _, l := range tip.Links {
		if l.Joint {
			joints = append(joints, l.State)
		}
	}
	var spots []model.Point
	v := make([]float64, states)
	for combo := range 1 << len(joints) {
		for k, state := range joints {
			v[state] = float64(combo >> k & 1)
		}
		at := tip.Point(v)
		found := false
		for i := range spots {
			if math.Hypot(spots[i].X-at.X, spots[i].Y-at.Y) < 1 {
				spots[i].Z, found = math.Min(spots[i].Z, at.Z), true
			}
		}
		if !found {
			spots = append(spots, at)
		}
	}
	return spots
}
