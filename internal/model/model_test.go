package model

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing/fstest"

	"testing"

	"github.com/toblu96/taktwin/catalog"
)

func TestExpressions(t *testing.T) {
	vars := env{"a": 4, "b": 2}
	for src, want := range map[string]float64{
		`3`: 3, `-2.5`: -2.5, `true`: 1, `false`: 0, `"a + b * 3"`: 10, `"(a + b) / -2"`: -3, `"sin(90) * a"`: 4, `"asin(1)"`: 90,
		`"a > b && !(b == 3)"`: 1, `"a < b || false"`: 0, `"max(a, b) - min(a, 1) + abs(-1)"`: 4, `"a != 4"`: 0,
	} {
		var n Num
		if err := json.Unmarshal([]byte(src), &n); err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		got, err := n.eval(vars)
		if err != nil || got != want {
			t.Errorf("%s = %v, %v, want %v", src, got, err, want)
		}
	}
	for _, src := range []string{`"nope + 1"`, `"a / 0"`, `"a +"`, `"os.Exit(1)"`, `"a = b"`, `"max(a)"`, `"sqrt(-1)"`, `"x.y"`} {
		var n Num
		if err := json.Unmarshal([]byte(src), &n); err != nil {
			continue
		}
		if _, err := n.eval(vars); err == nil {
			t.Errorf("%s should fail", src)
		}
	}
}

func moduleFile(module string) *Library {
	return NewLibrary(fstest.MapFS{
		"stations/x/station.json": {Data: []byte(`{"format":5,"id":"x","name":"X","workpieces":"pallet-loads","modules":[{"id":"m","module":"bad"}]}`)},
		"modules/bad/module.json": {Data: []byte(`{"format":2,"id":"bad","version":1,"name":"Bad",` + module + `}`)},
	}, catalog.Reference, catalog.Base)
}

func TestInvalidModulesGiveClearErrors(t *testing.T) {
	cyl := `"devices":[{"type":"cylinder","id":"c","valve":"monostable","rest":"retracted","stroke":1,"speed":1}]`
	cases := map[string]struct{ file, want string }{
		"reserved parameter":  {`"parameters":[{"id":"range","default":1}]`, `"range" is reserved`},
		"unknown geometry":    {`"model":[{"geometry":{"type":"TorusKnotGeometry"},"material":"dark"}]`, "TorusKnotGeometry"},
		"unknown material":    {`"model":[{"geometry":{"type":"BoxGeometry"},"material":"nope"}]`, "nope"},
		"joint without node":  {cyl + `,"joints":[{"node":"gone","type":"prismatic","driver":"c.position"}]`, "gone"},
		"joint bad driver":    {`"model":[{"name":"n"}],"joints":[{"node":"n","type":"prismatic","driver":"x.y"}]`, "x.y"},
		"lamp without node":   {`"devices":[{"type":"lamp","id":"l","node":"gone","on":"#fff","off":"#000"}]`, "gone"},
		"no valve":            {`"devices":[{"type":"cylinder","id":"c","rest":"retracted","stroke":1,"speed":1}]`, "valve"},
		"no stroke":           {`"devices":[{"type":"cylinder","id":"c","valve":"bistable","rest":"retracted"}]`, "stroke"},
		"unknown device":      {`"devices":[{"type":"warp","id":"w"}]`, "warp"},
		"pin not on type":     {`"terminal":{"connectors":[{"id":"T","type":"spring-4x4","pins":[{"pin":"I9"}]}]}`, "I9"},
		"unknown connector":   {`"terminal":{"connectors":[{"id":"T","type":"hdmi"}]}`, "hdmi"},
		"bridge misfit":       {`"terminal":{"connectors":[{"id":"A","type":"spring-8x8"},{"id":"B","type":"spring-4x4"}],"bridges":[{"a":"A","b":"B"}]}`, "does not fit"},
		"pin bad input":       {`"terminal":{"connectors":[{"id":"W","type":"wires","pins":[{"pin":"p","drives":"nope.run"}]}]}`, "nope.run"},
		"pin bad state":       {`"terminal":{"connectors":[{"id":"W","type":"wires","pins":[{"pin":"p","reads":"nope.on"}]}]}`, "nope.on"},
		"unknown action":      {`"controls":[{"type":"button","action":"gone"}]`, "gone"},
		"unknown widget":      {`"controls":[{"type":"hologram"}]`, "hologram"},
		"action no target":    {`"actions":[{"id":"a","type":"clearTransport","transport":"gone"}]`, "gone"},
		"mounted on nothing":  {`"modules":[{"id":"r","module":"photo-sensor","mountedOn":"gone"}]`, "gone"},
		"unknown action type": {`"transport":[{"id":"t","speed":1,"path":{"line":{"from":[0,0,0],"to":[1,0,0]}}}],"actions":[{"id":"a","type":"explode","transport":"t"}]`, "explode"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := moduleFile(c.file).Load("x")
			if err == nil {
				_, err = b.Resolve(nil)
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q should mention %q", err, c.want)
			}
		})
	}
}

