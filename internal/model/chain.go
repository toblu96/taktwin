package model

import "math"

// ChainLink is one level of the node tree between a module and a point in it: where
// the node sits, and the joint that moves what is under it.
type ChainLink struct {
	Position Point
	Rotation Point // degrees, as a node's rotation
	Joint    bool
	Revolute bool
	Axis     Point
	Origin   Point
	From, To float64
	State    int
}

// Chain follows a point through the moving parts of a module into the station, the same
// way the viewer moves them.
type Chain struct {
	At               Point
	RotX, RotY, RotZ float64 // degrees
	Links            []ChainLink
}

func rotateAbout(p, axis Point, deg float64) Point {
	n := math.Sqrt(axis.X*axis.X + axis.Y*axis.Y + axis.Z*axis.Z)
	if n == 0 {
		return p
	}
	k := Point{axis.X / n, axis.Y / n, axis.Z / n}
	c, s := math.Cos(deg*math.Pi/180), math.Sin(deg*math.Pi/180)
	dot := k.X*p.X + k.Y*p.Y + k.Z*p.Z
	return Point{
		p.X*c + (k.Y*p.Z-k.Z*p.Y)*s + k.X*dot*(1-c),
		p.Y*c + (k.Z*p.X-k.X*p.Z)*s + k.Y*dot*(1-c),
		p.Z*c + (k.X*p.Y-k.Y*p.X)*s + k.Z*dot*(1-c),
	}
}

// Point returns where the end of the chain is for the given states.
func (c *Chain) Point(states []float64) Point { return c.Apply(Point{}, states) }

// Apply returns where a point given in the frame of the end of the chain is. Without states,
// every joint is at rest.
func (c *Chain) Apply(p Point, states []float64) Point {
	for i := len(c.Links) - 1; i >= 0; i-- {
		l := &c.Links[i]
		if l.Joint {
			value := l.From
			if states != nil {
				value += (l.To - l.From) * states[l.State]
			}
			if l.Revolute {
				q := rotateAbout(Point{p.X - l.Origin.X, p.Y - l.Origin.Y, p.Z - l.Origin.Z}, l.Axis, value)
				p = Point{q.X + l.Origin.X, q.Y + l.Origin.Y, q.Z + l.Origin.Z}
			} else {
				p = Point{p.X + l.Axis.X*value, p.Y + l.Axis.Y*value, p.Z + l.Axis.Z*value}
			}
		}
		// A node's rotation is about x, then y, then z, applied to it from the inside out.
		p = rotateAbout(p, Point{Z: 1}, l.Rotation.Z)
		p = rotateAbout(p, Point{Y: 1}, l.Rotation.Y)
		p = rotateAbout(p, Point{X: 1}, l.Rotation.X)
		p = Point{p.X + l.Position.X, p.Y + l.Position.Y, p.Z + l.Position.Z}
	}
	p = rotateAbout(p, Point{X: 1}, c.RotX)
	p = rotateAbout(p, Point{Y: 1}, c.RotY)
	p = rotateAbout(p, Point{Z: 1}, c.RotZ)
	return Point{p.X + c.At.X, p.Y + c.At.Y, p.Z + c.At.Z}
}

// Local returns where a point of the station at rest is in the frame of the end of the chain.
func (c *Chain) Local(q Point) Point {
	o := c.Apply(Point{}, nil)
	axis := func(p Point) Point {
		a := c.Apply(p, nil)
		return Point{a.X - o.X, a.Y - o.Y, a.Z - o.Z}
	}
	x, y, z := axis(Point{X: 1}), axis(Point{Y: 1}), axis(Point{Z: 1})
	d := Point{q.X - o.X, q.Y - o.Y, q.Z - o.Z}
	return Point{dot(d, x), dot(d, y), dot(d, z)}
}

// riding returns the module that a module rides on a moving node with: itself, or the part
// it belongs to, as a reed switch on a cylinder on a lift's platform. It is nil for one that
// stays put.
func riding(in *instance) *instance {
	for ; in != nil; in = in.parent {
		if in.def.MountedOn != "" && in.parent != nil {
			return in
		}
	}
	return nil
}

// rider is the chain to the moving node that a module rides on, as a sensor on a lift's
// platform, or nil for one that stays put.
func (r *resolver) rider(in *instance) *Chain {
	on := riding(in)
	if on == nil {
		return nil
	}
	inst, vars, texts, where := r.inst, r.vars, r.texts, r.where
	r.enter(on.parent)
	c, ok := r.chainTo(on.parent, on.def.MountedOn)
	r.inst, r.vars, r.texts, r.where = inst, vars, texts, where
	if !ok {
		return nil
	}
	return c
}

// chainTo builds the chain to a named node of a module, or to the module itself without a name.
func (r *resolver) chainTo(in *instance, name string) (*Chain, bool) {
	if name == "" {
		return &Chain{At: in.at, RotX: in.rotX, RotY: in.rotY, RotZ: in.rotZ}, true
	}
	var walk func(nodes []NodeDef, path []*NodeDef) []*NodeDef
	walk = func(nodes []NodeDef, path []*NodeDef) []*NodeDef {
		for i := range nodes {
			n := &nodes[i]
			next := append(path[:len(path):len(path)], n)
			if n.Name == name {
				return next
			}
			if found := walk(n.Children, next); found != nil {
				return found
			}
		}
		return nil
	}
	path := walk(in.mod.Model, nil)
	if path == nil {
		return nil, false
	}
	c := &Chain{At: in.at, RotX: in.rotX, RotY: in.rotY, RotZ: in.rotZ}
	for _, n := range path {
		l := ChainLink{Position: r.vec(n.Position), Rotation: r.vec(n.Rotation)}
		for i := range in.mod.Joints {
			j := &in.mod.Joints[i]
			if j.Node == n.Name && r.enabled(j.When) {
				l.Joint, l.Revolute, l.Axis, l.Origin = true, j.Type == "revolute", r.vec(j.Axis), r.vec(j.Origin)
				l.From, l.To, l.State = r.num(j.Range[0]), r.num(j.Range[1]), r.local(j.Driver)
			}
		}
		c.Links = append(c.Links, l)
	}
	return c, true
}
