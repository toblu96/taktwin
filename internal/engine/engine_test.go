package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
	"github.com/toblu96/taktwin/internal/softplc"
)

// newEngine gives an engine on the reference station, run by a PLC that only drives the
// pallet conveyor.
func newEngine(t *testing.T) *Engine {
	t.Helper()
	bundle, err := model.NewLibrary(catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(bundle, &beltOn{})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// beltOn is a PLC whose program only runs the belt on output 0.0, from its 30th scan on, so
// that a run moves the workpiece placed before.
type beltOn struct {
	softplc.Idle
	scans int
}

func (b *beltOn) Reset() { b.scans = 0 }

func (b *beltOn) Scan(plcio.Image) plcio.Image {
	var out plcio.Image
	b.scans++
	out.Set(plcio.Addr{Byte: 0, Bit: 0}, b.scans >= 30)
	return out
}

// script puts a pallet onto the conveyor, lets it run, presses buttons of the console, and
// gives the snapshot at its end.
func script(t *testing.T, e *Engine) string {
	t.Helper()
	e.Advance(100 * time.Millisecond)
	if err := e.RunAction("conveyor.put", map[string]string{"kind": "pallet"}); err != nil {
		t.Fatal(err)
	}
	e.Advance(200 * time.Millisecond)
	_ = e.SetControl("console.start", true)
	e.Advance(100 * time.Millisecond)
	_ = e.SetControl("console.start", false)
	e.Advance(3 * time.Second)
	_ = e.SetControl("console.reset", true)
	e.Advance(100 * time.Millisecond)
	_ = e.SetControl("console.reset", false)
	e.Advance(1 * time.Second)
	b, err := json.Marshal(e.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSameInputsGiveSameResult(t *testing.T) {
	if a, b := script(t, newEngine(t)), script(t, newEngine(t)); a != b {
		t.Errorf("runs differ:\n%s\n%s", a, b)
	}
}

// The script moves the pallet, so that the runs compare something.
func TestTheScriptMovesThePallet(t *testing.T) {
	e := newEngine(t)
	if err := e.RunAction("conveyor.put", map[string]string{"kind": "pallet"}); err != nil {
		t.Fatal(err)
	}
	start := e.Snapshot().Workpieces[0].At
	e.Advance(2 * time.Second)
	if got := e.Snapshot().Workpieces[0].At; got == start {
		t.Errorf("the pallet stays at %v", got)
	}
}

func TestResetBehavesLikeANewEngine(t *testing.T) {
	want := script(t, newEngine(t))
	e := newEngine(t)
	script(t, e)
	e.Reset()
	if got := script(t, e); got != want {
		t.Errorf("run after Reset differs:\n%s\n%s", got, want)
	}
}

func TestSnapshotIsJSONWithNumericImages(t *testing.T) {
	b, err := json.Marshal(newEngine(t).Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Inputs []int `json:"inputs"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Inputs) != plcio.ImageBytes {
		t.Errorf("inputs = %v", got.Inputs)
	}
}

func TestRunAdvancesScaledRealTimeUntilCancelled(t *testing.T) {
	e := newEngine(t)
	e.SetSpeed(10)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	frames := 0
	e.Run(ctx, 10*time.Millisecond, func(Snapshot) { frames++ })
	if frames == 0 {
		t.Fatal("no frames emitted")
	}
	if got := e.Snapshot().Time; got < 1 || got > 3 {
		t.Errorf("simulated %.2f s in 0.2 s at 10x", got)
	}
}

func TestPausedRunDoesNotAdvance(t *testing.T) {
	e := newEngine(t)
	e.SetPaused(true)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	e.Run(ctx, 5*time.Millisecond, func(Snapshot) {})
	if got := e.Snapshot().Time; got != 0 {
		t.Errorf("paused engine advanced to %.3f s", got)
	}
}

// In an I/O test, an output is switched by the name the PLC module gives it.
func TestAnIOTestSwitchesOutputsByTheirNames(t *testing.T) {
	e, err := New(mustLoad(t), softplc.Idle{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetIOTestFor("", true); err != nil {
		t.Fatal(err)
	}
	if err := e.SetTestOutputFor("", "QB0Bit00", true); err != nil {
		t.Fatal(err)
	}
	e.Advance(50 * time.Millisecond)
	if !e.Snapshot().Outputs.Get(plcio.Addr{Byte: 0, Bit: 0}) {
		t.Error("QB0Bit00 is on")
	}
	if err := e.SetTestOutputFor("", "AB0Bit00", true); err == nil {
		t.Error("the cabinet has no output AB0Bit00")
	}
	if err := e.SetTestOutputFor("", "IB0Bit00", true); err == nil {
		t.Error("an input is no output")
	}
}

func mustLoad(t *testing.T) *model.Bundle {
	t.Helper()
	bundle, err := model.NewLibrary(catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}
