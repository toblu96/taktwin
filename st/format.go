package st

import (
	"errors"
	"strings"
)

// Format lays out a source file as CODESYS's Format Document does: keywords and the names of
// elementary types in capitals, one space around assignments and operators and after commas,
// and every line indented by the blocks it is in, one tab per level. Comments and strings stay
// as they are, as does the space before a comment at the end of a line, so that comments
// lined up stay lined up. A file with a syntax error is not formatted: Format gives it back as
// it is, with the error. Formatting never changes what the file means: if its tokens would not
// stay the same, the file is given back as it is.
func Format(name, src string) (string, error) {
	if _, err := Parse(name, src); err != nil {
		return src, err
	}
	out := layout(scan(src))
	if !sameTokens(name, src, out) {
		return src, errors.New("formatting would change the program, so it is left as it is")
	}
	return out, nil
}

// fmtTypes are the names of the elementary types, which Format writes in capitals as
// CODESYS does, though the lexer reads them as names.
var fmtTypes = map[string]bool{}

func init() {
	for _, t := range strings.Fields(`BOOL SINT INT DINT LINT USINT UINT UDINT ULINT BYTE WORD DWORD LWORD REAL LREAL
		TIME LTIME DATE TIME_OF_DAY TOD DATE_AND_TIME DT CHAR WCHAR`) {
		fmtTypes[t] = true
	}
}

type fmtKind int

const (
	fmtSpace fmtKind = iota
	fmtNewline
	fmtComment
	fmtString
	fmtWord   // a name or a keyword
	fmtNumber // a number or a typed literal such as T#1s
	fmtAddress
	fmtOp
)

type fmtToken struct {
	kind fmtKind
	text string
}

// scan splits a source into tokens, keeping every character: spaces, line ends and comments
// too. A comment that spans lines is one token.
func scan(src string) []fmtToken {
	var out []fmtToken
	r := []rune(src)
	i := 0
	emit := func(k fmtKind, end int) {
		out = append(out, fmtToken{k, string(r[i:end])})
		i = end
	}
	word := func(c rune) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c > 127
	}
	for i < len(r) {
		c := r[i]
		next := func(k int) rune {
			if i+k < len(r) {
				return r[i+k]
			}
			return 0
		}
		switch {
		case c == '\n':
			emit(fmtNewline, i+1)
		case c == ' ' || c == '\t' || c == '\r':
			j := i
			for j < len(r) && (r[j] == ' ' || r[j] == '\t' || r[j] == '\r') {
				j++
			}
			emit(fmtSpace, j)
		case c == '(' && next(1) == '*':
			depth, j := 0, i
			for j < len(r) {
				if r[j] == '(' && j+1 < len(r) && r[j+1] == '*' {
					depth++
					j += 2
				} else if r[j] == '*' && j+1 < len(r) && r[j+1] == ')' {
					j += 2
					if depth--; depth == 0 {
						break
					}
				} else {
					j++
				}
			}
			emit(fmtComment, j)
		case c == '/' && next(1) == '/':
			j := i
			for j < len(r) && r[j] != '\n' {
				j++
			}
			emit(fmtComment, j)
		case c == '{':
			j := i
			for j < len(r) && r[j] != '}' {
				j++
			}
			emit(fmtComment, min(len(r), j+1))
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(r) && r[j] != c && r[j] != '\n' {
				if r[j] == '$' {
					j++
				}
				j++
			}
			emit(fmtString, min(len(r), j+1))
		case c == '%':
			j := i + 1
			for j < len(r) && (word(r[j]) || r[j] == '.' || r[j] == '*') {
				j++
			}
			emit(fmtAddress, j)
		case c >= '0' && c <= '9':
			j := i
			for j < len(r) && (word(r[j]) || r[j] == '#' || r[j] == '.' && j+1 < len(r) && r[j+1] >= '0' && r[j+1] <= '9' ||
				(r[j] == '+' || r[j] == '-') && (r[j-1] == 'e' || r[j-1] == 'E') && !strings.ContainsRune(string(r[i:j]), '#')) {
				j++
			}
			emit(fmtNumber, j)
		case word(c):
			j := i
			for j < len(r) && word(r[j]) {
				j++
			}
			if j < len(r) && r[j] == '#' {
				// A typed literal: T#1s, INT#-5, Colour#Red, TIME#1h2m.
				j++
				for j < len(r) && (word(r[j]) || r[j] == '.' || r[j] == '#' || r[j] == ':' || (r[j] == '-' || r[j] == '+') && r[j-1] == '#') {
					j++
				}
				emit(fmtNumber, j)
			} else {
				emit(fmtWord, j)
			}
		default:
			two := string(r[i:min(len(r), i+2)])
			switch two {
			case ":=", "=>", "<>", "<=", ">=", "**", "..":
				emit(fmtOp, i+2)
			default:
				emit(fmtOp, i+1)
			}
		}
	}
	return out
}

