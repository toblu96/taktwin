package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"
)

// Num is a number in a model file: a JSON number, a boolean (1 or 0) or an
// expression over parameters such as "length - 20" or "radius * sin(45)".
// Angles in expressions are degrees.
type Num struct {
	src  string
	set  bool
	flag bool // written as true or false in the file
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *Num) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case bytes.Equal(b, []byte("true")):
		n.src, n.flag = "1", true
	case bytes.Equal(b, []byte("false")):
		n.src, n.flag = "0", true
	case len(b) > 0 && b[0] == '"':
		if err := json.Unmarshal(b, &n.src); err != nil {
			return err
		}
	default:
		if _, err := strconv.ParseFloat(string(b), 64); err != nil {
			return fmt.Errorf("%s is not a number or expression", b)
		}
		n.src = string(b)
	}
	n.set = true
	return nil
}

// Flag reports whether the file wrote it as true or false.
func (n Num) Flag() bool { return n.flag }

// Set reports whether the file gave a value.
func (n Num) Set() bool { return n.set }

// Source is the number or expression as written.
func (n Num) Source() string { return n.src }

// NumOf is a plain number, as a station written by the app has it.
func NumOf(v float64) Num { return Num{src: strconv.FormatFloat(v, 'f', -1, 64), set: true} }

type env map[string]float64

func (n Num) eval(vars env) (float64, error) {
	f, err := compileExpr(n.src, func(name string) (Expr, error) {
		v, ok := vars[name]
		if !ok {
			return nil, fmt.Errorf("unknown parameter %q", name)
		}
		return func([]float64) float64 { return v }, nil
	})
	if err != nil {
		return 0, err
	}
	v := f(nil)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%q has no value, as for a division by zero", n.src)
	}
	return v, nil
}

// Expr is an expression that reads states, such as "gate.extended && !stopper.extended".
// True is 1 and false 0, and a state counts as true from 0.5.
type Expr func(states []float64) float64

// compileExpr turns an expression into a function, with names, such as a parameter or a
// state written with dots, resolved by name.
func compileExpr(src string, name func(string) (Expr, error)) (Expr, error) {
	return CompileExpr(src, ExprEnv{Name: name})
}

// ExprEnv is what an expression can refer to beyond numbers and the built-in functions:
// names, and functions of its own, whose arguments are text or numbers, as count("slide1.slide").
type ExprEnv struct {
	Name func(name string) (Expr, error)
	Call func(fn string, args []any) (Expr, error)
}

// CompileExpr turns an expression into a function.
func CompileExpr(src string, in ExprEnv) (Expr, error) {
	if in.Name == nil {
		in.Name = func(n string) (Expr, error) { return nil, fmt.Errorf("unknown name %q", n) }
	}
	node, err := parser.ParseExpr(src)
	if err != nil {
		return nil, fmt.Errorf("invalid expression %q", src)
	}
	f, err := compileNode(node, in)
	if err != nil {
		return nil, fmt.Errorf("in %q: %w", src, err)
	}
	return f, nil
}

var functions = map[string]func(float64) float64{
	"sin":   func(deg float64) float64 { return math.Sin(deg * math.Pi / 180) },
	"cos":   func(deg float64) float64 { return math.Cos(deg * math.Pi / 180) },
	"asin":  func(v float64) float64 { return math.Asin(v) * 180 / math.Pi },
	"sqrt":  math.Sqrt,
	"abs":   math.Abs,
	"round": math.Round,
	"floor": math.Floor,
}

var functions2 = map[string]func(float64, float64) float64{"min": math.Min, "max": math.Max}

func truth(v float64) bool { return v >= 0.5 }

func flag(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// dotted is a name written with dots, such as gate.extended, or empty.
func dotted(node ast.Expr) string {
	switch e := node.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if head := dotted(e.X); head != "" {
			return head + "." + e.Sel.Name
		}
	}
	return ""
}

