package st

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ImageBytes is how many bytes the input and the output image each have.
const ImageBytes = 64

// image is a process image: bytes, with bits, words and double words in them, the low byte
// of a word first.
type image []byte

// load reads a variable at a direct address.
func (im image) load(v ioVar) Value {
	if v.Size == 'X' {
		return Value{I: b2i(im.get(v.Byte, v.Bit))}
	}
	n := sizeBytes(v.Size)
	if v.Byte+n > len(im) {
		return Value{}
	}
	var raw uint64
	for i := n - 1; i >= 0; i-- {
		raw = raw<<8 | uint64(im[v.Byte+i])
	}
	if v.typ.Kind == KReal {
		return Value{F: float64(math.Float32frombits(uint32(raw)))}
	}
	return Value{I: v.typ.wrap(int64(raw))}
}

// store writes a variable at a direct address.
func (im image) store(v ioVar, x Value) {
	if v.Size == 'X' {
		im.set(v.Byte, v.Bit, x.I != 0)
		return
	}
	n := sizeBytes(v.Size)
	if v.Byte+n > len(im) {
		return
	}
	raw := uint64(x.I)
	if v.typ.Kind == KReal {
		raw = uint64(math.Float32bits(float32(x.F)))
	}
	for i := range n {
		im[v.Byte+i] = byte(raw >> (8 * i))
	}
}

func (im image) get(b, bit int) bool { return b < len(im) && im[b]&(1<<bit) != 0 }

func (im image) set(b, bit int, on bool) {
	if b >= len(im) {
		return
	}
	if on {
		im[b] |= 1 << bit
	} else {
		im[b] &^= 1 << bit
	}
}

// State is what a runtime is doing.
type State string

const (
	Stopped State = "stopped"
	Running State = "running"
	// Failed is stopped by an error of the program, such as a division by zero.
	Failed State = "failed"
)

// RuntimeError is an error of a running program, where it happened.
type RuntimeError struct {
	Pos Pos    `json:"pos"`
	Msg string `json:"msg"`
}

func (e *RuntimeError) Error() string { return e.Pos.String() + ": " + e.Msg }

// Runtime runs a program as a PLC does: each cycle it reads the inputs, runs the task's
// programs, and writes the outputs. It is not safe for concurrent use; its owner locks it.
type Runtime struct {
	prog    *Program
	globals *Agg
	progs   map[*POU]*Agg
	in, out image
	directs map[string]*directVar // outputs written by their address, as %QX0.1 := TRUE
	nowMs   int64
	budget  int
	depth   int
	state   State
	err     *RuntimeError
	forced  map[string]forcing
	cycles  int64
}

// cycleBudget is how many statements one cycle may run before the watchdog stops it.
const cycleBudget = 2_000_000

// NewRuntime makes a runtime for a program, stopped, with every variable at its initial
// value.
func NewRuntime(p *Program) *Runtime {
	rt := &Runtime{prog: p, state: Stopped, forced: map[string]forcing{}}
	rt.ResetCold()
	return rt
}

// Program is the program the runtime runs.
func (rt *Runtime) Program() *Program { return rt.prog }

// ResetCold sets every variable to its initial value, as a cold reset of a PLC does, and
// stops the program.
func (rt *Runtime) ResetCold() {
	rt.reset(false)
}

// ResetWarm sets every variable but the RETAIN ones to its initial value, and stops.
func (rt *Runtime) ResetWarm() {
	rt.reset(true)
}

func (rt *Runtime) reset(keepRetain bool) {
	old, oldProgs := rt.globals, rt.progs
	rt.globals = &Agg{Elems: make([]Value, len(rt.prog.globals))}
	for i, v := range rt.prog.globals {
		rt.globals.Elems[i] = initial(v.Type, v.Init)
		if keepRetain && v.Retain && old != nil {
			rt.globals.Elems[i] = old.Elems[i]
		}
	}
	rt.progs = map[*POU]*Agg{}
	for _, p := range rt.prog.order {
		if p.Kind != KindProgram {
			continue
		}
		a := p.newInstance().A
		if keepRetain && oldProgs != nil {
			for _, v := range p.Vars {
				if v.Retain && oldProgs[p] != nil {
					a.Elems[v.Slot] = oldProgs[p].Elems[v.Slot]
				}
			}
		}
		rt.progs[p] = a
	}
	rt.in, rt.out = make(image, ImageBytes), make(image, ImageBytes)
	rt.directs = map[string]*directVar{}
	rt.state, rt.err, rt.cycles = Stopped, nil, 0
}

// instance gives the variables of a program.
func (rt *Runtime) instance(p *POU) *Agg {
	a := rt.progs[p]
	if a == nil {
		a = p.newInstance().A
		rt.progs[p] = a
	}
	return a
}

// directVar is an output written by its address, as %QX0.1 := TRUE or %QW8 := 100.
type directVar struct {
	at    ioVar
	value Value
}

// direct gives the cell of an output written by its address.
func (rt *Runtime) direct(at ioVar) *Value {
	k := fmt.Sprintf("%c%d.%d", at.Size, at.Byte, at.Bit)
	v := rt.directs[k]
	if v == nil {
		v = &directVar{at: at}
		rt.directs[k] = v
	}
	return &v.value
}

// Start lets the program run on the next cycles. A program that failed starts again only
// after a reset.
func (rt *Runtime) Start() error {
	if rt.state == Failed {
		return errors.New("the program stopped with an error: reset it first")
	}
	rt.state = Running
	return nil
}

// Stop holds the program, with its variables as they are. The outputs keep their values.
func (rt *Runtime) Stop() {
	if rt.state == Running {
		rt.state = Stopped
	}
}

// State says whether the program runs, and the error that stopped it, if any.
func (rt *Runtime) State() (State, *RuntimeError) { return rt.state, rt.err }

// Cycles counts the cycles run since the last reset.
func (rt *Runtime) Cycles() int64 { return rt.cycles }

// Scan runs one cycle at the time now: it takes the inputs, runs the task's programs if
// the runtime runs, and gives the outputs. A stopped program gives the outputs it last
// wrote; one that failed gives none.
func (rt *Runtime) Scan(in []byte, now time.Duration) []byte {
	copy(rt.in, in)
	rt.nowMs = now.Milliseconds()
	for _, b := range rt.prog.inputs {
		*rt.cell(b) = rt.in.load(b)
	}
	rt.applyForces()
	if rt.state == Running {
		if err := rt.runPrograms(); err != nil {
			rt.state, rt.err = Failed, err
			clear(rt.out)
			return append([]byte(nil), rt.out...)
		}
		rt.cycles++
		rt.applyForces()
		for _, b := range rt.prog.outputs {
			rt.out.store(b, *rt.cell(b))
		}
		for _, v := range rt.directs {
			rt.out.store(v.at, v.value)
		}
	}
	if rt.state == Failed {
		clear(rt.out)
	}
	return append([]byte(nil), rt.out...)
}

func (rt *Runtime) cell(b ioVar) *Value {
	if b.global != nil {
		return &rt.globals.Elems[b.slot]
	}
	return &rt.instance(b.pou).Elems[b.slot]
}

func (rt *Runtime) runPrograms() (err *RuntimeError) {
	defer func() {
		if r := recover(); r != nil {
			re, ok := r.(*runtimeError)
			if !ok {
				panic(r)
			}
			err = &RuntimeError{Pos: re.Pos, Msg: re.Msg}
		}
	}()
	rt.budget, rt.depth = cycleBudget, 0
	for _, p := range rt.prog.programs {
		run(rt, p, rt.instance(p))
	}
	return nil
}
