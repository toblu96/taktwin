package sim

import (
	"testing"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// palletLoading is the reference station of the app's own collection.
func palletLoading(t *testing.T) (*Plant, *model.Plan) {
	t.Helper()
	bundle, err := model.NewLibrary(catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(plan), plan
}

// The pallet loading station works as its description says, driven output by output: a
// pallet out of the magazine, to the stop, a carton picked off the belt and set onto it in
// each place of both layers, and the full pallet on to the end of the conveyor.
func TestPalletLoadingByHand(t *testing.T) {
	p, plan := palletLoading(t)
	on := map[string]bool{}
	image := func() plcio.Image {
		var names []string
		for n, v := range on {
			if v {
				names = append(names, n)
			}
		}
		return outputs(t, plan, names...)
	}
	in := func(name string) bool { return input(t, p, plan, name) }
	// waitFor runs until an input is on, or fails after a while.
	waitFor := func(what string, seconds float64, cond func() bool) {
		t.Helper()
		for i := 0; i < int(seconds/dt); i++ {
			if cond() {
				return
			}
			p.Step(dt, image())
		}
		for _, w := range p.Workpieces() {
			t.Logf("%s on %q at %+v, %d loads", w.Kind, w.On, w.At, len(w.Loads))
		}
		t.Fatalf("%s: not within %v s (collisions %v)", what, seconds, p.Collisions())
	}
	// move drives an axis of the gantry to a position by number, as a PLC does.
	move := func(axis string, to int) {
		t.Helper()
		on[axis+"_R0"], on[axis+"_R1"], on[axis+"_GO"] = to&1 != 0, to&2 != 0, false
		run(p, 0.05, image())
		on[axis+"_GO"] = true
		run(p, 0.05, image())
		waitFor(axis+" to position", 10, func() bool { return in(axis + "_DONE") })
		on[axis+"_GO"] = false
	}

	for range 3 {
		if err := p.RunAction("magazine.fill", map[string]string{"kind": "pallet"}); err != nil {
			t.Fatal(err)
		}
		run(p, 1, image())
	}
	run(p, 1, image())
	if in("B6") || !in("B4") {
		t.Fatalf("a filled magazine is not empty, and its pusher is back: B6 %v, B4 %v", in("B6"), in("B4"))
	}
	// Push the bottom pallet out onto the conveyor.
	on["Y2"] = true
	waitFor("the pusher out", 10, func() bool { return in("B5") })
	on["Y2"] = false
	waitFor("the pusher back", 10, func() bool { return in("B4") })
	if !in("B1") {
		t.Fatalf("the pallet is at the start of the conveyor: %+v", p.Workpieces())
	}
	// To the stop.
	on["M1"] = true
	waitFor("the pallet at the stop", 30, func() bool { return in("B2") })
	run(p, 3, image())
	pallet := -1
	for i, w := range p.Workpieces() {
		if w.Kind == "pallet" && w.On == "conveyor.surface" {
			pallet = i
		}
	}
	if pallet < 0 {
		t.Fatalf("no pallet on the conveyor: %+v", p.Workpieces())
	}
	on["M1"] = false
	// Eight cartons, one by one, into the four places of each layer.
	places := [][3]int{{1, 1, 2}, {2, 1, 2}, {1, 2, 2}, {2, 2, 2}, {1, 1, 3}, {2, 1, 3}, {1, 2, 3}, {2, 2, 3}}
	for n, at := range places {
		if err := p.RunAction("cartons.put", map[string]string{"kind": "carton-large"}); err != nil {
			t.Fatalf("carton %d: %v", n, err)
		}
		on["M2"] = true
		waitFor("the carton at the pick place", 20, func() bool { return in("B9") })
		run(p, 1, image())
		on["M2"] = false
		move("A1", 0)
		move("A2", 0)
		move("A3", 1)
		on["Y3"] = true
		waitFor("the vacuum switch", 2, func() bool { return in("B7") })
		move("A3", 0)
		move("A1", at[0])
		move("A2", at[1])
		move("A3", at[2])
		on["Y3"] = false
		run(p, 0.2, image())
		if in("B7") {
			t.Fatalf("carton %d is still held", n)
		}
		move("A3", 0)
		if c := p.Collisions(); len(c) > 0 {
			t.Fatalf("carton %d: %v", n, c)
		}
	}
	loaded := p.Workpieces()
	var loads int
	for _, w := range loaded {
		if w.Kind == "pallet" && w.On == "conveyor.surface" {
			loads = len(w.Loads)
		}
	}
	if loads != 8 {
		t.Fatalf("the pallet carries 8 cartons, not %d: %+v", loads, loaded)
	}
	// Lower the stop and run the full pallet to the end.
	on["Y1"], on["M1"] = true, true
	waitFor("the full pallet at the end", 30, func() bool { return in("B3") })
	run(p, 5, image())
	if p.FellOff() != 0 || len(p.Collisions()) != 0 {
		t.Fatalf("nothing lost and nothing hit: %d lost, %v", p.FellOff(), p.Collisions())
	}
}

// The suction frame holds a small carton too, which is lower than the large one the pick
// height is taught for: its bellows reach that far.
func TestTheGantryPicksSmallCartons(t *testing.T) {
	p, plan := palletLoading(t)
	if err := p.RunAction("cartons.put", map[string]string{"kind": "carton-small"}); err != nil {
		t.Fatal(err)
	}
	on := []string{"M2"}
	for i := 0; i < 2000 && !input(t, p, plan, "B9"); i++ {
		p.Step(dt, outputs(t, plan, on...))
	}
	run(p, 1, outputs(t, plan, on...))
	// Over the pick place and down to the pick height.
	for _, step := range [][]string{{}, {"A1_GO", "A2_GO"}, {"A3_R0"}, {"A3_R0", "A3_GO"}} {
		run(p, 4, outputs(t, plan, step...))
	}
	run(p, 0.5, outputs(t, plan, "A3_R0", "Y3"))
	if !input(t, p, plan, "B7") {
		t.Fatalf("the vacuum switch says the small carton is held: %+v", p.Workpieces())
	}
}

// The emergency stop latches, takes the power from the actuators until it is released and
// Reset is pressed, and stops the gantry where it is. The lamps of the console keep theirs.
func TestTheEmergencyStop(t *testing.T) {
	p, plan := palletLoading(t)
	press := func(device string, on bool) {
		t.Helper()
		if err := p.SetControl("console."+device, on); err != nil {
			t.Fatal(err)
		}
	}
	if !input(t, p, plan, "S0") || !input(t, p, plan, "K1") {
		t.Fatal("at power-on the emergency stop is released and the actuators have power")
	}
	// The bridge on its way to the pick place, the conveyor running, the reset lamp lit.
	run(p, 0.2, outputs(t, plan, "A1_GO", "M1", "P2"))
	x := plan.State("gantry.x.value")
	moving := p.v[x]
	press("estop", true)
	run(p, 0.2, outputs(t, plan, "A1_GO", "M1", "P2"))
	stopped := p.v[x]
	run(p, 0.5, outputs(t, plan, "A1_GO", "M1", "P2"))
	if input(t, p, plan, "S0") || input(t, p, plan, "K1") {
		t.Error("pressed, its contact opens and the actuators lose their power")
	}
	if moving <= 0 || p.v[x] != stopped || stopped != moving {
		t.Errorf("the bridge stops where it is: %v, then %v", stopped, p.v[x])
	}
	if p.v[plan.State("conveyor.drive.clockwise")] != 0 {
		t.Error("the conveyor's motor has no power")
	}
	if p.v[plan.State("console.resetLamp.on")] != 1 {
		t.Error("the lamps of the console keep their power")
	}
	// Released, it stays without power until Reset.
	press("estop", false)
	run(p, 0.2, outputs(t, plan, "M1"))
	if !input(t, p, plan, "S0") || input(t, p, plan, "K1") || p.v[plan.State("conveyor.drive.clockwise")] != 0 {
		t.Error("released, the emergency stop's contact closes, but the power waits for Reset")
	}
	press("reset", true)
	run(p, 0.1, outputs(t, plan, "M1"))
	press("reset", false)
	run(p, 0.1, outputs(t, plan, "M1"))
	if !input(t, p, plan, "K1") || p.v[plan.State("conveyor.drive.clockwise")] != 1 {
		t.Error("after Reset, the actuators have power again")
	}
}
