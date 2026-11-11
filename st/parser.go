package st

import (
	"fmt"
	"strings"
)

// ArrayInit is the initial value of an array: [1, 2, 3(0)], where n(x) repeats x n times.
type ArrayInit struct {
	Pos   Pos
	Items []InitItem
}

// InitItem is one item of an array initialisation, Count times its value.
type InitItem struct {
	Count Expr // nil for once
	Value Expr
}

// StructInit is the initial value of a structure or instance: (a := 1, b := 2).
type StructInit struct {
	Pos    Pos
	Fields []Arg
}

func (e *ArrayInit) exprPos() Pos  { return e.Pos }
func (e *StructInit) exprPos() Pos { return e.Pos }

// SyntaxError is a mistake in the structure of a source file.
type SyntaxError struct {
	Pos Pos
	Msg string
}

func (e *SyntaxError) Error() string { return e.Pos.String() + ": " + e.Msg }

// Parse reads one source file.
func Parse(name, src string) (*File, error) {
	toks, err := Lex(name, src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	f, err := p.file(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

type parser struct {
	toks []Token
	i    int
}

// bail stops parsing at the first mistake, which is reported with its place.
type bail struct{ err *SyntaxError }

func (p *parser) fail(pos Pos, format string, args ...any) {
	panic(bail{&SyntaxError{Pos: pos, Msg: fmt.Sprintf(format, args...)}})
}

func (p *parser) tok() Token     { return p.toks[p.i] }
func (p *parser) peekTok() Token { return p.toks[min(p.i+1, len(p.toks)-1)] }
func (p *parser) move() Token {
	t := p.toks[p.i]
	if t.Kind != tokEOF {
		p.i++
	}
	return t
}

// is reports whether the current token is the keyword or operator s.
func (p *parser) is(s string) bool {
	t := p.tok()
	return (t.Kind == tokKeyword || t.Kind == tokOp) && t.Text == s
}

func (p *parser) accept(s string) bool {
	if p.is(s) {
		p.move()
		return true
	}
	return false
}

func (p *parser) expect(s string) Token {
	if !p.is(s) {
		// A missing ; belongs right after what comes before it, not to the next line.
		if s == ";" && p.i > 0 {
			prev := p.toks[p.i-1]
			end := prev.Pos
			end.Col += len([]rune(prev.Text))
			if prev.Kind == tokString {
				end.Col++
			}
			p.fail(end, "; expected after %s", describe(prev))
		}
		p.fail(p.tok().Pos, "%s expected, found %s", s, describe(p.tok()))
	}
	return p.move()
}

func (p *parser) ident() (string, Pos) {
	t := p.tok()
	if t.Kind != tokIdent {
		if t.Kind == tokKeyword {
			p.fail(t.Pos, "%s is a keyword and cannot be a name", t.Text)
		}
		p.fail(t.Pos, "a name expected, found %s", describe(t))
	}
	p.move()
	return t.Text, t.Pos
}

func describe(t Token) string {
	switch t.Kind {
	case tokEOF:
		return "the end of the file"
	case tokString:
		return "a string"
	}
	return fmt.Sprintf("%q", strings.TrimPrefix(t.Text, "'"))
}

func (p *parser) file(name string) (f *File, err error) {
	defer func() {
		if r := recover(); r != nil {
			b, ok := r.(bail)
			if !ok {
				panic(r)
			}
			err = b.err
		}
	}()
	f = &File{Name: name}
	for p.tok().Kind != tokEOF {
		t := p.tok()
		switch {
		case p.is("PROGRAM"), p.is("FUNCTION_BLOCK"), p.is("FUNCTION"):
			f.Decls = append(f.Decls, p.pou())
		case p.is("VAR_GLOBAL"):
			f.Decls = append(f.Decls, &GlobalDecl{Pos: t.Pos, Vars: p.varBlock()})
		case p.is("TYPE"):
			f.Decls = append(f.Decls, p.typeDecl())
		default:
			p.fail(t.Pos, "PROGRAM, FUNCTION_BLOCK, FUNCTION, VAR_GLOBAL or TYPE expected, found %s", describe(t))
		}
	}
	return f, nil
}

func (p *parser) pou() *POUDecl {
	t := p.move()
	d := &POUDecl{Pos: t.Pos}
	end := "END_" + t.Text
	switch t.Text {
	case "PROGRAM":
		d.Kind = KindProgram
	case "FUNCTION_BLOCK":
		d.Kind = KindFunctionBlock
	default:
		d.Kind = KindFunction
	}
	d.Name, _ = p.ident()
	if d.Kind == KindFunction {
		if p.accept(":") {
			d.Result = p.typeRef()
		}
	}
	for p.isVarStart() {
		d.Vars = append(d.Vars, p.varBlock())
	}
	d.Body = p.stmts(end)
	d.End = p.expect(end).Pos
	p.accept(";")
	return d
}

func (p *parser) isVarStart() bool {
	for _, k := range []string{"VAR", "VAR_INPUT", "VAR_OUTPUT", "VAR_IN_OUT", "VAR_TEMP", "VAR_GLOBAL", "VAR_EXTERNAL", "VAR_STAT"} {
		if p.is(k) {
			return true
		}
	}
	return false
}

func (p *parser) varBlock() *VarBlock {
	t := p.move()
	b := &VarBlock{Pos: t.Pos}
	switch t.Text {
	case "VAR", "VAR_STAT":
		b.Kind = VarLocal
	case "VAR_INPUT":
		b.Kind = VarInput
	case "VAR_OUTPUT":
		b.Kind = VarOutput
	case "VAR_IN_OUT":
		b.Kind = VarInOut
	case "VAR_TEMP":
		b.Kind = VarTemp
	case "VAR_GLOBAL":
		b.Kind = VarGlobal
	case "VAR_EXTERNAL":
		b.Kind = VarExternal
	}
	for {
		switch {
		case p.accept("CONSTANT"):
			b.Constant = true
			continue
		case p.accept("RETAIN"), p.accept("PERSISTENT"):
			b.Retain = true
			continue
		case p.accept("NON_RETAIN"):
			continue
		}
		break
	}
	for !p.is("END_VAR") {
		if p.tok().Kind == tokEOF {
			p.fail(t.Pos, "this %s is not closed with END_VAR", t.Text)
		}
		b.Vars = append(b.Vars, p.varDecl())
	}
	p.move()
	p.accept(";")
	return b
}

func (p *parser) varDecl() *VarDecl {
	v := &VarDecl{Pos: p.tok().Pos}
	for {
		name, pos := p.ident()
		v.Names, v.NamePos = append(v.Names, name), append(v.NamePos, pos)
		if !p.accept(",") {
			break
		}
	}
	if p.accept("AT") {
		t := p.tok()
		if t.Kind != tokAddress {
			p.fail(t.Pos, "an address such as %%IX0.0 expected after AT, found %s", describe(t))
		}
		v.At = p.move().Text
	}
	p.expect(":")
	v.Type = p.typeRef()
	if p.accept(":=") {
		v.Init = p.initValue()
	}
	p.expect(";")
	return v
}

// initValue is an expression, or the initial value of an array or a structure.
func (p *parser) initValue() Expr {
	t := p.tok()
	if p.is("[") {
		p.move()
		a := &ArrayInit{Pos: t.Pos}
		for !p.is("]") {
			item := InitItem{}
			// n(x) repeats x n times.
			if p.tok().Kind == tokInt && p.peekTok().Kind == tokOp && p.peekTok().Text == "(" {
				item.Count = p.primary()
				p.expect("(")
				if !p.is(")") {
					item.Value = p.initValue()
				}
				p.expect(")")
			} else {
				item.Value = p.initValue()
			}
			a.Items = append(a.Items, item)
			if !p.accept(",") {
				break
			}
		}
		p.expect("]")
		return a
	}
	// (a := 1, b := 2) initialises a structure; (1 + 2) is an expression.
	if p.is("(") && p.peekTok().Kind == tokIdent && p.toks[min(p.i+2, len(p.toks)-1)].Text == ":=" {
		p.move()
		s := &StructInit{Pos: t.Pos}
		for !p.is(")") {
			name, pos := p.ident()
			p.expect(":=")
			s.Fields = append(s.Fields, Arg{Pos: pos, Name: name, Value: p.initValue()})
			if !p.accept(",") {
				break
			}
		}
		p.expect(")")
		return s
	}
	return p.expr()
}

func (p *parser) typeRef() *TypeRef {
	t := p.tok()
	r := &TypeRef{Pos: t.Pos}
	switch {
	case p.accept("ARRAY"):
		p.expect("[")
		for {
			lo := p.expr()
			p.expect("..")
			r.Dims = append(r.Dims, Range{Lo: lo, Hi: p.expr()})
			if !p.accept(",") {
				break
			}
		}
		p.expect("]")
		p.expect("OF")
		r.Elem = p.typeRef()
	case p.is("STRING"), p.is("WSTRING"):
		r.Name = p.move().Text
		if p.accept("(") {
			r.Length = p.expr()
			p.expect(")")
		} else if p.accept("[") {
			r.Length = p.expr()
			p.expect("]")
		}
	default:
		r.Name, _ = p.ident()
	}
	return r
}

func (p *parser) typeDecl() *TypeDecl {
	t := p.move()
	d := &TypeDecl{Pos: t.Pos}
	for !p.is("END_TYPE") {
		if p.tok().Kind == tokEOF {
			p.fail(t.Pos, "this TYPE is not closed with END_TYPE")
		}
		dt := &DataType{Pos: p.tok().Pos}
		dt.Name, _ = p.ident()
		p.expect(":")
		switch {
		case p.accept("STRUCT"):
			for !p.is("END_STRUCT") {
				if p.tok().Kind == tokEOF {
					p.fail(dt.Pos, "this STRUCT is not closed with END_STRUCT")
				}
				dt.Struct = append(dt.Struct, p.varDecl())
			}
			p.move()
		case p.is("("):
			p.move()
			for {
				name, pos := p.ident()
				ev := EnumValue{Pos: pos, Name: name}
				if p.accept(":=") {
					ev.Value = p.expr()
				}
				dt.Enum = append(dt.Enum, ev)
				if !p.accept(",") {
					break
				}
			}
			p.expect(")")
			if p.tok().Kind == tokIdent {
				dt.EnumOf = p.typeRef()
			}
		default:
			dt.Alias = p.typeRef()
		}
		if p.accept(":=") {
			dt.Init = p.initValue()
		}
		// CODESYS leaves the ; out after END_STRUCT and before END_TYPE.
		if !p.accept(";") && dt.Struct == nil && !p.is("END_TYPE") {
			p.expect(";")
		}
		d.Types = append(d.Types, dt)
	}
	p.move()
	p.accept(";")
	return d
}

// stmts reads statements up to one of the ending keywords.
func (p *parser) stmts(ends ...string) []Stmt {
	var out []Stmt
	for {
		for _, e := range ends {
			if p.is(e) {
				return out
			}
		}
		if p.tok().Kind == tokEOF {
			p.fail(p.tok().Pos, "%s expected before the end of the file", strings.Join(ends, " or "))
		}
		out = append(out, p.stmt())
	}
}

func (p *parser) stmt() Stmt {
	t := p.tok()
	switch {
	case p.accept(";"):
		return &EmptyStmt{Pos: t.Pos}
	case p.accept("IF"):
		s := &IfStmt{Pos: t.Pos}
		s.Conds = append(s.Conds, p.expr())
		p.expect("THEN")
		s.Thens = append(s.Thens, p.stmts("ELSIF", "ELSE", "END_IF"))
		for p.accept("ELSIF") {
			s.Conds = append(s.Conds, p.expr())
			p.expect("THEN")
			s.Thens = append(s.Thens, p.stmts("ELSIF", "ELSE", "END_IF"))
		}
		if p.accept("ELSE") {
			s.Else = p.stmts("END_IF")
		}
		p.expect("END_IF")
		p.accept(";")
		return s
	case p.accept("CASE"):
		s := &CaseStmt{Pos: t.Pos, Value: p.expr()}
		p.expect("OF")
		for !p.is("ELSE") && !p.is("END_CASE") {
			if p.tok().Kind == tokEOF {
				p.fail(t.Pos, "this CASE is not closed with END_CASE")
			}
			c := CaseClause{Pos: p.tok().Pos}
			for {
				lo := p.expr()
				r := Range{Lo: lo}
				if p.accept("..") {
					r.Hi = p.expr()
				}
				c.Labels = append(c.Labels, r)
				if !p.accept(",") {
					break
				}
			}
			p.expect(":")
			c.Body = p.caseBody()
			s.Clauses = append(s.Clauses, c)
		}
		if p.accept("ELSE") {
			s.HasElse = true
			s.Else = p.stmts("END_CASE")
		}
		p.expect("END_CASE")
		p.accept(";")
		return s
	case p.accept("FOR"):
		s := &ForStmt{Pos: t.Pos}
		name, pos := p.ident()
		s.Var = &Ident{Pos: pos, Name: name}
		p.expect(":=")
		s.From = p.expr()
		p.expect("TO")
		s.To = p.expr()
		if p.accept("BY") {
			s.By, s.HasStep = p.expr(), true
		}
		p.expect("DO")
		s.Body = p.stmts("END_FOR")
		s.EndPos = p.expect("END_FOR").Pos
		p.accept(";")
		return s
	case p.accept("WHILE"):
		s := &WhileStmt{Pos: t.Pos, Cond: p.expr()}
		p.expect("DO")
		s.Body = p.stmts("END_WHILE")
		p.expect("END_WHILE")
		p.accept(";")
		return s
	case p.accept("REPEAT"):
		s := &RepeatStmt{Pos: t.Pos}
		s.Body = p.stmts("UNTIL")
		p.expect("UNTIL")
		s.Cond = p.expr()
		p.expect("END_REPEAT")
		p.accept(";")
		return s
	case p.accept("EXIT"):
		p.expect(";")
		return &ExitStmt{Pos: t.Pos}
	case p.accept("CONTINUE"):
		p.expect(";")
		return &ContinueStmt{Pos: t.Pos}
	case p.accept("RETURN"):
		p.expect(";")
		return &ReturnStmt{Pos: t.Pos}
	}
	if t.Kind != tokIdent {
		p.fail(t.Pos, "a statement expected, found %s", describe(t))
	}
	target := p.postfix()
	if p.accept(":=") {
		s := &AssignStmt{Pos: t.Pos, Target: target, Value: p.expr()}
		p.expect(";")
		return s
	}
	if call, ok := target.(*CallExpr); ok {
		p.expect(";")
		return &CallStmt{Pos: t.Pos, Call: call}
	}
	if p.is("=") {
		p.fail(p.tok().Pos, "= compares; to assign a value, write :=")
	}
	p.fail(p.tok().Pos, ":= or a call expected after %s, found %s", exprText(target), describe(p.tok()))
	return nil
}

// caseBody reads the statements of a case clause, which end where the next label begins: a
// value followed by : or .. or ,.
func (p *parser) caseBody() []Stmt {
	var out []Stmt
	for {
		if p.is("ELSE") || p.is("END_CASE") || p.tok().Kind == tokEOF || p.atCaseLabel() {
			return out
		}
		out = append(out, p.stmt())
	}
}

func (p *parser) atCaseLabel() bool {
	t := p.tok()
	switch t.Kind {
	case tokInt, tokTyped:
	case tokIdent:
		// An enumeration value or a constant as a label: name : or name, or name..
	case tokOp:
		if t.Text != "-" {
			return false
		}
	default:
		return false
	}
	// Find what follows the label's expression on the same level.
	j := p.i
	if p.toks[j].Text == "-" {
		j++
	}
	j++
	for j < len(p.toks) && p.toks[j].Kind == tokOp && p.toks[j].Text == "." && j+1 < len(p.toks) && p.toks[j+1].Kind == tokIdent {
		j += 2
	}
	if j >= len(p.toks) {
		return false
	}
	n := p.toks[j]
	return n.Kind == tokOp && (n.Text == ":" || n.Text == ".." || n.Text == ",")
}

// Expressions, from the loosest binding operator to the tightest, as in IEC 61131-3.
func (p *parser) expr() Expr { return p.binary(0) }

var levels = [][]string{
	{"OR"},
	{"XOR"},
	{"AND", "&"},
	{"=", "<>"},
	{"<", ">", "<=", ">="},
	{"+", "-"},
	{"*", "/", "MOD"},
}

func (p *parser) binary(level int) Expr {
	if level == len(levels) {
		return p.unary()
	}
	x := p.binary(level + 1)
	for {
		t := p.tok()
		op := ""
		for _, o := range levels[level] {
			if p.is(o) {
				op = o
			}
		}
		if op == "" {
			return x
		}
		p.move()
		if op == "&" {
			op = "AND"
		}
		x = &BinaryExpr{Pos: t.Pos, Op: op, X: x, Y: p.binary(level + 1)}
	}
}

func (p *parser) unary() Expr {
	t := p.tok()
	switch {
	case p.accept("NOT"):
		return &UnaryExpr{Pos: t.Pos, Op: "NOT", X: p.unary()}
	case p.accept("-"):
		return &UnaryExpr{Pos: t.Pos, Op: "-", X: p.unary()}
	case p.accept("+"):
		return p.unary()
	}
	return p.power()
}

func (p *parser) power() Expr {
	x := p.postfix()
	for p.is("**") {
		t := p.move()
		x = &BinaryExpr{Pos: t.Pos, Op: "**", X: x, Y: p.postfix()}
	}
	return x
}

func (p *parser) postfix() Expr {
	x := p.primary()
	for {
		t := p.tok()
		switch {
		case p.accept("."):
			// x.0 is a bit of x.
			if p.tok().Kind == tokInt {
				x = &MemberExpr{Pos: t.Pos, X: x, Name: p.move().Text}
				continue
			}
			name, _ := p.ident()
			x = &MemberExpr{Pos: t.Pos, X: x, Name: name}
		case p.accept("["):
			ix := &IndexExpr{Pos: t.Pos, X: x}
			for {
				ix.Indices = append(ix.Indices, p.expr())
				if !p.accept(",") {
					break
				}
			}
			p.expect("]")
			x = ix
		case p.is("("):
			p.move()
			c := &CallExpr{Pos: x.exprPos(), Func: x}
			for !p.is(")") {
				c.Args = append(c.Args, p.arg())
				if !p.accept(",") {
					break
				}
			}
			p.expect(")")
			x = c
		default:
			return x
		}
	}
}

func (p *parser) arg() Arg {
	t := p.tok()
	a := Arg{Pos: t.Pos}
	if p.is("NOT") && p.peekTok().Kind == tokIdent && p.toks[min(p.i+2, len(p.toks)-1)].Text == "=>" {
		p.move()
		a.Not = true
	}
	if p.tok().Kind == tokIdent && (p.peekTok().Text == ":=" || p.peekTok().Text == "=>") {
		a.Name, _ = p.ident()
		a.Output = p.move().Text == "=>"
		if a.Output && p.is(")") || a.Output && p.is(",") {
			return a // an output left unread
		}
	}
	a.Value = p.expr()
	return a
}

func (p *parser) primary() Expr {
	t := p.tok()
	switch t.Kind {
	case tokIdent:
		p.move()
		return &Ident{Pos: t.Pos, Name: t.Text}
	case tokInt, tokReal, tokString, tokTyped:
		p.move()
		return &Literal{Pos: t.Pos, Kind: t.Kind, Text: t.Text}
	case tokKeyword:
		if t.Text == "TRUE" || t.Text == "FALSE" {
			p.move()
			return &Literal{Pos: t.Pos, Kind: tokKeyword, Text: t.Text}
		}
	case tokAddress:
		p.move()
		return &Ident{Pos: t.Pos, Name: t.Text}
	case tokOp:
		if t.Text == "(" {
			p.move()
			x := p.expr()
			p.expect(")")
			return &ParenExpr{Pos: t.Pos, X: x}
		}
	}
	p.fail(t.Pos, "a value expected, found %s", describe(t))
	return nil
}

// exprText writes an expression back as source, for messages.
func exprText(e Expr) string {
	switch e := e.(type) {
	case *Ident:
		return e.Name
	case *Literal:
		return e.Text
	case *MemberExpr:
		return exprText(e.X) + "." + e.Name
	case *IndexExpr:
		var parts []string
		for _, i := range e.Indices {
			parts = append(parts, exprText(i))
		}
		return exprText(e.X) + "[" + strings.Join(parts, ", ") + "]"
	case *CallExpr:
		return exprText(e.Func) + "(…)"
	case *ParenExpr:
		return "(" + exprText(e.X) + ")"
	case *UnaryExpr:
		if e.Op == "NOT" {
			return "NOT " + exprText(e.X)
		}
		return e.Op + exprText(e.X)
	case *BinaryExpr:
		return exprText(e.X) + " " + e.Op + " " + exprText(e.Y)
	}
	return "…"
}
