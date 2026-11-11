package st

import (
	"math"
	"strconv"
	"strings"
)

// cexpr is a compiled expression: its type, and how to get its value, or where it is kept.
type cexpr struct {
	t    *Type
	eval func(fr *frame) Value
	// lval gives the variable itself, for an expression that can be assigned to.
	lval func(fr *frame) *Value
	// readonly forbids assigning to a variable that has lval: a constant, or an output of
	// an instance seen from outside.
	readonly bool
	// konst is the value of a constant expression.
	konst *Value
	// untyped is a literal whose type follows from where it is used, as 5 or 2.5.
	untyped bool
	// pou is set where the expression names a POU, for a call; list a global variable list;
	// typ an enumeration, for its values.
	pou  *POU
	list map[string]*Var
	typ  *Type
	// what names it in messages, such as "the variable x".
	what string
	bad  bool
	// setBit writes a bit of an integer, as x.3 := TRUE does.
	setBit func(fr *frame, on bool)
}

func bad() cexpr { return cexpr{bad: true} }

func konst(t *Type, v Value) cexpr {
	return cexpr{t: t, konst: &v, eval: func(*frame) Value { return v }}
}

// runtimeError stops the program where it went wrong, as a PLC's exception does.
type runtimeError struct {
	Pos Pos
	Msg string
}

func (e *runtimeError) Error() string { return e.Pos.String() + ": " + e.Msg }

func fail(p Pos, msg string) { panic(&runtimeError{Pos: p, Msg: msg}) }

// expr compiles an expression. want is the type the context expects, or nil; a literal
// takes it where it fits.
func (c *compiler) expr(e Expr, want *Type) cexpr {
	switch e := e.(type) {
	case *ParenExpr:
		return c.expr(e.X, want)
	case *Literal:
		return c.literal(e, want)
	case *Ident:
		return c.ident(e)
	case *MemberExpr:
		return c.member(e)
	case *IndexExpr:
		return c.index(e)
	case *UnaryExpr:
		return c.unary(e, want)
	case *BinaryExpr:
		return c.binary(e, want)
	case *CallExpr:
		return c.callExpr(e)
	case *ArrayInit, *StructInit:
		c.errAt(e.exprPos(), "[ … ] and ( name := … ) only give the initial value of a declaration")
		return bad()
	}
	c.errAt(e.exprPos(), "this is no expression")
	return bad()
}

func (c *compiler) literal(e *Literal, want *Type) cexpr {
	switch e.Kind {
	case tokKeyword:
		return konst(tBool, Value{I: b2i(e.Text == "TRUE")})
	case tokString:
		t := tStr
		if strings.HasPrefix(e.Text, `"`) {
			t = tWStr
		}
		return konst(t, Value{S: e.Text[1:]})
	case tokInt:
		v, ok := parseInt(e.Text)
		if !ok {
			c.errAt(e.Pos, "%s is too large for any integer", e.Text)
			return bad()
		}
		if want != nil && (want.isInt() || want.isBits()) && want.fits(v) {
			x := konst(want, Value{I: v})
			x.untyped = true
			return x
		}
		if want != nil && want.isReal() {
			x := konst(want, Value{F: float64(v)})
			x.untyped = true
			return x
		}
		// Without a type to take, the smallest of INT, DINT and LINT it fits.
		t := tLInt
		if tInt.fits(v) {
			t = tInt
		} else if tDInt.fits(v) {
			t = tDInt
		}
		x := konst(t, Value{I: v})
		x.untyped = true
		return x
	case tokReal:
		f, err := strconv.ParseFloat(strings.ReplaceAll(e.Text, "_", ""), 64)
		if err != nil {
			c.errAt(e.Pos, "%s is no number", e.Text)
			return bad()
		}
		t := tLReal
		if want != nil && want.Kind == KReal {
			t = tReal
		}
		x := konst(t, Value{F: store(t, Value{F: f}).F})
		x.untyped = true
		return x
	case tokTyped:
		return c.typedLiteral(e)
	}
	return bad()
}

