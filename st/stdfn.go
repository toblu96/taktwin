package st

import (
	"math"
	"math/bits"
	"strings"
	"unicode/utf8"
)

// stdNames are the standard functions of IEC 61131-3 besides the type conversions.
var stdNames = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`ABS SQRT LN LOG EXP SIN COS TAN ASIN ACOS ATAN EXPT TRUNC MOVE SEL MAX MIN LIMIT MUX
		SHL SHR ROL ROR LEN LEFT RIGHT MID CONCAT INSERT DELETE REPLACE FIND ADD SUB MUL DIV GT GE EQ LE LT NE`) {
		stdNames[n] = true
	}
}

// isStdFunction tells a standard function by its upper-case name, including the type
// conversions such as INT_TO_REAL and TO_REAL.
func isStdFunction(up string) bool {
	if stdNames[up] {
		return true
	}
	_, _, ok := conversion(up)
	return ok
}

// conversion reads A_TO_B, TO_B and TRUNC_B: the source type, or nil for any, and the target.
func conversion(up string) (from, to *Type, ok bool) {
	if t, ok := strings.CutPrefix(up, "TO_"); ok {
		to = elementary[t]
		return nil, to, to != nil
	}
	if t, ok := strings.CutPrefix(up, "TRUNC_"); ok {
		to = elementary[t]
		return nil, to, to != nil && to.isInt()
	}
	a, b, found := strings.Cut(up, "_TO_")
	if !found {
		return nil, nil, false
	}
	from, to = elementary[a], elementary[b]
	return from, to, from != nil && to != nil
}

// callArgs compiles the arguments of a standard function, by position. A literal takes
// the type of the other arguments, as in MAX(x, 10).
func (c *compiler) callArgs(e *CallExpr) ([]cexpr, bool) {
	out := make([]cexpr, len(e.Args))
	ok := true
	var typed *Type
	for i, a := range e.Args {
		if a.Output {
			c.errAt(a.Pos, "a standard function has no outputs to read with =>")
			return nil, false
		}
		if isUntyped(a.Value) {
			continue
		}
		out[i] = c.value(c.expr(a.Value, nil), a.Value.exprPos())
		if out[i].bad {
			ok = false
			continue
		}
		if typed == nil && out[i].t.isNum() {
			typed = out[i].t
		}
	}
	for i, a := range e.Args {
		if isUntyped(a.Value) {
			out[i] = c.value(c.expr(a.Value, typed), a.Value.exprPos())
			ok = ok && !out[i].bad
		}
	}
	return out, ok
}

