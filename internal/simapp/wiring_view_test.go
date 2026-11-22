package simapp

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/toblu96/taktwin/catalog"
)

// The wiring of a running station can be looked at without building on it: a project on a
// collection's station shows its PLC, its I/O terminals and its cables, and still has no station
// of its own afterwards.
func TestTheWiringOfARunningStationIsShownWithoutBuilding(t *testing.T) {
	s, _ := newService(t)
	b := NewBuilderService(s)
	w, err := b.StationWiring()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, st := range w.Stations {
		for _, box := range st.Boxes {
			kinds[box.Kind]++
		}
	}
	if kinds["plc"] != 1 || kinds["terminal"] == 0 || len(w.Cables) == 0 {
		t.Errorf("it shows the PLC, the terminals and the cables: %v boxes, %d cables", kinds, len(w.Cables))
	}
	if p := s.Project(); p.Own {
		t.Errorf("the project builds on no station of its own: %+v", p)
	}
}

// An exercise that swaps two pins of a terminal shows them swapped in the wiring, as its
// station runs, though the station's file has them as they were.
func TestTheWiringShowsWhatAnExerciseSwapped(t *testing.T) {
	s, home := newService(t)
	plain, err := NewBuilderService(s).StationWiring()
	if err != nil {
		t.Fatal(err)
	}
	var ex map[string]any
	data, err := fs.ReadFile(catalog.Reference, "exercises/load-a-pallet/exercise.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &ex); err != nil {
		t.Fatal(err)
	}
	ex["id"], ex["name"] = "swapped", "Swapped"
	ex["changes"] = map[string]any{"pins": []map[string]string{{"terminal": "conveyor/io.T", "from": "I0", "to": "I3"}}}
	data, _ = json.Marshal(ex)
	dir := filepath.Join(home, "exercises", "swapped")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "exercise.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewProject("Swapped", Base{Exercise: "swapped"}); err != nil {
		t.Fatal(err)
	}
	swapped, err := NewBuilderService(s).StationWiring()
	if err != nil {
		t.Fatal(err)
	}
	pin := func(w WiringState, name string) string {
		for _, st := range w.Stations {
			for _, b := range st.Boxes {
				for _, c := range b.Connectors {
					for _, p := range c.Pins {
						if b.ID+"."+c.ID == "conveyor/io.T" && p.Pin == name {
							return p.Function
						}
					}
				}
			}
		}
		return "missing"
	}
	if pin(plain, "I0") == "missing" || pin(swapped, "I3") != pin(plain, "I0") || pin(swapped, "I0") != pin(plain, "I3") {
		t.Errorf("I0 and I3 are swapped: before %q and %q, after %q and %q", pin(plain, "I0"), pin(plain, "I3"), pin(swapped, "I0"), pin(swapped, "I3"))
	}
}
