package st

// The syntax tree of Structured Text, as the parser reads it. Names keep the spelling of the
// source; the compiler compares them without regard to case.

// File is one source file: any number of declarations.
type File struct {
	Name  string
	Decls []Decl
}

// Decl is a declaration at the top of a file.
type Decl interface{ declPos() Pos }

// POUKind is the kind of a program organisation unit.
type POUKind int

const (
	KindProgram POUKind = iota
	KindFunctionBlock
	KindFunction
)

func (k POUKind) String() string {
	return [...]string{"PROGRAM", "FUNCTION_BLOCK", "FUNCTION"}[k]
}

// POUDecl is a PROGRAM, FUNCTION_BLOCK or FUNCTION with its variables and body.
type POUDecl struct {
	Pos    Pos
	Kind   POUKind
	Name   string
	Result *TypeRef // the return type of a function, or nil
	Vars   []*VarBlock
	Body   []Stmt
	End    Pos
}

// GlobalDecl is a list of global variables, VAR_GLOBAL … END_VAR, as in a GVL.
type GlobalDecl struct {
	Pos  Pos
	Vars *VarBlock
}

// TypeDecl is a TYPE … END_TYPE with one or more data types.
type TypeDecl struct {
	Pos   Pos
	Types []*DataType
}

// DataType is one named data type: a structure, an enumeration, or another type by a new name.
type DataType struct {
	Pos    Pos
	Name   string
	Struct []*VarDecl // for a STRUCT
	Enum   []EnumValue
	EnumOf *TypeRef // the base type of an enumeration, as in (A, B) INT
	Alias  *TypeRef // for TYPE T : INT; END_TYPE and arrays
	Init   Expr
}

// EnumValue is one value of an enumeration, with its number if given.
type EnumValue struct {
	Pos   Pos
	Name  string
	Value Expr
}

func (d *POUDecl) declPos() Pos    { return d.Pos }
func (d *GlobalDecl) declPos() Pos { return d.Pos }
func (d *TypeDecl) declPos() Pos   { return d.Pos }

// VarKind is the section a variable is declared in.
type VarKind int

const (
	VarLocal VarKind = iota
	VarInput
	VarOutput
	VarInOut
	VarTemp
	VarGlobal
	VarExternal
)

func (k VarKind) String() string {
	return [...]string{"VAR", "VAR_INPUT", "VAR_OUTPUT", "VAR_IN_OUT", "VAR_TEMP", "VAR_GLOBAL", "VAR_EXTERNAL"}[k]
}

// VarBlock is a section of variable declarations.
type VarBlock struct {
	Pos      Pos
	Kind     VarKind
	Constant bool
	Retain   bool
	Vars     []*VarDecl
}

// VarDecl declares one or more variables of a type: a, b : INT := 1;
type VarDecl struct {
	Pos     Pos
	Names   []string
	NamePos []Pos
	At      string // a direct address such as %IX0.0, or empty
	Type    *TypeRef
	Init    Expr
	Comment string
}

// TypeRef is a type as written: a name, a STRING of a length, or an ARRAY.
type TypeRef struct {
	Pos    Pos
	Name   string // an elementary or declared type, STRING or WSTRING
	Length Expr   // of a STRING(n)
	Dims   []Range
	Elem   *TypeRef // of an ARRAY
}

// Range is lo..hi, of an array dimension or a case label.
type Range struct {
	Lo, Hi Expr
}

// Stmt is a statement.
type Stmt interface{ stmtPos() Pos }