// typedLiteral reads TYPE#value: T#1s, INT#5, BOOL#TRUE, or an enumeration's value Color#Red.
func (c *compiler) typedLiteral(e *Literal) cexpr {
	typ, val, _ := strings.Cut(e.Text, "#")
	switch typ {
	case "T", "TIME":
		ms, ok := parseTime(val)
		if !ok {
			c.errAt(e.Pos, "%s is no time: write T#1s, T#500ms or T#1m30s, for example", e.Text)
			return bad()
		}
		return konst(tTime, Value{I: ms})
	}
	t := elementary[typ]
	if t == nil {
		if et, ok := c.prog.types[typ]; ok && et.Kind == KEnum {
			for _, v := range et.Values {
				if strings.EqualFold(v.Name, val) {
					return konst(et, Value{I: v.Value})
				}
			}
			c.errAt(e.Pos, "the enumeration %s has no value %s", et.Name, val)
			return bad()
		}
		c.errAt(e.Pos, "%s# names no type", typ)
		return bad()
	}
	switch {
	case t.Kind == KBool:
		switch strings.ToUpper(val) {
		case "TRUE", "1":
			return konst(tBool, Value{I: 1})
		case "FALSE", "0":
			return konst(tBool, Value{})
		}
	case t.isInt() || t.isBits():
		neg := strings.HasPrefix(val, "-")
		v, ok := parseInt(strings.TrimPrefix(strings.TrimPrefix(val, "-"), "+"))
		if neg {
			v = -v
		}
		if ok && (t.fits(v) || t.isBits() && v >= 0 && t.wrap(v) == v) {
			return konst(t, Value{I: v})
		}
		if ok {
			c.errAt(e.Pos, "%s does not fit into %s", val, t)
			return bad()
		}
	case t.isReal():
		if f, err := strconv.ParseFloat(strings.ReplaceAll(val, "_", ""), 64); err == nil {
			return konst(t, store(t, Value{F: f}))
		}
	case t.isString():
		if strings.HasPrefix(val, "'") || strings.HasPrefix(val, `"`) {
			return konst(t, Value{S: val[1 : len(val)-1]})
		}
	}
	c.errAt(e.Pos, "%s is no value of %s", val, t)
	return bad()
}

// parseInt reads an integer literal: 123, 1_000, 16#FF, 2#1010, 8#17.
func parseInt(s string) (int64, bool) {
	s = strings.ReplaceAll(s, "_", "")
	base := 10
	if b, digits, ok := strings.Cut(s, "#"); ok {
		n, err := strconv.Atoi(b)
		if err != nil {
			return 0, false
		}
		base, s = n, digits
	}
	u, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return 0, false
	}
	return int64(u), true
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// ident resolves a name: a variable of the POU, a global, a program, a POU or type, an
// enumeration value, or a direct address.
func (c *compiler) ident(e *Ident) cexpr {
	up := strings.ToUpper(e.Name)
	if strings.HasPrefix(up, "%") {
		return c.address(e)
	}
	if p := c.pou; p != nil {
		if v := p.lookup(up); v != nil {
			return c.varExpr(v, e.Name)
		}
	}
	if v, ok := c.prog.global[up]; ok {
		return c.varExpr(v, e.Name)
	}
	if pou, ok := c.prog.pous[up]; ok {
		if pou.Kind == KindProgram {
			// A program's variables are read as PRG.x; the program itself is called.
			return cexpr{pou: pou, what: "the program " + pou.Name}
		}
		return cexpr{pou: pou, what: "the " + strings.ToLower(pou.Kind.String()) + " " + pou.Name}
	}
	if list, ok := c.prog.lists[up]; ok {
		return cexpr{list: list, what: "the global variable list " + e.Name}
	}
	if t, ok := c.prog.types[up]; ok && t.Kind == KEnum {
		return cexpr{typ: t, what: "the type " + t.Name}
	}
	if _, ok := c.dataTypes[up]; ok {
		if t := c.namedType(up, e.Pos); t != nil && t.Kind == KEnum {
			return cexpr{typ: t, what: "the type " + t.Name}
		}
	}
	// An enumeration's value by its name alone, where only one enumeration has it.
	var found cexpr
	n := 0
	for _, t := range c.prog.types {
		if t.Kind != KEnum {
			continue
		}
		for _, v := range t.Values {
			if strings.EqualFold(v.Name, e.Name) {
				found = konst(t, Value{I: v.Value})
				n++
			}
		}
	}
	if n == 1 {
		return found
	}
	if n > 1 {
		c.errAt(e.Pos, "%s is a value of more than one enumeration: write Type.%s", e.Name, e.Name)
		return bad()
	}
	if isStdFunction(up) {
		return cexpr{what: "the function " + up}
	}
	c.errAt(e.Pos, "%s is not declared", e.Name)
	return bad()
}

