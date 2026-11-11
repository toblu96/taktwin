package st

import (
	"fmt"
	"strings"
)

// Var is a declared variable: of a POU, a global, or a field of an instance.
type Var struct {
	Name     string
	Pos      Pos
	Type     *Type
	Kind     VarKind
	Constant bool
	Retain   bool
	At       string
	Init     *Init
	Slot     int
	Owner    *POU // nil for a global
	Comment  string
}

// Init makes the initial value of a variable or a type.
type Init struct {
	make func() Value
}

// POU is a compiled program, function block or function.
type POU struct {
	Name   string
	Kind   POUKind
	Decl   *POUDecl
	Vars   []*Var
	byName map[string]*Var
	// Params are the inputs, outputs and in-outs, in declaration order, for calls.
	Params []*Var
	// Result is the variable a function's value is kept in, named after it.
	Result *Var
	temps  []*Var
	body   func(fr *frame) ctl
	// native runs a standard function block written in Go instead of a body.
	native func(rt *Runtime, a *Agg)
	// compiling guards against a body that is being compiled, for recursion.
	compiling, compiled bool
}

// newInstance makes the variables of an instance, at their initial values.
func (p *POU) newInstance() Value {
	a := &Agg{Elems: make([]Value, len(p.Vars))}
	for i, v := range p.Vars {
		a.Elems[i] = initial(v.Type, v.Init)
	}
	return Value{A: a}
}

// initial is a variable's value at a start: its declared initial value, or its type's.
func initial(t *Type, init *Init) Value {
	if init != nil {
		return init.make()
	}
	if t.Init != nil {
		return t.Init.make()
	}
	return zero(t)
}

func (p *POU) lookup(name string) *Var { return p.byName[strings.ToUpper(name)] }

// ctl is how a statement ends: normally, or leaving a loop or the POU.
type ctl int

const (
	ctlNext ctl = iota
	ctlExit
	ctlContinue
	ctlReturn
)

// frame is where a body runs: the runtime, and the variables of the instance or call.
type frame struct {
	rt *Runtime
	a  *Agg
}

type compiler struct {
	prog  *Program
	diags []Diagnostic
	// dataTypes are the declared data types, resolved as they are first used.
	dataTypes map[string]*DataType
	resolving map[string]bool
	// pou is the POU whose body is being compiled, and loops how deep in loops it is.
	pou   *POU
	loops int
}

func newCompiler() *compiler {
	c := &compiler{
		prog: &Program{pous: map[string]*POU{}, types: map[string]*Type{}, global: map[string]*Var{}, lists: map[string]map[string]*Var{}},
		dataTypes: map[string]*DataType{}, resolving: map[string]bool{},
	}
	for _, fb := range standardFBs() {
		c.prog.pous[fb.Name] = fb
	}
	return c
}

func (c *compiler) errAt(p Pos, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{Pos: p, Severity: SevError, Msg: fmt.Sprintf(format, args...)})
}

func (c *compiler) warnAt(p Pos, format string, args ...any) {
	c.diags = append(c.diags, Diagnostic{Pos: p, Severity: SevWarning, Msg: fmt.Sprintf(format, args...)})
}

