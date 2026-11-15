package plcprog

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/toblu96/taktwin/internal/link"
	"github.com/toblu96/taktwin/internal/link/opcua"
	"github.com/toblu96/taktwin/internal/opcsim"
	"github.com/toblu96/taktwin/internal/plcio"
)

var signals = []Signal{
	{Name: "B1", Address: "IB0Bit00", Variable: "partThere", Description: "Workpiece there", Input: true},
	{Name: "S1", Address: "IB1Bit00", Variable: "start", Description: "Start button", Input: true, Byte: 1},
	{Name: "G1KF1-2", Address: "QB0Bit00", Description: "Conveyor forward"},
}

// A new project has the template's structure, with the station's signals in the Bits list and
// by name in IO_Mapping, and it builds as it is.

func TestANewProjectBuilds(t *testing.T) {
	p, err := Open(t.TempDir(), signals)
	if err != nil {
		t.Fatal(err)
	}
	files, err := p.Files()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name+":"+f.Kind)
	}
	if got := strings.Join(names, " "); got != "Bits.st:GVL GVL.st:GVL IO_Mapping.st:PROGRAM PLC_PRG.st:PROGRAM StepChain.st:PROGRAM StepHandler.st:FUNCTION_BLOCK" {
		t.Errorf("files: %s", got)
	}
	bits, _ := p.Read(BitsFile)
	io, _ := p.Read("IO_Mapping.st")
	if !strings.Contains(bits, "QB0Bit00 AT %QX0.0 : BOOL; (* G1KF1-2: Conveyor forward *)") || !strings.Contains(io, "partThere := IB0Bit00;") || !strings.Contains(io, "QB0Bit00 := G1KF1_2;") {
		t.Errorf("the Bits list and IO_Mapping follow the station:\n%s\n%s", bits, io)
	}
	if err := p.Write(BitsFile, "x"); err == nil {
		t.Error("the Bits list is the station's")
	}
	prog, diags, err := p.Build()
	if err != nil || prog == nil {
		t.Fatalf("it builds: %v %v", diags, err)
	}
	if _, err := p.Add("Lamp", "FUNCTION_BLOCK"); err != nil {
		t.Fatal(err)
	}
	if prog, diags, _ := p.Build(); prog == nil {
		t.Errorf("a new function block builds as it is: %v", diags)
	}
}

// The soft PLC serves its image over OPC UA, so the simulation reaches it as it would any
// other PLC: what the station sends in, the program answers.
func TestTheSoftPLCAnswersOverOPCUA(t *testing.T) {
	dir := t.TempDir()
	p, err := Open(dir, signals)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Write("StepChain.st", `PROGRAM StepChain
G1KF1_2 := partThere AND start;
IF G1KF1_2 THEN nextStep := 42; END_IF
END_PROGRAM`); err != nil {
		t.Fatal(err)
	}
	prog, diags, _ := p.Build()
	if prog == nil {
		t.Fatal(diags)
	}
	s := NewSoft("Soft PLC test")
	s.SetSpace(opcsim.Space{PLC: "cabinet"})
	if err := s.Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Download(prog, p.Meta.Step); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	resolved, err := opcua.SoftPLC(s.Endpoint(), "cabinet").Resolve(link.Image(), nil)
	if err != nil {
		t.Fatal(err)
	}
	l, err := link.Start(context.Background(), link.Options{Name: "test", Connection: resolved, Program: s.LinkProgram()})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var in plcio.Image
	in.Set(plcio.Addr{Byte: 0, Bit: 0}, true)
	in.Set(plcio.Addr{Byte: 1, Bit: 0}, true)
	conveyor := plcio.Addr{Byte: 0, Bit: 0}
	deadline := time.Now().Add(8 * time.Second)
	for !l.Scan(in).Get(conveyor) {
		if time.Now().After(deadline) {
			t.Fatalf("the conveyor should run: %+v %+v", l.Link(), s.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	for step, _ := s.LinkProgram().Step(); step.Number != 42; step, _ = s.LinkProgram().Step() {
		if time.Now().After(deadline) {
			t.Fatalf("the step handler moves on to step 42, which the link shows: %+v", step)
		}
		time.Sleep(10 * time.Millisecond)
	}
	l.Close()
	if st := s.Status(); st.State != "running" || st.Endpoint == "" {
		t.Errorf("closing the link leaves the PLC running: %+v", st)
	}
	if err := s.Force("GVL.G1KF1_2", "FALSE"); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); len(st.Forced) != 1 {
		t.Errorf("the status lists what is forced: %+v", st)
	}
}

// An analogue channel is in the Bits list at its word, and a program reads its raw value
// by the name the station gives it, and drives an analogue output.
func TestAProgramReadsAnAnalogueInput(t *testing.T) {
	analog := append(slices.Clone(signals),
		Signal{Name: "H1", Address: "AI0", At: "%IW8", Type: "INT", Description: "Height", Input: true},
		Signal{Name: "Y1", Address: "AQ0", At: "%QW8", Type: "INT", Description: "Speed"},
	)
	p, err := Open(t.TempDir(), analog)
	if err != nil {
		t.Fatal(err)
	}
	bits, err := p.Read(BitsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AI0 AT %IW8 : INT;", "AQ0 AT %QW8 : INT;", "IB0Bit00 AT %IX0.0 : BOOL;"} {
		if !strings.Contains(bits, want) {
			t.Errorf("the Bits list lacks %s:\n%s", want, bits)
		}
	}
	if err := p.Write("StepChain.st", "PROGRAM StepChain\nY1 := H1 / 2;\nEND_PROGRAM"); err != nil {
		t.Fatal(err)
	}
	prog, diags, _ := p.Build()
	if prog == nil {
		t.Fatal(diags)
	}
	s := NewSoft("test")
	if err := s.Download(prog, p.Meta.Step); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	var in plcio.Image
	in.SetRaw(plcio.WordAddress(true, 8), 27648)
	// The template maps the Bits list to the names after the step chain, which so sees an
	// input one cycle later.
	s.Scan(in)
	out := s.Scan(in)
	if got := out.Raw(plcio.WordAddress(false, 8)); got != 13824 {
		t.Errorf("the output is %d", got)
	}
}