// varExpr reads and writes a variable where it is kept.
func (c *compiler) varExpr(v *Var, name string) cexpr {
	if v.Type == nil {
		return bad()
	}
	x := cexpr{t: v.Type, what: "the variable " + name, readonly: v.Constant}
	slot := v.Slot
	switch {
	case v.Owner == nil:
		x.lval = func(fr *frame) *Value { return &fr.rt.globals.Elems[slot] }
	case v.Owner != c.pou:
		// A variable of another program, as PRG.x.
		owner := v.Owner
		x.lval = func(fr *frame) *Value { return &fr.rt.instance(owner).Elems[slot] }
	case v.Kind == VarInOut:
		x.lval = func(fr *frame) *Value {
			r := fr.a.Refs[slot]
			if r == nil {
				fail(v.Pos, "the VAR_IN_OUT "+name+" was not given in the call")
			}
			return r
		}
	default:
		x.lval = func(fr *frame) *Value { return &fr.a.Elems[slot] }
	}
	if v.Constant && v.Init != nil && v.Type.Kind < KArray {
		k := v.Init.make()
		x.konst = &k
	}
	lv := x.lval
	x.eval = func(fr *frame) Value { return *lv(fr) }
	if x.konst != nil {
		k := *x.konst
		x.eval = func(*frame) Value { return k }
	}
	return x
}

// address reads or writes a bit of the process image directly, as %IX0.0 or %QX1.2.
func (c *compiler) address(e *Ident) cexpr {
	input, b, bit, ok := parseAddress(e.Name)
	if !ok {
		c.errAt(e.Pos, "%s is no address of a bit: write %%IX0.0 for an input or %%QX0.0 for an output", e.Name)
		return bad()
	}
	if input {
		return cexpr{t: tBool, what: e.Name, readonly: true, lval: func(fr *frame) *Value {
			v := Value{I: b2i(fr.rt.in.get(b, bit))}
			return &v
		}, eval: func(fr *frame) Value { return Value{I: b2i(fr.rt.in.get(b, bit))} }}
	}
	return cexpr{t: tBool, what: e.Name, lval: func(fr *frame) *Value { return fr.rt.direct(b, bit) },
		eval: func(fr *frame) Value { return *fr.rt.direct(b, bit) }}
}

