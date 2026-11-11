package plcprog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/toblu96/taktwin/internal/opclink"
	"github.com/toblu96/taktwin/internal/plcio"
)

var signals = []Signal{
	{Name: "B1", Address: "IB0Bit00", Variable: "partThere", Description: "Workpiece there", Input: true},
	{Name: "S1", Address: "IB1Bit00", Variable: "start", Description: "Start button", Input: true, Byte: 1},
	{Name: "G1KF1-2", Address: "QB0Bit00", Description: "Conveyor forward"},
}

// A new project has the template's structure, with the station's signals in the Bits list and
// by name in IO_Mapping, and it builds as it is.
// naming is how the PLC of these tests names its variables.
var naming = plcio.Naming{Inputs: "IB{byte}Bit{bit:2}", Outputs: "QB{byte}Bit{bit:2}", Analog: "AI{channel}", List: "Bits"}

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

// The soft PLC serves its Bits list over OPC UA, so the simulation reaches it as it would
// CODESYS: what the station sends in, the program answers.
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
	s.SetNaming(naming)
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
	l := opclink.Start(context.Background(), opclink.Options{Endpoint: s.Endpoint(), Name: "test", Naming: naming, Program: s.LinkProgram()})
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
