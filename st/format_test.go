package st

import (
	"strings"
	"testing"
)

// Format lays out a messy file as CODESYS's Format Document does: keywords and elementary
// types in capitals, spaces around assignments and operators, signs kept on their values,
// blocks indented by a tab, CASE labels once and their statements twice, and comments as they
// were, also lined up after declarations.
func TestFormatLaysOutAFile(t *testing.T) {
	src := strings.Join([]string{
		"program Main",
		"var",
		"x,y : int;      (* both *)",
		"  t:TON;",
		"a : array[1..3] of bool;",
		"end_var",
		"   x:=-1+y*2;",
		"if x>0 and not a[1] then",
		"y:=x;",
		"elsif x<-5 then",
		"	y:=0;",
		"else",
		"y := ABS(x);",
		"end_if",
		"case x of",
		"1,2: y:=1;",
		"3..5:",
		"if y=1 then y:=2; end_if",
		"else",
		"y:=0;",
		"end_case",
		"t(IN:=x>0,PT:=T#1s,Q=>a[2]);",
		"for x:=1 to 3 by 1 do",
		"a[x]:=true; // set",
		"end_for",
		"repeat",
		"x:=x+1;",
		"until x>=10",
		"end_repeat",
		"end_program",
		"",
	}, "\n")
	want := strings.Join([]string{
		"PROGRAM Main",
		"VAR",
		"\tx, y : INT;      (* both *)",
		"\tt : TON;",
		"\ta : ARRAY[1..3] OF BOOL;",
		"END_VAR",
		"x := -1 + y * 2;",
		"IF x > 0 AND NOT a[1] THEN",
		"\ty := x;",
		"ELSIF x < -5 THEN",
		"\ty := 0;",
		"ELSE",
		"\ty := ABS(x);",
		"END_IF",
		"CASE x OF",
		"\t1, 2: y := 1;",
		"\t3..5:",
		"\t\tIF y = 1 THEN y := 2; END_IF",
		"\tELSE",
		"\t\ty := 0;",
		"END_CASE",
		"t(IN := x > 0, PT := T#1s, Q => a[2]);",
		"FOR x := 1 TO 3 BY 1 DO",
		"\ta[x] := TRUE; // set",
		"END_FOR",
		"REPEAT",
		"\tx := x + 1;",
		"UNTIL x >= 10",
		"END_REPEAT",
		"END_PROGRAM",
		"",
	}, "\n")
	got, err := Format("main.st", src)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("formatted:\n%s\nwant:\n%s", got, want)
	}
	if again, _ := Format("main.st", got); again != got {
		t.Errorf("formatting again changes it:\n%s", again)
	}
}

// Nested blocks, types, a global variable list and comments over several lines keep their
// places; a call over several lines goes on one level deeper.
func TestFormatKeepsBlocksAndComments(t *testing.T) {
	src := strings.Join([]string{
		"TYPE Point : STRUCT",
		"x : REAL;",
		"y : REAL;",
		"END_STRUCT",
		"END_TYPE",
		"",
		"FUNCTION_BLOCK Axis",
		"VAR_INPUT go : BOOL; END_VAR",
		"(* A comment",
		"   over two lines *)",
		"WHILE go DO",
		"IF go THEN",
		"go := LIMIT(0,",
		"1, 2) > 0;",
		"END_IF",
		"END_WHILE",
		"END_FUNCTION_BLOCK",
		"",
	}, "\n")
	want := strings.Join([]string{
		"TYPE Point : STRUCT",
		"\tx : REAL;",
		"\ty : REAL;",
		"END_STRUCT",
		"END_TYPE",
		"",
		"FUNCTION_BLOCK Axis",
		"VAR_INPUT go : BOOL; END_VAR",
		"(* A comment",
		"   over two lines *)",
		"WHILE go DO",
		"\tIF go THEN",
		"\t\tgo := LIMIT(0,",
		"\t\t\t1, 2) > 0;",
		"\tEND_IF",
		"END_WHILE",
		"END_FUNCTION_BLOCK",
		"",
	}, "\n")
	got, err := Format("axis.st", src)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("formatted:\n%s\nwant:\n%s", got, want)
	}
}

// A file with a syntax error is given back as it is, with the error.
func TestFormatLeavesAFileWithAnError(t *testing.T) {
	src := "PROGRAM Main\nx := ;\n"
	got, err := Format("main.st", src)
	if err == nil || got != src {
		t.Errorf("formatted a file with an error: %q %v", got, err)
	}
}
