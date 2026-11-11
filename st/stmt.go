package st

import (
	"strconv"
	"strings"
)

// stmts compiles a list of statements into one function that runs them in order.
func (c *compiler) stmts(list []Stmt) func(fr *frame) ctl {
	var fns []func(fr *frame) ctl
	for _, s := range list {
		if f := c.stmt(s); f != nil {
			fns = append(fns, f)
		}
	}
	return func(fr *frame) ctl {
		for _, f := range fns {
			if r := f(fr); r != ctlNext {
				return r
			}
		}
		return ctlNext
	}
}

func (c *compiler) stmt(s Stmt) func(fr *frame) ctl {
	pos := s.stmtPos()
	tick := func(fr *frame) {
		if fr.rt.budget--; fr.rt.budget < 0 {
			fail(pos, "the watchdog stopped the program: a cycle ran too long, maybe in an endless loop")
		}
	}
	switch s := s.(type) {
	case *EmptyStmt:
		return nil
	case *AssignStmt:
		set := c.assignment(s.Target, s.Value, s.Pos)
		if set == nil {
			return nil
		}
		return func(fr *frame) ctl { tick(fr); set(fr); return ctlNext }
	case *CallStmt:
		call := c.callStmt(s.Call)
		if call == nil {
			return nil
		}
		return func(fr *frame) ctl { tick(fr); call(fr); return ctlNext }
	case *IfStmt:
		var conds []func(*frame) Value
		var thens []func(*frame) ctl
		for i, e := range s.Conds {
			conds = append(conds, c.condition(e, "IF"))
			thens = append(thens, c.stmts(s.Thens[i]))
		}
		els := c.stmts(s.Else)
		return func(fr *frame) ctl {
			tick(fr)
			for i, cond := range conds {
				if cond(fr).I != 0 {
					return thens[i](fr)
				}
			}
			return els(fr)
		}
	case *CaseStmt:
		return c.caseStmt(s, tick)
	case *ForStmt:
		return c.forStmt(s, tick)
	case *WhileStmt:
		cond := c.condition(s.Cond, "WHILE")
		c.loops++
		body := c.stmts(s.Body)
		c.loops--
		return func(fr *frame) ctl {
			for {
				tick(fr)
				if cond(fr).I == 0 {
					return ctlNext
				}
				switch body(fr) {
				case ctlExit:
					return ctlNext
				case ctlReturn:
					return ctlReturn
				}
			}
		}
	case *RepeatStmt:
		c.loops++
		body := c.stmts(s.Body)
		c.loops--
		cond := c.condition(s.Cond, "UNTIL")
		return func(fr *frame) ctl {
			for {
				tick(fr)
				switch body(fr) {
				case ctlExit:
					return ctlNext
				case ctlReturn:
					return ctlReturn
				}
				if cond(fr).I != 0 {
					return ctlNext
				}
			}
		}
	case *ExitStmt:
		if c.loops == 0 {
			c.errAt(pos, "EXIT is only allowed inside FOR, WHILE or REPEAT")
		}
		return func(*frame) ctl { return ctlExit }
	case *ContinueStmt:
		if c.loops == 0 {
			c.errAt(pos, "CONTINUE is only allowed inside FOR, WHILE or REPEAT")
		}
		return func(*frame) ctl { return ctlContinue }
	case *ReturnStmt:
		return func(*frame) ctl { return ctlReturn }
	}
	c.errAt(pos, "this statement is not supported")
	return nil
}

// condition compiles the condition of IF, WHILE or UNTIL, which is a BOOL.
func (c *compiler) condition(e Expr, of string) func(*frame) Value {
	x := c.value(c.expr(e, tBool), e.exprPos())
	if x.bad {
		return func(*frame) Value { return Value{} }
	}
	if x.t.Kind != KBool {
		c.errAt(e.exprPos(), "the condition of %s is %s, but must be a BOOL: compare it, as x <> 0", of, x.t)
		return func(*frame) Value { return Value{} }
	}
	return x.eval
}

