package model

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
)

// railGapMargin is how much room in mm a gap leaves on each side of what it is cut for,
// and minRailPiece the shortest piece of rail worth keeping: one that still takes two screws.
const (
	railGapMargin = 3.0
	minRailPiece  = 30.0
)

// PlaceError is a module that does not fit where it is placed. By names the module a rail
// was cut for, when that gap is what is in the way.
type PlaceError struct {
	Instance string
	By       string
	Text     string
}

func (e *PlaceError) Error() string { return e.Text }

func (r *resolver) failPlace(instance, by, format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("%s: %w", r.where, &PlaceError{Instance: instance, By: by, Text: fmt.Sprintf(format, args...)})
	}
}

// nameOf is how people know an instance: its name, or that of its module.
func (r *resolver) nameOf(in *instance) string {
	switch {
	case in == nil:
		return "?"
	case in.def.Name != "":
		return r.text(in.def.Name)
	default:
		return in.mod.Name
	}
}

// local turns a point of the station into the instance's own coordinates.
func (in *instance) local(p Point) Point {
	x, y, z := p.X-in.at.X, p.Y-in.at.Y, p.Z-in.at.Z
	x, y = x*in.cos+y*in.sin, -x*in.sin+y*in.cos
	x, z = x*in.cosY-z*in.sinY, x*in.sinY+z*in.cosY
	y, z = y*in.cosX+z*in.sinX, -y*in.sinX+z*in.cosX
	return Point{x, y, z}
}

