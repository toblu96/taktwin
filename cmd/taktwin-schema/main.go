// Command taktwin-schema writes the JSON Schemas of the files a collection is made of into
// schema/, from the Go types the app reads them into and the doc comments on those types.
// The app reads the files strictly, so the schemas allow no other properties. A test keeps
// them in step with the types:
//
//	go run ./cmd/taktwin-schema
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/toblu96/taktwin/internal/model"
)

// baseURL is where the schemas are published, so that files elsewhere can name them.
const baseURL = "https://raw.githubusercontent.com/toblu96/taktwin/main/schema/"

// others are the packages next to the model whose types the files hold too.
var others = []string{"plcio"}

// file is one kind of file of a collection and the type it is read into.
type file struct {
	name, title, description string
	root                     reflect.Type
	// entries makes it an object of named entries of root, as materials.json.
	entries bool
}

var files = []file{
	{"collection", "Collection", "collection.json at the root of a collection: its id, name, version, and the station and workpieces it opens with.", reflect.TypeFor[model.CollectionDef](), false},
	{"module", "Module", "modules/<id>/module.json or accessories/<id>/module.json: a unit that is screwed onto a site of another one, with its 3D model, joints, devices, transports, terminal, sites and mounts.", reflect.TypeFor[model.ModuleDef](), false},
	{"part", "Part", "parts/<id>.json: shared 3D geometry that modules and workpieces draw with a node's part.", reflect.TypeFor[model.PartDef](), false},
	{"station", "Station", "stations/<id>/station.json: which modules a station is made of, where they are screwed on, how they are cabled and wired, and the names of the PLC's signals.", reflect.TypeFor[model.StationDef](), false},
	{"workpieces", "Workpiece set", "workpieces/<id>.json: the shape and kinds of the workpieces a station works with.", reflect.TypeFor[model.WorkpieceSet](), false},
	{"exercise", "Exercise", "exercises/<id>/exercise.json: a task on a station, and the check of a run of it, line by line.", reflect.TypeFor[model.ExerciseDef](), false},
	{"materials", "Materials", "materials.json: the materials that nodes name, by name. Collections add theirs to the others'.", reflect.TypeFor[model.Material](), true},
	{"connectors", "Connector types", "connectors.json: the types of connector, by name, with how many inputs and outputs they carry. Collections add theirs to the others'.", reflect.TypeFor[model.ConnectorType](), true},
}

func main() {
	src := flag.String("src", "internal/model", "the package with the types and their doc comments")
	out := flag.String("out", "schema", "the folder to write the schemas into")
	flag.Parse()
	schemas, err := generate(*src)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for name, data := range schemas {
		if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

// generate gives each schema's file name and content.
func generate(src string) (map[string][]byte, error) {
	docs, err := readDocs(src)
	if err != nil {
		return nil, err
	}
	// The files also hold types of the packages next to the model's, such as plcio's Naming.
	for _, pkg := range others {
		more, err := readDocs(filepath.Join(filepath.Dir(src), pkg))
		if err != nil {
			return nil, err
		}
		for k, v := range more {
			if _, ok := docs[k]; !ok {
				docs[k] = v
			}
		}
	}
	out := map[string][]byte{}
	for _, f := range files {
		g := &generator{docs: docs, defs: map[string]any{}}
		ref := g.schema(f.root)
		s := map[string]any{
			"$schema":     "https://json-schema.org/draft/2020-12/schema",
			"$id":         baseURL + f.name + ".schema.json",
			"title":       f.title,
			"description": f.description,
		}
		if f.entries {
			s["type"] = "object"
			s["additionalProperties"] = ref
		} else {
			s["$ref"] = ref["$ref"]
		}
		s["$defs"] = g.defs
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(s); err != nil {
			return nil, err
		}
		out[f.name+".schema.json"] = buf.Bytes()
	}
	return out, nil
}

// readDocs reads the doc comments of the package's types and their fields, as "Type" and
// "Type.Field".
func readDocs(dir string) (map[string]string, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	docs := map[string]string{}
	text := func(groups ...*ast.CommentGroup) string {
		for _, g := range groups {
			if t := strings.Join(strings.Fields(g.Text()), " "); g != nil && t != "" {
				return t
			}
		}
		return ""
	}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts := spec.(*ast.TypeSpec)
					if d := text(nonNil(ts.Doc, gen.Doc)...); d != "" {
						docs[ts.Name.Name] = d
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, field := range st.Fields.List {
						d := text(nonNil(field.Doc, field.Comment)...)
						for _, n := range field.Names {
							if d != "" {
								docs[ts.Name.Name+"."+n.Name] = d
							}
						}
					}
				}
			}
		}
	}
	return docs, nil
}

func nonNil(groups ...*ast.CommentGroup) []*ast.CommentGroup {
	var out []*ast.CommentGroup
	for _, g := range groups {
		if g != nil {
			out = append(out, g)
		}
	}
	return out
}

type generator struct {
	docs map[string]string
	defs map[string]any
}

var (
	numType = reflect.TypeFor[model.Num]()
	rawType = reflect.TypeFor[json.RawMessage]()
)

// schema gives the schema of a type, adding named structs to the definitions.
func (g *generator) schema(t reflect.Type) map[string]any {
	switch t {
	case numType:
		if _, ok := g.defs["Num"]; !ok {
			g.defs["Num"] = map[string]any{
				"description": "A number, an expression over the module's parameters as a string, such as \"length - 47\" or \"radius * sin(45)\", or true or false for 1 or 0. Angles are degrees.",
				"type":        []string{"number", "string", "boolean"},
			}
		}
		return map[string]any{"$ref": "#/$defs/Num"}
	case rawType:
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.schema(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Array:
		return map[string]any{"type": "array", "items": g.schema(t.Elem()), "minItems": t.Len(), "maxItems": t.Len()}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		name := t.Name()
		if name == "" {
			// An unnamed struct, written where it is used.
			props := map[string]any{}
			g.fields(t, "", props)
			return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
		}
		if _, ok := g.defs[name]; !ok {
			g.defs[name] = nil // being written, for types that contain themselves
			def := map[string]any{"type": "object", "additionalProperties": false}
			if d := g.docs[name]; d != "" {
				def["description"] = d
			}
			props := map[string]any{}
			g.fields(t, name, props)
			def["properties"] = props
			g.defs[name] = def
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	}
	panic(fmt.Sprintf("no schema for %v", t))
}

// fields adds the properties of a struct's fields, and of the structs it embeds.
func (g *generator) fields(t reflect.Type, name string, props map[string]any) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tag == "-" || !f.IsExported() {
			continue
		}
		if f.Anonymous && tag == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			g.fields(ft, ft.Name(), props)
			continue
		}
		if tag == "" {
			tag = f.Name
		}
		s := g.schema(f.Type)
		if d := g.docs[name+"."+f.Name]; d != "" {
			// A reference can carry a description of its own since draft 2019-09.
			s = withDescription(s, d)
		}
		props[tag] = s
	}
}

func withDescription(s map[string]any, d string) map[string]any {
	out := map[string]any{"description": d}
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = s[k]
	}
	return out
}