// target compiles the left side of an assignment, or a variable given to => or VAR_IN_OUT.
func (c *compiler) target(e Expr, p Pos) (cexpr, bool) {
	x := c.expr(e, nil)
	if x.bad {
		return x, false
	}
	if x.lval == nil && x.setBit == nil {
		if x.konst != nil && x.t != nil {
			c.errAt(p, "%s is a constant and cannot be assigned", exprText(e))
		} else {
			c.errAt(p, "%s cannot be assigned", x.whatOr(exprText(e)))
		}
		return x, false
	}
	if x.readonly {
		c.errAt(p, "%s cannot be written here: it is a constant, an input address, or an output of an instance seen from outside", exprText(e))
		return x, false
	}
	if x.t != nil && x.t.Kind == KFB {
		c.errAt(p, "an instance of %s cannot be assigned: set its inputs in a call, as %s(IN := …)", x.t.Name, exprText(e))
		return x, false
	}
	return x, true
}

func (x cexpr) whatOr(s string) string {
	if x.what != "" {
		return x.what
	}
	return s
}

// assignment compiles target := value.
func (c *compiler) assignment(target, value Expr, p Pos) func(fr *frame) {
	t, ok := c.target(target, p)
	if !ok {
		c.expr(value, nil) // still report mistakes on the right
		return nil
	}
	v := c.value(c.expr(value, t.t), value.exprPos())
	if v.bad {
		return nil
	}
	v = c.convert(v, t.t, value.exprPos(), "assign")
	if v.bad {
		return nil
	}
	get := v.eval
	if t.setBit != nil {
		set := t.setBit
		return func(fr *frame) { set(fr, get(fr).I != 0) }
	}
	put := c.putter(t.t)
	lv := t.lval
	return func(fr *frame) { put(lv(fr), get(fr)) }
}

// putter stores a value in a variable of a type: a copy of an aggregate, a string cut to
// its length, a REAL at its precision.
func (c *compiler) putter(t *Type) func(dst *Value, v Value) {
	switch {
	case t.Kind == KArray || t.Kind == KStruct:
		return func(dst *Value, v Value) { *dst = v.clone() }
	case t.isString():
		n := t.Len
		return func(dst *Value, v Value) {
			if r := []rune(v.S); len(r) > n {
				v.S = string(r[:n])
			}
			*dst = v
		}
	case t.Kind == KReal:
		return func(dst *Value, v Value) { *dst = Value{F: float64(float32(v.F))} }
	}
	return func(dst *Value, v Value) { *dst = v }
}

func (c *compiler) caseStmt(s *CaseStmt, tick func(*frame)) func(fr *frame) ctl {
	sel := c.value(c.expr(s.Value, nil), s.Value.exprPos())
	if !sel.bad && !(sel.t.isInt() || sel.t.isBits() || sel.t.Kind == KEnum) {
		c.errAt(s.Value.exprPos(), "CASE selects by an integer or an enumeration, not by %s", sel.t)
		sel = bad()
	}
	type label struct {
		lo, hi int64
		body   int
	}
	var labels []label
	var bodies []func(*frame) ctl
	for _, cl := range s.Clauses {
		for _, r := range cl.Labels {
			lo, ok1 := c.caseLabel(r.Lo, sel)
			hi, ok2 := lo, true
			if r.Hi != nil {
				hi, ok2 = c.caseLabel(r.Hi, sel)
			}
			if !ok1 || !ok2 {
				continue
			}
			if hi < lo {
				c.errAt(r.Lo.exprPos(), "the range %d..%d is empty: the lower value comes first", lo, hi)
				continue
			}
			for _, l := range labels {
				if lo <= l.hi && hi >= l.lo {
					c.errAt(r.Lo.exprPos(), "the value %d is already a label of this CASE", max(lo, l.lo))
					break
				}
			}
			labels = append(labels, label{lo, hi, len(bodies)})
		}
		bodies = append(bodies, c.stmts(cl.Body))
	}
	els := c.stmts(s.Else)
	if sel.bad {
		return nil
	}
	get := sel.eval
	unsigned := sel.t.isUnsigned()
	return func(fr *frame) ctl {
		tick(fr)
		v := get(fr).I
		for _, l := range labels {
			in := v >= l.lo && v <= l.hi
			if unsigned {
				in = uint64(v) >= uint64(l.lo) && uint64(v) <= uint64(l.hi)
			}
			if in {
				return bodies[l.body](fr)
			}
		}
		return els(fr)
	}
}

