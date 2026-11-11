package sim

import (
	"math"
	"slices"

	"github.com/toblu96/taktwin/internal/model"
)

// Mark is a special position of a module that others are lined up with, such as where a
// stopper holds a workpiece or where a gripper comes down, in station coordinates.
type Mark struct {
	Owner string      `json:"owner"`
	Label string      `json:"label"`
	At    model.Point `json:"at"`
}

// Marks finds the special positions of a station at rest: where a part standing in a
// transport stops a workpiece, where a gripper comes down, where a magazine pushes to, where
// a transport branches off another and where one hands its workpieces on.
func Marks(plan *model.Plan) []Mark {
	p := New(plan)
	out := []Mark{}
	radius := plan.Scene.Workpieces.Diameter / 2
	for i := range plan.Solids {
		s := &plan.Solids[i]
		b := solidBox(s, p.v)
		c := b.center()
		for j := range plan.Segments {
			seg := &plan.Segments[j]
			if seg.Arc || seg.Drive < 0 || root(seg.Name) == s.Root || seg.Length == 0 {
				continue
			}
			d := model.Point{X: (seg.To.X - seg.From.X) / seg.Length, Y: (seg.To.Y - seg.From.Y) / seg.Length, Z: (seg.To.Z - seg.From.Z) / seg.Length}
			along := (c.X-seg.From.X)*d.X + (c.Y-seg.From.Y)*d.Y + (c.Z-seg.From.Z)*d.Z
			on := model.PointOn(seg, math.Max(0, math.Min(seg.Length, along)))
			half := (math.Abs(dot(b.u, d)) + math.Abs(dot(b.v, d)) + math.Abs(dot(b.w, d))) / 2
			// It stands in the way when its corners reach into the band a workpiece sweeps along
			// the path, between the belt and a workpiece's top.
			n := model.Point{X: -d.Y, Y: d.X}
			lo, hi, low, high := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
			for k := range 8 {
				q := b.o
				for e, edge := range []model.Point{b.u, b.v, b.w} {
					if k>>e&1 == 1 {
						q = model.Point{X: q.X + edge.X, Y: q.Y + edge.Y, Z: q.Z + edge.Z}
					}
				}
				off := (q.X-on.X)*n.X + (q.Y-on.Y)*n.Y
				lo, hi, low, high = math.Min(lo, off), math.Max(hi, off), math.Min(low, q.Z), math.Max(high, q.Z)
			}
			if hi < -radius || lo > radius || high < on.Z || low > on.Z+plan.Scene.Workpieces.Height {
				continue
			}
			if stop := along - half - radius; stop >= 0 && stop <= seg.Length {
				out = append(out, Mark{Owner: s.Root, Label: "Holds a workpiece here", At: model.PointOn(seg, stop)})
			}
		}
	}
	for _, g := range plan.Grippers {
		for _, at := range lowered(&g.Tip, len(plan.States)) {
			out = append(out, Mark{Owner: root(g.Name), Label: "The gripper comes down here", At: at})
		}
	}
	for _, push := range plan.Pushers {
		to := &plan.Segments[push.To]
		out = append(out, Mark{Owner: root(push.Name), Label: "Pushes workpieces to here", At: model.PointOn(to, to.Length)})
	}
	for i := range plan.Segments {
		seg := &plan.Segments[i]
		for _, b := range seg.Branches {
			to := &plan.Segments[b.To]
			out = append(out, Mark{Owner: root(to.Name), Label: "Takes workpieces off here", At: to.From})
		}
		if seg.HandsOver {
			out = append(out, Mark{Owner: root(seg.Name), Label: "Hands workpieces on here", At: seg.To})
		}
		// A nest that rides on a table stops where it stands at rest, for modules to line up with.
		if seg.Rides != nil && slices.ContainsFunc(seg.Rides.Links, func(l model.ChainLink) bool { return l.Joint && l.Revolute }) {
			out = append(out, Mark{Owner: root(seg.Name), Label: "A nest stops here", At: seg.To})
		}
	}
	return out
}