// member resolves x.name: a field, a variable of an instance or program, a global of a
// list, an enumeration's value, or a bit x.3.
func (c *compiler) member(e *MemberExpr) cexpr {
	x := c.expr(e.X, nil)
	if x.bad {
		return x
	}
	up := strings.ToUpper(e.Name)
	switch {
	case x.list != nil:
		v, ok := x.list[up]
		if !ok {
			c.errAt(e.Pos, "%s has no variable %s", x.what, e.Name)
			return bad()
		}
		return c.varExpr(v, e.Name)
	case x.typ != nil:
		for _, v := range x.typ.Values {
			if strings.EqualFold(v.Name, e.Name) {
				return konst(x.typ, Value{I: v.Value})
			}
		}
		c.errAt(e.Pos, "the enumeration %s has no value %s", x.typ.Name, e.Name)
		return bad()
	case x.pou != nil:
		if x.pou.Kind != KindProgram {
			c.errAt(e.Pos, "%s has no variables of its own to read: only an instance or a program has", x.what)
			return bad()
		}
		v := x.pou.lookup(up)
		if v == nil || v.Owner != x.pou {
			c.errAt(e.Pos, "the program %s has no variable %s", x.pou.Name, e.Name)
			return bad()
		}
		y := c.varExprOf(v, e.Name)
		y.readonly = y.readonly || v.Kind == VarOutput && c.pou != x.pou
		return y
	}
	if x.t == nil {
		c.errAt(e.Pos, "%s has no %s", x.what, e.Name)
		return bad()
	}
	// A bit of an integer or bit string: x.0 to x.63.
	if n, err := strconv.Atoi(e.Name); err == nil {
		if !x.t.isInt() && !x.t.isBits() || n < 0 || n >= x.t.bits() {
			c.errAt(e.Pos, "%s has no bit %d", x.what, n)
			return bad()
		}
		bit := uint(n)
		get := x.eval
		y := cexpr{t: tBool, what: x.what + "." + e.Name, readonly: x.readonly || x.lval == nil,
			eval: func(fr *frame) Value { return Value{I: get(fr).I >> bit & 1} }}
		if x.lval != nil {
			lv, t := x.lval, x.t
			y.setBit = func(fr *frame, on bool) {
				p := lv(fr)
				if on {
					p.I = t.wrap(p.I | 1<<bit)
				} else {
					p.I = t.wrap(p.I &^ (1 << bit))
				}
			}
		}
		return y
	}
	var fieldT *Type
	var slot int
	readonly := x.readonly
	switch x.t.Kind {
	case KStruct:
		found := false
		for i, f := range x.t.Fields {
			if strings.EqualFold(f.Name, e.Name) {
				fieldT, slot, found = f.Type, i, true
			}
		}
		if !found {
			c.errAt(e.Pos, "the structure %s has no field %s", x.t.Name, e.Name)
			return bad()
		}
	case KFB:
		v := x.t.FB.lookup(up)
		if v == nil || v.Owner != x.t.FB {
			c.errAt(e.Pos, "the function block %s has no variable %s", x.t.FB.Name, e.Name)
			return bad()
		}
		if v.Kind != VarInput && v.Kind != VarOutput && v.Kind != VarInOut && c.pou != x.t.FB {
			c.errAt(e.Pos, "%s is inside %s: from outside, only its inputs and outputs can be read", e.Name, x.t.FB.Name)
			return bad()
		}
		fieldT, slot = v.Type, v.Slot
		readonly = readonly || v.Kind == VarOutput
	default:
		c.errAt(e.Pos, "%s is %s, which has no %s", x.what, x.t, e.Name)
		return bad()
	}
	if fieldT == nil {
		return bad()
	}
	y := cexpr{t: fieldT, what: x.what + "." + e.Name, readonly: readonly}
	if x.lval != nil {
		lv := x.lval
		y.lval = func(fr *frame) *Value { return &lv(fr).A.Elems[slot] }
		y.eval = func(fr *frame) Value { return lv(fr).A.Elems[slot] }
	} else {
		get := x.eval
		y.eval = func(fr *frame) Value { return get(fr).A.Elems[slot] }
	}
	if x.konst != nil {
		k := x.konst.A.Elems[slot]
		y.konst = &k
	}
	return y
}

// varExprOf is varExpr for a variable of another POU, a program's.
func (c *compiler) varExprOf(v *Var, name string) cexpr {
	saved := c.pou
	c.pou = nil
	defer func() { c.pou = saved }()
	x := c.varExpr(v, name)
	if saved == v.Owner {
		// Inside the program itself, PRG.x is its own x.
		c.pou = saved
		return c.varExpr(v, name)
	}
	return x
}

