package st

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Kind is the kind of a type.
type Kind int

const (
	KBool Kind = iota
	KSInt
	KInt
	KDInt
	KLInt
	KUSInt
	KUInt
	KUDInt
	KULInt
	KByte
	KWord
	KDWord
	KLWord
	KReal
	KLReal
	KTime
	KString
	KWString
	KArray
	KStruct
	KEnum
	KFB
)

// Type is a data type: elementary, or declared by the program.
type Type struct {
	Kind Kind
	Name string
	// Len is the length of a string.
	Len int
	// Dims and Elem are an array's dimensions and the type of its elements.
	Dims []Dim
	Elem *Type
	// Fields are a structure's fields; a function block's variables are in FB.
	Fields []*Field
	// Values and Base are an enumeration's values, in order, and the type they are kept as.
	Values []EnumConst
	Base   *Type
	// FB is the function block a type of that kind is an instance of.
	FB *POU
	// Init is the declared initial value of a declared type, or nil.
	Init *Init
}

// Dim is one dimension of an array.
type Dim struct{ Lo, Hi int64 }

// Field is a field of a structure.
type Field struct {
	Name string
	Type *Type
	Init *Init
}

// EnumConst is one value of an enumeration.
type EnumConst struct {
	Name  string
	Value int64
}

func (t *Type) String() string {
	switch t.Kind {
	case KString, KWString:
		if t.Len != defaultStringLen {
			return fmt.Sprintf("%s(%d)", t.Name, t.Len)
		}
	case KArray:
		var dims []string
		for _, d := range t.Dims {
			dims = append(dims, fmt.Sprintf("%d..%d", d.Lo, d.Hi))
		}
		return "ARRAY[" + strings.Join(dims, ", ") + "] OF " + t.Elem.String()
	}
	return t.Name
}

const defaultStringLen = 80

var (
	tBool  = &Type{Kind: KBool, Name: "BOOL"}
	tSInt  = &Type{Kind: KSInt, Name: "SINT"}
	tInt   = &Type{Kind: KInt, Name: "INT"}
	tDInt  = &Type{Kind: KDInt, Name: "DINT"}
	tLInt  = &Type{Kind: KLInt, Name: "LINT"}
	tUSInt = &Type{Kind: KUSInt, Name: "USINT"}
	tUInt  = &Type{Kind: KUInt, Name: "UINT"}
	tUDInt = &Type{Kind: KUDInt, Name: "UDINT"}
	tULInt = &Type{Kind: KULInt, Name: "ULINT"}
	tByte  = &Type{Kind: KByte, Name: "BYTE"}
	tWord  = &Type{Kind: KWord, Name: "WORD"}
	tDWord = &Type{Kind: KDWord, Name: "DWORD"}
	tLWord = &Type{Kind: KLWord, Name: "LWORD"}
	tReal  = &Type{Kind: KReal, Name: "REAL"}
	tLReal = &Type{Kind: KLReal, Name: "LREAL"}
	tTime  = &Type{Kind: KTime, Name: "TIME"}
	tStr   = &Type{Kind: KString, Name: "STRING", Len: defaultStringLen}
	tWStr  = &Type{Kind: KWString, Name: "WSTRING", Len: defaultStringLen}
)

// elementary are the elementary types by name.
var elementary = map[string]*Type{
	"BOOL": tBool, "SINT": tSInt, "INT": tInt, "DINT": tDInt, "LINT": tLInt,
	"USINT": tUSInt, "UINT": tUInt, "UDINT": tUDInt, "ULINT": tULInt,
	"BYTE": tByte, "WORD": tWord, "DWORD": tDWord, "LWORD": tLWord,
	"REAL": tReal, "LREAL": tLReal, "TIME": tTime, "STRING": tStr, "WSTRING": tWStr,
}

func (t *Type) isInt() bool    { return t.Kind >= KSInt && t.Kind <= KULInt }
func (t *Type) isBits() bool   { return t.Kind >= KByte && t.Kind <= KLWord }
func (t *Type) isReal() bool   { return t.Kind == KReal || t.Kind == KLReal }
func (t *Type) isNum() bool    { return t.isInt() || t.isReal() }
func (t *Type) isString() bool { return t.Kind == KString || t.Kind == KWString }
func (t *Type) isAggregate() bool {
	return t.Kind == KArray || t.Kind == KStruct || t.Kind == KFB
}

