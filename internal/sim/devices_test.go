package sim

// The devices of modules, each in a module of the test's own, with the test PLC and
// workpieces of testLayer.

import (
	"math"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// testLayer is what the tests build their stations from besides their own modules: a PLC with
// three bytes of digital I/O on spring terminals, and round cups with lids that go onto them.
var testLayer = fstest.MapFS{
	"accessories/test-plc/module.json": {Data: []byte(`{"format":2,"id":"test-plc","version":1,"name":"PLC","role":"plc",
		"addresses":{"inputs":"IB{byte}Bit{bit:2}","outputs":"QB{byte}Bit{bit:2}","analog":"AI{channel}","list":"Bits"},
		"devices":[{"type":"plc","id":"plc"}],
		"terminal":{"connectors":[{"id":"X1","type":"spring-8x8","byte":0,"internal":true},{"id":"X2","type":"spring-8x8","byte":1,"internal":true},
			{"id":"X3","type":"spring-8x8","byte":2,"internal":true}]}}`)},
	"workpieces/test-cups.json": {Data: []byte(`{"format":2,"id":"test-cups","diameter":40,"height":25,
		"marks":[{"id":"coins","label":"Coins","fill":2,"material":{"type":"MeshLambertMaterial","color":"#c9a63a"}}],
		"kinds":[
			{"id":"red","label":"Red","family":"cup","material":{"type":"MeshLambertMaterial","color":"#c0392b"}},
			{"id":"black","label":"Black","family":"cup","material":{"type":"MeshLambertMaterial","color":"#222222"}},
			{"id":"lid","label":"Lid","family":"lid","height":10,"onto":{"family":"cup","depth":6},"material":{"type":"MeshLambertMaterial","color":"#444444"}}]}`)},
}

// transport returns the index of a named transport.
func transport(t *testing.T, plan *model.Plan, name string) int {
	t.Helper()
	for i := range plan.Segments {
		if plan.Segments[i].Name == name {
			return i
		}
	}
	t.Fatalf("no transport %s", name)
	return -1
}

func segmentName(p *Plant, w *part) string { return p.plan.Segments[w.seg].Name }

// kindsOf lists the kinds of loads, from the bottom up.
func kindsOf(loads []load) []string {
	var out []string
	for _, l := range loads {
		out = append(out, l.kind)
	}
	return out
}

// Logic devices work out states from others, hold off changes, latch, and drive inputs that
// the PLC leaves alone.
func TestLogicDevices(t *testing.T) {
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/l/station.json": {Data: []byte(`{"format":5,"id":"l","name":"L","workpieces":"test-cups",
			"modules":[{"id":"plc","module":"test-plc"},{"id":"b","module":"relay"}],
			"wires":[{"from":"plc.X1.Q0","to":"b.W.in"}],
			"plc":{"signals":[{"address":"QB0Bit00","name":"r"}]}}`)},
		"modules/relay/module.json": {Data: []byte(`{"format":2,"id":"relay","version":1,"name":"Relay",
			"parameters":[{"id":"wait","default":0.3}],
			"devices":[
				{"type":"cylinder","id":"c","valve":"monostable","rest":"retracted","stroke":10,"speed":100},
				{"type":"cylinder","id":"r","valve":"monostable","rest":"retracted","stroke":10,"speed":100},
				{"type":"logic","id":"start","expr":"1","onDelay":"wait","drives":"c.extend"},
				{"type":"logic","id":"latch","set":"c.extended","reset":"r.extended"},
				{"type":"logic","id":"both","expr":"latch.on && !r.extended"},
				{"type":"logic","id":"force","expr":"true","drives":"r.extend"}],
			"terminal":{"connectors":[{"id":"W","type":"wires","pins":[{"pin":"in","drives":"r.extend"}]}]}}`)},
	}, testLayer, catalog.Base).Load("l")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := New(plan)
	state := func(name string) float64 { return p.v[plan.State(name)] }
	run(p, 0.2, plcio.Image{})
	if state("b.start.on") != 0 || state("b.c.position") != 0 {
		t.Fatalf("start waits 0.3 s: %v, c at %v", state("b.start.on"), state("b.c.position"))
	}
	run(p, 0.4, plcio.Image{})
	if state("b.start.on") != 1 || state("b.c.extended") != 1 || state("b.latch.on") != 1 || state("b.both.on") != 1 {
		t.Fatalf("start extends c, which sets the latch: start %v, c %v, latch %v, both %v",
			state("b.start.on"), state("b.c.extended"), state("b.latch.on"), state("b.both.on"))
	}
	if state("b.r.position") != 0 {
		t.Errorf("the PLC drives r, not the logic: r at %v", state("b.r.position"))
	}
	run(p, 0.3, outputs(t, plan, "r"))
	if state("b.r.extended") != 1 || state("b.latch.on") != 0 || state("b.both.on") != 0 {
		t.Errorf("r resets the latch: r %v, latch %v, both %v", state("b.r.extended"), state("b.latch.on"), state("b.both.on"))
	}
	run(p, 0.3, plcio.Image{})
	if state("b.latch.on") != 1 {
		t.Errorf("with r back, c sets the latch again: %v", state("b.latch.on"))
	}
	p.Reset()
	if state("b.start.on") != 0 || state("b.latch.on") != 0 {
		t.Errorf("reset clears the logic: start %v, latch %v", state("b.start.on"), state("b.latch.on"))
	}
}

// An axis stops wherever its motor stops, and a turntable turns on endlessly, with an index
// cam made of a logic device.
func TestAxes(t *testing.T) {
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/x/station.json": {Data: []byte(`{"format":5,"id":"x","name":"X","workpieces":"test-cups",
			"modules":[{"id":"plc","module":"test-plc"},{"id":"m","module":"axes"}],
			"wires":[{"from":"plc.X1.Q0","to":"m.W.turn"},{"from":"plc.X1.Q1","to":"m.W.out"},{"from":"plc.X1.Q2","to":"m.W.in"}],
			"plc":{"signals":[{"address":"QB0Bit00","name":"turn"},{"address":"QB0Bit01","name":"out"},{"address":"QB0Bit02","name":"in"}]}}`)},
		"modules/axes/module.json": {Data: []byte(`{"format":2,"id":"axes","version":1,"name":"Axes",
			"devices":[
				{"type":"axis","id":"table","stroke":360,"speed":120,"wrap":true},
				{"type":"axis","id":"y","stroke":200,"speed":100},
				{"type":"logic","id":"index","expr":"abs(table.value - 60 * round(table.value / 60)) < 2"}],
			"terminal":{"connectors":[{"id":"W","type":"wires","pins":[
				{"pin":"turn","drives":"table.forward"},{"pin":"out","drives":"y.forward"},{"pin":"in","drives":"y.backward"}]}]}}`)},
	}, testLayer, catalog.Base).Load("x")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := New(plan)
	state := func(name string) float64 { return p.v[plan.State(name)] }
	if state("m.y.start") != 1 || state("m.index.on") != 1 {
		t.Fatalf("at power on, y is at its start and the table at an index: %v %v", state("m.y.start"), state("m.index.on"))
	}
	run(p, 0.5, outputs(t, plan, "out"))
	run(p, 0.5, plcio.Image{})
	if v := state("m.y.value"); math.Abs(v-50) > 1 || state("m.y.moving") != 0 || state("m.y.start") != 0 {
		t.Errorf("y stops halfway out of its first 100 mm: %v mm", v)
	}
	run(p, 3, outputs(t, plan, "out"))
	if state("m.y.end") != 1 || state("m.y.value") != 200 {
		t.Errorf("y stops at its end: %v mm", state("m.y.value"))
	}
	run(p, 0.25, outputs(t, plan, "turn"))
	if state("m.index.on") != 0 || state("m.table.moving") != 1 {
		t.Errorf("between two indexes: %v°", state("m.table.value"))
	}
	run(p, 0.25, outputs(t, plan, "turn"))
	if state("m.index.on") != 1 {
		t.Errorf("at the next index: %v°", state("m.table.value"))
	}
	run(p, 3, outputs(t, plan, "turn"))
	if v := state("m.table.value"); v < 0 || v >= 360 || state("m.table.end") != 0 {
		t.Errorf("the table turns on past a full turn: %v°", v)
	}
}

// A positioning axis drives to the target its record inputs select as go comes on, says
// when it is there, and still jogs without a record.
func TestAPositioningAxisDrivesToItsRecords(t *testing.T) {
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/x/station.json": {Data: []byte(`{"format":5,"id":"x","name":"X","workpieces":"test-cups",
			"modules":[{"id":"plc","module":"test-plc"},{"id":"m","module":"axes"}],
			"wires":[{"from":"plc.X1.Q0","to":"m.W.r0"},{"from":"plc.X1.Q1","to":"m.W.r1"},{"from":"plc.X1.Q2","to":"m.W.go"},{"from":"plc.X1.Q3","to":"m.W.jog"},
				{"from":"plc.X1.I0","to":"m.W.done"}],
			"plc":{"signals":[{"address":"QB0Bit00","name":"r0"},{"address":"QB0Bit01","name":"r1"},{"address":"QB0Bit02","name":"go"},{"address":"QB0Bit03","name":"jog"},
				{"address":"IB0Bit00","name":"done"}]}}`)},
		"modules/axes/module.json": {Data: []byte(`{"format":2,"id":"axes","version":1,"name":"Axes",
			"devices":[{"type":"axis","id":"x","stroke":400,"speed":200,"targets":[0,100,250,400]}],
			"terminal":{"connectors":[{"id":"W","type":"wires","pins":[
				{"pin":"r0","drives":"x.record0"},{"pin":"r1","drives":"x.record1"},{"pin":"go","drives":"x.go"},{"pin":"jog","drives":"x.forward"},{"pin":"done","reads":"x.done"}]}]}}`)},
	}, testLayer, catalog.Base).Load("x")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := New(plan)
	value := func() float64 { return p.v[plan.State("m.x.value")] }
	if !input(t, p, plan, "done") {
		t.Fatal("at power on, it stands still and is done")
	}
	run(p, 0.1, outputs(t, plan, "r1"))
	run(p, 0.5, outputs(t, plan, "r1", "go"))
	if input(t, p, plan, "done") || value() < 50 || value() > 150 {
		t.Fatalf("on its way to record 2 at 250 mm, not done: %v mm", value())
	}
	run(p, 1, outputs(t, plan, "r1", "go"))
	if !input(t, p, plan, "done") || value() != 250 {
		t.Fatalf("there, and done: %v mm", value())
	}
	run(p, 0.1, outputs(t, plan, "r0"))
	if value() != 250 {
		t.Fatalf("a record alone does not move it: %v mm", value())
	}
	run(p, 1, outputs(t, plan, "r0", "go"))
	if !input(t, p, plan, "done") || value() != 100 {
		t.Fatalf("back to record 1 at 100 mm: %v mm", value())
	}
	run(p, 0.25, outputs(t, plan, "jog"))
	if input(t, p, plan, "done") || math.Abs(value()-150) > 1 {
		t.Errorf("jogging moves it on without a record, and it is not done: %v mm", value())
	}
}

// A transport can ride on a moving node, as the platform of a lift: it takes a workpiece on
// below and hands it over above, wherever its ends line up with another transport.
func TestALiftCarriesAWorkpieceUp(t *testing.T) {
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/l/station.json": {Data: []byte(`{"format":5,"id":"l","name":"L","workpieces":"test-cups",
			"modules":[{"id":"plc","module":"test-plc"},{"id":"feed","module":"track"},{"id":"lift","module":"lift","at":[102,0,0]},{"id":"out","module":"track","at":[144,0,100]}],
			"wires":[{"from":"plc.X1.Q0","to":"lift.W.up"}],
			"plc":{"signals":[{"address":"QB0Bit00","name":"up"}]}}`)},
		"modules/track/module.json": {Data: []byte(`{"format":2,"id":"track","version":1,"name":"Track",
			"transport":[{"id":"t","speed":100,"path":{"line":{"from":[0,0,0],"to":[100,0,0]}}}],
			"actions":[{"id":"place","type":"placeWorkpiece","transport":"t","at":10}]}`)},
		"modules/lift/module.json": {Data: []byte(`{"format":2,"id":"lift","version":1,"name":"Lift",
			"model":[{"name":"platform"}],
			"joints":[{"node":"platform","type":"prismatic","axis":[0,0,1],"range":[0,100],"driver":"z.position"}],
			"devices":[{"type":"cylinder","id":"z","valve":"monostable","rest":"retracted","stroke":100,"speed":200}],
			"transport":[{"id":"deck","node":"platform","speed":100,"path":{"line":{"from":[0,0,0],"to":[40,0,0]}}}],
			"terminal":{"connectors":[{"id":"W","type":"wires","pins":[{"pin":"up","drives":"z.extend"}]}]}}`)},
	}, testLayer, catalog.Base).Load("l")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := New(plan)
	if err := p.RunAction("feed.place", map[string]string{"kind": "red"}); err != nil {
		t.Fatal(err)
	}
	run(p, 2, plcio.Image{})
	if len(p.parts) != 1 || segmentName(p, &p.parts[0]) != "lift.deck" || p.parts[0].s < 39 {
		t.Fatalf("the workpiece waits at the end of the platform: %+v", p.parts)
	}
	run(p, 0.25, outputs(t, plan, "up"))
	if at := p.world(&p.parts[0]); segmentName(p, &p.parts[0]) != "lift.deck" || at.Z < 40 || at.Z > 60 {
		t.Fatalf("halfway up, it rides along: %+v at %+v", p.parts[0], at)
	}
	run(p, 2, outputs(t, plan, "up"))
	if len(p.parts) != 1 || segmentName(p, &p.parts[0]) != "out.t" || p.FellOff() != 0 {
		t.Errorf("at the top, it goes on to the upper track: %+v", p.parts)
	}
}

// A pick and place with a suction cup on two axes: it takes a workpiece off one table and
// sets it down on the other, and its vacuum switch says when it holds one.
func TestASuctionCupPicksAndPlaces(t *testing.T) {
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/p/station.json": {Data: []byte(`{"format":5,"id":"p","name":"P","workpieces":"test-cups",
			"modules":[{"id":"plc","module":"test-plc"},{"id":"pp","module":"pick"}],
			"wires":[{"from":"plc.X1.Q0","to":"pp.W.right"},{"from":"plc.X1.Q1","to":"pp.W.down"},{"from":"plc.X1.Q2","to":"pp.W.suck"},
				{"from":"plc.X1.I0","to":"pp.W.vacuum"}],
			"plc":{"signals":[{"address":"QB0Bit00","name":"right"},{"address":"QB0Bit01","name":"down"},{"address":"QB0Bit02","name":"suck"},
				{"address":"IB0Bit00","name":"vacuum"}]}}`)},
		"modules/pick/module.json": {Data: []byte(`{"format":2,"id":"pick","version":1,"name":"Pick",
			"model":[{"name":"arm","position":[0,0,60],"children":[{"name":"head"}]}],
			"joints":[{"node":"arm","type":"prismatic","axis":[1,0,0],"range":[0,200],"driver":"x.position"},
				{"node":"head","type":"prismatic","axis":[0,0,1],"range":[0,-35],"driver":"z.position"}],
			"devices":[{"type":"axis","id":"x","stroke":200,"speed":200},
				{"type":"cylinder","id":"z","valve":"monostable","rest":"retracted","stroke":35,"speed":350},
				{"type":"gripper","id":"cup","tip":"head","reach":3}],
			"transport":[{"id":"in","static":true,"path":{"line":{"from":[90,0,0],"to":[110,0,0]}}},
				{"id":"out","static":true,"path":{"line":{"from":[190,0,0],"to":[210,0,0]}}}],
			"actions":[{"id":"place","type":"placeWorkpiece","transport":"in","at":10}],
			"terminal":{"connectors":[{"id":"W","type":"wires","pins":[{"pin":"right","drives":"x.forward"},{"pin":"down","drives":"z.extend"},
				{"pin":"suck","drives":"cup.on"},{"pin":"vacuum","reads":"cup.holding"}]}]}}`)},
	}, testLayer, catalog.Base).Load("p")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := New(plan)
	if err := p.RunAction("pp.place", map[string]string{"kind": "red"}); err != nil {
		t.Fatal(err)
	}
	run(p, 0.5, outputs(t, plan, "right"))
	run(p, 0.3, outputs(t, plan, "suck"))
	if input(t, p, plan, "vacuum") {
		t.Fatal("the cup holds nothing above the workpiece")
	}
	run(p, 0.3, outputs(t, plan, "down", "suck"))
	if !input(t, p, plan, "vacuum") {
		t.Fatalf("down on the workpiece, the cup holds it: %+v", p.parts)
	}
	run(p, 0.3, outputs(t, plan, "suck"))
	run(p, 0.5, outputs(t, plan, "suck", "right"))
	if at := p.world(&p.parts[0]); math.Abs(at.X-200) > 1 || at.Z < 30 {
		t.Fatalf("it carries the workpiece up and across: %+v", at)
	}
	run(p, 0.3, outputs(t, plan, "suck", "down"))
	run(p, 0.3, outputs(t, plan, "down"))
	if input(t, p, plan, "vacuum") || segmentName(p, &p.parts[0]) != "pp.out" {
		t.Errorf("without vacuum, it sets the workpiece down on the other table: %+v", p.parts)
	}
}

// Workpieces change: a filler adds coins each time it runs, a probe sees only what has them,
// and a cap that drops onto a cup joins it, which then stands higher.
func TestWorkpiecesAreFilledAndCapped(t *testing.T) {
	bundle, err := model.NewLibrary(fstest.MapFS{
		"stations/c/station.json": {Data: []byte(`{"format":5,"id":"c","name":"C","workpieces":"test-cups",
			"modules":[{"id":"plc","module":"test-plc"},{"id":"m","module":"cell"}],
			"wires":[{"from":"plc.X1.Q0","to":"m.W.fill"},{"from":"plc.X1.I0","to":"m.W.done"},{"from":"plc.X1.I1","to":"m.W.probe"}],
			"plc":{"signals":[{"address":"QB0Bit00","name":"fill"},{"address":"IB0Bit00","name":"done"},{"address":"IB0Bit01","name":"probe"}]}}`)},
		"modules/cell/module.json": {Data: []byte(`{"format":2,"id":"cell","version":1,"name":"Cell",
			"devices":[
				{"type":"process","id":"filler","origin":[20,0,40],"reach":20,"mark":"coins","adds":true,"time":0.5},
				{"type":"sensor","id":"probe","origin":[20,-40,10],"direction":[0,1,0],"reach":80,"detect":"beam","threshold":100,"has":"coins"}],
			"transport":[{"id":"table","static":true,"path":{"line":{"from":[0,0,0],"to":[40,0,0]}}},
				{"id":"chute","speed":100,"open":true,"path":{"line":{"from":[-60,0,40],"to":[2,0,40]}}}],
			"actions":[{"id":"place","type":"placeWorkpiece","transport":"table","at":20},{"id":"cap","type":"placeWorkpiece","transport":"chute","at":0}],
			"terminal":{"connectors":[{"id":"W","type":"wires","pins":[{"pin":"fill","drives":"filler.on"},{"pin":"done","reads":"filler.done"},{"pin":"probe","reads":"probe.detected"}]}]}}`)},
	}, testLayer, catalog.Base).Load("c")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := New(plan)
	if err := p.RunAction("m.place", map[string]string{"kind": "red"}); err != nil {
		t.Fatal(err)
	}
	run(p, 0.2, plcio.Image{})
	if input(t, p, plan, "probe") {
		t.Fatal("the probe sees no coins in an empty cup")
	}
	run(p, 0.3, outputs(t, plan, "fill"))
	if input(t, p, plan, "done") {
		t.Fatal("the filler takes half a second")
	}
	run(p, 0.5, outputs(t, plan, "fill"))
	if !input(t, p, plan, "done") || p.parts[0].marks["coins"] != 1 || !input(t, p, plan, "probe") {
		t.Fatalf("one cycle, one coin: %+v", p.parts[0].marks)
	}
	run(p, 0.2, plcio.Image{})
	run(p, 1, outputs(t, plan, "fill"))
	if p.parts[0].marks["coins"] != 2 {
		t.Errorf("a second cycle adds a second coin: %+v", p.parts[0].marks)
	}
	if err := p.RunAction("m.cap", map[string]string{"kind": "lid"}); err != nil {
		t.Fatal(err)
	}
	run(p, 2, plcio.Image{})
	if len(p.parts) != 1 || !slices.Equal(kindsOf(p.parts[0].loads), []string{"lid"}) || p.FellOff() != 0 {
		t.Fatalf("the cap joins the cup: %+v, %d lost", p.parts, p.FellOff())
	}
	if h := p.heightOf(&p.parts[0]); h != 29 {
		t.Errorf("with its cap, the cup is 29 mm high, not %v", h)
	}
	if v := p.Workpieces(); len(v) != 1 || len(v[0].Loads) != 1 || v[0].Loads[0].Kind != "lid" || v[0].Loads[0].At.Z != 19 || v[0].Marks["coins"] != 2 {
		t.Errorf("the view shows the cap and the coins: %+v", v)
	}
}