// index resolves x[i, j], checking each index against its bounds while it runs.
func (c *compiler) index(e *IndexExpr) cexpr {
	x := c.expr(e.X, nil)
	if x.bad {
		return x
	}
	if x.t == nil || x.t.Kind != KArray {
		if x.t != nil && x.t.isString() {
			c.errAt(e.Pos, "a string's characters are read with MID, not [ ]")
		} else {
			c.errAt(e.Pos, "%s is no array", x.what)
		}
		return bad()
	}
	if len(e.Indices) != len(x.t.Dims) {
		c.errAt(e.Pos, "%s has %d dimensions, but %d indices are given", x.what, len(x.t.Dims), len(e.Indices))
		return bad()
	}
	type ix struct {
		eval   func(*frame) Value
		lo, hi int64
		stride int64
	}
	var idx []ix
	stride := int64(1)
	for i := len(x.t.Dims) - 1; i >= 0; i-- {
		d := x.t.Dims[i]
		ie := c.expr(e.Indices[i], tDInt)
		if ie.bad {
			return bad()
		}
		if !ie.t.isInt() && ie.t.Kind != KEnum {
			c.errAt(e.Indices[i].exprPos(), "an index is an integer, not %s", ie.t)
			return bad()
		}
		if ie.konst != nil && (ie.konst.I < d.Lo || ie.konst.I > d.Hi) {
			c.errAt(e.Indices[i].exprPos(), "%d is outside the array, which goes from %d to %d", ie.konst.I, d.Lo, d.Hi)
			return bad()
		}
		idx = append([]ix{{ie.eval, d.Lo, d.Hi, stride}}, idx...)
		stride *= d.Hi - d.Lo + 1
	}
	pos, what := e.Pos, x.what
	flat := func(fr *frame) int {
		n := int64(0)
		for _, i := range idx {
			v := i.eval(fr).I
			if v < i.lo || v > i.hi {
				fail(pos, "the index "+strconv.FormatInt(v, 10)+" is outside "+what+", which goes from "+strconv.FormatInt(i.lo, 10)+" to "+strconv.FormatInt(i.hi, 10))
			}
			n += (v - i.lo) * i.stride
		}
		return int(n)
	}
	y := cexpr{t: x.t.Elem, what: x.what + "[…]", readonly: x.readonly}
	if x.lval != nil {
		lv := x.lval
		y.lval = func(fr *frame) *Value { return &lv(fr).A.Elems[flat(fr)] }
		y.eval = func(fr *frame) Value { return lv(fr).A.Elems[flat(fr)] }
	} else {
		get := x.eval
		y.eval = func(fr *frame) Value { return get(fr).A.Elems[flat(fr)] }
	}
	return y
}

// value checks that an expression has a value, as opposed to naming a POU or a type.
func (c *compiler) value(x cexpr, p Pos) cexpr {
	if x.bad {
		return x
	}
	if x.t == nil || x.eval == nil {
		c.errAt(p, "%s has no value here", x.what)
		return bad()
	}
	if x.t.Kind == KFB {
		c.errAt(p, "%s is an instance of %s: call it, or read one of its outputs as %s.Q", x.what, x.t.Name, strings.TrimPrefix(x.what, "the variable "))
		return bad()
	}
	return x
}

func (c *compiler) unary(e *UnaryExpr, want *Type) cexpr {
	x := c.value(c.expr(e.X, want), e.Pos)
	if x.bad {
		return x
	}
	t, get := x.t, x.eval
	var f func(Value) Value
	switch {
	case e.Op == "NOT" && t.Kind == KBool:
		f = func(v Value) Value { return Value{I: 1 - v.I} }
	case e.Op == "NOT" && (t.isBits() || t.isInt()):
		f = func(v Value) Value { return Value{I: t.wrap(^v.I)} }
	case e.Op == "-" && t.isInt():
		if t.isUnsigned() {
			c.errAt(e.Pos, "%s is unsigned and has no negative values", t)
			return bad()
		}
		f = func(v Value) Value { return Value{I: t.wrap(-v.I)} }
	case e.Op == "-" && t.isReal():
		f = func(v Value) Value { return Value{F: -v.F} }
	case e.Op == "-" && t.Kind == KTime:
		f = func(v Value) Value { return Value{I: -v.I} }
	default:
		c.errAt(e.Pos, "%s cannot be applied to %s", e.Op, t)
		return bad()
	}
	y := cexpr{t: t, eval: func(fr *frame) Value { return f(get(fr)) }, untyped: x.untyped}
	if x.konst != nil {
		k := f(*x.konst)
		y.konst = &k
		y.eval = func(*frame) Value { return k }
	}
	return y
}

// isUntyped tells an expression made only of literals without a type, as -5 or (2 * 3).
func isUntyped(e Expr) bool {
	switch e := e.(type) {
	case *Literal:
		return e.Kind == tokInt || e.Kind == tokReal
	case *ParenExpr:
		return isUntyped(e.X)
	case *UnaryExpr:
		return e.Op == "-" && isUntyped(e.X)
	case *BinaryExpr:
		return isUntyped(e.X) && isUntyped(e.Y)
	}
	return false
}

