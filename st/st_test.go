package st

import (
	"strings"
	"testing"
	"time"
)

// build compiles files and fails the test on any error.
func build(t *testing.T, files map[string]string) *Runtime {
	t.Helper()
	p, diags := Compile(Project{Files: files, Cycle: 10 * time.Millisecond})
	if p == nil {
		for _, d := range diags {
			t.Error(d)
		}
		t.FailNow()
	}
	rt := NewRuntime(p)
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	return rt
}

// errorsOf compiles files that should not build, and gives the messages.
func errorsOf(t *testing.T, files map[string]string) string {
	t.Helper()
	p, diags := Compile(Project{Files: files})
	if p != nil {
		t.Fatalf("it should not build")
	}
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.String())
	}
	return strings.Join(msgs, "\n")
}

func read(t *testing.T, rt *Runtime, path string) string {
	t.Helper()
	v, err := rt.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func prg(body string) map[string]string {
	return map[string]string{"PLC_PRG.st": body}
}

func TestArithmeticFollowsTheTypes(t *testing.T) {
	rt := build(t, prg(`
PROGRAM PLC_PRG
VAR
	i : INT := 32767;
	u : USINT := 250;
	d : DINT;
	r : REAL;
	lr : LREAL;
	b : BYTE := 16#F0;
	m : INT;
	neg : INT;
	tm : TIME;
	s : STRING(10);
	bits : WORD;
END_VAR
i := i + 1;              (* wraps, as an INT does *)
u := u + 10;
d := INT_TO_DINT(i) * 2;
r := 1.0 / 3.0;
lr := 2 ** 10;
b := NOT b;
m := 17 MOD 5;
neg := -7 / 2;
tm := T#1s500ms + T#250ms;
s := CONCAT('Hello ', 'world, longer');
bits.3 := TRUE;
END_PROGRAM`))
	rt.Scan(nil, 0)
	for path, want := range map[string]string{
		"PLC_PRG.i": "-32768", "PLC_PRG.u": "4", "PLC_PRG.d": "-65536", "PLC_PRG.r": "0.33333334", "PLC_PRG.lr": "1024.0",
		"PLC_PRG.b": "16#F", "PLC_PRG.m": "2", "PLC_PRG.neg": "-3", "PLC_PRG.tm": "T#1s750ms", "PLC_PRG.s": "'Hello worl'", "PLC_PRG.bits": "16#8",
	} {
		if got := read(t, rt, path); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestControlStatements(t *testing.T) {
	rt := build(t, prg(`
PROGRAM PLC_PRG
VAR
	i, sum, odd, n, w, case1, case2 : INT;
	arr : ARRAY[1..5] OF INT := [1, 2, 3, 2(10)];
	grid : ARRAY[0..1, 0..2] OF INT;
END_VAR
sum := 0;
FOR i := 1 TO 5 DO
	sum := sum + arr[i];
END_FOR
odd := 0;
FOR i := 10 TO 1 BY -1 DO
	IF i MOD 2 = 0 THEN CONTINUE; END_IF
	IF i < 4 THEN EXIT; END_IF
	odd := odd + 1;
END_FOR
n := 0;
WHILE n < 100 DO n := n + 7; END_WHILE
w := 0;
REPEAT w := w + 1; UNTIL w >= 3 END_REPEAT
CASE n OF
	1..99: case1 := 1;
	105, 106: case1 := 2;
ELSE
	case1 := 3;
END_CASE
CASE w OF
	1: case2 := 10;
	2: case2 := 20;
	3:
		case2 := 30;
		case2 := case2 + 1;
END_CASE
grid[1, 2] := 7;
END_PROGRAM`))
	rt.Scan(nil, 0)
	for path, want := range map[string]string{
		"PLC_PRG.sum": "26", "PLC_PRG.odd": "3", "PLC_PRG.n": "105", "PLC_PRG.w": "3", "PLC_PRG.case1": "2", "PLC_PRG.case2": "31", "PLC_PRG.grid[1, 2]": "7",
	} {
		if got := read(t, rt, path); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestStandardFunctionBlocks(t *testing.T) {
	rt := build(t, prg(`
PROGRAM PLC_PRG
VAR
	go : BOOL;
	t : TON;
	off : TOF;
	pulse : TP;
	edge : R_TRIG;
	fall : F_TRIG;
	count : CTU;
	latch : SR;
	edges, falls : INT;
END_VAR
t(IN := go, PT := T#100ms);
off(IN := go, PT := T#50ms);
pulse(IN := go, PT := T#30ms);
edge(CLK := go);
fall(CLK := go);
IF edge.Q THEN edges := edges + 1; END_IF
IF fall.Q THEN falls := falls + 1; END_IF
count(CU := go, R := FALSE, PV := 2);
latch(S1 := edge.Q, R := FALSE);
END_PROGRAM`))
	step := func(ms int, on bool) {
		if err := rt.Write("PLC_PRG.go", map[bool]string{true: "TRUE", false: "FALSE"}[on]); err != nil {
			t.Fatal(err)
		}
		rt.Scan(nil, time.Duration(ms)*time.Millisecond)
	}
	step(0, false)
	if read(t, rt, "PLC_PRG.fall.Q") != "FALSE" {
		t.Error("F_TRIG gives no pulse in the first cycle")
	}
	step(10, true)
	if read(t, rt, "PLC_PRG.t.Q") != "FALSE" || read(t, rt, "PLC_PRG.edge.Q") != "TRUE" || read(t, rt, "PLC_PRG.pulse.Q") != "TRUE" || read(t, rt, "PLC_PRG.latch.Q1") != "TRUE" {
		t.Error("at the rising edge: TON not yet, R_TRIG and TP on, SR set")
	}
	step(20, true)
	if read(t, rt, "PLC_PRG.edge.Q") != "FALSE" || read(t, rt, "PLC_PRG.t.ET") != "T#10ms" {
		t.Errorf("R_TRIG lasts one cycle; TON counts: %s", read(t, rt, "PLC_PRG.t.ET"))
	}
	step(50, true)
	if read(t, rt, "PLC_PRG.pulse.Q") != "FALSE" {
		t.Error("TP ends after its time")
	}
	step(110, true)
	if read(t, rt, "PLC_PRG.t.Q") != "TRUE" || read(t, rt, "PLC_PRG.t.ET") != "T#100ms" {
		t.Error("TON is on after its time")
	}
	step(120, false)
	if read(t, rt, "PLC_PRG.t.Q") != "FALSE" || read(t, rt, "PLC_PRG.off.Q") != "TRUE" || read(t, rt, "PLC_PRG.falls") != "1" {
		t.Error("TON off with IN; TOF holds; F_TRIG fired once")
	}
	step(175, false)
	if read(t, rt, "PLC_PRG.off.Q") != "FALSE" {
		t.Error("TOF goes off after its time")
	}
	step(180, true)
	if read(t, rt, "PLC_PRG.count.Q") != "TRUE" || read(t, rt, "PLC_PRG.count.CV") != "2" || read(t, rt, "PLC_PRG.edges") != "2" {
		t.Errorf("CTU counts the rising edges: %s", read(t, rt, "PLC_PRG.count.CV"))
	}
}

// A typical project structure: a global variable list, a step handler that moves to the next
// step after the step's time, the step chain in its own program, and the I/O mapping.
func TestAStepChainWithItsOwnStepHandler(t *testing.T) {
	rt := build(t, map[string]string{
		"Bits.st": `
VAR_GLOBAL
	EB0Bit00 AT %IX0.0 : BOOL;  (* workpiece there *)
	AB0Bit00 AT %QX0.0 : BOOL;  (* belt *)
END_VAR`,
		"GVL.st": `
VAR_GLOBAL
	currentStep : INT;
	nextStep : INT;
	stepTime : TIME;
	partThere, belt : BOOL;
END_VAR`,
		"StepHandler.st": `
(* Moves on to nextStep once stepTime has passed since it was set. *)
FUNCTION_BLOCK StepHandler
VAR_IN_OUT
	current : INT;
END_VAR
VAR_INPUT
	next : INT;
	delay : TIME;
END_VAR
VAR
	timer : TON;
END_VAR
IF current <> next THEN
	timer(IN := TRUE, PT := delay);
	IF timer.Q THEN
		current := next;
		timer(IN := FALSE);
	END_IF
ELSE
	timer(IN := FALSE);
END_IF
END_FUNCTION_BLOCK`,
		"StepChain.st": `
PROGRAM StepChain
CASE currentStep OF
	0:
		belt := FALSE;
		IF partThere THEN nextStep := 10; stepTime := T#0ms; END_IF
	10:
		belt := TRUE;
		IF NOT partThere THEN nextStep := 20; stepTime := T#200ms; END_IF
	20:
		belt := FALSE;
		nextStep := 0;
END_CASE
END_PROGRAM`,
		"IO_Mapping.st": `
PROGRAM IO_Mapping
partThere := EB0Bit00;
AB0Bit00 := belt;
END_PROGRAM`,
		"PLC_PRG.st": `
PROGRAM PLC_PRG
VAR
	steps : StepHandler;
END_VAR
StepChain();
steps(current := currentStep, next := nextStep, delay := stepTime);
IO_Mapping();
END_PROGRAM`,
	})
	in := make([]byte, ImageBytes)
	var out []byte
	scan := func(ms int, part bool) {
		in[0] = byte(b2i(part))
		out = rt.Scan(in, time.Duration(ms)*time.Millisecond)
	}
	scan(0, false)
	scan(10, true)
	scan(20, true)
	scan(30, true)
	if read(t, rt, "GVL.currentStep") != "10" || out[0]&1 != 1 {
		t.Fatalf("with a workpiece there, it goes to step 10 and runs the belt: step %s, out %v", read(t, rt, "GVL.currentStep"), out[0])
	}
	scan(40, false)
	scan(50, false)
	for ms := 60; ms < 240; ms += 10 {
		scan(ms, false)
	}
	if read(t, rt, "GVL.currentStep") != "10" {
		t.Errorf("step 20 waits for its time: %s", read(t, rt, "GVL.currentStep"))
	}
	scan(260, false)
	scan(270, false)
	if read(t, rt, "GVL.currentStep") != "20" {
		t.Errorf("after 200 ms it is in step 20: %s", read(t, rt, "GVL.currentStep"))
	}
	// Online, the step handler's VAR_IN_OUT shows the variable it stands for.
	if got := read(t, rt, "PLC_PRG.steps.current"); got != read(t, rt, "GVL.currentStep") {
		t.Errorf("steps.current reads the current step: %s", got)
	}
	for _, p := range rt.Variables() {
		for _, v := range p.Children {
			for _, c := range v.Children {
				if c.Path == "PLC_PRG.steps.current" && (c.Value != read(t, rt, "GVL.currentStep") || c.Writable) {
					t.Errorf("the watch shows steps.current as the current step, not to write: %+v", c)
				}
			}
		}
	}
}

func TestFunctionsStructsAndEnums(t *testing.T) {
	rt := build(t, map[string]string{
		"Types.st": `
TYPE
	Colour : (Red, Silver := 5, Black);
	Part : STRUCT
		colour : Colour := Colour.Silver;
		height : REAL := 25.0;
	END_STRUCT
END_TYPE`,
		"Twice.st": `
FUNCTION Twice : INT
VAR_INPUT x : INT; END_VAR
Twice := x * 2;
END_FUNCTION`,
		"PLC_PRG.st": `
PROGRAM PLC_PRG
VAR
	p : Part;
	c : Colour := Colour#Black;
	n, m : INT;
	isSilver, isRed : BOOL;
	k : INT;
END_VAR
n := Twice(Twice(3));
m := Twice(x := 5);
isSilver := p.colour = Silver;
isRed := c = Colour.Red;
k := TO_INT(c);
p.height := p.height + 1.5;
END_PROGRAM`,
	})
	rt.Scan(nil, 0)
	for path, want := range map[string]string{
		"PLC_PRG.n": "12", "PLC_PRG.m": "10", "PLC_PRG.isSilver": "TRUE", "PLC_PRG.isRed": "FALSE", "PLC_PRG.k": "6", "PLC_PRG.p.height": "26.5", "PLC_PRG.c": "Black",
	} {
		if got := read(t, rt, path); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestMistakesAreReportedWhereTheyAre(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"undeclared":      {"PROGRAM PLC_PRG\nx := 1;\nEND_PROGRAM", "PLC_PRG.st:2:1: error: x is not declared"},
		"narrowing":       {"PROGRAM PLC_PRG\nVAR a : INT; b : DINT; END_VAR\na := b;\nEND_PROGRAM", "cannot assign DINT to INT: convert it with DINT_TO_INT"},
		"equals":          {"PROGRAM PLC_PRG\nVAR a : INT; END_VAR\na = 1;\nEND_PROGRAM", "= compares; to assign a value, write :="},
		"semicolon":       {"PROGRAM PLC_PRG\nVAR a : INT; END_VAR\na := 1\nEND_PROGRAM", "3:7: error: ; expected after"},
		"condition":       {"PROGRAM PLC_PRG\nVAR a : INT; END_VAR\nIF a THEN a := 1; END_IF\nEND_PROGRAM", "the condition of IF is INT, but must be a BOOL"},
		"duplicate label": {"PROGRAM PLC_PRG\nVAR a : INT; END_VAR\nCASE a OF 1: a := 2; 0..3: a := 1; END_CASE\nEND_PROGRAM", "the value 1 is already a label"},
		"exit":            {"PROGRAM PLC_PRG\nEXIT;\nEND_PROGRAM", "EXIT is only allowed inside"},
		"literal range":   {"PROGRAM PLC_PRG\nVAR a : SINT; END_VAR\na := 300;\nEND_PROGRAM", "300 does not fit into SINT"},
		"fb output":       {"PROGRAM PLC_PRG\nVAR t : TON; END_VAR\nt.Q := TRUE;\nEND_PROGRAM", "cannot be written here"},
		"comment":         {"PROGRAM PLC_PRG\n(* open\nEND_PROGRAM", "this comment is not closed"},
		"no main":         {"PROGRAM Main\nEND_PROGRAM", "there is no program of that name"},
		"in_out":          {"FUNCTION_BLOCK F\nVAR_IN_OUT x : INT; END_VAR\nEND_FUNCTION_BLOCK\nPROGRAM PLC_PRG\nVAR f : F; END_VAR\nf();\nEND_PROGRAM", "does not give its VAR_IN_OUT x"},
		"string plus":     {"PROGRAM PLC_PRG\nVAR s : STRING; END_VAR\ns := s + 'a';\nEND_PROGRAM", "strings are joined with CONCAT"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := errorsOf(t, prg(c.src)); !strings.Contains(got, c.want) {
				t.Errorf("%s\nshould say %q", got, c.want)
			}
		})
	}
}

func TestRuntimeErrorsStopTheProgram(t *testing.T) {
	rt := build(t, prg(`
PROGRAM PLC_PRG
VAR a : ARRAY[1..3] OF INT; i : INT := 4; z : INT; q : INT; loop : BOOL; END_VAR
IF loop THEN WHILE TRUE DO z := z + 1; END_WHILE END_IF
a[i] := 1;
END_PROGRAM`))
	rt.Scan(nil, 0)
	state, err := rt.State()
	if state != Failed || err == nil || !strings.Contains(err.Msg, "the index 4 is outside") || err.Pos.Line != 5 {
		t.Fatalf("an index out of bounds stops it where it is: %s %+v", state, err)
	}
	rt.ResetCold()
	_ = rt.Write("PLC_PRG.loop", "TRUE")
	_ = rt.Start()
	rt.Scan(nil, 0)
	if state, err := rt.State(); state != Failed || !strings.Contains(err.Msg, "watchdog") {
		t.Errorf("an endless loop is stopped by the watchdog: %s %+v", state, err)
	}
}

func TestForcingHoldsAVariable(t *testing.T) {
	rt := build(t, map[string]string{
		"PLC_PRG.st": `
PROGRAM PLC_PRG
VAR
	lamp AT %QX0.0 : BOOL;
	button AT %IX0.0 : BOOL;
	count : INT;
END_VAR
lamp := button;
count := count + 1;
END_PROGRAM`})
	in := make([]byte, ImageBytes)
	in[0] = 1
	if out := rt.Scan(in, 0); out[0]&1 != 1 {
		t.Fatal("the lamp follows the button")
	}
	if err := rt.Force("PLC_PRG.lamp", "FALSE"); err != nil {
		t.Fatal(err)
	}
	if out := rt.Scan(in, 0); out[0]&1 != 0 {
		t.Error("a forced output stays at its value")
	}
	if err := rt.Force("PLC_PRG.button", "FALSE"); err != nil {
		t.Fatal(err)
	}
	rt.Release("PLC_PRG.lamp")
	if out := rt.Scan(in, 0); out[0]&1 != 0 {
		t.Error("a forced input overrides the image")
	}
	rt.Release("")
	if out := rt.Scan(in, 0); out[0]&1 != 1 {
		t.Error("released, it follows the image again")
	}
	if err := rt.Write("PLC_PRG.count", "100"); err != nil || read(t, rt, "PLC_PRG.count") != "100" {
		t.Error("a value can be written")
	}
	if err := rt.Write("PLC_PRG.count", "TRUE"); err == nil {
		t.Error("a value must be of the variable's type")
	}
	vars := rt.Variables()
	if len(vars) != 1 || vars[0].Name != "PLC_PRG" || len(vars[0].Children) != 3 {
		t.Errorf("the watch shows the program with its variables: %+v", vars)
	}
}

// A host runs a program through handles to its variables, as the simulation runs a machine's
// own controller: no process image, BOOL as 0 or 1, TIME in seconds.
func TestHandles(t *testing.T) {
	prog, diags := Compile(Project{Files: map[string]string{"c.st": `
PROGRAM Controller
VAR_INPUT
	start : BOOL;
	target : REAL;
END_VAR
VAR_OUTPUT
	busy : BOOL;
	setpoint : REAL;
	count : INT;
	waited : TIME;
END_VAR
VAR
	t : TON;
END_VAR
t(IN := start, PT := T#1s);
busy := start AND NOT t.Q;
setpoint := target * 2;
IF t.Q THEN
	count := count + 1;
END_IF
waited := t.ET;
END_PROGRAM`}, Programs: []string{"Controller"}})
	if prog == nil {
		t.Fatal(diags)
	}
	rt := NewRuntime(prog)
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	h := func(name string) Handle {
		x, err := rt.Handle("Controller." + name)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	start, target, busy, setpoint, count, waited := h("start"), h("target"), h("busy"), h("setpoint"), h("count"), h("waited")
	if _, err := rt.Handle("Controller.t"); err == nil {
		t.Error("an instance has no handle")
	}
	start.Set(1)
	target.Set(12.5)
	for ms := 0; ms <= 1200; ms += 100 {
		if err := rt.Run(time.Duration(ms) * time.Millisecond); err != nil {
			t.Fatal(err)
		}
		if ms == 500 && (busy.Get() != 1 || setpoint.Get() != 25 || waited.Get() != 0.5) {
			t.Fatalf("at 500 ms: busy %v, setpoint %v, waited %v", busy.Get(), setpoint.Get(), waited.Get())
		}
	}
	if busy.Get() != 0 || count.Get() != 3 {
		t.Errorf("after a second the timer is done: busy %v, count %v", busy.Get(), count.Get())
	}
	count.Set(40000)
	if count.Get() != 40000-65536 {
		t.Errorf("an INT wraps: %v", count.Get())
	}
}