func compileNode(node ast.Expr, in ExprEnv) (Expr, error) {
	switch e := node.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT && e.Kind != token.FLOAT {
			return nil, fmt.Errorf("unexpected %s", e.Value)
		}
		v, err := strconv.ParseFloat(e.Value, 64)
		return func([]float64) float64 { return v }, err
	case *ast.ParenExpr:
		return compileNode(e.X, in)
	case *ast.Ident, *ast.SelectorExpr:
		n := dotted(e)
		switch n {
		case "pi":
			return func([]float64) float64 { return math.Pi }, nil
		case "true", "false":
			v := flag(n == "true")
			return func([]float64) float64 { return v }, nil
		case "":
			return nil, fmt.Errorf("unsupported name")
		}
		return in.Name(n)
	case *ast.UnaryExpr:
		x, err := compileNode(e.X, in)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case token.SUB:
			return func(v []float64) float64 { return -x(v) }, nil
		case token.ADD:
			return x, nil
		case token.NOT:
			return func(v []float64) float64 { return flag(!truth(x(v))) }, nil
		}
	case *ast.BinaryExpr:
		a, err := compileNode(e.X, in)
		if err != nil {
			return nil, err
		}
		b, err := compileNode(e.Y, in)
		if err != nil {
			return nil, err
		}
		if op, ok := binary[e.Op]; ok {
			return func(v []float64) float64 { return op(a(v), b(v)) }, nil
		}
		if e.Op == token.QUO {
			return func(v []float64) float64 {
				d := b(v)
				if d == 0 {
					return math.NaN()
				}
				return a(v) / d
			}, nil
		}
	case *ast.CallExpr:
		fn, ok := e.Fun.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("unknown function")
		}
		if functions[fn.Name] == nil && functions2[fn.Name] == nil && in.Call != nil {
			return callOwn(in.Call, fn.Name, e.Args)
		}
		args := make([]Expr, len(e.Args))
		for i, arg := range e.Args {
			f, err := compileNode(arg, in)
			if err != nil {
				return nil, err
			}
			args[i] = f
		}
		if f := functions[fn.Name]; f != nil && len(args) == 1 {
			return func(v []float64) float64 { return f(args[0](v)) }, nil
		}
		if f := functions2[fn.Name]; f != nil && len(args) == 2 {
			return func(v []float64) float64 { return f(args[0](v), args[1](v)) }, nil
		}
		return nil, fmt.Errorf("unknown function %s with %d arguments", fn.Name, len(args))
	}
	return nil, fmt.Errorf("unsupported expression")
}

// callOwn compiles a call of a function of the environment, whose arguments are literals.
func callOwn(call func(string, []any) (Expr, error), fn string, list []ast.Expr) (Expr, error) {
	args := make([]any, 0, len(list))
	for _, a := range list {
		neg := false
		if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.SUB {
			a, neg = u.X, true
		}
		lit, ok := a.(*ast.BasicLit)
		switch {
		case ok && lit.Kind == token.STRING && !neg:
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return nil, err
			}
			args = append(args, text)
		case ok && (lit.Kind == token.INT || lit.Kind == token.FLOAT):
			v, err := strconv.ParseFloat(lit.Value, 64)
			if err != nil {
				return nil, err
			}
			if neg {
				v = -v
			}
			args = append(args, v)
		default:
			return nil, fmt.Errorf("the arguments of %s are text in quotes or numbers", fn)
		}
	}
	return call(fn, args)
}

var binary = map[token.Token]func(a, b float64) float64{
	token.ADD:  func(a, b float64) float64 { return a + b },
	token.SUB:  func(a, b float64) float64 { return a - b },
	token.MUL:  func(a, b float64) float64 { return a * b },
	token.LSS:  func(a, b float64) float64 { return flag(a < b) },
	token.GTR:  func(a, b float64) float64 { return flag(a > b) },
	token.LEQ:  func(a, b float64) float64 { return flag(a <= b) },
	token.GEQ:  func(a, b float64) float64 { return flag(a >= b) },
	token.EQL:  func(a, b float64) float64 { return flag(a == b) },
	token.NEQ:  func(a, b float64) float64 { return flag(a != b) },
	token.LAND: func(a, b float64) float64 { return flag(truth(a) && truth(b)) },
	token.LOR:  func(a, b float64) float64 { return flag(truth(a) || truth(b)) },
}
