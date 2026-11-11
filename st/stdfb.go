package st

import "strings"

// fbVar declares a variable of a standard function block.
type fbVar struct {
	name string
	kind VarKind
	t    *Type
}

// nativeFB makes a standard function block whose body is Go.
func nativeFB(name string, vars []fbVar, body func(rt *Runtime, v []Value)) *POU {
	p := &POU{Name: name, Kind: KindFunctionBlock, byName: map[string]*Var{}, compiled: true,
		Decl: &POUDecl{Pos: Pos{File: "(standard)", Line: 1, Col: 1}, Name: name, Kind: KindFunctionBlock}}
	for i, fv := range vars {
		v := &Var{Name: fv.name, Type: fv.t, Kind: fv.kind, Slot: i, Owner: p}
		p.Vars = append(p.Vars, v)
		p.byName[strings.ToUpper(fv.name)] = v
		if fv.kind == VarInput || fv.kind == VarOutput {
			p.Params = append(p.Params, v)
		}
	}
	p.native = func(rt *Runtime, a *Agg) { body(rt, a.Elems) }
	return p
}

func set(v *Value, b bool) { v.I = b2i(b) }

// standardFBs are the standard function blocks of IEC 61131-3: timers, edge detection,
// counters and bistables. Timers run on the time the runtime is given for each cycle.
func standardFBs() []*POU {
	in, out, local := VarInput, VarOutput, VarLocal
	timer := []fbVar{{"IN", in, tBool}, {"PT", in, tTime}, {"Q", out, tBool}, {"ET", out, tTime}, {"_running", local, tBool}, {"_start", local, tTime}, {"_last", local, tBool}}
	const IN, PT, Q, ET, running, start, last = 0, 1, 2, 3, 4, 5, 6
	elapsed := func(rt *Runtime, v []Value) int64 { return min(rt.nowMs-v[start].I, v[PT].I) }
	ton := nativeFB("TON", timer, func(rt *Runtime, v []Value) {
		if v[IN].I == 0 {
			set(&v[Q], false)
			v[ET].I, v[running].I = 0, 0
			return
		}
		if v[running].I == 0 {
			v[running].I, v[start].I = 1, rt.nowMs
		}
		v[ET].I = elapsed(rt, v)
		set(&v[Q], v[ET].I >= v[PT].I)
	})
	tof := nativeFB("TOF", timer, func(rt *Runtime, v []Value) {
		if v[IN].I != 0 {
			set(&v[Q], true)
			v[ET].I, v[running].I = 0, 0
			return
		}
		if v[Q].I == 0 {
			return
		}
		if v[running].I == 0 {
			v[running].I, v[start].I = 1, rt.nowMs
		}
		v[ET].I = elapsed(rt, v)
		if v[ET].I >= v[PT].I {
			set(&v[Q], false)
			v[running].I = 0
		}
	})
	tp := nativeFB("TP", timer, func(rt *Runtime, v []Value) {
		rising := v[IN].I != 0 && v[last].I == 0
		v[last].I = v[IN].I
		if v[running].I == 0 && rising && v[Q].I == 0 {
			v[running].I, v[start].I = 1, rt.nowMs
			set(&v[Q], true)
		}
		if v[running].I != 0 {
			v[ET].I = elapsed(rt, v)
			if v[ET].I >= v[PT].I {
				set(&v[Q], false)
				v[running].I = 0
			}
			return
		}
		if v[IN].I == 0 {
			v[ET].I = 0
		}
	})
	edge := []fbVar{{"CLK", in, tBool}, {"Q", out, tBool}, {"M", local, tBool}}
	rtrig := nativeFB("R_TRIG", edge, func(_ *Runtime, v []Value) {
		set(&v[1], v[0].I != 0 && v[2].I == 0)
		v[2].I = v[0].I
	})
	// F_TRIG keeps CLK of the last call, so that it gives no pulse in the first cycle.
	ftrig := nativeFB("F_TRIG", edge, func(_ *Runtime, v []Value) {
		set(&v[1], v[0].I == 0 && v[2].I != 0)
		v[2].I = v[0].I
	})
	ctu := nativeFB("CTU", []fbVar{{"CU", in, tBool}, {"R", in, tBool}, {"PV", in, tInt}, {"Q", out, tBool}, {"CV", out, tInt}, {"_cu", local, tBool}},
		func(_ *Runtime, v []Value) {
			rising := v[0].I != 0 && v[5].I == 0
			v[5].I = v[0].I
			switch {
			case v[1].I != 0:
				v[4].I = 0
			case rising && v[4].I < 32767:
				v[4].I++
			}
			set(&v[3], v[4].I >= v[2].I)
		})
	ctd := nativeFB("CTD", []fbVar{{"CD", in, tBool}, {"LD", in, tBool}, {"PV", in, tInt}, {"Q", out, tBool}, {"CV", out, tInt}, {"_cd", local, tBool}},
		func(_ *Runtime, v []Value) {
			falling := v[0].I != 0 && v[5].I == 0
			v[5].I = v[0].I
			switch {
			case v[1].I != 0:
				v[4].I = v[2].I
			case falling && v[4].I > -32768:
				v[4].I--
			}
			set(&v[3], v[4].I <= 0)
		})
	ctud := nativeFB("CTUD", []fbVar{{"CU", in, tBool}, {"CD", in, tBool}, {"R", in, tBool}, {"LD", in, tBool}, {"PV", in, tInt},
		{"QU", out, tBool}, {"QD", out, tBool}, {"CV", out, tInt}, {"_cu", local, tBool}, {"_cd", local, tBool}},
		func(_ *Runtime, v []Value) {
			up := v[0].I != 0 && v[8].I == 0
			down := v[1].I != 0 && v[9].I == 0
			v[8].I, v[9].I = v[0].I, v[1].I
			switch {
			case v[2].I != 0:
				v[7].I = 0
			case v[3].I != 0:
				v[7].I = v[4].I
			case up && !down && v[7].I < 32767:
				v[7].I++
			case down && !up && v[7].I > -32768:
				v[7].I--
			}
			set(&v[5], v[7].I >= v[4].I)
			set(&v[6], v[7].I <= 0)
		})
	sr := nativeFB("SR", []fbVar{{"S1", in, tBool}, {"R", in, tBool}, {"Q1", out, tBool}}, func(_ *Runtime, v []Value) {
		set(&v[2], v[0].I != 0 || v[1].I == 0 && v[2].I != 0)
	})
	rs := nativeFB("RS", []fbVar{{"S", in, tBool}, {"R1", in, tBool}, {"Q1", out, tBool}}, func(_ *Runtime, v []Value) {
		set(&v[2], v[1].I == 0 && (v[0].I != 0 || v[2].I != 0))
	})
	return []*POU{ton, tof, tp, rtrig, ftrig, ctu, ctd, ctud, sr, rs}
}