type (
	// AssignStmt is target := value.
	AssignStmt struct {
		Pos    Pos
		Target Expr
		Value  Expr
	}
	// CallStmt calls a function block instance, a program, or a function for its effect.
	CallStmt struct {
		Pos  Pos
		Call *CallExpr
	}
	// IfStmt is IF … THEN … ELSIF … ELSE … END_IF.
	IfStmt struct {
		Pos   Pos
		Conds []Expr
		Thens [][]Stmt
		Else  []Stmt
	}
	// CaseStmt is CASE … OF … ELSE … END_CASE.
	CaseStmt struct {
		Pos     Pos
		Value   Expr
		Clauses []CaseClause
		Else    []Stmt
		HasElse bool
	}
	// ForStmt is FOR v := a TO b BY c DO … END_FOR.
	ForStmt struct {
		Pos     Pos
		Var     *Ident
		From    Expr
		To      Expr
		By      Expr
		Body    []Stmt
		EndPos  Pos
		HasStep bool
	}
	// WhileStmt is WHILE … DO … END_WHILE.
	WhileStmt struct {
		Pos  Pos
		Cond Expr
		Body []Stmt
	}
	// RepeatStmt is REPEAT … UNTIL … END_REPEAT.
	RepeatStmt struct {
		Pos  Pos
		Body []Stmt
		Cond Expr
	}
	// ExitStmt leaves the innermost loop.
	ExitStmt struct{ Pos Pos }
	// ContinueStmt goes on with the next round of the innermost loop.
	ContinueStmt struct{ Pos Pos }
	// ReturnStmt ends the POU.
	ReturnStmt struct{ Pos Pos }
	// EmptyStmt is a lone ;.
	EmptyStmt struct{ Pos Pos }
)

// CaseClause is one or more labels and their statements.
type CaseClause struct {
	Pos    Pos
	Labels []Range // a single value has Hi nil
	Body   []Stmt
}

func (s *AssignStmt) stmtPos() Pos   { return s.Pos }
func (s *CallStmt) stmtPos() Pos     { return s.Pos }
func (s *IfStmt) stmtPos() Pos       { return s.Pos }
func (s *CaseStmt) stmtPos() Pos     { return s.Pos }
func (s *ForStmt) stmtPos() Pos      { return s.Pos }
func (s *WhileStmt) stmtPos() Pos    { return s.Pos }
func (s *RepeatStmt) stmtPos() Pos   { return s.Pos }
func (s *ExitStmt) stmtPos() Pos     { return s.Pos }
func (s *ContinueStmt) stmtPos() Pos { return s.Pos }
func (s *ReturnStmt) stmtPos() Pos   { return s.Pos }
func (s *EmptyStmt) stmtPos() Pos    { return s.Pos }

// Expr is an expression.
type Expr interface{ exprPos() Pos }

type (
	// Ident is a name: a variable, an instance, a POU, or an enumeration value.
	Ident struct {
		Pos  Pos
		Name string
	}
	// Literal is a number, string, TRUE or FALSE, or a typed literal such as T#1s.
	Literal struct {
		Pos  Pos
		Kind TokenKind // tokInt, tokReal, tokString, tokTyped, or tokKeyword for TRUE and FALSE
		Text string
	}
	// UnaryExpr is NOT x or -x.
	UnaryExpr struct {
		Pos Pos
		Op  string
		X   Expr
	}
	// BinaryExpr is x op y.
	BinaryExpr struct {
		Pos  Pos
		Op   string // upper case: OR XOR AND = <> < > <= >= + - * / MOD **
		X, Y Expr
	}
	// MemberExpr is x.name: a field of a structure, or a variable of an instance.
	MemberExpr struct {
		Pos  Pos
		X    Expr
		Name string
	}
	// IndexExpr is x[i, j].
	IndexExpr struct {
		Pos     Pos
		X       Expr
		Indices []Expr
	}
	// CallExpr calls a function, function block instance or program, with arguments by
	// position or by name, and outputs read with =>.
	CallExpr struct {
		Pos  Pos
		Func Expr
		Args []Arg
	}
	// ParenExpr is (x), kept for positions.
	ParenExpr struct {
		Pos Pos
		X   Expr
	}
)

// Arg is an argument of a call: positional (Name empty), name := value, or name => variable.
type Arg struct {
	Pos    Pos
	Name   string
	Output bool // =>
	Not    bool // NOT name => variable
	Value  Expr
}

func (e *Ident) exprPos() Pos      { return e.Pos }
func (e *Literal) exprPos() Pos    { return e.Pos }
func (e *UnaryExpr) exprPos() Pos  { return e.Pos }
func (e *BinaryExpr) exprPos() Pos { return e.Pos }
func (e *MemberExpr) exprPos() Pos { return e.Pos }
func (e *IndexExpr) exprPos() Pos  { return e.Pos }
func (e *CallExpr) exprPos() Pos   { return e.Pos }
func (e *ParenExpr) exprPos() Pos  { return e.Pos }
