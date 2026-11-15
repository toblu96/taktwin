package model

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/plcio"
)

const cabinetFile = "accessories/control-cabinet/module.json"

// analogFiles gives the reference cabinet a connector with two analogue inputs and an output,
// with plc set as given, and the station joins UA1 to UE1 and names the channels where they
// are without plc.
func analogFiles(t *testing.T, plc map[string]any) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"connectors.json": {Data: []byte(`{"analog-test": {"label": "Analogue", "inputs": 0, "outputs": 0, "analog": ["UE1", "UE2", "UA1"]}}`)},
		cabinetFile: {Data: changedReference(t, cabinetFile, func(m map[string]any) {
			term := m["terminal"].(map[string]any)
			term["connectors"] = append(term["connectors"].([]any), map[string]any{"id": "XA", "type": "analog-test", "analog": true, "internal": true})
			if plc != nil {
				m["plc"] = plc
			}
		})},
		referenceStation: {Data: changedReference(t, referenceStation, func(st map[string]any) {
			st["wires"] = []any{map[string]any{"from": "cabinet.XA.UA1", "to": "cabinet.XA.UE1"}}
			p := st["plc"].(map[string]any)
			if plc != nil {
				return
			}
			p["signals"] = append(p["signals"].([]any),
				map[string]any{"address": "%IW8", "name": "H1", "description": "Height"},
				map[string]any{"address": "AI1", "name": "H2"}, // as older files name it
				map[string]any{"address": "%QW8", "name": "Y1"},
			)
		})},
	}
}

func resolveWith(files fstest.MapFS) (*Plan, error) {
	b, err := NewLibrary(files, catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		return nil, err
	}
	return b.Resolve(nil)
}

// Without saying, a PLC module has 16 bytes each way, and analogue channel n is an INT of 0
// to 27648 for 0 to 10 V at the word at byte 8 + 2n.
func TestAnalogueChannelsHaveAPlaceInTheImage(t *testing.T) {
	plan, err := resolveWith(analogFiles(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Images[0]; got != (ImagePlan{Inputs: 16, Outputs: 16}) {
		t.Errorf("image: %+v", got)
	}
	want := map[string]int{"%IW8": 0, "%IW10": 1, "%QW8": 0}
	for _, list := range [][]PLCAnalogPlan{plan.PLCAnalog, plan.PLCAnalogOut} {
		for _, a := range list {
			if ch, ok := want[a.Address.String()]; !ok || ch != a.Channel || a.Scaling.Raw != [2]float64{0, 27648} || a.Scaling.Range != [2]float64{0, 10} || a.Scaling.Unit != "V" {
				t.Errorf("channel %+v", a)
			}
			delete(want, a.Address.String())
		}
	}
	if len(want) > 0 {
		t.Errorf("missing %v", want)
	}
	scene := plan.Scene.PLC
	if len(scene.Analog) != 2 || scene.Analog[0].Name != "H1" || scene.Analog[0].Shown != "AI0" || scene.Analog[1].Name != "H2" || scene.Analog[1].Address != "%IW10" {
		t.Errorf("inputs: %+v", scene.Analog)
	}
	if len(scene.AnalogOutputs) != 1 || scene.AnalogOutputs[0].Name != "Y1" || scene.AnalogOutputs[0].Unit != "V" {
		t.Errorf("outputs: %+v", scene.AnalogOutputs)
	}
}

// A PLC module places its channels as it says: a REAL in a double word holds the value
// itself.
func TestAPLCModulePlacesItsAnalogueChannels(t *testing.T) {
	plan, err := resolveWith(analogFiles(t, map[string]any{
		"image": map[string]any{"inputs": 24, "outputs": 12},
		"analog": map[string]any{
			"inputs": []any{
				map[string]any{"pin": "UE1", "address": "%ID16", "type": "REAL", "range": []any{0, 100}, "unit": "mm"},
				map[string]any{"pin": "UE2", "address": "%IW12", "raw": []any{0, 32000}},
			},
			"outputs": []any{map[string]any{"pin": "UA1", "address": "%QW10"}},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Images[0] != (ImagePlan{Inputs: 24, Outputs: 12}) {
		t.Errorf("image: %+v", plan.Images[0])
	}
	in, ok := plan.Analog(0, true, 0)
	if !ok || in.Address.String() != "%ID16" || !in.Scaling.Real || in.Scaling.Unit != "mm" || in.Scaling.Range != [2]float64{0, 100} {
		t.Errorf("UE1: %+v", in)
	}
	if in2, _ := plan.Analog(0, true, 1); in2.Scaling.Raw != [2]float64{0, 32000} {
		t.Errorf("UE2: %+v", in2)
	}
	if out, _ := plan.Analog(0, false, 0); out.Address.String() != "%QW10" {
		t.Errorf("UA1: %+v", out)
	}
	// Its channels have no name, as nothing but the wire joins them.
	if len(plan.Scene.PLC.Analog) != 0 {
		t.Errorf("unnamed channels: %+v", plan.Scene.PLC.Analog)
	}
}

func TestBadAnalogueLayoutsAreRefused(t *testing.T) {
	layout := func(in ...any) map[string]any {
		return map[string]any{"analog": map[string]any{"inputs": in, "outputs": []any{map[string]any{"pin": "UA1", "address": "%QW8"}}}}
	}
	ue := func(pin, address string, more ...string) map[string]any {
		d := map[string]any{"pin": pin, "address": address}
		for i := 0; i+1 < len(more); i += 2 {
			d[more[i]] = more[i+1]
		}
		return d
	}
	for name, c := range map[string]struct {
		plc  map[string]any
		want string
	}{
		"over a connector's byte": {layout(ue("UE1", "%IW0"), ue("UE2", "%IW10")), "which a connector uses for bits"},
		"overlapping":             {layout(ue("UE1", "%IW8"), ue("UE2", "%IW9")), "overlap"},
		"outside the image":       {layout(ue("UE1", "%IW8"), ue("UE2", "%IW16")), "outside the image of 16 bytes"},
		"INT in a double word":    {layout(ue("UE1", "%ID8"), ue("UE2", "%IW12")), "takes a word"},
		"REAL in a word":          {layout(ue("UE1", "%IW8", "type", "REAL"), ue("UE2", "%IW12")), "takes a double word"},
		"unknown type":            {layout(ue("UE1", "%IW8", "type", "LREAL"), ue("UE2", "%IW12")), "INT or REAL"},
		"an output's place":       {layout(ue("UE1", "%QW8"), ue("UE2", "%IW12")), "is an input"},
		"not placed":              {layout(ue("UE1", "%IW8")), "does not place"},
		"too large an image":      {map[string]any{"image": map[string]any{"inputs": 65, "outputs": 8}}, "at most 64"},
		"a connector outside":     {map[string]any{"image": map[string]any{"inputs": 4, "outputs": 16}}, "outside the image of 4 bytes"},
	} {
		_, err := resolveWith(analogFiles(t, c.plc))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The schema of a PLC module's analogue layout reads as written.
func TestAnAnalogueLayoutReads(t *testing.T) {
	var d PLCModuleDef
	if err := json.Unmarshal([]byte(`{"image": {"inputs": 32, "outputs": 32}, "analog": {"inputs": [{"pin": "UE1", "address": "%IW16", "range": [0, 10], "raw": [0, 27648]}]}}`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Image.Inputs != 32 || d.Analog.Inputs[0].Raw[1] != 27648 {
		t.Errorf("%+v", d)
	}
	_ = plcio.ImageBytes
}