func (c *compiler) caseLabel(e Expr, sel cexpr) (int64, bool) {
	if sel.bad {
		c.expr(e, nil)
		return 0, false
	}
	x := c.expr(e, sel.t)
	if x.bad {
		return 0, false
	}
	x = c.convert(x, sel.t, e.exprPos(), "compare")
	if x.bad {
		return 0, false
	}
	if x.konst == nil {
		c.errAt(e.exprPos(), "a CASE label is a constant, but %s is not", exprText(e))
		return 0, false
	}
	return x.konst.I, true
}

func (c *compiler) forStmt(s *ForStmt, tick func(*frame)) func(fr *frame) ctl {
	v, ok := c.target(s.Var, s.Var.Pos)
	if ok && !v.t.isInt() {
		c.errAt(s.Var.Pos, "the variable of a FOR loop is an integer, not %s", v.t)
		ok = false
	}
	var from, to, by func(*frame) Value
	compile := func(e Expr) func(*frame) Value {
		if !ok {
			c.expr(e, nil)
			return nil
		}
		x := c.convert(c.value(c.expr(e, v.t), e.exprPos()), v.t, e.exprPos(), "count")
		if x.bad {
			ok = false
			return nil
		}
		return x.eval
	}
	from, to = compile(s.From), compile(s.To)
	if s.By != nil {
		by = compile(s.By)
	}
	c.loops++
	body := c.stmts(s.Body)
	c.loops--
	if !ok {
		return nil
	}
	lv, t := v.lval, v.t
	pos := s.Pos
	return func(fr *frame) ctl {
		tick(fr)
		step := int64(1)
		if by != nil {
			if step = by(fr).I; step == 0 {
				fail(pos, "the FOR loop counts BY 0, and would never end")
			}
		}
		end := to(fr).I
		i := from(fr).I
		for {
			if step > 0 && i > end || step < 0 && i < end {
				return ctlNext
			}
			*lv(fr) = Value{I: i}
			tick(fr)
			switch body(fr) {
			case ctlExit:
				return ctlNext
			case ctlReturn:
				return ctlReturn
			}
			// The body may change the variable; the loop goes on from there, as a PLC's does.
			i = lv(fr).I + step
			if t.wrap(i) != i {
				return ctlNext // past the end of the type: no further round
			}
		}
	}
}

// callStmt compiles a call on its own: of an instance, a program, or a function.
func (c *compiler) callStmt(e *CallExpr) func(fr *frame) {
	x := c.expr(e.Func, nil)
	if x.bad {
		c.args(e)
		return nil
	}
	switch {
	case x.t != nil && x.t.Kind == KFB:
		if x.lval == nil {
			c.errAt(e.Pos, "%s cannot be called here", x.what)
			return nil
		}
		lv := x.lval
		return discard(c.invoke(x.t.FB, e, func(fr *frame) *Agg { return lv(fr).A }))
	case x.pou != nil && x.pou.Kind == KindProgram:
		p := x.pou
		if p == c.pou {
			c.errAt(e.Pos, "the program %s calls itself", p.Name)
			return nil
		}
		return discard(c.invoke(p, e, func(fr *frame) *Agg { return fr.rt.instance(p) }))
	case x.pou != nil && x.pou.Kind == KindFunctionBlock:
		c.errAt(e.Pos, "%s is a function block: declare an instance, as fb : %s;, and call that", x.pou.Name, x.pou.Name)
		return nil
	}
	call := c.callExpr(e)
	if call.bad {
		return nil
	}
	get := call.eval
	return func(fr *frame) { get(fr) }
}

