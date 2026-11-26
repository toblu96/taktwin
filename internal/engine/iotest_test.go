package engine

import (
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// twoCabinets is the reference station with a second control cabinet beside it, which is a
// second PLC of the station.
func twoCabinets(t *testing.T) *model.Bundle {
	t.Helper()
	const station = "stations/pallet-loading/station.json"
	data, err := fs.ReadFile(catalog.Reference, station)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	st["modules"] = append(st["modules"].([]any), map[string]any{"id": "cabinet2", "module": "control-cabinet", "at": []any{0, 3000, 0}})
	data, _ = json.Marshal(st)
	b, err := model.NewLibrary(fstest.MapFS{station: {Data: data}}, catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The I/O test is the whole station's: it cuts every PLC's program off, the outputs of each
// are switched by hand, and all of them go off together.
func TestTheIOTestCoversEveryPLC(t *testing.T) {
	e, err := New(twoCabinets(t), &beltOn{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(e.Plan().PLCs); got != 2 {
		t.Fatalf("the station has %d PLCs, not 2", got)
	}
	second := e.Plan().PLCs[1]
	first := plcio.Addr{Byte: 0, Bit: 0}
	e.Advance(time.Second)
	if !e.Snapshot().PLCs[0].Outputs.Get(first) {
		t.Fatal("the first PLC's program runs the belt")
	}

	if err := e.SetTestOutputFor(second, "%QX0.1", true); err == nil {
		t.Error("outputs are switched by hand only during an I/O test")
	}
	e.SetIOTest(true)
	e.Advance(50 * time.Millisecond)
	snap := e.Snapshot()
	if !snap.IOTest || snap.PLCs[0].Outputs.Get(first) {
		t.Fatalf("the test cuts the first PLC's program off: test %v, belt %v", snap.IOTest, snap.PLCs[0].Outputs.Get(first))
	}
	if snap.PLCs[0].Step != nil {
		t.Error("a PLC shows no step during the test")
	}
	if err := e.SetTestOutputFor(second, "%QX0.1", true); err != nil {
		t.Fatal(err)
	}
	if err := e.SetTestOutputFor("", "%QX0.2", true); err != nil {
		t.Fatal(err)
	}
	e.Advance(50 * time.Millisecond)
	snap = e.Snapshot()
	if !snap.PLCs[1].Outputs.Get(plcio.Addr{Byte: 0, Bit: 1}) || !snap.PLCs[0].Outputs.Get(plcio.Addr{Byte: 0, Bit: 2}) {
		t.Error("each PLC's outputs are switched by hand")
	}

	e.ClearTestOutputs()
	e.Advance(50 * time.Millisecond)
	snap = e.Snapshot()
	if snap.PLCs[0].Outputs != (plcio.Image{}) || snap.PLCs[1].Outputs != (plcio.Image{}) {
		t.Error("all outputs of both PLCs go off")
	}

	e.SetIOTest(false)
	e.Advance(time.Second)
	if snap = e.Snapshot(); snap.IOTest || !snap.PLCs[0].Outputs.Get(first) {
		t.Error("ending the test gives the outputs back to the programs")
	}
}
