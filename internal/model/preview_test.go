package model

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/toblu96/taktwin/catalog"
)

// TestAModuleIsTriedOutWithWhatItWasMadeFor shows that a module previewed on its own meets the
// workpieces of its own collection, even when another collection in front of it brings other
// ones, and that its transport offers only the kinds its own actions take: cartons on the
// carton conveyor, no pallets.
func TestAModuleIsTriedOutWithWhatItWasMadeFor(t *testing.T) {
	other := fstest.MapFS{
		"collection.json": {Data: []byte(`{"id": "other", "name": "Other", "workpieces": "caps"}`)},
		"workpieces/caps.json": {Data: []byte(`{"format": 2, "id": "caps", "diameter": 40, "height": 30, "points": [],
			"kinds": [{"id": "cap", "label": "Cap", "material": {"type": "MeshLambertMaterial", "color": "#ffffff"}}]}`)},
	}
	lib := NewLibrary(other, catalog.Reference, catalog.Base)
	if lib.DefaultWorkpieces() != "caps" {
		t.Fatalf("the test needs the other collection's workpieces to come first, not %q", lib.DefaultWorkpieces())
	}
	b, err := lib.Preview(LevelModule, "carton-conveyor")
	if err != nil {
		t.Fatal(err)
	}
	if b.Station.Workpieces != "pallet-loads" {
		t.Errorf("the carton conveyor is tried out with %q, not with its own collection's pallet-loads", b.Station.Workpieces)
	}
	plan, err := b.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	var placing []SceneTest
	for _, in := range plan.Scene.Instances {
		for _, test := range in.Tests {
			if !test.Holds {
				placing = append(placing, test)
			}
		}
	}
	if len(placing) == 0 {
		t.Fatal("the carton conveyor offers no transport to place on")
	}
	for _, test := range placing {
		if !slices.Equal(test.Kinds, []string{"carton-large", "carton-small"}) {
			t.Errorf("%s offers %v, not the two cartons", test.Label, test.Kinds)
		}
	}
}
