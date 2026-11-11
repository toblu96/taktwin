package model

import "math"

// mat3 is a rotation, as the columns of the three axes it turns x, y and z into.
type mat3 [3]Point

// rotation is the instance rotation as a matrix: about x first, then y, then z.
func rotation(rx, ry, rz float64) mat3 {
	in := &instance{}
	in.cosX, in.sinX = math.Cos(rx*math.Pi/180), math.Sin(rx*math.Pi/180)
	in.cosY, in.sinY = math.Cos(ry*math.Pi/180), math.Sin(ry*math.Pi/180)
	in.cos, in.sin = math.Cos(rz*math.Pi/180), math.Sin(rz*math.Pi/180)
	return mat3{in.direction(Point{1, 0, 0}), in.direction(Point{0, 1, 0}), in.direction(Point{0, 0, 1})}
}

func (m mat3) apply(p Point) Point {
	return Point{
		m[0].X*p.X + m[1].X*p.Y + m[2].X*p.Z,
		m[0].Y*p.X + m[1].Y*p.Y + m[2].Y*p.Z,
		m[0].Z*p.X + m[1].Z*p.Y + m[2].Z*p.Z,
	}
}

func (m mat3) mul(n mat3) mat3 { return mat3{m.apply(n[0]), m.apply(n[1]), m.apply(n[2])} }

// euler is the rotation about x, then y, then z, in degrees, that m is.
func (m mat3) euler() (rx, ry, rz float64) {
	deg := func(v float64) float64 { return math.Round(v*180/math.Pi*1e9) / 1e9 }
	// m[0] is the x axis turned: (cy*cz, cy*sz, -sy).
	sy := -m[0].Z
	if math.Abs(sy) > 1-1e-9 {
		// Turned straight up or down: x and z turn about the same axis, so z takes it all.
		ry = math.Copysign(math.Pi/2, sy)
		rz = math.Atan2(-m[1].X, m[1].Y)
		return 0, deg(ry), deg(rz)
	}
	return deg(math.Atan2(m[1].Z, m[2].Z)), deg(math.Asin(sy)), deg(math.Atan2(m[0].Y, m[0].X))
}

func cross(a, b Point) Point {
	return Point{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func dot(a, b Point) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

// frame is the rotation whose x axis runs along dir and whose z axis is as close to up as
// dir allows, so y lies to the left of dir.
func frame(dir, up Point) mat3 {
	x := scale(dir, 1/length(dir))
	z := Point{up.X - x.X*dot(up, x), up.Y - x.Y*dot(up, x), up.Z - x.Z*dot(up, x)}
	z = scale(z, 1/length(z))
	return mat3{x, cross(z, x), z}
}

// setRotation turns an instance by rotations about x, then y, then z.
func (in *instance) setRotation(rx, ry, rz float64) {
	in.rotX, in.rotY, in.rotZ = rx, ry, rz
	in.cosX, in.sinX = math.Cos(rx*math.Pi/180), math.Sin(rx*math.Pi/180)
	in.cosY, in.sinY = math.Cos(ry*math.Pi/180), math.Sin(ry*math.Pi/180)
	in.cos, in.sin = math.Cos(rz*math.Pi/180), math.Sin(rz*math.Pi/180)
}

func (in *instance) rotation() mat3 { return rotation(in.rotX, in.rotY, in.rotZ) }