func (c *compiler) binary(e *BinaryExpr, want *Type) cexpr {
	// A literal takes the type of the other side: in i + 1, 1 is of i's type.
	opWant := want
	switch e.Op {
	case "=", "<>", "<", ">", "<=", ">=":
		opWant = nil
	}
	var x, y cexpr
	if isUntyped(e.X) && !isUntyped(e.Y) {
		y = c.value(c.expr(e.Y, opWant), e.Y.exprPos())
		hint := opWant
		if !y.bad && (e.Op == "**" || y.t.Kind != KTime) {
			hint = y.t
		}
		x = c.value(c.expr(e.X, hint), e.X.exprPos())
	} else {
		x = c.value(c.expr(e.X, opWant), e.X.exprPos())
		hint := opWant
		if !x.bad && (x.t.Kind != KTime || e.Op != "*" && e.Op != "/") && e.Op != "**" {
			hint = x.t
		}
		y = c.value(c.expr(e.Y, hint), e.Y.exprPos())
	}
	if x.bad || y.bad {
		return bad()
	}
	r := c.operate(e, x, y)
	if r.bad {
		return r
	}
	r.untyped = x.untyped && y.untyped
	if x.konst != nil && y.konst != nil {
		k := func() (v Value, err *runtimeError) {
			defer func() {
				if p := recover(); p != nil {
					if re, ok := p.(*runtimeError); ok {
						err = re
						return
					}
					panic(p)
				}
			}()
			return r.eval(nil), nil
		}
		v, err := k()
		if err != nil {
			c.errAt(e.Pos, "%s", err.Msg)
			return bad()
		}
		r.konst = &v
		r.eval = func(*frame) Value { return v }
	}
	return r
}