// stdCall compiles a call of a standard function.
func (c *compiler) stdCall(up string, e *CallExpr) cexpr {
	// The operators as functions: ADD(a, b, c) is a + b + c.
	ops := map[string]string{"ADD": "+", "SUB": "-", "MUL": "*", "DIV": "/", "GT": ">", "GE": ">=", "EQ": "=", "LE": "<=", "LT": "<", "NE": "<>"}
	if op, ok := ops[up]; ok {
		if len(e.Args) < 2 || (up != "ADD" && up != "MUL" && len(e.Args) != 2) {
			c.errAt(e.Pos, "%s takes two values", up)
			return bad()
		}
		var x Expr = e.Args[0].Value
		for _, a := range e.Args[1:] {
			x = &BinaryExpr{Pos: e.Pos, Op: op, X: x, Y: a.Value}
		}
		return c.expr(x, nil)
	}
	args, ok := c.callArgs(e)
	if !ok {
		return bad()
	}
	n := len(args)
	need := func(k int) bool {
		if n != k {
			c.errAt(e.Pos, "%s takes %d values, not %d", up, k, n)
			return false
		}
		return true
	}
	wrongType := func(i int, want string) cexpr {
		c.errAt(e.Args[i].Value.exprPos(), "%s needs %s, not %s", up, want, args[i].t)
		return bad()
	}
	fn := func(t *Type, f func(fr *frame, v []Value) Value) cexpr {
		gets := make([]func(*frame) Value, n)
		for i, a := range args {
			gets[i] = a.eval
		}
		x := cexpr{t: t, what: "the result of " + up, eval: func(fr *frame) Value {
			v := make([]Value, len(gets))
			for i, g := range gets {
				v[i] = g(fr)
			}
			return f(fr, v)
		}}
		// A call of constants is a constant, so that it can be a bound or a label.
		allConst := true
		for _, a := range args {
			allConst = allConst && a.konst != nil
		}
		if allConst {
			k := x.eval(nil)
			x.konst = &k
		}
		return x
	}
	// convertTo converts an argument to a type it widens to, for functions of one type.
	convertTo := func(t *Type) {
		for i := range args {
			if !same(args[i].t, t) {
				args[i] = cexpr{t: t, eval: c.convertFn(args[i], t), konst: args[i].konst}
				if args[i].konst != nil {
					k := args[i].eval(nil)
					args[i].konst = &k
				}
			}
		}
	}
	commonOf := func(list []cexpr) *Type {
		t := list[0].t
		for _, a := range list[1:] {
			if t = common(t, a.t); t == nil {
				return nil
			}
		}
		return t
	}

	if from, to, ok := conversion(up); ok {
		if !need(1) {
			return bad()
		}
		a := args[0]
		switch {
		case from != nil && !same(a.t, from) && !(a.untyped && (from.isNum() || from.isBits()) && a.t.isNum()):
			return wrongType(0, from.Name)
		case a.t.Kind >= KArray && a.t.Kind != KEnum:
			return wrongType(0, "an elementary value")
		}
		src := a.t
		if from != nil {
			src = from
			if a.untyped {
				args[0] = c.convert(a, from, e.Pos, "convert")
				if args[0].bad {
					return bad()
				}
			}
		}
		if strings.HasPrefix(up, "TRUNC_") {
			return fn(to, func(_ *frame, v []Value) Value { return convertValue(Value{F: math.Trunc(toFloat(v[0], src))}, tLReal, to) })
		}
		return fn(to, func(_ *frame, v []Value) Value { return convertValue(v[0], src, to) })
	}

	switch up {
	case "TRUNC":
		if !need(1) {
			return bad()
		}
		if !args[0].t.isReal() {
			return wrongType(0, "a REAL or LREAL")
		}
		return fn(tDInt, func(_ *frame, v []Value) Value { return convertValue(Value{F: math.Trunc(v[0].F)}, tLReal, tDInt) })
	case "ABS":
		if !need(1) {
			return bad()
		}
		t := args[0].t
		switch {
		case t.isReal():
			return fn(t, func(_ *frame, v []Value) Value { return Value{F: math.Abs(v[0].F)} })
		case t.isInt():
			return fn(t, func(_ *frame, v []Value) Value {
				if v[0].I < 0 {
					return Value{I: t.wrap(-v[0].I)}
				}
				return v[0]
			})
		}
		return wrongType(0, "a number")
	case "SQRT", "LN", "LOG", "EXP", "SIN", "COS", "TAN", "ASIN", "ACOS", "ATAN":
		if !need(1) {
			return bad()
		}
		if !args[0].t.isNum() {
			return wrongType(0, "a number")
		}
		t := args[0].t
		if !t.isReal() {
			t = tLReal
			convertTo(t)
		}
		f := map[string]func(float64) float64{"SQRT": math.Sqrt, "LN": math.Log, "LOG": math.Log10, "EXP": math.Exp, "SIN": math.Sin, "COS": math.Cos,
			"TAN": math.Tan, "ASIN": math.Asin, "ACOS": math.Acos, "ATAN": math.Atan}[up]
		return fn(t, func(_ *frame, v []Value) Value { return store(t, Value{F: f(v[0].F)}) })
	case "EXPT":
		if !need(2) {
			return bad()
		}
		return c.expr(&BinaryExpr{Pos: e.Pos, Op: "**", X: e.Args[0].Value, Y: e.Args[1].Value}, nil)
	case "MOVE":
		if !need(1) {
			return bad()
		}
		a := args[0]
		return cexpr{t: a.t, eval: func(fr *frame) Value { return a.eval(fr).clone() }, konst: a.konst}
	case "SEL":
		if !need(3) {
			return bad()
		}
		if args[0].t.Kind != KBool {
			return wrongType(0, "a BOOL to select by")
		}
		t := commonOf(args[1:])
		if t == nil {
			c.errAt(e.Pos, "SEL chooses between %s and %s, which have no common type", args[1].t, args[2].t)
			return bad()
		}
		g, a, b := args[0].eval, c.convertFn(args[1], t), c.convertFn(args[2], t)
		// Only the chosen value is evaluated.
		return cexpr{t: t, eval: func(fr *frame) Value {
			if g(fr).I != 0 {
				return b(fr)
			}
			return a(fr)
		}}
	case "MAX", "MIN", "LIMIT":
		if up == "LIMIT" && !need(3) || n < 2 {
			if n < 2 {
				c.errAt(e.Pos, "%s takes at least two values", up)
			}
			return bad()
		}
		t := commonOf(args)
		if t == nil || !(t.isNum() || t.isBits() || t.Kind == KTime) {
			c.errAt(e.Pos, "%s compares numbers or times of a common type", up)
			return bad()
		}
		convertTo(t)
		less := func(a, b Value) bool {
			switch {
			case t.isReal():
				return a.F < b.F
			case t.isUnsigned():
				return uint64(a.I) < uint64(b.I)
			}
			return a.I < b.I
		}
		switch up {
		case "MAX":
			return fn(t, func(_ *frame, v []Value) Value {
				m := v[0]
				for _, x := range v[1:] {
					if less(m, x) {
						m = x
					}
				}
				return m
			})
		case "MIN":
			return fn(t, func(_ *frame, v []Value) Value {
				m := v[0]
				for _, x := range v[1:] {
					if less(x, m) {
						m = x
					}
				}
				return m
			})
		}
		return fn(t, func(_ *frame, v []Value) Value {
			mn, in, mx := v[0], v[1], v[2]
			if less(in, mn) {
				return mn
			}
			if less(mx, in) {
				return mx
			}
			return in
		})
	case "MUX":
		if n < 3 {
			c.errAt(e.Pos, "MUX takes a selector and at least two values")
			return bad()
		}
		if !args[0].t.isInt() {
			return wrongType(0, "an integer to select by")
		}
		t := commonOf(args[1:])
		if t == nil {
			c.errAt(e.Pos, "the values of MUX have no common type")
			return bad()
		}
		k := args[0].eval
		var choices []func(*frame) Value
		for _, a := range args[1:] {
			choices = append(choices, c.convertFn(a, t))
		}
		pos := e.Pos
		return cexpr{t: t, eval: func(fr *frame) Value {
			i := k(fr).I
			if i < 0 || i >= int64(len(choices)) {
				fail(pos, "MUX selects value "+itoa(i)+", but has only "+itoa(int64(len(choices))))
			}
			return choices[i](fr)
		}}
	case "SHL", "SHR", "ROL", "ROR":
		if !need(2) {
			return bad()
		}
		t := args[0].t
		if !t.isBits() && !t.isInt() {
			return wrongType(0, "a bit string such as BYTE or WORD")
		}
		if !args[1].t.isInt() {
			return wrongType(1, "an integer number of bits")
		}
		w := t.bits()
		return fn(t, func(_ *frame, v []Value) Value {
			x, k := uint64(v[0].I)&(1<<w-1|bitsAll(w)), int(v[1].I)
			switch up {
			case "SHL":
				if k >= w {
					return Value{}
				}
				return Value{I: t.wrap(int64(x << k))}
			case "SHR":
				if k >= w {
					return Value{}
				}
				return Value{I: t.wrap(int64(x >> k))}
			}
			k %= w
			if up == "ROR" {
				k = (w - k) % w
			}
			if w == 64 {
				return Value{I: int64(bits.RotateLeft64(x, k))}
			}
			return Value{I: t.wrap(int64(x<<k | x>>(w-k)))}
		})
	case "LEN":
		if !need(1) {
			return bad()
		}
		if !args[0].t.isString() {
			return wrongType(0, "a string")
		}
		return fn(tInt, func(_ *frame, v []Value) Value { return Value{I: int64(utf8.RuneCountInString(v[0].S))} })
	case "LEFT", "RIGHT", "MID", "INSERT", "DELETE", "REPLACE", "CONCAT", "FIND":
		return c.stringFunc(up, e, args, fn, wrongType, need)
	}
	c.errAt(e.Pos, "%s is not supported", up)
	return bad()
}

