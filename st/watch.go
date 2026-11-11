package st

import (
	"fmt"
	"strconv"
	"strings"
)

// WatchVar is a variable as a programming tool shows it online: with its value, and its
// fields, elements or variables below it.
type WatchVar struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Section  string     `json:"section,omitempty"`
	Value    string     `json:"value"`
	Forced   bool       `json:"forced,omitempty"`
	Writable bool       `json:"writable"`
	Children []WatchVar `json:"children,omitempty"`
}

// maxElements is how many elements of an array a watch shows.
const maxElements = 100

// Variables lists the variables of the running program: each global variable list and
// each program, with their variables.
func (rt *Runtime) Variables() []WatchVar {
	var out []WatchVar
	lists := map[string][]*Var{}
	var names []string
	for name, vars := range rt.prog.lists {
		for _, v := range vars {
			lists[name] = append(lists[name], v)
		}
		names = append(names, name)
	}
	sortStrings(names)
	for _, up := range names {
		vars := lists[up]
		sortVars(vars)
		node := WatchVar{Name: rt.listDisplayName(up), Path: rt.listDisplayName(up), Type: "GVL"}
		for _, v := range vars {
			node.Children = append(node.Children, rt.watch(node.Path+"."+v.Name, v.Name, v.Type, &rt.globals.Elems[v.Slot], v.Kind, !v.Constant))
		}
		out = append(out, node)
	}
	for _, p := range rt.prog.order {
		if p.Kind != KindProgram {
			continue
		}
		a := rt.instance(p)
		node := WatchVar{Name: p.Name, Path: p.Name, Type: "PROGRAM"}
		for _, v := range p.Vars {
			node.Children = append(node.Children, rt.watch(p.Name+"."+v.Name, v.Name, v.Type, &a.Elems[v.Slot], v.Kind, !v.Constant && v.Kind != VarInOut))
		}
		out = append(out, node)
	}
	return out
}

// listDisplayName is a global variable list's name as its file spells it.
func (rt *Runtime) listDisplayName(up string) string {
	for _, v := range rt.prog.globals {
		if strings.EqualFold(rt.listOf(v), up) {
			return rt.listOf(v)
		}
	}
	return up
}

func (rt *Runtime) listOf(v *Var) string {
	return listName(v.Pos.File)
}

func (rt *Runtime) watch(path, name string, t *Type, v *Value, kind VarKind, writable bool) WatchVar {
	w := WatchVar{Name: name, Path: path, Type: t.String(), Section: kind.String(), Value: formatValue(t, *v)}
	_, w.Forced = rt.forced[strings.ToUpper(path)]
	switch t.Kind {
	case KStruct:
		for i, f := range t.Fields {
			w.Children = append(w.Children, rt.watch(path+"."+f.Name, f.Name, f.Type, &v.A.Elems[i], kind, writable))
		}
	case KFB:
		for _, fv := range t.FB.Vars {
			if strings.HasPrefix(fv.Name, "_") {
				continue // the inner state of a standard block
			}
			w.Children = append(w.Children, rt.watch(path+"."+fv.Name, fv.Name, fv.Type, member(v.A, fv), fv.Kind, writable && fv.Kind != VarInOut))
		}
	case KArray:
		for i := range min(len(v.A.Elems), maxElements) {
			label := "[" + arrayIndex(t, i) + "]"
			w.Children = append(w.Children, rt.watch(path+label, label, t.Elem, &v.A.Elems[i], kind, writable))
		}
	default:
		w.Writable = writable
	}
	return w
}

// member is a variable of an instance; for a VAR_IN_OUT, the variable it stands for since
// the last call, as an online view shows it.
func member(a *Agg, v *Var) *Value {
	if v.Kind == VarInOut {
		if r := a.Refs[v.Slot]; r != nil {
			return r
		}
	}
	return &a.Elems[v.Slot]
}

