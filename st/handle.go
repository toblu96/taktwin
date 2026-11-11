package st

import (
	"fmt"
	"math"
	"time"
)

// Handle is a variable of a program that a host reads and writes directly, without a process
// image, as the simulation does with a program that runs a machine's own controller: a BOOL
// as 0 or 1, a number as it is, and a TIME in seconds.
type Handle struct {
	t *Type
	v *Value
}

// Handle finds a variable by its path, as PLC_PRG.busy or GVL.target. It must be of an
// elementary type other than a string.
func (rt *Runtime) Handle(path string) (Handle, error) {
	t, v, err := rt.resolve(path)
	if err != nil {
		return Handle{}, err
	}
	switch {
	case t.Kind == KBool, t.Kind == KTime, t.isInt(), t.isBits(), t.isReal():
		return Handle{t, v}, nil
	}
	return Handle{}, fmt.Errorf("%s is %s, not a BOOL, a number or a TIME", path, t)
}

// Type names the variable's type.
func (h Handle) Type() string { return h.t.String() }

// Get gives its value.
func (h Handle) Get() float64 {
	switch {
	case h.t.isReal():
		return h.v.F
	case h.t.Kind == KTime:
		return float64(h.v.I) / 1000
	}
	return float64(h.v.I)
}

// Set sets its value, rounded to its type and wrapped as an assignment would.
func (h Handle) Set(x float64) {
	switch {
	case h.t.Kind == KBool:
		h.v.I = b2i(x >= 0.5)
	case h.t.isReal():
		if h.t.Kind == KReal {
			x = float64(float32(x))
		}
		h.v.F = x
	case h.t.Kind == KTime:
		h.v.I = int64(math.Round(x * 1000))
	default:
		h.v.I = h.t.wrap(int64(math.Round(x)))
	}
}

// Run runs one cycle at the time now, for a program whose host reads and writes its
// variables through handles. It reports a runtime error, after which the program stays
// failed until it is reset.
func (rt *Runtime) Run(now time.Duration) error {
	rt.Scan(nil, now)
	if rt.state == Failed {
		return rt.err
	}
	return nil
}