// operate applies a binary operator to two compiled operands.
func (c *compiler) operate(e *BinaryExpr, x, y cexpr) cexpr {
	op, pos := e.Op, e.Pos
	gx, gy := x.eval, y.eval
	mismatch := func() cexpr {
		c.errAt(pos, "%s cannot be applied to %s and %s", op, x.t, y.t)
		return bad()
	}
	switch op {
	case "AND", "OR", "XOR":
		t := common(x.t, y.t)
		if t == nil || !(t.Kind == KBool || t.isBits() || t.isInt()) {
			return mismatch()
		}
		gx, gy = c.convertFn(x, t), c.convertFn(y, t)
		var f func(a, b int64) int64
		switch op {
		case "AND":
			f = func(a, b int64) int64 { return a & b }
		case "OR":
			f = func(a, b int64) int64 { return a | b }
		default:
			f = func(a, b int64) int64 { return a ^ b }
		}
		return cexpr{t: t, eval: func(fr *frame) Value { return Value{I: f(gx(fr).I, gy(fr).I)} }}
	case "=", "<>", "<", ">", "<=", ">=":
		var cmp func(a, b Value) int
		switch {
		case x.t.Kind == KEnum || y.t.Kind == KEnum:
			if x.t != y.t {
				return mismatch()
			}
			cmp = func(a, b Value) int { return cmpInt(a.I, b.I) }
		case x.t.isString() && y.t.isString():
			cmp = func(a, b Value) int { return strings.Compare(a.S, b.S) }
		case x.t.Kind == KTime && y.t.Kind == KTime, x.t.Kind == KBool && y.t.Kind == KBool:
			cmp = func(a, b Value) int { return cmpInt(a.I, b.I) }
		default:
			t := common(x.t, y.t)
			if t == nil || !(t.isNum() || t.isBits()) {
				return mismatch()
			}
			gx, gy = c.convertFn(x, t), c.convertFn(y, t)
			switch {
			case t.isReal():
				cmp = func(a, b Value) int {
					switch {
					case a.F < b.F:
						return -1
					case a.F > b.F:
						return 1
					}
					return 0
				}
			case t.isUnsigned():
				cmp = func(a, b Value) int {
					switch {
					case uint64(a.I) < uint64(b.I):
						return -1
					case uint64(a.I) > uint64(b.I):
						return 1
					}
					return 0
				}
			default:
				cmp = func(a, b Value) int { return cmpInt(a.I, b.I) }
			}
		}
		var test func(int) bool
		switch op {
		case "=":
			test = func(n int) bool { return n == 0 }
		case "<>":
			test = func(n int) bool { return n != 0 }
		case "<":
			test = func(n int) bool { return n < 0 }
		case ">":
			test = func(n int) bool { return n > 0 }
		case "<=":
			test = func(n int) bool { return n <= 0 }
		default:
			test = func(n int) bool { return n >= 0 }
		}
		return cexpr{t: tBool, eval: func(fr *frame) Value { return Value{I: b2i(test(cmp(gx(fr), gy(fr))))} }}
	case "**":
		if !x.t.isNum() || !y.t.isNum() {
			return mismatch()
		}
		t := tLReal
		if x.t.Kind == KReal {
			t = tReal
		}
		fx, fy := c.convertFn(x, tLReal), c.convertFn(y, tLReal)
		return cexpr{t: t, eval: func(fr *frame) Value { return store(t, Value{F: math.Pow(fx(fr).F, fy(fr).F)}) }}
	}
	// + - * / MOD
	if x.t.Kind == KTime || y.t.Kind == KTime {
		switch {
		case (op == "+" || op == "-") && x.t.Kind == KTime && y.t.Kind == KTime:
			sign := int64(1)
			if op == "-" {
				sign = -1
			}
			return cexpr{t: tTime, eval: func(fr *frame) Value { return Value{I: gx(fr).I + sign*gy(fr).I} }}
		case (op == "*" || op == "/") && x.t.Kind == KTime && y.t.isNum(), op == "*" && y.t.Kind == KTime && x.t.isNum():
			tm, n := gx, c.convertFn(y, tLReal)
			if y.t.Kind == KTime {
				tm, n = gy, c.convertFn(x, tLReal)
			}
			div := op == "/"
			return cexpr{t: tTime, eval: func(fr *frame) Value {
				f := n(fr).F
				if div {
					if f == 0 {
						fail(pos, "division by zero")
					}
					return Value{I: int64(math.Round(float64(tm(fr).I) / f))}
				}
				return Value{I: int64(math.Round(float64(tm(fr).I) * f))}
			}}
		}
		return mismatch()
	}
	t := common(x.t, y.t)
	if t == nil || !(t.isNum() || t.isBits() && (op == "+" || op == "-")) {
		if x.t.isString() || y.t.isString() {
			c.errAt(pos, "strings are joined with CONCAT(a, b), not with %s", op)
			return bad()
		}
		return mismatch()
	}
	if op == "MOD" && t.isReal() {
		c.errAt(pos, "MOD is for integers, not %s", t)
		return bad()
	}
	gx, gy = c.convertFn(x, t), c.convertFn(y, t)
	if t.isReal() {
		var f func(a, b float64) float64
		switch op {
		case "+":
			f = func(a, b float64) float64 { return a + b }
		case "-":
			f = func(a, b float64) float64 { return a - b }
		case "*":
			f = func(a, b float64) float64 { return a * b }
		default:
			f = func(a, b float64) float64 { return a / b }
		}
		return cexpr{t: t, eval: func(fr *frame) Value { return store(t, Value{F: f(gx(fr).F, gy(fr).F)}) }}
	}
	unsigned := t.isUnsigned()
	var f func(a, b int64) int64
	switch op {
	case "+":
		f = func(a, b int64) int64 { return a + b }
	case "-":
		f = func(a, b int64) int64 { return a - b }
	case "*":
		f = func(a, b int64) int64 { return a * b }
	case "/":
		f = func(a, b int64) int64 {
			if b == 0 {
				fail(pos, "division by zero")
			}
			if unsigned {
				return int64(uint64(a) / uint64(b))
			}
			return a / b
		}
	default:
		f = func(a, b int64) int64 {
			if b == 0 {
				fail(pos, "division by zero in MOD")
			}
			if unsigned {
				return int64(uint64(a) % uint64(b))
			}
			return a % b
		}
	}
	return cexpr{t: t, eval: func(fr *frame) Value { return Value{I: t.wrap(f(gx(fr).I, gy(fr).I))} }}
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// convertFn gives an operand's value in a type it widens to.
func (c *compiler) convertFn(x cexpr, to *Type) func(*frame) Value {
	if same(x.t, to) {
		return x.eval
	}
	from, get := x.t, x.eval
	return func(fr *frame) Value { return convertValue(get(fr), from, to) }
}

// convert makes an expression of a type it can be implicitly converted to: a literal
// that fits, or a type that widens to it, as IEC 61131-3 allows. doing names the use in
// messages, such as "assign".
func (c *compiler) convert(x cexpr, to *Type, p Pos, doing string) cexpr {
	if x.bad || to == nil {
		return x
	}
	if same(x.t, to) {
		if to.isString() && x.konst != nil && len([]rune(x.konst.S)) > to.Len {
			c.warnAt(p, "the string is longer than the %d characters of %s, and is cut off", to.Len, to)
		}
		return x
	}
	if x.konst != nil && x.untyped {
		switch {
		case (to.isInt() || to.isBits()) && (x.t.isInt() || x.t.isBits()):
			if to.fits(x.konst.I) {
				return konst(to, Value{I: x.konst.I})
			}
			c.errAt(p, "%d does not fit into %s", x.konst.I, to)
			return bad()
		case to.isReal() && x.t.isNum():
			return konst(to, store(to, convertValue(*x.konst, x.t, tLReal)))
		}
	}
	if x.konst != nil && x.t.Kind == KInt && to.Kind == KBool {
		c.errAt(p, "use TRUE or FALSE for a BOOL, not a number")
		return bad()
	}
	if widensTo(x.t, to) {
		get, from := x.eval, x.t
		y := cexpr{t: to, eval: func(fr *frame) Value { return convertValue(get(fr), from, to) }}
		if x.konst != nil {
			k := convertValue(*x.konst, from, to)
			y.konst = &k
		}
		return y
	}
	hint := ""
	if x.t.Kind < KArray && to.Kind < KArray {
		hint = ": convert it with " + x.t.Name + "_TO_" + to.Name
	}
	c.errAt(p, "cannot %s %s to %s%s", doing, x.t, to, hint)
	return bad()
}

// store brings a value into its type, as storing it in a variable does: a REAL loses the
// precision of an LREAL.
func store(t *Type, v Value) Value {
	if t.Kind == KReal {
		v.F = float64(float32(v.F))
	}
	return v
}

// convertValue converts a value between elementary types, as the conversion functions do:
// integers wrap, reals round to the nearest integer, ties to even.
func convertValue(v Value, from, to *Type) Value {
	if from == to {
		return v
	}
	f := from
	if f.Kind == KEnum {
		f = f.Base
	}
	switch {
	case to.Kind == KBool:
		switch {
		case f.isReal():
			return Value{I: b2i(v.F != 0)}
		case f.isString():
			return Value{I: b2i(strings.EqualFold(strings.TrimSpace(v.S), "TRUE") || strings.TrimSpace(v.S) == "1")}
		}
		return Value{I: b2i(v.I != 0)}
	case to.isInt() || to.isBits() || to.Kind == KTime || to.Kind == KEnum:
		w := to
		if to.Kind == KEnum {
			w = to.Base
		}
		var n int64
		switch {
		case f.isReal():
			r := math.RoundToEven(v.F)
			switch {
			case math.IsNaN(r):
				n = 0
			case r >= math.MaxInt64:
				n = math.MaxInt64
			case r <= math.MinInt64:
				n = math.MinInt64
			default:
				n = int64(r)
			}
		case f.isString():
			s := strings.TrimSpace(v.S)
			if to.Kind == KTime {
				ms, _ := parseTime(strings.TrimPrefix(strings.TrimPrefix(strings.ToUpper(s), "TIME#"), "T#"))
				n = ms
			} else if i, ok := parseInt(s); ok {
				n = i
			} else if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				n = i
			}
		default:
			n = v.I
		}
		if w.Kind == KTime {
			return Value{I: n}
		}
		return Value{I: w.wrap(n)}
	case to.isReal():
		switch {
		case f.isReal():
			return store(to, v)
		case f.isString():
			x, _ := strconv.ParseFloat(strings.TrimSpace(v.S), 64)
			return store(to, Value{F: x})
		case f.isUnsigned():
			return store(to, Value{F: float64(uint64(v.I))})
		}
		return store(to, Value{F: float64(v.I)})
	case to.isString():
		if f.isString() {
			return v
		}
		s := formatValue(f, v)
		if f.isBits() {
			s = strconv.FormatUint(uint64(v.I), 10)
		}
		return Value{S: s}
	}
	return v
}