// arrayIndex writes the indices of an array's i-th element, as 1, 2.
func arrayIndex(t *Type, i int) string {
	idx := make([]string, len(t.Dims))
	for d := len(t.Dims) - 1; d >= 0; d-- {
		n := int(t.Dims[d].Hi - t.Dims[d].Lo + 1)
		idx[d] = strconv.FormatInt(t.Dims[d].Lo+int64(i%n), 10)
		i /= n
	}
	return strings.Join(idx, ", ")
}

// Read gives a variable's value as a literal, by its path, as PLC_PRG.timer.ET.
func (rt *Runtime) Read(path string) (string, error) {
	t, v, err := rt.resolve(path)
	if err != nil {
		return "", err
	}
	return formatValue(t, *v), nil
}

// Write sets a variable once, by its path, to a value written as a literal, as a
// programming tool's "write values" does.
func (rt *Runtime) Write(path, value string) error {
	t, v, err := rt.resolve(path)
	if err != nil {
		return err
	}
	val, err := rt.parseValue(t, value)
	if err != nil {
		return err
	}
	*v = val
	return nil
}

// forcing is a value a variable is held at.
type forcing struct {
	path string
	v    Value
}

// Force holds a variable at a value, against the program and the inputs, until it is
// released.
func (rt *Runtime) Force(path, value string) error {
	t, v, err := rt.resolve(path)
	if err != nil {
		return err
	}
	val, err := rt.parseValue(t, value)
	if err != nil {
		return err
	}
	rt.forced[strings.ToUpper(path)] = forcing{path: path, v: val}
	*v = val
	return nil
}

// Release lets a forced variable go; "" lets all go.
func (rt *Runtime) Release(path string) {
	if path == "" {
		clear(rt.forced)
		return
	}
	delete(rt.forced, strings.ToUpper(path))
}

// Forced lists the paths of the forced variables.
func (rt *Runtime) Forced() []string {
	var out []string
	for _, f := range rt.forced {
		out = append(out, f.path)
	}
	sortStrings(out)
	return out
}

func (rt *Runtime) applyForces() {
	for _, f := range rt.forced {
		if _, v, err := rt.resolve(f.path); err == nil {
			*v = f.v.clone()
		}
	}
}

// resolve finds a variable by its path: a program or global variable list, then variables,
// fields and indices, as PLC_PRG.t.Q, GVL.speed or PLC_PRG.list[2].
func (rt *Runtime) resolve(path string) (*Type, *Value, error) {
	parts, err := splitPath(path)
	if err != nil {
		return nil, nil, err
	}
	head := strings.ToUpper(parts[0])
	var t *Type
	var v *Value
	rest := parts[1:]
	if p, ok := rt.prog.pous[head]; ok && p.Kind == KindProgram {
		if len(rest) == 0 {
			return nil, nil, fmt.Errorf("%s is a program; name one of its variables", parts[0])
		}
		pv := p.lookup(rest[0])
		if pv == nil || pv.Owner != p {
			return nil, nil, fmt.Errorf("the program %s has no variable %s", p.Name, rest[0])
		}
		if pv.Kind == VarInOut {
			return nil, nil, fmt.Errorf("%s is a VAR_IN_OUT and stands for a variable of the caller", rest[0])
		}
		t, v, rest = pv.Type, &rt.instance(p).Elems[pv.Slot], rest[1:]
	} else if list, ok := rt.prog.lists[head]; ok {
		if len(rest) == 0 {
			return nil, nil, fmt.Errorf("%s is a global variable list; name one of its variables", parts[0])
		}
		g := list[strings.ToUpper(rest[0])]
		if g == nil {
			return nil, nil, fmt.Errorf("%s has no variable %s", parts[0], rest[0])
		}
		t, v, rest = g.Type, &rt.globals.Elems[g.Slot], rest[1:]
	} else if g := rt.prog.global[head]; g != nil {
		t, v = g.Type, &rt.globals.Elems[g.Slot]
	} else {
		return nil, nil, fmt.Errorf("there is no program, global variable list or global variable %s", parts[0])
	}
	for _, part := range rest {
		switch {
		case strings.HasPrefix(part, "["):
			if t.Kind != KArray {
				return nil, nil, fmt.Errorf("%s is no array", path)
			}
			var idx []int64
			for _, s := range strings.Split(strings.Trim(part, "[]"), ",") {
				n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
				if err != nil {
					return nil, nil, fmt.Errorf("%s is no index", s)
				}
				idx = append(idx, n)
			}
			if len(idx) != len(t.Dims) {
				return nil, nil, fmt.Errorf("%s has %d dimensions", path, len(t.Dims))
			}
			flat, stride := int64(0), int64(1)
			for d := len(t.Dims) - 1; d >= 0; d-- {
				if idx[d] < t.Dims[d].Lo || idx[d] > t.Dims[d].Hi {
					return nil, nil, fmt.Errorf("%d is outside %d..%d", idx[d], t.Dims[d].Lo, t.Dims[d].Hi)
				}
				flat += (idx[d] - t.Dims[d].Lo) * stride
				stride *= t.Dims[d].Hi - t.Dims[d].Lo + 1
			}
			t, v = t.Elem, &v.A.Elems[flat]
		case t.Kind == KStruct:
			found := false
			for i, f := range t.Fields {
				if strings.EqualFold(f.Name, part) {
					t, v, found = f.Type, &v.A.Elems[i], true
					break
				}
			}
			if !found {
				return nil, nil, fmt.Errorf("%s has no field %s", t.Name, part)
			}
		case t.Kind == KFB:
			fv := t.FB.lookup(part)
			if fv == nil || fv.Owner != t.FB {
				return nil, nil, fmt.Errorf("%s has no variable %s", t.Name, part)
			}
			t, v = fv.Type, member(v.A, fv)
		default:
			return nil, nil, fmt.Errorf("%s has no %s", t, part)
		}
	}
	return t, v, nil
}

