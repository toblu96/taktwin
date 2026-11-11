package simapp

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/toblu96/taktwin/internal/engine"
)

// The builder on the app's own collection and on modules of people's own. Tests of the
// stations of other collections go with those collections.

// newBuilder gives a builder with a user folder of its own.
func newBuilder(t *testing.T) *BuilderService {
	t.Helper()
	t.Setenv("TAKTWIN_HOME", t.TempDir())
	s := &BuilderService{ctx: context.Background(), emit: func(engine.Snapshot) {}}
	t.Cleanup(s.Close)
	return s
}

// module is the module of the state with that id, or nil.
func module(st BuilderState, id string) *BuilderModule {
	for i := range st.Modules {
		if st.Modules[i].ID == id {
			return &st.Modules[i]
		}
	}
	return nil
}

// writeModule puts a module of people's own into the user folder.
func writeModule(t *testing.T, id, data string) {
	t.Helper()
	dir := filepath.Join(UserFolder(), "accessories", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// cabled says whether the station has a cable between the two connectors.
func cabled(d *draft, from, to string) bool {
	return slices.ContainsFunc(d.Cables, func(c draftJoin) bool { return c.From == from && c.To == to })
}

// A station can stand on a module of people's own, with its own PLC, as the builder goes by
// what modules are and not by their ids.
func TestTheBuilderGoesByRoles(t *testing.T) {
	s := newBuilder(t)
	writeModule(t, "work-table", `{
	  "format": 2, "id": "work-table", "version": 1, "name": "Work table", "role": "station",
	  "parameters": [{ "id": "length", "default": 500, "label": "Length", "unit": "mm", "kind": "build" }, { "id": "lamp", "default": true }],
	  "floor": { "width": "length", "depth": 600, "beside": { "lamp": false } },
	  "sites": [{ "id": "shelf", "type": "controller-shelf", "from": [100, 100, 300], "range": [0, 0] }, { "id": "front", "type": "table-front", "from": [250, 0, 700], "range": [0, 0] }]
	}`)
	writeModule(t, "table-panel", `{
	  "format": 2, "id": "table-panel", "version": 1, "name": "Panel", "role": "console",
	  "mounts": [{ "id": "front", "type": "table-front", "at": [0, 0, 0] }],
	  "terminal": { "connectors": [{ "id": "P", "type": "m23-io", "prefer": "B" }] }
	}`)
	writeModule(t, "small-plc", `{
	  "format": 2, "id": "small-plc", "version": 1, "name": "Small PLC", "role": "plc",
	  "addresses": { "inputs": "IB{byte}Bit{bit:2}", "outputs": "QB{byte}Bit{bit:2}", "list": "Bits" },
	  "mounts": [{ "id": "shelf", "type": "controller-shelf", "at": [0, 0, 0] }],
	  "terminal": { "connectors": [{ "id": "A", "type": "m23-io", "byte": 0 }, { "id": "B", "type": "m23-io", "byte": 1 }] }
	}`)
	station := filepath.Join(UserFolder(), "stations", "table")
	if err := os.MkdirAll(station, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(station, "station.json"), []byte(`{
	  "format": 5, "id": "table", "name": "Table", "workpieces": "pallet-loads",
	  "modules": [{ "id": "table", "module": "work-table" }]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := s.Start("table")
	if err != nil {
		t.Fatal(err)
	}
	if table := module(st, "table"); table == nil || table.Place == nil || table.Place.On != floorSite || table.Floor == nil || table.Floor.Width != 500 {
		t.Fatalf("the table stands on the floor, 500 mm wide: %+v", st.Modules)
	}
	if len(st.Scene.Checks) == 0 || st.Scene.Checks[0].Text != "The station has no PLC: put one onto its stand" {
		t.Errorf("the checks miss a PLC: %+v", st.Scene.Checks)
	}
	if st, err = s.AddAt("small-plc", BuilderPlace{On: "table.shelf"}); err != nil {
		t.Fatal(err)
	}
	if s.draft.PLC.Module != "plc" {
		t.Errorf("the small PLC is the station's: %q", s.draft.PLC.Module)
	}
	// The panel's socket goes where it prefers, not to the first one.
	if _, err = s.AddAt("table-panel", BuilderPlace{On: "table.front"}); err != nil {
		t.Fatal(err)
	}
	if !cabled(s.draft, "table-panel.P", "plc.B") {
		t.Errorf("the panel is cabled to B of its PLC: %+v", s.draft.Cables)
	}
	if _, err := s.Remove("table"); err == nil {
		t.Error("the table with the PLC stays")
	}

	// Beside it: another table, with the settings its floor gives one there.
	if st, err = s.AddStand("table", 1); err != nil {
		t.Fatal(err)
	}
	other := module(st, "work-table")
	if other == nil || other.Place.At != 500 || !other.Optional {
		t.Fatalf("another table stands right of the first: %+v", other)
	}
	if v := s.draft.Modules[slices.IndexFunc(s.draft.Modules, func(m draftModule) bool { return m.ID == "work-table" })].Params["lamp"]; string(v) != "false" {
		t.Errorf("the table beside has its lamp off: %s", v)
	}
	w, err := s.Wiring()
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Stations) != 2 || w.Stations[0].ID != "table" || !slices.ContainsFunc(w.Stations[0].Boxes, func(b WiringBox) bool { return b.ID == "plc" && b.Kind == "plc" }) {
		t.Errorf("the wiring shows both tables, the PLC in the first: %+v", w.Stations)
	}
}

// The reference station opens in the builder as it is, and what is changed is saved as a
// station of the user's own, which opens again the same.
func TestBuildingOnTheReferenceStation(t *testing.T) {
	s := newBuilder(t)
	st, err := s.Start("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"base", "cabinet", "console", "conveyor", "magazine", "cartons", "gantry"} {
		if module(st, id) == nil {
			t.Fatalf("the station has %s: %+v", id, st.Modules)
		}
	}
	if len(st.Scene.Checks) != 0 {
		t.Errorf("nothing is missing: %+v", st.Scene.Checks)
	}
	w, err := s.Wiring()
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Stations) != 1 || !slices.ContainsFunc(w.Stations[0].Boxes, func(b WiringBox) bool { return b.ID == "cabinet" && b.Kind == "plc" }) {
		t.Errorf("the wiring shows one station with the cabinet as its PLC: %+v", w.Stations)
	}

	// The console moves along the rails.
	place := *module(st, "console").Place
	place.At -= 100
	if st, err = s.Move("console", place); err != nil {
		t.Fatal(err)
	}
	if got := module(st, "console").Place.At; got != place.At {
		t.Errorf("the console stands at %v, not %v", got, place.At)
	}
	// Signals are named by the cabinet's addresses.
	if _, err = s.SetSignal("", "IB0Bit00", "palletIn"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetSignal("", "EB0Bit00", "palletIn"); err == nil || !strings.Contains(err.Error(), "no signal EB0Bit00") {
		t.Errorf("the cabinet has no input named EB0Bit00: %v", err)
	}
	if _, err = s.SetStation("My pallet line", ""); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Save(false)
	if err != nil {
		t.Fatal(err)
	}
	id := filepath.Base(filepath.Dir(saved))
	if !slices.Contains(s.UserStations(), id) {
		t.Fatalf("%s is among the user's stations: %v", id, s.UserStations())
	}

	again := newBuilderIn(t, UserFolder())
	st, err = again.Start(id)
	if err != nil {
		t.Fatal(err)
	}
	if got := module(st, "console").Place.At; got != place.At {
		t.Errorf("the saved console stands at %v, not %v", got, place.At)
	}
	if !slices.ContainsFunc(again.draft.PLC.Signals, func(sig draftSignal) bool { return sig.Address == "IB0Bit00" && sig.Name == "palletIn" }) {
		t.Errorf("the signal keeps its name: %+v", again.draft.PLC.Signals)
	}
}

// newBuilderIn gives a builder on a user folder that is there already.
func newBuilderIn(t *testing.T, folder string) *BuilderService {
	t.Helper()
	t.Setenv("TAKTWIN_HOME", folder)
	s := &BuilderService{ctx: context.Background(), emit: func(engine.Snapshot) {}}
	t.Cleanup(s.Close)
	return s
}

// A new station of the app's collection stands on its base plate, with a PLC.
func TestANewStationStandsOnItsBase(t *testing.T) {
	s := newBuilder(t)
	st, err := s.Start("empty:taktwin")
	if err != nil {
		t.Fatal(err)
	}
	if module(st, "base-plate") == nil {
		t.Fatalf("the station stands on a base plate: %+v", st.Modules)
	}
	plc, console := module(st, s.draft.PLC.Module), module(st, "operator-console")
	if plc == nil || plc.Module != "control-cabinet" || console == nil {
		t.Fatalf("the station has a cabinet and a console: %+v", st.Modules)
	}
	if plc.Place.Slot == console.Place.Slot && math.Abs(plc.Place.At-console.Place.At) < 500 {
		t.Errorf("the console stands apart from the cabinet: %+v %+v", plc.Place, console.Place)
	}
	if !slices.ContainsFunc(s.draft.Cables, func(c draftJoin) bool {
		return strings.HasPrefix(c.From, "operator-console.") && strings.HasPrefix(c.To, plc.ID+".")
	}) {
		t.Errorf("the console is cabled to the cabinet: %+v", s.draft.Cables)
	}
	if len(st.Scene.Checks) != 0 {
		t.Errorf("nothing is missing: %+v", st.Scene.Checks)
	}
	if _, err := s.Add("pallet-conveyor"); err != nil {
		t.Fatal(err)
	}
}
