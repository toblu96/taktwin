package sim

import (
	"math"

	"github.com/toblu96/taktwin/internal/model"
)

// foot is the outline of an upright workpiece seen from above: a circle, as a round cup,
// or a rectangle turned to its heading, as a pallet or a cardboard box.
type foot struct {
	x, y   float64 // its centre
	r      float64 // radius of a round one; 0 for a rectangle
	a, b   float64 // half its length along its heading and half its width, for a rectangle
	ux, uy float64 // its heading, a unit vector
}

func round(c model.Point, r float64) foot { return foot{x: c.X, y: c.Y, r: r, ux: 1} }

// rect is a rectangle of length along dir and width across it, centred at c.
func rect(c model.Point, length, width float64, dir model.Point) foot {
	n := math.Hypot(dir.X, dir.Y)
	ux, uy := 1.0, 0.0
	if n > 1e-9 {
		ux, uy = dir.X/n, dir.Y/n
	}
	return foot{x: c.X, y: c.Y, a: length / 2, b: width / 2, ux: ux, uy: uy}
}

func (f foot) round() bool { return f.r > 0 || f.a == 0 && f.b == 0 }

// local gives a point in the frame of the foot: along its heading, and to the left of it.
func (f foot) local(x, y float64) (float64, float64) {
	dx, dy := x-f.x, y-f.y
	return dx*f.ux + dy*f.uy, -dx*f.uy + dy*f.ux
}

// contains tells whether a point is inside the foot grown by margin, or shrunk by a negative one.
func (f foot) contains(x, y, margin float64) bool {
	if f.round() {
		return math.Hypot(x-f.x, y-f.y) <= f.r+margin
	}
	lx, ly := f.local(x, y)
	return math.Abs(lx) <= f.a+margin && math.Abs(ly) <= f.b+margin
}

// middle tells whether a point is over the middle of the foot: within half its radius, or
// half its length and width, as a tool must be to work on it.
func (f foot) middle(x, y float64) bool {
	if f.round() {
		return math.Hypot(x-f.x, y-f.y) <= f.r/2
	}
	lx, ly := f.local(x, y)
	return math.Abs(lx) <= f.a/2 && math.Abs(ly) <= f.b/2
}

// support is how far the foot reaches from its centre in the direction of the unit vector n.
func (f foot) support(nx, ny float64) float64 {
	if f.round() {
		return f.r
	}
	return f.a*math.Abs(nx*f.ux+ny*f.uy) + f.b*math.Abs(-nx*f.uy+ny*f.ux)
}

// extent is the radius of the circle around the foot.
func (f foot) extent() float64 {
	if f.round() {
		return f.r
	}
	return math.Hypot(f.a, f.b)
}

// narrow is half its smaller width: the radius of a round one.
func (f foot) narrow() float64 {
	if f.round() {
		return f.r
	}
	return math.Min(f.a, f.b)
}

// gapTo returns how far apart two feet are, and below zero how deep they overlap. Two round
// ones keep the plain distance of their centres less both radii. A round one and a rectangle
// measure from the circle's centre to the nearest point of the rectangle, which is no deeper
// than zero while the centre is inside it. Two rectangles separate along their sides.
func (f foot) gapTo(g foot) float64 {
	switch {
	case f.round() && g.round():
		return math.Hypot(f.x-g.x, f.y-g.y) - f.r - g.r
	case f.round():
		return g.toPoint(f.x, f.y) - f.r
	case g.round():
		return f.toPoint(g.x, g.y) - g.r
	}
	dx, dy := g.x-f.x, g.y-f.y
	gap := math.Inf(-1)
	for _, n := range [][2]float64{{f.ux, f.uy}, {-f.uy, f.ux}, {g.ux, g.uy}, {-g.uy, g.ux}} {
		gap = math.Max(gap, math.Abs(dx*n[0]+dy*n[1])-f.support(n[0], n[1])-g.support(n[0], n[1]))
	}
	return gap
}

// toPoint is how far a point is from a rectangle, zero inside it.
func (f foot) toPoint(x, y float64) float64 {
	lx, ly := f.local(x, y)
	return math.Hypot(math.Max(0, math.Abs(lx)-f.a), math.Max(0, math.Abs(ly)-f.b))
}

// boxFoot is the footprint of an upright box, from a corner o and its two level edges u and v.
func boxFoot(o, u, v model.Point) foot {
	c := model.Point{X: o.X + (u.X+v.X)/2, Y: o.Y + (u.Y+v.Y)/2}
	return rect(c, math.Hypot(u.X, u.Y), math.Hypot(v.X, v.Y), u)
}

// enter returns where a level ray o + t·d first meets a rectangle, as t, and whether it does
// at all for some t in [0, reach]. d need not be a unit vector.
func (f foot) enter(ox, oy, dx, dy, reach float64) (float64, bool) {
	lx, ly := f.local(ox, oy)
	ldx, ldy := dx*f.ux+dy*f.uy, -dx*f.uy+dy*f.ux
	lo, hi := math.Inf(-1), math.Inf(1)
	for _, axis := range [][3]float64{{lx, ldx, f.a}, {ly, ldy, f.b}} {
		p, d, half := axis[0], axis[1], axis[2]
		if math.Abs(d) < 1e-12 {
			if math.Abs(p) > half {
				return 0, false
			}
			continue
		}
		t1, t2 := (-half-p)/d, (half-p)/d
		lo, hi = math.Max(lo, math.Min(t1, t2)), math.Min(hi, math.Max(t1, t2))
	}
	if lo > hi || hi < 0 || lo > reach {
		return 0, false
	}
	return math.Max(lo, 0), true
}