// isUnsigned is an unsigned integer or a bit string, both kept without sign.
func (t *Type) isUnsigned() bool {
	return t.Kind >= KUSInt && t.Kind <= KULInt || t.isBits()
}

// bits is the width of an integer, bit string or BOOL.
func (t *Type) bits() int {
	switch t.Kind {
	case KBool:
		return 1
	case KSInt, KUSInt, KByte:
		return 8
	case KInt, KUInt, KWord:
		return 16
	case KDInt, KUDInt, KDWord:
		return 32
	}
	return 64
}

// wrap brings an integer into the range of an integer type or bit string, as its width would.
func (t *Type) wrap(v int64) int64 {
	n := t.bits()
	if n == 64 {
		return v
	}
	if t.isUnsigned() {
		return int64(uint64(v) & (1<<n - 1))
	}
	shift := 64 - n
	return v << shift >> shift
}

// fits reports whether an integer is in the range of a type without wrapping.
func (t *Type) fits(v int64) bool {
	switch {
	case t.Kind == KBool:
		return v == 0 || v == 1
	case t.isInt() || t.isBits():
		if t.bits() == 64 {
			return !t.isUnsigned() || v >= 0
		}
		return t.wrap(v) == v
	case t.isReal(), t.Kind == KTime:
		return true
	}
	return false
}

// intRank orders integer types for widening: a type widens to one that holds all its values.
func widensTo(from, to *Type) bool {
	if from == to {
		return true
	}
	switch {
	case from.Kind == KBool:
		return false
	case from.isInt() && to.isInt():
		if from.isUnsigned() == to.isUnsigned() {
			return from.bits() < to.bits()
		}
		return from.isUnsigned() && !to.isUnsigned() && from.bits() < to.bits()
	case from.isBits() && to.isBits():
		return from.bits() < to.bits()
	case from.isInt() && to.isReal():
		// Every integer of up to 16 bits is exact in a REAL, of up to 32 in an LREAL.
		return to.Kind == KLReal && from.bits() <= 32 || to.Kind == KReal && from.bits() <= 16
	case from.Kind == KReal && to.Kind == KLReal:
		return true
	case from.isString() && to.isString():
		return from.Kind == to.Kind
	}
	return false
}

// common is the type both operands of an operation widen to, or nil.
func common(a, b *Type) *Type {
	switch {
	case same(a, b):
		return a
	case widensTo(a, b):
		return b
	case widensTo(b, a):
		return a
	}
	// INT and UINT meet in DINT, as both widen to it.
	for _, t := range []*Type{tInt, tDInt, tLInt, tReal, tLReal} {
		if widensTo(a, t) && widensTo(b, t) {
			return t
		}
	}
	return nil
}

// same reports whether two types are the same; strings of different lengths are.
func same(a, b *Type) bool {
	if a == b {
		return true
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KString, KWString:
		return true
	case KArray:
		if len(a.Dims) != len(b.Dims) || !same(a.Elem, b.Elem) {
			return false
		}
		for i := range a.Dims {
			if a.Dims[i].Hi-a.Dims[i].Lo != b.Dims[i].Hi-b.Dims[i].Lo {
				return false
			}
		}
		return true
	}
	return a.Kind < KArray
}

// Value is a value of a variable. Which field holds it follows from its type: I for BOOL,
// integers, bit strings, TIME in milliseconds and enumerations, F for REAL and LREAL, S for
// strings, and A for arrays, structures and function block instances.
type Value struct {
	I int64
	F float64
	S string
	A *Agg
}

// Agg holds the elements of an array, the fields of a structure, or the variables of an
// instance, in order.
type Agg struct {
	Elems []Value
	// Refs are the variables a VAR_IN_OUT of an instance stands for, by slot.
	Refs map[int]*Value
}

// clone copies a value, with its aggregate, as an assignment does.
func (v Value) clone() Value {
	if v.A == nil {
		return v
	}
	c := &Agg{Elems: make([]Value, len(v.A.Elems))}
	for i, e := range v.A.Elems {
		c.Elems[i] = e.clone()
	}
	return Value{A: c}
}