// railCuts finds the gaps that modules which cut rails need, in every rail they stand in,
// once everything is placed. A rail that may not be cut makes the module not fit.
func (r *resolver) railCuts() (map[string][]gap, error) {
	cutters := map[string]*instance{}
	for _, in := range r.order {
		if in.parent == nil && in.mod.CutsRails {
			cutters[in.def.ID] = in
		}
	}
	if len(cutters) == 0 {
		return nil, nil
	}
	cuts := map[string][]gap{}
	for _, rail := range r.order {
		box := rail.mod.CutBox
		if box == nil || cutters[rootOf(rail)] != nil {
			continue
		}
		r.enter(rail)
		a, b := r.vec(box.From), r.vec(box.To)
		lo := Point{math.Min(a.X, b.X), math.Min(a.Y, b.Y), math.Min(a.Z, b.Z)}
		hi := Point{math.Max(a.X, b.X), math.Max(a.Y, b.Y), math.Max(a.Z, b.Z)}
		var gaps []gap
		for _, o := range r.plan.Obstacles {
			cutter := cutters[o.Root]
			if cutter == nil {
				continue
			}
			from := Point{math.Inf(1), math.Inf(1), math.Inf(1)}
			to := Point{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
			for k := range 8 {
				c := o.Origin
				for e, edge := range []Point{o.U, o.V, o.W} {
					if k>>e&1 == 1 {
						c = Point{c.X + edge.X, c.Y + edge.Y, c.Z + edge.Z}
					}
				}
				l := rail.local(c)
				from = Point{math.Min(from.X, l.X), math.Min(from.Y, l.Y), math.Min(from.Z, l.Z)}
				to = Point{math.Max(to.X, l.X), math.Max(to.Y, l.Y), math.Max(to.Z, l.Z)}
			}
			if to.X <= lo.X || from.X >= hi.X || to.Y <= lo.Y || from.Y >= hi.Y || to.Z <= lo.Z || from.Z >= hi.Z {
				continue
			}
			if !rail.def.Cut {
				where := r.nameOf(rail)
				if rail.parent != nil {
					where = strings.ToLower(where) + " of " + r.nameOf(rail.parent)
				}
				return nil, fmt.Errorf("station %s: %w", r.bundle.Station.ID, &PlaceError{Instance: cutter.def.ID, Text: fmt.Sprintf(
					"%s does not fit there: it stands in the %s, which cannot be cut. A rail is cut only where its module allows it, such as the rear rail of a conveyor",
					r.nameOf(cutter), where)})
			}
			gaps = append(gaps, gap{From: from.X - railGapMargin, To: to.X + railGapMargin, By: cutter.def.ID, name: r.nameOf(cutter)})
		}
		if len(gaps) == 0 {
			continue
		}
		cuts[rail.def.ID] = joinGaps(gaps, lo.X, hi.X)
	}
	return cuts, nil
}

// joinGaps sorts gaps along a rail from lo to hi, and joins those that leave too short a
// piece of rail between them, or to an end.
func joinGaps(gaps []gap, lo, hi float64) []gap {
	slices.SortFunc(gaps, func(a, b gap) int { return cmp.Compare(a.From, b.From) })
	var out []gap
	for _, g := range gaps {
		if g.From-lo < minRailPiece {
			g.From = lo
		}
		if hi-g.To < minRailPiece {
			g.To = hi
		}
		if n := len(out); n > 0 && g.From-out[n-1].To < minRailPiece {
			out[n-1].To = math.Max(out[n-1].To, g.To)
			continue
		}
		out = append(out, g)
	}
	for i := range out {
		out[i].From, out[i].To = math.Round(out[i].From*10)/10, math.Round(out[i].To*10)/10
	}
	return out
}

// railPieces is what is left of a contained rail with gaps cut into it: a copy for each
// piece, with its length and both end caps where it was cut. The first keeps the rail's id.
func (r *resolver) railPieces(rail *InstanceDef, parent *instance, gaps []gap) []*InstanceDef {
	mod := r.bundle.Modules[rail.Module]
	r.vars = parent.vars
	at, along := r.railLine(rail)
	r.vars = parent.vars
	lo, hi := 0.0, 0.0
	if mod.CutBox != nil {
		// The box is in the rail's own terms, such as its length.
		vars := env{}
		for _, p := range mod.Parameters {
			r.vars = vars
			if v, ok := rail.Params[p.ID]; ok {
				r.vars = parent.vars
				vars[p.ID] = r.num(v)
				continue
			}
			vars[p.ID] = r.num(p.Default)
		}
		r.vars = vars
		lo, hi = math.Min(r.num(mod.CutBox.From[0]), r.num(mod.CutBox.To[0])), math.Max(r.num(mod.CutBox.From[0]), r.num(mod.CutBox.To[0]))
		r.vars = parent.vars
	}
	var pieces [][2]float64
	start := lo
	for _, g := range gaps {
		if g.From > start {
			pieces = append(pieces, [2]float64{start, g.From})
		}
		start = math.Max(start, g.To)
	}
	if start < hi {
		pieces = append(pieces, [2]float64{start, hi})
	}
	var out []*InstanceDef
	for n, p := range pieces {
		piece := *rail
		if n > 0 {
			piece.ID = fmt.Sprintf("%s-%d", rail.ID, n+1)
		}
		piece.Params = map[string]Num{}
		for k, v := range rail.Params {
			piece.Params[k] = v
		}
		piece.Params["length"] = NumOf(p[1] - p[0])
		if p[0] > lo {
			piece.Params["capStart"] = NumOf(1)
		}
		if p[1] < hi {
			piece.Params["capEnd"] = NumOf(1)
		}
		shift := p[0] - lo
		piece.At = Vec3{NumOf(at.X + along.X*shift), NumOf(at.Y + along.Y*shift), NumOf(at.Z + along.Z*shift)}
		out = append(out, &piece)
	}
	return out
}

// railLine is where a contained rail starts and which way it runs, in the coordinates of
// the module around it, as r.vars sees them.
func (r *resolver) railLine(rail *InstanceDef) (at, along Point) {
	turn := &instance{}
	rx, ry, rz := r.num(rail.RotateX)*math.Pi/180, r.num(rail.RotateY)*math.Pi/180, r.num(rail.RotateZ)*math.Pi/180
	turn.cosX, turn.sinX, turn.cosY, turn.sinY, turn.cos, turn.sin = math.Cos(rx), math.Sin(rx), math.Cos(ry), math.Sin(ry), math.Cos(rz), math.Sin(rz)
	return r.vec(rail.At), turn.direction(Point{1, 0, 0})
}

// siteGaps is where the rail of a site is cut, in mm along the site.
func (r *resolver) siteGaps(in *instance, s *SiteDef, from, dir Point) []gap {
	if s.Rail == "" {
		return nil
	}
	gaps := r.cuts[in.def.ID+"/"+s.Rail]
	if len(gaps) == 0 {
		return nil
	}
	i := slices.IndexFunc(in.mod.Modules, func(c InstanceDef) bool { return c.ID == s.Rail })
	if i < 0 {
		r.fail("site %s follows rail %s, which %s does not contain", s.ID, s.Rail, in.mod.ID)
		return nil
	}
	r.vars = in.vars
	at, along := r.railLine(&in.mod.Modules[i])
	on := func(x float64) float64 {
		p := Point{at.X + along.X*x - from.X, at.Y + along.Y*x - from.Y, at.Z + along.Z*x - from.Z}
		return p.X*dir.X + p.Y*dir.Y + p.Z*dir.Z
	}
	var out []gap
	for _, g := range gaps {
		a, b := on(g.From), on(g.To)
		out = append(out, gap{From: math.Round(math.Min(a, b)*10) / 10, To: math.Round(math.Max(a, b)*10) / 10, By: g.By, name: g.name})
	}
	return out
}