func TestPartThatIncludesItselfIsRefused(t *testing.T) {
	lib := NewLibrary(fstest.MapFS{
		"stations/x/station.json":  {Data: []byte(`{"format":5,"id":"x","name":"X","workpieces":"pallet-loads","modules":[{"id":"m","module":"loop"}]}`)},
		"modules/loop/module.json": {Data: []byte(`{"format":2,"id":"loop","version":1,"name":"Loop","model":[{"part":"a"}]}`)},
		"parts/a.json":             {Data: []byte(`{"format":2,"id":"a","name":"A","model":[{"children":[{"part":"b"}]}]}`)},
		"parts/b.json":             {Data: []byte(`{"format":2,"id":"b","name":"B","model":[{"part":"a"}]}`)},
	}, catalog.Reference, catalog.Base)
	if _, err := lib.Load("x"); err == nil || !strings.Contains(err.Error(), "includes itself") {
		t.Errorf("got %v", err)
	}
}

func TestPackageModuleShadowsCatalog(t *testing.T) {
	pkg := fstest.MapFS{
		"stations/x/station.json":        {Data: []byte(`{"format":5,"id":"x","name":"X","workpieces":"pallet-loads","modules":[{"id":"g","module":"guide-rail"}]}`)},
		"modules/guide-rail/module.json": {Data: []byte(`{"format":2,"id":"guide-rail","version":2,"name":"Refined rail"}`)},
	}
	b, err := NewLibrary(pkg, catalog.Reference, catalog.Base).Load("x")
	if err != nil {
		t.Fatal(err)
	}
	if b.Modules["guide-rail"].Version != 2 {
		t.Errorf("package module should win, got version %d", b.Modules["guide-rail"].Version)
	}
}

func TestModuleThatContainsItselfIsRefused(t *testing.T) {
	lib := NewLibrary(fstest.MapFS{
		"stations/x/station.json":   {Data: []byte(`{"format":5,"id":"x","name":"X","workpieces":"pallet-loads","modules":[{"id":"a","module":"outer"}]}`)},
		"modules/outer/module.json": {Data: []byte(`{"format":2,"id":"outer","version":1,"name":"O","modules":[{"id":"i","module":"inner"}]}`)},
		"modules/inner/module.json": {Data: []byte(`{"format":2,"id":"inner","version":1,"name":"I","modules":[{"id":"o","module":"outer"}]}`)},
	}, catalog.Reference, catalog.Base)
	if _, err := lib.Load("x"); err == nil || !strings.Contains(err.Error(), "contains itself") {
		t.Errorf("got %v", err)
	}
}

func TestEntriesListABrokenFileWithItsError(t *testing.T) {
	lib := NewLibrary(fstest.MapFS{"accessories/broken/module.json": {Data: []byte(`{"nope"`)}}, catalog.Reference, catalog.Base)
	for _, e := range lib.Entries() {
		if e.ID == "broken" {
			if e.Error == "" {
				t.Error("it should say why it cannot be read")
			}
			return
		}
	}
	t.Error("the broken file is not listed")
}

func TestEveryEntryCanBePreviewedOnItsOwn(t *testing.T) {
	lib := NewLibrary(catalog.Reference, catalog.Base)
	for _, e := range lib.Entries() {
		b, err := lib.Preview(e.Level, e.ID)
		if err != nil {
			t.Errorf("%s %s: %v", e.Level, e.ID, err)
			continue
		}
		plan, err := b.Resolve(nil)
		if err != nil {
			t.Errorf("%s %s: %v", e.Level, e.ID, err)
			continue
		}
		if len(plan.Scene.Instances) == 0 {
			t.Errorf("%s %s has nothing to show", e.Level, e.ID)
		}
	}
}

func TestSettingsSayWhatKindTheyAre(t *testing.T) {
	err := fs.WalkDir(catalog.Reference, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Name() != "module.json" {
			return err
		}
		data, err := fs.ReadFile(catalog.Reference, path)
		if err != nil {
			return err
		}
		var mod ModuleDef
		if err := json.Unmarshal(data, &mod); err != nil {
			return err
		}
		for _, p := range mod.Parameters {
			if p.Label == "" {
				continue
			}
			if p.Kind != KindBuild && p.Kind != KindPhysics {
				t.Errorf("%s: setting %s is of kind %q, not build or physics", path, p.ID, p.Kind)
			}
			// Every setting says what it does, and keeps within limits of its own.
			if len(p.Description) < 20 {
				t.Errorf("%s: setting %s does not say what it does", path, p.ID)
			}
			if !p.Default.Flag() && len(p.Options) == 0 && (p.Min == nil || p.Max == nil) {
				t.Errorf("%s: setting %s has no limits", path, p.ID)
			}
			if len(p.Names) > 0 && len(p.Names) != len(p.Options) {
				t.Errorf("%s: setting %s names %d of its %d options", path, p.ID, len(p.Names), len(p.Options))
			}
			if v, err := mod.Value(p.Default, nil); err != nil || p.Allows(v) != nil {
				t.Errorf("%s: setting %s starts outside its limits: %v %v", path, p.ID, err, p.Allows(v))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