// A fmtFrame is a block a line is in.
type fmtFrame struct {
	kind string // VAR, STRUCT, TYPE, IF, CASE, FOR, WHILE, REPEAT
	// indent is that of the line that opened it, where its end and ELSE stand.
	indent int
	// labelled says that a CASE's label was seen, so its statements are a level deeper.
	labelled bool
}

var (
	openers = map[string]string{"IF": "IF", "CASE": "CASE", "FOR": "FOR", "WHILE": "WHILE", "REPEAT": "REPEAT", "STRUCT": "STRUCT", "TYPE": "TYPE",
		"VAR": "VAR", "VAR_INPUT": "VAR", "VAR_OUTPUT": "VAR", "VAR_IN_OUT": "VAR", "VAR_TEMP": "VAR", "VAR_GLOBAL": "VAR", "VAR_EXTERNAL": "VAR", "VAR_STAT": "VAR"}
	closers = map[string]string{"END_IF": "IF", "END_CASE": "CASE", "END_FOR": "FOR", "END_WHILE": "WHILE", "END_REPEAT": "REPEAT", "END_STRUCT": "STRUCT",
		"END_TYPE": "TYPE", "END_VAR": "VAR"}
)

// layout writes the tokens back line by line, each indented by its blocks and spaced.
func layout(toks []fmtToken) string {
	// Lines of tokens, without the spaces at their start and end.
	var lines [][]fmtToken
	var cur []fmtToken
	for _, t := range toks {
		if t.kind == fmtNewline {
			lines = append(lines, cur)
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	lines = append(lines, cur)

	var stack []fmtFrame
	depth := 0 // brackets open across lines
	var b strings.Builder
	for n, line := range lines {
		line = trim(line)
		if n > 0 {
			b.WriteByte('\n')
		}
		if len(line) == 0 {
			continue
		}
		first := ""
		if line[0].kind == fmtWord {
			first = strings.ToUpper(line[0].text)
		}
		top := func() *fmtFrame {
			if len(stack) == 0 {
				return nil
			}
			return &stack[len(stack)-1]
		}
		body := 0
		if t := top(); t != nil {
			body = t.indent + 1
			if t.kind == "CASE" && t.labelled {
				body++
			}
		}
		// Where the line itself stands.
		level := body
		switch t := top(); {
		case depth > 0:
			level = body + 1
		case closers[first] != "":
			// An end without a block of its own, such as END_STRUCT after TYPE X : STRUCT on
			// one line, stands where the block around it started.
			if t != nil {
				level = t.indent
			}
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].kind == closers[first] {
					level = stack[i].indent
					break
				}
			}
		case t != nil && (first == "ELSIF" || first == "ELSE" && t.kind == "IF" || first == "UNTIL"):
			level = t.indent
		case t != nil && t.kind == "CASE" && (first == "ELSE" || isLabel(line)):
			t.labelled = true
			level = t.indent + 1
		}
		inside := ""
		if t := top(); t != nil {
			inside = t.kind
		}
		b.WriteString(strings.Repeat("\t", level))
		b.WriteString(spaced(line, inside))
		// What the line opens and closes, for the lines after it.
		for _, t := range line {
			switch {
			case t.kind == fmtOp && (t.text == "(" || t.text == "["):
				depth++
			case t.kind == fmtOp && (t.text == ")" || t.text == "]"):
				depth = max(0, depth-1)
			case t.kind != fmtWord:
			case openers[strings.ToUpper(t.text)] != "":
				kind := openers[strings.ToUpper(t.text)]
				// TYPE X : STRUCT … opens one block, as does a TYPE written on one line.
				if kind == "STRUCT" && top() != nil && top().kind == "TYPE" && len(stack) > 0 && opensOnLine(line, "TYPE") {
					continue
				}
				stack = append(stack, fmtFrame{kind: kind, indent: level})
			case closers[strings.ToUpper(t.text)] != "":
				want := closers[strings.ToUpper(t.text)]
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i].kind == want {
						stack = stack[:i]
						break
					}
				}
			}
		}
	}
	out := strings.TrimRight(b.String(), "\n")
	return out + "\n"
}

// opensOnLine says whether a line opens a block of a kind itself.
func opensOnLine(line []fmtToken, kind string) bool {
	for _, t := range line {
		if t.kind == fmtWord && openers[strings.ToUpper(t.text)] == kind {
			return true
		}
	}
	return false
}