// discard drops the variables a call gives back.
func discard(call func(fr *frame) *Agg) func(fr *frame) {
	if call == nil {
		return nil
	}
	return func(fr *frame) { call(fr) }
}

// args compiles a call's arguments just to report their mistakes.
func (c *compiler) args(e *CallExpr) {
	for _, a := range e.Args {
		if a.Value != nil {
			c.expr(a.Value, nil)
		}
	}
}

// invoke calls a program or function block instance, or a user function, whose variables
// inst gives: it sets the inputs and in-outs, runs the body, and hands the outputs on. It
// gives back the variables it ran on.
func (c *compiler) invoke(p *POU, e *CallExpr, inst func(fr *frame) *Agg) func(fr *frame) *Agg {
	type input struct {
		slot int
		get  func(*frame) Value
		put  func(*Value, Value)
	}
	type inout struct {
		slot int
		ref  func(*frame) *Value
	}
	type output struct {
		slot int
		from *Type
		to   cexpr
		not  bool
		put  func(*Value, Value)
	}
	var inputs []input
	var inouts []inout
	var outputs []output
	given := map[*Var]bool{}
	positional := 0
	for _, a := range e.Args {
		var v *Var
		if a.Name == "" {
			if p.Kind == KindFunctionBlock || p.Kind == KindProgram {
				c.errAt(a.Pos, "the inputs of %s are given by name, as %s(IN := …)", p.Name, exprText(e.Func))
				return nil
			}
			for positional < len(p.Params) && p.Params[positional].Kind == VarOutput {
				positional++
			}
			if positional >= len(p.Params) {
				c.errAt(a.Pos, "%s has only %d inputs", p.Name, positional)
				return nil
			}
			v = p.Params[positional]
			positional++
		} else {
			v = p.lookup(a.Name)
			if v == nil || v.Owner != p || v.Kind != VarInput && v.Kind != VarOutput && v.Kind != VarInOut {
				c.errAt(a.Pos, "%s has no input or output %s", p.Name, a.Name)
				continue
			}
		}
		if given[v] {
			c.errAt(a.Pos, "%s is given twice", v.Name)
			continue
		}
		given[v] = true
		if v.Type == nil {
			continue
		}
		switch {
		case a.Output:
			if v.Kind != VarOutput {
				c.errAt(a.Pos, "%s is an input of %s: give it with :=", v.Name, p.Name)
				continue
			}
			if a.Value == nil {
				continue
			}
			t, ok := c.target(a.Value, a.Pos)
			if !ok {
				continue
			}
			if !widensTo(v.Type, t.t) && !same(v.Type, t.t) {
				c.errAt(a.Pos, "the output %s is %s and cannot go into %s, which is %s", v.Name, v.Type, exprText(a.Value), t.t)
				continue
			}
			if a.Not && v.Type.Kind != KBool {
				c.errAt(a.Pos, "NOT only reads a BOOL output")
				continue
			}
			outputs = append(outputs, output{slot: v.Slot, from: v.Type, to: t, not: a.Not, put: c.putter(t.t)})
		case v.Kind == VarOutput:
			c.errAt(a.Pos, "%s is an output of %s: read it with =>, as %s => variable", v.Name, p.Name, v.Name)
		case v.Kind == VarInOut:
			t, ok := c.target(a.Value, a.Pos)
			if !ok || t.lval == nil {
				if ok {
					c.errAt(a.Pos, "the VAR_IN_OUT %s needs a variable, not a bit of one", v.Name)
				}
				continue
			}
			if !same(t.t, v.Type) {
				c.errAt(a.Pos, "the VAR_IN_OUT %s is %s, but %s is %s", v.Name, v.Type, exprText(a.Value), t.t)
				continue
			}
			inouts = append(inouts, inout{v.Slot, t.lval})
		default:
			x := c.value(c.expr(a.Value, v.Type), a.Value.exprPos())
			x = c.convert(x, v.Type, a.Value.exprPos(), "give")
			if x.bad {
				continue
			}
			inputs = append(inputs, input{v.Slot, x.eval, c.putter(v.Type)})
		}
	}
	for _, v := range p.Params {
		if v.Kind == VarInOut && !given[v] {
			c.errAt(e.Pos, "the call of %s does not give its VAR_IN_OUT %s", p.Name, v.Name)
		}
	}
	if p != c.pou {
		c.compilePOU(p)
	}
	return func(fr *frame) *Agg {
		a := inst(fr)
		for _, in := range inputs {
			in.put(&a.Elems[in.slot], in.get(fr))
		}
		if len(inouts) > 0 {
			if a.Refs == nil {
				a.Refs = map[int]*Value{}
			}
			for _, io := range inouts {
				a.Refs[io.slot] = io.ref(fr)
			}
		}
		run(fr.rt, p, a)
		for _, out := range outputs {
			v := convertValue(a.Elems[out.slot], out.from, out.to.t)
			if out.not {
				v.I = 1 - v.I
			}
			out.put(out.to.lval(fr), v)
		}
		return a
	}
}

