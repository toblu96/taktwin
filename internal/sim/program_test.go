package sim

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

const moverModule = `{"format":2,"id":"mover","version":1,"name":"Mover",
	"devices":[
		{"type":"axis","id":"x","stroke":100,"speed":100,"setpoint":true},
		{"type":"program","id":"ctrl","source":"ctrl.st","cycle":0.02,
			"reads":{"pos":"x.value","arrived":"x.done","n":"tray.count"},
			"writes":{"target":"x.setpoint","move":"x.go"}}],
	"transport":[{"id":"tray","static":true,"path":{"line":{"from":[0,0,0],"to":[100,0,0]}}}],
	"actions":[{"id":"put","type":"placeWorkpiece","transport":"tray","at":50}],
	"terminal":{}}`

// mover is a module with an axis driven to a setpoint by a controller of its own, written in
// Structured Text next to its module.json.
func mover(t *testing.T, source string) (*Plant, *model.Plan, error) {
	t.Helper()
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/m/station.json":   {Data: []byte(`{"format":5,"id":"m","name":"M","workpieces":"w","modules":[{"id":"m","module":"mover"}],"plc":{}}`)},
		"modules/mover/module.json": {Data: []byte(moverModule)},
		"modules/mover/ctrl.st":     {Data: []byte(source)},
		"workpieces/w.json":         {Data: []byte(`{"format":2,"id":"w","diameter":40,"height":25,"points":[[0,0],[20,0],[20,25],[0,25]],"kinds":[{"id":"a","label":"A","material":{"type":"MeshLambertMaterial","color":"#888888"}}]}`)},
	}, catalog.Base).Load("m")
	if err != nil {
		return nil, nil, err
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		return nil, nil, err
	}
	return New(plan), plan, nil
}

// A module's own controller reads states of its module, drives its devices' inputs, and has
// inputs and outputs of its own for pins, on the simulation's clock.
func TestAModuleRunsItsOwnController(t *testing.T) {
	p, plan, err := mover(t, `
PROGRAM Ctrl
VAR_INPUT
	start : BOOL;           (* an input of the device, for a pin *)
	pos : REAL;             (* the axis's value *)
	arrived : BOOL;
	n : INT;                (* workpieces on the tray *)
END_VAR
VAR_OUTPUT
	busy : BOOL;            (* a state of the device, for a pin *)
	target : REAL;
	move : BOOL;
	seen : INT;
	waited : TIME;
END_VAR
VAR
	t : TON;
END_VAR
target := 60;
move := start;
busy := start AND NOT arrived;
seen := n;
t(IN := start, PT := T#10s);
waited := t.ET;
END_PROGRAM`)
	if err != nil {
		t.Fatal(err)
	}
	state := func(name string) float64 { return p.v[plan.State(name)] }
	run(p, 0.1, plcio.Image{})
	if state("m.ctrl.busy") != 0 || state("m.x.value") != 0 {
		t.Fatal("it waits for its input")
	}
	p.v[plan.State("m.ctrl.start")] = 1
	run(p, 0.3, plcio.Image{})
	if state("m.ctrl.busy") != 1 || state("m.x.value") <= 0 || state("m.x.value") >= 60 {
		t.Fatalf("started, it drives the axis towards 60: busy %v, at %v", state("m.ctrl.busy"), state("m.x.value"))
	}
	run(p, 1, plcio.Image{})
	if state("m.ctrl.busy") != 0 || state("m.x.value") != 60 || state("m.x.done") != 1 {
		t.Fatalf("the axis stops at its setpoint: busy %v, at %v", state("m.ctrl.busy"), state("m.x.value"))
	}
	if w := state("m.ctrl.waited"); w < 1.2 || w > 1.4 {
		t.Errorf("its timers run on the simulation's clock: %v s", w)
	}
	if err := p.RunAction("m.put", map[string]string{"kind": "a"}); err != nil {
		t.Fatal(err)
	}
	run(p, 0.1, plcio.Image{})
	if state("m.ctrl.seen") != 1 || state("m.tray.count") != 1 {
		t.Errorf("it sees how many workpieces are on the tray: %v", state("m.ctrl.seen"))
	}
	// Reset starts it again from its initial values.
	p.Reset()
	if state("m.ctrl.busy") != 0 || state("m.ctrl.waited") != 0 {
		t.Error("a reset starts the controller anew")
	}
}

// A mistake in a controller's source is reported where it is, when the station loads.
func TestAControllerThatDoesNotBuild(t *testing.T) {
	_, _, err := mover(t, "PROGRAM Ctrl\nVAR_OUTPUT target : REAL; move : BOOL; END_VAR\ntarget := 60\nEND_PROGRAM")
	if err == nil || !strings.Contains(err.Error(), "ctrl.st:3") || !strings.Contains(err.Error(), "program ctrl") {
		t.Errorf("the mistake is reported with its file and line: %v", err)
	}
	// Reads and writes name variables the program has.
	_, _, err = mover(t, "PROGRAM Ctrl\nVAR_INPUT pos : REAL; arrived : BOOL; n : INT; END_VAR\nVAR_OUTPUT move : BOOL; END_VAR\nEND_PROGRAM")
	if err == nil || !strings.Contains(err.Error(), "no VAR_OUTPUT variable target") {
		t.Errorf("a write to a variable it does not have: %v", err)
	}
}