// isLabel says whether a line in a CASE starts with a label: values before a colon that is
// no assignment.
func isLabel(line []fmtToken) bool {
	for _, t := range line {
		switch {
		case t.kind == fmtOp && t.text == ":":
			return true
		case t.kind == fmtOp && (t.text == ":=" || t.text == ";" || t.text == "=>" || t.text == "("):
			return false
		case t.kind == fmtWord && (openers[strings.ToUpper(t.text)] != "" || closers[strings.ToUpper(t.text)] != ""):
			return false
		}
	}
	return false
}

func trim(line []fmtToken) []fmtToken {
	for len(line) > 0 && line[0].kind == fmtSpace {
		line = line[1:]
	}
	for len(line) > 0 && line[len(line)-1].kind == fmtSpace {
		line = line[:len(line)-1]
	}
	return line
}

// fmtBinary are the operators with a space on either side.
var fmtBinary = map[string]bool{":=": true, "=>": true, "+": true, "-": true, "*": true, "/": true, "**": true, "=": true, "<>": true, "<": true, ">": true, "<=": true, ">=": true}

// spaced writes a line's tokens with the spaces between them; inside is the block it is in,
// which tells a declaration's colon from a CASE label's.
func spaced(line []fmtToken, inside string) string {
	var b strings.Builder
	var prev *fmtToken // the last token written that is not a space
	spaceBefore := false
	for i := range line {
		t := line[i]
		if t.kind == fmtSpace {
			spaceBefore = true
			continue
		}
		text := t.text
		if t.kind == fmtWord && (keywords[strings.ToUpper(text)] || fmtTypes[strings.ToUpper(text)]) {
			text = strings.ToUpper(text)
		}
		if prev != nil {
			switch {
			case t.kind == fmtComment:
				// Space before a comment stays as it was, so that comments lined up stay so.
				if spaceBefore {
					b.WriteString(spaceRun(line, i))
				}
			case gap(*prev, t, prevBefore(line, i), inside):
				b.WriteByte(' ')
			}
		}
		b.WriteString(text)
		tt := t
		prev = &tt
		spaceBefore = false
	}
	return b.String()
}

// spaceRun is the run of spaces just before token i of a line.
func spaceRun(line []fmtToken, i int) string {
	if i > 0 && line[i-1].kind == fmtSpace {
		return line[i-1].text
	}
	return " "
}

// prevBefore is the token before the one before i that is not a space: what decides whether a
// minus is a sign.
func prevBefore(line []fmtToken, i int) *fmtToken {
	seen := 0
	for j := i - 1; j >= 0; j-- {
		if line[j].kind == fmtSpace || line[j].kind == fmtComment {
			continue
		}
		if seen++; seen == 2 {
			return &line[j]
		}
	}
	return nil
}

// isValue says whether a token ends a value, after which + and - are operators, not signs.
func isValue(t fmtToken) bool {
	switch t.kind {
	case fmtNumber, fmtString, fmtAddress:
		return true
	case fmtWord:
		u := strings.ToUpper(t.text)
		return !keywords[u] || u == "TRUE" || u == "FALSE"
	case fmtOp:
		return t.text == ")" || t.text == "]"
	}
	return false
}

// gap says whether a space goes between two tokens of a line.
func gap(a, b fmtToken, beforeA *fmtToken, inside string) bool {
	op := func(t fmtToken, s ...string) bool {
		if t.kind != fmtOp {
			return false
		}
		for _, x := range s {
			if t.text == x {
				return true
			}
		}
		return false
	}
	switch {
	case op(b, ",", ";", ")", "]", ".", ".."), op(a, "(", "[", ".", ".."):
		return false
	case op(b, "(") && (a.kind == fmtWord && !keywords[strings.ToUpper(a.text)] || op(a, ")", "]")):
		// A call, or an array's bounds: no space before the bracket.
		return false
	case op(b, "[") && a.kind == fmtWord:
		return false
	case op(b, ":"):
		// A declaration's colon stands apart; a CASE label's follows its value.
		return inside != "CASE"
	case op(a, ":"):
		return true
	case op(a, ","):
		return true
	case (op(a, "-") || op(a, "+")) && (beforeA == nil || !isValue(*beforeA)):
		// A sign sticks to its value.
		return false
	case b.kind == fmtOp && fmtBinary[b.text], a.kind == fmtOp && fmtBinary[a.text]:
		return true
	case a.kind == fmtOp || b.kind == fmtOp:
		return !op(a, "#") && !op(b, "#")
	}
	return true
}

// sameTokens says whether two sources lex to the same tokens, keywords in any case.
func sameTokens(name, a, b string) bool {
	ta, err1 := Lex(name, a)
	tb, err2 := Lex(name, b)
	if err1 != nil || err2 != nil || len(ta) != len(tb) {
		return false
	}
	for i := range ta {
		if ta[i].Kind != tb[i].Kind || !strings.EqualFold(ta[i].Text, tb[i].Text) {
			return false
		}
	}
	return true
}