// declare registers every type, POU and global variable of the files, so that bodies can
// use them in any order.
func (c *compiler) declare(files []*File) {
	type gvl struct {
		file string
		d    *GlobalDecl
	}
	var globals []gvl
	var pous []*POUDecl
	taken := map[string]Pos{}
	claim := func(name string, p Pos) bool {
		up := strings.ToUpper(name)
		if prev, ok := taken[up]; ok {
			c.errAt(p, "%s is declared twice; the other one is at %s", name, prev)
			return false
		}
		if _, std := c.prog.pous[up]; std {
			c.errAt(p, "%s is a standard function block and cannot be declared again", name)
			return false
		}
		if _, ok := elementary[up]; ok || isStdFunction(up) {
			c.errAt(p, "%s is a standard type or function and cannot be a name", name)
			return false
		}
		taken[up] = p
		return true
	}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *TypeDecl:
				for _, dt := range d.Types {
					if claim(dt.Name, dt.Pos) {
						c.dataTypes[strings.ToUpper(dt.Name)] = dt
					}
				}
			case *POUDecl:
				if claim(d.Name, d.Pos) {
					pous = append(pous, d)
				}
			case *GlobalDecl:
				globals = append(globals, gvl{f.Name, d})
			}
		}
	}
	// POUs first by name, so that their types are known to variables.
	for _, d := range pous {
		p := &POU{Name: d.Name, Kind: d.Kind, Decl: d, byName: map[string]*Var{}}
		c.prog.pous[strings.ToUpper(d.Name)] = p
		c.prog.order = append(c.prog.order, p)
	}
	for name := range c.dataTypes {
		c.namedType(name, Pos{})
	}
	// Global variable lists, named after their files.
	for _, g := range globals {
		list := listName(g.file)
		if c.prog.lists[strings.ToUpper(list)] == nil {
			c.prog.lists[strings.ToUpper(list)] = map[string]*Var{}
		}
		for _, d := range g.d.Vars.Vars {
			t := c.typeOf(d.Type)
			for i, name := range d.Names {
				up := strings.ToUpper(name)
				if prev, ok := c.prog.global[up]; ok {
					c.errAt(d.NamePos[i], "the global variable %s is declared twice; the other one is at %s", name, prev.Pos)
					continue
				}
				if _, ok := taken[up]; ok {
					c.errAt(d.NamePos[i], "%s is already the name of a POU or type", name)
					continue
				}
				v := &Var{Name: name, Pos: d.NamePos[i], Type: t, Kind: VarGlobal, Constant: g.d.Vars.Constant, Retain: g.d.Vars.Retain, At: d.At, Slot: len(c.prog.globals), Comment: d.Comment}
				c.prog.globals = append(c.prog.globals, v)
				c.prog.global[up] = v
				c.prog.lists[strings.ToUpper(list)][up] = v
				c.bindAddress(v, d, nil)
			}
		}
	}
	// Initial values of globals, which may use constants declared before them.
	for _, g := range globals {
		for _, d := range g.d.Vars.Vars {
			for _, name := range d.Names {
				if v := c.prog.global[strings.ToUpper(name)]; v != nil && v.Init == nil && v.Type != nil && d.Init != nil {
					v.Init = c.initOf(v.Type, d.Init, nil)
				}
			}
		}
	}
	for _, p := range c.prog.order {
		c.declareVars(p)
	}
}

// declareVars sets up the variables of a POU, its parameters and its result.
func (c *compiler) declareVars(p *POU) {
	d := p.Decl
	add := func(v *Var) {
		up := strings.ToUpper(v.Name)
		if prev, ok := p.byName[up]; ok {
			c.errAt(v.Pos, "%s is declared twice in %s; the other one is at %s", v.Name, p.Name, prev.Pos)
			return
		}
		v.Slot, v.Owner = len(p.Vars), p
		p.Vars = append(p.Vars, v)
		p.byName[up] = v
	}
	if p.Kind == KindFunction {
		if d.Result == nil {
			c.errAt(d.Pos, "the function %s has no type: write FUNCTION %s : INT, for example", d.Name, d.Name)
		} else {
			p.Result = &Var{Name: d.Name, Pos: d.Pos, Type: c.typeOf(d.Result), Kind: VarOutput}
			add(p.Result)
		}
	}
	for _, b := range d.Vars {
		switch {
		case b.Kind == VarGlobal:
			c.errAt(b.Pos, "VAR_GLOBAL belongs in a global variable list, not in %s: use VAR_EXTERNAL here", p.Name)
			continue
		case b.Kind == VarInOut && p.Kind == KindProgram:
			c.warnAt(b.Pos, "a program's VAR_IN_OUT can only be given when another POU calls it")
		case (b.Kind == VarInput || b.Kind == VarOutput || b.Kind == VarInOut) && b.Constant:
			c.errAt(b.Pos, "%s cannot be CONSTANT", b.Kind)
		}
		for _, vd := range b.Vars {
			t := c.typeOf(vd.Type)
			for i, name := range vd.Names {
				v := &Var{Name: name, Pos: vd.NamePos[i], Type: t, Kind: b.Kind, Constant: b.Constant, Retain: b.Retain, At: vd.At, Comment: vd.Comment}
				if b.Kind == VarExternal {
					g := c.prog.global[strings.ToUpper(name)]
					if g == nil {
						c.errAt(v.Pos, "VAR_EXTERNAL %s: there is no global variable of that name", name)
						continue
					}
					if t != nil && g.Type != nil && !same(t, g.Type) {
						c.errAt(v.Pos, "VAR_EXTERNAL %s is %s here, but the global is %s", name, t, g.Type)
					}
					// It stands for the global, and is not a variable of its own.
					p.byName[strings.ToUpper(name)] = g
					continue
				}
				if t != nil && t.Kind == KFB && p.Kind == KindFunction {
					c.errAt(v.Pos, "a function cannot hold a function block instance, as it keeps nothing from one call to the next: make %s a FUNCTION_BLOCK", p.Name)
				}
				add(v)
				if b.Kind == VarInput || b.Kind == VarOutput || b.Kind == VarInOut {
					p.Params = append(p.Params, v)
				}
				if b.Kind == VarTemp {
					p.temps = append(p.temps, v)
				}
				if vd.At != "" {
					if p.Kind != KindProgram {
						c.errAt(v.Pos, "only a program's or a global variable can be at an address, not one of a %s", p.Kind)
					} else {
						c.bindAddress(v, vd, p)
					}
				}
			}
		}
	}
	// Initial values, once all variables are known.
	for _, b := range d.Vars {
		for _, vd := range b.Vars {
			if vd.Init == nil {
				continue
			}
			for _, name := range vd.Names {
				if v := p.byName[strings.ToUpper(name)]; v != nil && v.Owner == p && v.Type != nil {
					if v.Kind == VarInOut {
						c.errAt(v.Pos, "a VAR_IN_OUT has no initial value, as it stands for the caller's variable")
						continue
					}
					v.Init = c.initOf(v.Type, vd.Init, p)
				}
			}
		}
	}
}

// bindAddress connects a variable at a direct address to the process image.
func (c *compiler) bindAddress(v *Var, d *VarDecl, p *POU) {
	if d.At == "" || v.Type == nil {
		return
	}
	input, byteN, bit, ok := parseAddress(d.At)
	if !ok {
		c.errAt(v.Pos, "%s is no address of a bit: write %%IX0.0 for an input or %%QX0.0 for an output", d.At)
		return
	}
	if v.Type.Kind != KBool {
		c.errAt(v.Pos, "%s at %s is %s, but a bit of the image is a BOOL", v.Name, d.At, v.Type)
		return
	}
	b := ioBit{Byte: byteN, Bit: bit, global: v, slot: v.Slot}
	if p != nil {
		b.global, b.pou = nil, p
	}
	if input {
		c.prog.inputs = append(c.prog.inputs, b)
	} else {
		c.prog.outputs = append(c.prog.outputs, b)
	}
}

// parseAddress reads %IXn.m or %QXn.m.
func parseAddress(a string) (input bool, byteN, bit int, ok bool) {
	a = strings.ToUpper(a)
	switch {
	case strings.HasPrefix(a, "%IX"):
		input = true
	case strings.HasPrefix(a, "%QX"):
	default:
		return false, 0, 0, false
	}
	if _, err := fmt.Sscanf(a[3:], "%d.%d", &byteN, &bit); err != nil || bit < 0 || bit > 7 || byteN < 0 {
		return false, 0, 0, false
	}
	return input, byteN, bit, true
}

// typeOf resolves a type as written. It gives nil after reporting why it cannot.
func (c *compiler) typeOf(r *TypeRef) *Type {
	if r == nil {
		return nil
	}
	if r.Elem != nil {
		elem := c.typeOf(r.Elem)
		if elem == nil {
			return nil
		}
		t := &Type{Kind: KArray, Name: "ARRAY", Elem: elem}
		for _, d := range r.Dims {
			lo, ok1 := c.constInt(d.Lo)
			hi, ok2 := c.constInt(d.Hi)
			if !ok1 || !ok2 {
				return nil
			}
			if hi < lo {
				c.errAt(r.Pos, "the array goes from %d down to %d: the lower bound comes first", lo, hi)
				return nil
			}
			t.Dims = append(t.Dims, Dim{lo, hi})
		}
		return t
	}
	up := strings.ToUpper(r.Name)
	if up == "STRING" || up == "WSTRING" {
		base := tStr
		if up == "WSTRING" {
			base = tWStr
		}
		if r.Length == nil {
			return base
		}
		n, ok := c.constInt(r.Length)
		if !ok {
			return nil
		}
		if n < 1 || n > 65535 {
			c.errAt(r.Pos, "a string holds 1 to 65535 characters, not %d", n)
			return nil
		}
		return &Type{Kind: base.Kind, Name: base.Name, Len: int(n)}
	}
	return c.namedType(up, r.Pos)
}