func bitsAll(w int) uint64 {
	if w >= 64 {
		return math.MaxUint64
	}
	return 0
}

func toFloat(v Value, t *Type) float64 {
	if t.isReal() {
		return v.F
	}
	return convertValue(v, t, tLReal).F
}

// stringFunc compiles the string functions, with positions counted from 1 as IEC 61131-3 does.
func (c *compiler) stringFunc(up string, e *CallExpr, args []cexpr, fn func(*Type, func(*frame, []Value) Value) cexpr,
	wrongType func(int, string) cexpr, need func(int) bool) cexpr {
	strs, ints := map[string][]int{"LEFT": {0}, "RIGHT": {0}, "MID": {0}, "INSERT": {0, 1}, "DELETE": {0}, "REPLACE": {0, 1}, "FIND": {0, 1}},
		map[string][]int{"LEFT": {1}, "RIGHT": {1}, "MID": {1, 2}, "INSERT": {2}, "DELETE": {1, 2}, "REPLACE": {2, 3}}
	counts := map[string]int{"LEFT": 2, "RIGHT": 2, "MID": 3, "INSERT": 3, "DELETE": 3, "REPLACE": 4, "FIND": 2}
	if up == "CONCAT" {
		if len(args) < 2 {
			c.errAt(e.Pos, "CONCAT joins at least two strings")
			return bad()
		}
		for i, a := range args {
			if !a.t.isString() {
				return wrongType(i, "strings")
			}
		}
		return fn(args[0].t, func(_ *frame, v []Value) Value {
			var b strings.Builder
			for _, x := range v {
				b.WriteString(x.S)
			}
			return Value{S: b.String()}
		})
	}
	if !need(counts[up]) {
		return bad()
	}
	for _, i := range strs[up] {
		if !args[i].t.isString() {
			return wrongType(i, "a string")
		}
	}
	for _, i := range ints[up] {
		if !args[i].t.isInt() {
			return wrongType(i, "an integer")
		}
	}
	clamp := func(n, lo, hi int64) int {
		return int(max(lo, min(hi, n)))
	}
	t := args[0].t
	switch up {
	case "LEFT":
		return fn(t, func(_ *frame, v []Value) Value {
			r := []rune(v[0].S)
			return Value{S: string(r[:clamp(v[1].I, 0, int64(len(r)))])}
		})
	case "RIGHT":
		return fn(t, func(_ *frame, v []Value) Value {
			r := []rune(v[0].S)
			return Value{S: string(r[len(r)-clamp(v[1].I, 0, int64(len(r))):])}
		})
	case "MID":
		return fn(t, func(_ *frame, v []Value) Value {
			r := []rune(v[0].S)
			start := clamp(v[2].I-1, 0, int64(len(r)))
			end := clamp(int64(start)+v[1].I, int64(start), int64(len(r)))
			return Value{S: string(r[start:end])}
		})
	case "INSERT":
		return fn(t, func(_ *frame, v []Value) Value {
			r := []rune(v[0].S)
			p := clamp(v[2].I, 0, int64(len(r)))
			return Value{S: string(r[:p]) + v[1].S + string(r[p:])}
		})
	case "DELETE":
		return fn(t, func(_ *frame, v []Value) Value {
			r := []rune(v[0].S)
			start := clamp(v[2].I-1, 0, int64(len(r)))
			end := clamp(int64(start)+v[1].I, int64(start), int64(len(r)))
			return Value{S: string(r[:start]) + string(r[end:])}
		})
	case "REPLACE":
		return fn(t, func(_ *frame, v []Value) Value {
			r := []rune(v[0].S)
			start := clamp(v[3].I-1, 0, int64(len(r)))
			end := clamp(int64(start)+v[2].I, int64(start), int64(len(r)))
			return Value{S: string(r[:start]) + v[1].S + string(r[end:])}
		})
	}
	return fn(tInt, func(_ *frame, v []Value) Value {
		i := strings.Index(v[0].S, v[1].S)
		if i < 0 {
			return Value{}
		}
		return Value{I: int64(utf8.RuneCountInString(v[0].S[:i]) + 1)}
	})
}
