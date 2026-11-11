package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The schemas in schema/ are those the types give: change a type, then run
// go run ./cmd/taktwin-schema.
func TestTheSchemasAreUpToDate(t *testing.T) {
	schemas, err := generate("../../internal/model")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range schemas {
		got, err := os.ReadFile(filepath.Join("..", "..", "schema", name))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("schema/%s is not up to date (%v): run go run ./cmd/taktwin-schema", name, err)
		}
	}
	files, _ := filepath.Glob(filepath.Join("..", "..", "schema", "*.json"))
	if len(files) != len(schemas) {
		t.Errorf("schema/ has %d files, the types give %d", len(files), len(schemas))
	}
}

// Every file of the built-in catalog and the collections in this repository fits its schema.
func TestTheFilesFitTheirSchemas(t *testing.T) {
	schemas, err := generate("../../internal/model")
	if err != nil {
		t.Fatal(err)
	}
	kinds := []struct{ schema, pattern string }{
		{"collection", "collection.json"},
		{"module", "*/module.json"},
		{"part", "parts/*.json"},
		{"station", "stations/*/station.json"},
		{"workpieces", "workpieces/*.json"},
		{"exercise", "exercises/*/exercise.json"},
		{"materials", "materials.json"},
		{"connectors", "connectors.json"},
	}
	roots, _ := filepath.Glob(filepath.Join("..", "..", "collections", "*"))
	roots = append(roots, filepath.Join("..", "..", "catalog", "base"), filepath.Join("..", "..", "catalog", "reference"))
	checked := 0
	for _, root := range roots {
		for _, k := range kinds {
			var s map[string]any
			if err := json.Unmarshal(schemas[k.schema+".schema.json"], &s); err != nil {
				t.Fatal(err)
			}
			patterns := []string{filepath.Join(root, k.pattern)}
			if k.schema == "module" {
				patterns = []string{filepath.Join(root, "modules", "*", "module.json"), filepath.Join(root, "accessories", "*", "module.json")}
			}
			for _, pattern := range patterns {
				found, _ := filepath.Glob(pattern)
				for _, f := range found {
					data, err := os.ReadFile(f)
					if err != nil {
						t.Fatal(err)
					}
					var v any
					if err := json.Unmarshal(data, &v); err != nil {
						t.Errorf("%s: %v", f, err)
						continue
					}
					if problem := validate(s, s, v, ""); problem != "" {
						t.Errorf("%s does not fit the %s schema: %s", f, k.schema, problem)
					}
					checked++
				}
			}
		}
	}
	if checked < 20 {
		t.Errorf("only %d files checked", checked)
	}
}

// validate checks a value against the parts of JSON Schema the schemas use, and says where it
// does not fit.
func validate(root, s map[string]any, v any, at string) string {
	if ref, ok := s["$ref"].(string); ok {
		def := root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		return validate(root, def, v, at)
	}
	if types, ok := s["type"]; ok {
		var list []string
		switch tt := types.(type) {
		case string:
			list = []string{tt}
		case []any:
			for _, x := range tt {
				list = append(list, x.(string))
			}
		}
		if !slices.ContainsFunc(list, func(name string) bool { return fits(name, v) }) {
			return fmt.Sprintf("%s is not %v", cmp.Or(at, "the file"), list)
		}
	}
	switch val := v.(type) {
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		for k, x := range val {
			if p, ok := props[k].(map[string]any); ok {
				if problem := validate(root, p, x, at+"/"+k); problem != "" {
					return problem
				}
				continue
			}
			switch extra := s["additionalProperties"].(type) {
			case bool:
				if !extra {
					return fmt.Sprintf("%s/%s is not a known property", at, k)
				}
			case map[string]any:
				if problem := validate(root, extra, x, at+"/"+k); problem != "" {
					return problem
				}
			}
		}
	case []any:
		if items, ok := s["items"].(map[string]any); ok {
			for i, x := range val {
				if problem := validate(root, items, x, fmt.Sprintf("%s[%d]", at, i)); problem != "" {
					return problem
				}
			}
		}
		if n, ok := s["minItems"].(float64); ok && float64(len(val)) < n {
			return fmt.Sprintf("%s has fewer than %v items", at, n)
		}
		if n, ok := s["maxItems"].(float64); ok && float64(len(val)) > n {
			return fmt.Sprintf("%s has more than %v items", at, n)
		}
	}
	return ""
}

func fits(name string, v any) bool {
	switch name {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == float64(int64(f))
	}
	return false
}

// The check finds what the app would refuse: an unknown property, or a value of a wrong type.
func TestTheSchemaFindsMistakes(t *testing.T) {
	schemas, err := generate("../../internal/model")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(schemas["module.schema.json"], &s); err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		`{"format": 2, "id": "m", "colour": "red"}`:                                      "/colour is not a known property",
		`{"format": 2, "id": "m", "model": [{"position": [0, 0]}]}`:                      "has fewer than 3 items",
		`{"format": 2, "id": "m", "devices": [{"type": "sensor", "reach": []}]}`:         "/devices[0]/reach is not",
		`{"format": 2, "id": "m", "parameters": [{"id": "l", "default": "length / 2"}]}`: "",
	} {
		var v any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			t.Fatal(err)
		}
		got := validate(s, s, v, "")
		if want == "" && got != "" || want != "" && !strings.Contains(got, want) {
			t.Errorf("%s: %q, want %q", text, got, want)
		}
	}
}