// namedType resolves a type by its upper-case name: elementary, declared, or a function block.
func (c *compiler) namedType(up string, p Pos) *Type {
	if t, ok := elementary[up]; ok {
		return t
	}
	if t, ok := c.prog.types[up]; ok {
		return t
	}
	if pou, ok := c.prog.pous[up]; ok {
		if pou.Kind != KindFunctionBlock {
			c.errAt(p, "%s is a %s, which is no type: only a function block has instances", pou.Name, pou.Kind)
			return nil
		}
		t := &Type{Kind: KFB, Name: pou.Name, FB: pou}
		c.prog.types[up] = t
		return t
	}
	switch up {
	case "DATE", "TIME_OF_DAY", "TOD", "DATE_AND_TIME", "DT", "LTIME", "LDATE", "LTOD", "LDT", "CHAR", "WCHAR":
		c.errAt(p, "the type %s is not supported yet", up)
		return nil
	}
	dt, ok := c.dataTypes[up]
	if !ok {
		c.errAt(p, "there is no type %s", up)
		return nil
	}
	if c.resolving[up] {
		c.errAt(dt.Pos, "the type %s contains itself", dt.Name)
		return nil
	}
	c.resolving[up] = true
	defer delete(c.resolving, up)
	var t *Type
	switch {
	case dt.Struct != nil:
		t = &Type{Kind: KStruct, Name: dt.Name}
		c.prog.types[up] = t
		seen := map[string]bool{}
		for _, vd := range dt.Struct {
			ft := c.typeOf(vd.Type)
			for i, name := range vd.Names {
				if seen[strings.ToUpper(name)] {
					c.errAt(vd.NamePos[i], "the field %s is declared twice", name)
					continue
				}
				seen[strings.ToUpper(name)] = true
				f := &Field{Name: name, Type: ft}
				if vd.Init != nil && ft != nil {
					f.Init = c.initOf(ft, vd.Init, nil)
				}
				t.Fields = append(t.Fields, f)
			}
		}
	case dt.Enum != nil:
		t = &Type{Kind: KEnum, Name: dt.Name, Base: tInt}
		if dt.EnumOf != nil {
			if b := c.typeOf(dt.EnumOf); b != nil {
				if !b.isInt() && !b.isBits() {
					c.errAt(dt.EnumOf.Pos, "an enumeration is kept as an integer, not as %s", b)
				} else {
					t.Base = b
				}
			}
		}
		c.prog.types[up] = t
		next := int64(0)
		seen := map[string]bool{}
		for _, ev := range dt.Enum {
			if ev.Value != nil {
				if v, ok := c.constInt(ev.Value); ok {
					next = v
				}
			}
			if seen[strings.ToUpper(ev.Name)] {
				c.errAt(ev.Pos, "the value %s is declared twice", ev.Name)
			}
			seen[strings.ToUpper(ev.Name)] = true
			t.Values = append(t.Values, EnumConst{Name: ev.Name, Value: next})
			next++
		}
	default:
		base := c.typeOf(dt.Alias)
		if base == nil {
			return nil
		}
		// A new name for a type is that type, with its own initial value.
		cp := *base
		cp.Name = dt.Name
		if base.Kind < KArray {
			cp.Name = base.Name
		}
		t = &cp
		c.prog.types[up] = t
	}
	if dt.Init != nil {
		t.Init = c.initOf(t, dt.Init, nil)
	}
	return t
}

