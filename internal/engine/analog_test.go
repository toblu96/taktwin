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
	"github.com/toblu96/taktwin/internal/softplc"
)

// loopStation is the reference station with an analogue connector on its cabinet, whose
// output UA1 a wire joins to its input UE1.
func loopStation(t *testing.T) *model.Bundle {
	t.Helper()
	edit := func(path string, change func(map[string]any)) []byte {
		data, err := fs.ReadFile(catalog.Reference, path)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		change(v)
		out, _ := json.Marshal(v)
		return out
	}
	const cabinet, station = "accessories/control-cabinet/module.json", "stations/pallet-loading/station.json"
	files := fstest.MapFS{
		"connectors.json": {Data: []byte(`{"analog-test": {"label": "Analogue", "inputs": 0, "outputs": 0, "analog": ["UE1", "UA1"]}}`)},
		cabinet: {Data: edit(cabinet, func(m map[string]any) {
			term := m["terminal"].(map[string]any)
			term["connectors"] = append(term["connectors"].([]any), map[string]any{"id": "XA", "type": "analog-test", "analog": true, "internal": true})
		})},
		station: {Data: edit(station, func(st map[string]any) {
			st["wires"] = []any{map[string]any{"from": "cabinet.XA.UA1", "to": "cabinet.XA.UE1"}}
		})},
	}
	b, err := model.NewLibrary(files, catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// halve answers each cycle with half of what its first analogue input reads, on its first
// analogue output.
type halve struct{}

func (halve) Name() string         { return "halve" }
func (halve) Cycle() time.Duration { return 10 * time.Millisecond }
func (halve) Reset()               {}
func (halve) Scan(in plcio.Image) plcio.Image {
	var out plcio.Image
	raw := int16(uint16(in.Raw(plcio.WordAddress(true, 8))))
	out.SetRaw(plcio.WordAddress(false, 8), uint32(uint16(raw/2)))
	return out
}

// An analogue output set by hand in the I/O test drives its net, which the wire takes to
// the analogue input, where the PLC reads it as its raw INT.
func TestAnAnalogueOutputDrivesItsNet(t *testing.T) {
	e, err := New(loopStation(t), softplc.Idle{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetTestAnalogFor("", "%QW8", 5); err == nil {
		t.Error("outputs are set by hand only during an I/O test")
	}
	if err := e.SetIOTestFor("", true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"%IW8", "%QW10", "%QX0.0"} {
		if err := e.SetTestAnalogFor("", bad, 5); err == nil {
			t.Errorf("%s is no analogue output", bad)
		}
	}
	if err := e.SetTestAnalogFor("", "%QW8", 5); err != nil {
		t.Fatal(err)
	}
	e.Advance(50 * time.Millisecond)
	snap := e.Snapshot().PLCs[0]
	if snap.Analog["%QW8"] != 5 || snap.Analog["%IW8"] != 5 {
		t.Errorf("both ends of the wire carry 5 V: %v", snap.Analog)
	}
	if raw := snap.Inputs.Raw(plcio.WordAddress(true, 8)); raw != 13824 {
		t.Errorf("the PLC reads %d", raw)
	}
}

// A program reads the analogue input and drives the output through the image; the wire
// brings the output back, so the value halves until it is 0.
func TestAProgramDrivesAnAnalogueOutput(t *testing.T) {
	e, err := New(loopStation(t), halve{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetIOTestFor("", true); err != nil {
		t.Fatal(err)
	}
	if err := e.SetTestAnalogFor("", "%QW8", 8); err != nil {
		t.Fatal(err)
	}
	e.Advance(20 * time.Millisecond)
	if err := e.SetIOTestFor("", false); err != nil {
		t.Fatal(err)
	}
	e.Advance(15 * time.Millisecond)
	if got := e.Snapshot().PLCs[0].Analog["%QW8"]; got <= 0 || got >= 8 {
		t.Errorf("the program halves what it reads: %v V", got)
	}
	e.Advance(500 * time.Millisecond)
	if got := e.Snapshot().PLCs[0].Analog["%QW8"]; got != 0 {
		t.Errorf("halved down to 0 V, not %v", got)
	}
}