// zero is the value of a type before any initial value: FALSE, 0, empty, and the
// aggregates with their elements at theirs. Initial values are applied on top.
func zero(t *Type) Value {
	switch t.Kind {
	case KArray:
		n := 1
		for _, d := range t.Dims {
			n *= int(d.Hi - d.Lo + 1)
		}
		a := &Agg{Elems: make([]Value, n)}
		for i := range a.Elems {
			a.Elems[i] = initial(t.Elem, nil)
		}
		return Value{A: a}
	case KStruct:
		a := &Agg{Elems: make([]Value, len(t.Fields))}
		for i, f := range t.Fields {
			if f.Type != nil {
				a.Elems[i] = initial(f.Type, f.Init)
			}
		}
		return Value{A: a}
	case KFB:
		return t.FB.newInstance()
	case KEnum:
		if len(t.Values) > 0 {
			return Value{I: t.Values[0].Value}
		}
	}
	return Value{}
}

// formatValue writes a value as a literal of its type, as a programming tool shows it.
func formatValue(t *Type, v Value) string {
	switch t.Kind {
	case KBool:
		if v.I != 0 {
			return "TRUE"
		}
		return "FALSE"
	case KReal:
		return formatReal(v.F, 32)
	case KLReal:
		return formatReal(v.F, 64)
	case KTime:
		return formatTime(v.I)
	case KString:
		return "'" + v.S + "'"
	case KWString:
		return `"` + v.S + `"`
	case KEnum:
		for _, c := range t.Values {
			if c.Value == v.I {
				return c.Name
			}
		}
		return strconv.FormatInt(v.I, 10)
	case KByte, KWord, KDWord, KLWord:
		return fmt.Sprintf("16#%X", uint64(v.I))
	case KULInt:
		return strconv.FormatUint(uint64(v.I), 10)
	case KArray, KStruct, KFB:
		return ""
	}
	return strconv.FormatInt(v.I, 10)
}

func formatReal(f float64, bits int) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return strconv.FormatFloat(f, 'g', -1, bits)
	}
	s := strconv.FormatFloat(f, 'g', -1, bits)
	if !strings.ContainsAny(s, ".eEn") {
		s += ".0"
	}
	return s
}

// formatTime writes milliseconds as a TIME literal, such as T#1m30s500ms.
func formatTime(ms int64) string {
	if ms == 0 {
		return "T#0ms"
	}
	var b strings.Builder
	b.WriteString("T#")
	if ms < 0 {
		b.WriteByte('-')
		ms = -ms
	}
	for _, u := range []struct {
		n    int64
		unit string
	}{{86400000, "d"}, {3600000, "h"}, {60000, "m"}, {1000, "s"}, {1, "ms"}} {
		if ms >= u.n {
			fmt.Fprintf(&b, "%d%s", ms/u.n, u.unit)
			ms %= u.n
		}
	}
	return b.String()
}

// parseTime reads the value of a TIME literal after T#, such as 1m30s or 1.5s, in
// milliseconds.
func parseTime(s string) (int64, bool) {
	s = strings.ToLower(strings.ReplaceAll(s, "_", ""))
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if s == "" {
		return 0, false
	}
	var total float64
	for s != "" {
		i := 0
		for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
			i++
		}
		if i == 0 {
			return 0, false
		}
		n, err := strconv.ParseFloat(s[:i], 64)
		if err != nil {
			return 0, false
		}
		s = s[i:]
		var unit float64
		switch {
		case strings.HasPrefix(s, "ms"):
			unit, s = 1, s[2:]
		case strings.HasPrefix(s, "us"):
			unit, s = 0.001, s[2:]
		case strings.HasPrefix(s, "ns"):
			unit, s = 0.000001, s[2:]
		case strings.HasPrefix(s, "d"):
			unit, s = 86400000, s[1:]
		case strings.HasPrefix(s, "h"):
			unit, s = 3600000, s[1:]
		case strings.HasPrefix(s, "m"):
			unit, s = 60000, s[1:]
		case strings.HasPrefix(s, "s"):
			unit, s = 1000, s[1:]
		default:
			return 0, false
		}
		total += n * unit
	}
	ms := int64(math.Round(total))
	if neg {
		ms = -ms
	}
	return ms, true
}

// ParseTime reads a TIME literal, such as T#20ms or TIME#1m30s.
func ParseTime(s string) (time.Duration, error) {
	t := strings.TrimSpace(s)
	up := strings.ToUpper(t)
	switch {
	case strings.HasPrefix(up, "TIME#"):
		t = t[5:]
	case strings.HasPrefix(up, "T#"):
		t = t[2:]
	default:
		return 0, fmt.Errorf("%q is no TIME literal such as T#20ms", s)
	}
	ms, ok := parseTime(t)
	if !ok {
		return 0, fmt.Errorf("%q is no TIME literal such as T#20ms", s)
	}
	return time.Duration(ms) * time.Millisecond, nil
}