// constInt evaluates an integer constant expression, such as an array bound.
func (c *compiler) constInt(e Expr) (int64, bool) {
	x := c.expr(e, tDInt)
	if x.bad {
		return 0, false
	}
	if x.konst == nil {
		c.errAt(e.exprPos(), "%s is not a constant: a bound or label must be known before the program runs", exprText(e))
		return 0, false
	}
	if !x.t.isInt() && !x.t.isBits() && x.t.Kind != KEnum {
		c.errAt(e.exprPos(), "%s is %s, but an integer is needed here", exprText(e), x.t)
		return 0, false
	}
	return x.konst.I, true
}

// initOf compiles an initial value: a constant expression, an array or a structure.
func (c *compiler) initOf(t *Type, e Expr, p *POU) *Init {
	switch e := e.(type) {
	case *ArrayInit:
		if t.Kind != KArray {
			c.errAt(e.Pos, "[ … ] initialises an array, but this is %s", t)
			return nil
		}
		n := len(zero(t).A.Elems)
		var items []func() Value
		for _, it := range e.Items {
			count := int64(1)
			if it.Count != nil {
				var ok bool
				if count, ok = c.constInt(it.Count); !ok {
					return nil
				}
			}
			var one func() Value
			if it.Value == nil {
				one = func() Value { return initial(t.Elem, nil) }
			} else {
				in := c.initOf(t.Elem, it.Value, p)
				if in == nil {
					return nil
				}
				one = in.make
			}
			for range count {
				items = append(items, one)
			}
		}
		if len(items) > n {
			c.errAt(e.Pos, "%d values for an array of %d elements", len(items), n)
			return nil
		}
		return &Init{make: func() Value {
			v := zero(t)
			for i, f := range items {
				v.A.Elems[i] = f()
			}
			for i := len(items); i < n; i++ {
				v.A.Elems[i] = initial(t.Elem, nil)
			}
			return v
		}}
	case *StructInit:
		var fields []string
		var types []*Type
		var inits []*Init
		switch t.Kind {
		case KStruct:
			for _, f := range t.Fields {
				fields, types, inits = append(fields, f.Name), append(types, f.Type), append(inits, f.Init)
			}
		case KFB:
			for _, v := range t.FB.Vars {
				fields, types, inits = append(fields, v.Name), append(types, v.Type), append(inits, v.Init)
			}
		default:
			c.errAt(e.Pos, "( name := … ) initialises a structure or an instance, but this is %s", t)
			return nil
		}
		given := map[int]*Init{}
		for _, f := range e.Fields {
			i := -1
			for k, name := range fields {
				if strings.EqualFold(name, f.Name) {
					i = k
				}
			}
			if i < 0 {
				c.errAt(f.Pos, "%s has no %s", t, f.Name)
				continue
			}
			if in := c.initOf(types[i], f.Value, p); in != nil {
				given[i] = in
			}
		}
		return &Init{make: func() Value {
			v := zero(t)
			for i := range fields {
				if in, ok := given[i]; ok {
					v.A.Elems[i] = in.make()
				} else if types[i] != nil {
					v.A.Elems[i] = initial(types[i], inits[i])
				}
			}
			return v
		}}
	}
	saved := c.pou
	c.pou = p
	x := c.expr(e, t)
	c.pou = saved
	if x.bad {
		return nil
	}
	x = c.convert(x, t, e.exprPos(), "initialise")
	if x.bad {
		return nil
	}
	if x.konst == nil {
		c.errAt(e.exprPos(), "%s is not a constant: an initial value must be known before the program runs", exprText(e))
		return nil
	}
	v := *x.konst
	return &Init{make: func() Value { return v.clone() }}
}

// compileBodies compiles the statements of every POU.
func (c *compiler) compileBodies() {
	for _, p := range c.prog.order {
		c.compilePOU(p)
	}
}

func (c *compiler) compilePOU(p *POU) {
	if p.compiled || p.compiling || p.native != nil {
		return
	}
	p.compiling = true
	saved, savedLoops := c.pou, c.loops
	c.pou, c.loops = p, 0
	body := c.stmts(p.Decl.Body)
	c.pou, c.loops = saved, savedLoops
	temps := p.temps
	p.body = func(fr *frame) ctl {
		for _, v := range temps {
			fr.a.Elems[v.Slot] = initial(v.Type, v.Init)
		}
		body(fr)
		return ctlNext
	}
	p.compiling, p.compiled = false, true
}