// splitPath splits a.b[1, 2].c into a, b, [1, 2], c.
func splitPath(path string) ([]string, error) {
	var out []string
	cur := ""
	for i := 0; i < len(path); i++ {
		switch ch := path[i]; ch {
		case '.':
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		case '[':
			if cur != "" {
				out = append(out, cur)
			}
			j := strings.IndexByte(path[i:], ']')
			if j < 0 {
				return nil, fmt.Errorf("%s: the [ is not closed", path)
			}
			out = append(out, path[i:i+j+1])
			i += j
			cur = ""
		default:
			cur += string(ch)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no variable named")
	}
	return out, nil
}

// parseValue reads a value written as a literal of a type, as TRUE, 42, T#2s or 'text'.
func (rt *Runtime) parseValue(t *Type, text string) (Value, error) {
	if t.isAggregate() {
		return Value{}, fmt.Errorf("a %s is written one element at a time", t)
	}
	text = strings.TrimSpace(text)
	// A string without quotes is taken as it is.
	if t.isString() && !strings.HasPrefix(text, "'") && !strings.HasPrefix(text, `"`) {
		return Value{S: text}, nil
	}
	toks, err := Lex("value", text)
	if err != nil {
		return Value{}, fmt.Errorf("%s is no value of %s", text, t)
	}
	p := &parser{toks: toks}
	var e Expr
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(bail); !ok {
					panic(r)
				}
			}
		}()
		e = p.expr()
		if p.tok().Kind != tokEOF {
			e = nil
		}
	}()
	if e == nil {
		return Value{}, fmt.Errorf("%s is no value of %s", text, t)
	}
	c := &compiler{prog: rt.prog, dataTypes: map[string]*DataType{}, resolving: map[string]bool{}}
	x := c.expr(e, t)
	if !x.bad {
		x = c.convert(x, t, Pos{}, "write")
	}
	if x.bad || x.konst == nil || len(c.diags) > 0 {
		if len(c.diags) > 0 {
			return Value{}, fmt.Errorf("%s", c.diags[0].Msg)
		}
		return Value{}, fmt.Errorf("%s is no value of %s", text, t)
	}
	return store(t, *x.konst), nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sortVars(v []*Var) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j].Slot < v[j-1].Slot; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
