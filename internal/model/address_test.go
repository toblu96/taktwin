package model

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/toblu96/taktwin/catalog"
)

// referenceData reads a file of the reference collection.
func referenceData(t *testing.T, path string) []byte {
	t.Helper()
	data, err := fs.ReadFile(catalog.Reference, path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// changedReference decodes a file of the reference collection, lets change edit it, and encodes it.
func changedReference(t *testing.T, path string, change func(map[string]any)) []byte {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(referenceData(t, path), &v); err != nil {
		t.Fatal(err)
	}
	change(v)
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// referencePlan resolves the reference station with files of the reference collection replaced.
func referencePlan(t *testing.T, files fstest.MapFS) *Plan {
	t.Helper()
	b, err := NewLibrary(files, catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := b.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

const referenceStation = "stations/pallet-loading/station.json"

// Older station files name signals as the PLC module names its variables, IB0Bit03 for
// %IX0.3. They give the same I/O table as the addresses do.
func TestStationsWithOlderAddressesGiveTheSameTable(t *testing.T) {
	now := referencePlan(t, nil)
	bit := regexp.MustCompile(`"%([IQ])X(\d+)\.(\d)"`)
	older := bit.ReplaceAll(referenceData(t, referenceStation), []byte(`"${1}B${2}Bit0${3}"`))
	if slices.Equal(older, referenceData(t, referenceStation)) {
		t.Fatal("the reference station names no signal by address")
	}
	then := referencePlan(t, fstest.MapFS{referenceStation: {Data: older}})
	for _, p := range []*Plan{now, then} {
		in := p.Scene.PLC.Inputs
		if len(in) == 0 || in[0].Address != "%IX0.0" || in[0].Shown != "IB0Bit00" || in[0].Name != "B1" {
			t.Fatalf("the first input is %+v", in)
		}
	}
	if !slices.Equal(now.Scene.PLC.Inputs, then.Scene.PLC.Inputs) || !slices.Equal(now.Scene.PLC.Outputs, then.Scene.PLC.Outputs) {
		t.Errorf("the tables differ:\n%+v\n%+v", now.Scene.PLC, then.Scene.PLC)
	}
	for _, in := range now.Scene.Instances {
		for _, c := range in.Connectors {
			for _, p := range c.Pins {
				if p.Address != "" && p.Shown == "" {
					t.Errorf("%s.%s.%s reaches %s, which is not shown", in.ID, c.ID, p.Pin, p.Address)
				}
			}
		}
	}
}

// A PLC module shows its addresses as its addressFormat says, whatever its program calls
// the variables.
func TestThePLCModuleShowsItsAddresses(t *testing.T) {
	const cabinet = "accessories/control-cabinet/module.json"
	plan := referencePlan(t, fstest.MapFS{cabinet: {Data: changedReference(t, cabinet, func(m map[string]any) {
		m["plc"] = map[string]any{"addressFormat": map[string]any{"input": "E{byte}.{bit}", "output": "A{byte}.{bit}"}}
	})}})
	if got := plan.Scene.PLC.Inputs[0]; got.Address != "%IX0.0" || got.Shown != "E0.0" {
		t.Errorf("the first input is %s, shown as %s", got.Address, got.Shown)
	}
	if got := plan.Scene.PLC.Outputs[0]; got.Shown != "A0.0" {
		t.Errorf("the first output is shown as %s", got.Shown)
	}
}

// A module can give its pins a symbol, which names their signal where the station names
// none. A symbol is not taken twice.
func TestModulesGiveTheirPinsSymbols(t *testing.T) {
	const sensor = "accessories/photo-sensor/module.json"
	plan := referencePlan(t, fstest.MapFS{
		referenceStation: {Data: changedReference(t, referenceStation, func(st map[string]any) {
			delete(st["plc"].(map[string]any), "signals")
		})},
		sensor: {Data: changedReference(t, sensor, func(m map[string]any) {
			pins := m["terminal"].(map[string]any)["connectors"].([]any)[0].(map[string]any)["pins"].([]any)
			pins[0].(map[string]any)["symbol"] = "B_Beam"
		})},
	})
	var symbol, fallback int
	for _, s := range plan.Scene.PLC.Inputs {
		switch {
		case s.Name == "B_Beam":
			symbol++
		case regexp.MustCompile(`^[a-z/]+\.[A-Za-z0-9]+$`).MatchString(s.Name):
			fallback++
		}
	}
	if symbol != 1 || fallback == 0 {
		t.Errorf("one input is named by the symbol (%d), the others after their pin (%d): %+v", symbol, fallback, plan.Scene.PLC.Inputs)
	}
}

// Signals named the older way follow their devices when an exercise swaps pins, and are
// then named by address.
func TestSignalsWithOlderAddressesFollowSwappedPins(t *testing.T) {
	bit := regexp.MustCompile(`"%([IQ])X(\d+)\.(\d)"`)
	older := bit.ReplaceAll(referenceData(t, referenceStation), []byte(`"${1}B${2}Bit0${3}"`))
	b, err := NewLibrary(fstest.MapFS{referenceStation: {Data: older}}, catalog.Reference, catalog.Base).Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Change(StationChanges{Pins: []PinSwap{{Terminal: "conveyor/io.T", From: "I0", To: "I3"}}}); err != nil {
		t.Fatal(err)
	}
	plan, err := b.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	var b1 SceneSignal
	for _, s := range plan.Scene.PLC.Inputs {
		if s.Name == "B1" {
			b1 = s
		}
	}
	if b1.Address != "%IX0.3" {
		t.Errorf("B1 moved with its sensor to %%IX0.3, not %q", b1.Address)
	}
}