// run runs a POU's body on its variables.
func run(rt *Runtime, p *POU, a *Agg) {
	if p.native != nil {
		p.native(rt, a)
		return
	}
	if p.body == nil {
		fail(p.Decl.Pos, p.Name+" calls itself, which a PLC program cannot")
	}
	rt.depth++
	defer func() { rt.depth-- }()
	if rt.depth > 64 {
		fail(p.Decl.Pos, "calls nest too deep in "+p.Name+": a POU calls itself, directly or through others")
	}
	p.body(&frame{rt: rt, a: a})
}

// callExpr compiles a function call that gives a value: of a standard function or a
// user's FUNCTION.
func (c *compiler) callExpr(e *CallExpr) cexpr {
	id, isName := e.Func.(*Ident)
	x := c.expr(e.Func, nil)
	if x.bad && !isName {
		return x
	}
	if isName && x.pou == nil && (x.t == nil || x.bad) {
		up := strings.ToUpper(id.Name)
		if isStdFunction(up) {
			return c.stdCall(up, e)
		}
		return bad()
	}
	switch {
	case x.pou != nil && x.pou.Kind == KindFunction:
		p := x.pou
		if p.Result == nil || p.Result.Type == nil {
			c.args(e)
			return bad()
		}
		if p == c.pou {
			c.errAt(e.Pos, "the function %s calls itself, which a PLC program cannot", p.Name)
			return bad()
		}
		call := c.invoke(p, e, func(fr *frame) *Agg { return p.newInstance().A })
		if call == nil {
			return bad()
		}
		slot, t := p.Result.Slot, p.Result.Type
		return cexpr{t: t, what: "the result of " + p.Name, eval: func(fr *frame) Value { return call(fr).Elems[slot] }}
	case x.pou != nil:
		c.errAt(e.Pos, "%s is called as a statement on its own, as %s(…);, and gives no value", x.what, x.pou.Name)
	case x.t != nil && x.t.Kind == KFB:
		c.errAt(e.Pos, "an instance is called as a statement on its own, as %s(…);, and its outputs read as %s.Q", exprText(e.Func), exprText(e.Func))
	default:
		c.errAt(e.Pos, "%s is no function", x.whatOr(exprText(e.Func)))
	}
	c.args(e)
	return bad()
}

// itoa is strconv.FormatInt for messages.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }
