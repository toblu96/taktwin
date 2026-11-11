package st

import (
	"fmt"
	"strings"
	"unicode"
)

// Pos is a place in a source file, with lines and columns counted from 1.
type Pos struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

func (p Pos) String() string { return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col) }

// TokenKind is the kind of a token.
type TokenKind int

const (
	tokEOF TokenKind = iota
	tokIdent
	tokKeyword
	tokInt     // an integer literal, possibly based: 16#FF, 2#1010
	tokReal    // a real literal: 1.5, 1E3
	tokString  // '…' or "…"
	tokTyped   // a typed literal: INT#5, T#1s, TIME#1m, BOOL#TRUE; Text is the whole of it
	tokAddress // a directly represented variable: %IX0.0, %QX1.2, %MW4
	tokOp      // an operator or punctuation
)

// Token is one token of a source file.
type Token struct {
	Kind TokenKind
	Text string // as written; keywords upper-cased
	Pos  Pos
}

// keywords are the reserved words of Structured Text that the parser knows, upper case.
var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`PROGRAM END_PROGRAM FUNCTION END_FUNCTION FUNCTION_BLOCK END_FUNCTION_BLOCK
		VAR VAR_INPUT VAR_OUTPUT VAR_IN_OUT VAR_TEMP VAR_GLOBAL VAR_EXTERNAL VAR_STAT END_VAR CONSTANT RETAIN NON_RETAIN PERSISTENT AT
		TYPE END_TYPE STRUCT END_STRUCT ARRAY OF STRING WSTRING
		IF THEN ELSIF ELSE END_IF CASE END_CASE FOR TO BY DO END_FOR WHILE END_WHILE REPEAT UNTIL END_REPEAT
		EXIT CONTINUE RETURN AND OR XOR NOT MOD TRUE FALSE`) {
		keywords[k] = true
	}
}

// LexError is a mistake in the characters of a source file.
type LexError struct {
	Pos Pos
	Msg string
}

func (e *LexError) Error() string { return e.Pos.String() + ": " + e.Msg }

// Lex splits a source file into tokens. Comments, (* … *) nesting as in the 3rd edition and
// // to the end of the line, and pragmas in { … } are left out.
func Lex(file, src string) ([]Token, error) {
	l := &lexer{file: file, src: []rune(src), line: 1, col: 1}
	var out []Token
	for {
		t, err := l.next()
		if err != nil {
			return out, err
		}
		out = append(out, t)
		if t.Kind == tokEOF {
			return out, nil
		}
	}
}

type lexer struct {
	file      string
	src       []rune
	i         int
	line, col int
}

func (l *lexer) pos() Pos { return Pos{File: l.file, Line: l.line, Col: l.col} }

func (l *lexer) peek(k int) rune {
	if l.i+k < len(l.src) {
		return l.src[l.i+k]
	}
	return 0
}

func (l *lexer) advance() rune {
	r := l.src[l.i]
	l.i++
	if r == '\n' {
		l.line, l.col = l.line+1, 1
	} else {
		l.col++
	}
	return r
}

func (l *lexer) errorf(p Pos, format string, args ...any) error {
	return &LexError{Pos: p, Msg: fmt.Sprintf(format, args...)}
}

// skip leaves out white space, comments and pragmas.
func (l *lexer) skip() error {
	for l.i < len(l.src) {
		r := l.peek(0)
		switch {
		case unicode.IsSpace(r):
			l.advance()
		case r == '(' && l.peek(1) == '*':
			start := l.pos()
			depth := 0
			for {
				if l.i >= len(l.src) {
					return l.errorf(start, "this comment is not closed with *)")
				}
				if l.peek(0) == '(' && l.peek(1) == '*' {
					l.advance()
					l.advance()
					depth++
					continue
				}
				if l.peek(0) == '*' && l.peek(1) == ')' {
					l.advance()
					l.advance()
					if depth--; depth == 0 {
						break
					}
					continue
				}
				l.advance()
			}
		case r == '/' && l.peek(1) == '/':
			for l.i < len(l.src) && l.peek(0) != '\n' {
				l.advance()
			}
		case r == '{':
			start := l.pos()
			for l.i < len(l.src) && l.peek(0) != '}' {
				l.advance()
			}
			if l.i >= len(l.src) {
				return l.errorf(start, "this pragma is not closed with }")
			}
			l.advance()
		default:
			return nil
		}
	}
	return nil
}

func isIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func isIdentPart(r rune) bool  { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (l *lexer) next() (Token, error) {
	if err := l.skip(); err != nil {
		return Token{}, err
	}
	p := l.pos()
	if l.i >= len(l.src) {
		return Token{Kind: tokEOF, Pos: p}, nil
	}
	r := l.peek(0)
	switch {
	case isIdentStart(r):
		start := l.i
		for l.i < len(l.src) && isIdentPart(l.peek(0)) {
			l.advance()
		}
		word := string(l.src[start:l.i])
		// A typed literal: the type, a #, and the value, such as T#1s or INT#16#FF.
		if l.peek(0) == '#' {
			l.advance()
			vstart := l.i
			if l.peek(0) == '-' || l.peek(0) == '+' {
				l.advance()
			}
			// Only a date has - inside it, as D#2024-01-31.
			date := strings.HasPrefix(strings.ToUpper(word), "D")
			for l.i < len(l.src) && (isIdentPart(l.peek(0)) || l.peek(0) == '.' || l.peek(0) == '#' || date && (l.peek(0) == ':' || l.peek(0) == '-' && l.i > vstart)) {
				l.advance()
			}
			if l.i == vstart {
				return Token{}, l.errorf(p, "%s# has no value after it", word)
			}
			return Token{Kind: tokTyped, Text: strings.ToUpper(word) + "#" + string(l.src[vstart:l.i]), Pos: p}, nil
		}
		if up := strings.ToUpper(word); keywords[up] {
			return Token{Kind: tokKeyword, Text: up, Pos: p}, nil
		}
		return Token{Kind: tokIdent, Text: word, Pos: p}, nil
	case unicode.IsDigit(r):
		return l.number(p)
	case r == '\'' || r == '"':
		return l.str(p, r)
	case r == '%':
		l.advance()
		start := l.i
		for l.i < len(l.src) && (isIdentPart(l.peek(0)) || l.peek(0) == '.' || l.peek(0) == '*') {
			l.advance()
		}
		return Token{Kind: tokAddress, Text: "%" + strings.ToUpper(string(l.src[start:l.i])), Pos: p}, nil
	}
	for _, op := range []string{":=", "=>", "<>", "<=", ">=", "**", "..", "^"} {
		if l.hasPrefix(op) {
			for range len(op) {
				l.advance()
			}
			return Token{Kind: tokOp, Text: op, Pos: p}, nil
		}
	}
	if strings.ContainsRune("+-*/=<>&()[],;:.#", r) {
		l.advance()
		return Token{Kind: tokOp, Text: string(r), Pos: p}, nil
	}
	return Token{}, l.errorf(p, "%q is not part of Structured Text", r)
}

func (l *lexer) hasPrefix(s string) bool {
	for k, r := range s {
		if l.peek(k) != r {
			return false
		}
	}
	return true
}

// number reads an integer or real literal, with _ between digits, or a based integer.
func (l *lexer) number(p Pos) (Token, error) {
	start := l.i
	digits := func(ok func(rune) bool) {
		for l.i < len(l.src) && (ok(l.peek(0)) || l.peek(0) == '_' && ok(l.peek(1))) {
			l.advance()
		}
	}
	dec := func(r rune) bool { return r >= '0' && r <= '9' }
	digits(dec)
	if l.peek(0) == '#' {
		base := strings.ReplaceAll(string(l.src[start:l.i]), "_", "")
		if base != "2" && base != "8" && base != "16" {
			return Token{}, l.errorf(p, "%s# is no base: integers are written in base 2, 8 or 16", base)
		}
		l.advance()
		vstart := l.i
		digits(func(r rune) bool { return dec(r) || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' })
		if l.i == vstart {
			return Token{}, l.errorf(p, "%s# has no digits after it", base)
		}
		return Token{Kind: tokInt, Text: string(l.src[start:l.i]), Pos: p}, nil
	}
	kind := tokInt
	if l.peek(0) == '.' && dec(l.peek(1)) {
		kind = tokReal
		l.advance()
		digits(dec)
	}
	if (l.peek(0) == 'e' || l.peek(0) == 'E') && (dec(l.peek(1)) || (l.peek(1) == '-' || l.peek(1) == '+') && dec(l.peek(2))) {
		kind = tokReal
		l.advance()
		if l.peek(0) == '-' || l.peek(0) == '+' {
			l.advance()
		}
		digits(dec)
	}
	return Token{Kind: kind, Text: string(l.src[start:l.i]), Pos: p}, nil
}

// str reads a string literal, '…' a STRING and "…" a WSTRING, with its $ escapes.
func (l *lexer) str(p Pos, quote rune) (Token, error) {
	l.advance()
	var b strings.Builder
	b.WriteRune(quote)
	for {
		if l.i >= len(l.src) || l.peek(0) == '\n' {
			return Token{}, l.errorf(p, "this string is not closed with %c", quote)
		}
		r := l.advance()
		if r == quote {
			break
		}
		if r == '$' {
			if l.i >= len(l.src) {
				return Token{}, l.errorf(p, "this string ends in a lone $")
			}
			e := l.advance()
			switch unicode.ToUpper(e) {
			case '$', '\'', '"':
				b.WriteRune(e)
			case 'L', 'N':
				b.WriteRune('\n')
			case 'P':
				b.WriteRune('\f')
			case 'R':
				b.WriteRune('\r')
			case 'T':
				b.WriteRune('\t')
			default:
				h1, h2 := e, l.peek(0)
				if isHex(h1) && isHex(h2) {
					l.advance()
					b.WriteRune(rune(hexVal(h1)<<4 | hexVal(h2)))
					continue
				}
				return Token{}, l.errorf(p, "$%c is no escape in a string", e)
			}
			continue
		}
		b.WriteRune(r)
	}
	return Token{Kind: tokString, Text: b.String(), Pos: p}, nil
}

func isHex(r rune) bool { return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' }

func hexVal(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	default:
		return int(r-'A') + 10
	}
}
