package model

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/toblu96/taktwin/st"
	"io"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// PackageExtension is the file extension of a collection, or of a station package, as a
// file: a zip with the same folders as the catalog.
const PackageExtension = ".taktwin"

// CollectionFile is the manifest at the root of a collection: a library layer of stations,
// modules, accessories, parts, workpieces and exercises that is installed, updated and
// removed as a whole, such as the app's own reference models. A layer without one, such as
// the user's own stations, belongs to no collection.
const CollectionFile = "collection.json"

// CollectionDef is the content of collection.json.
type CollectionDef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	// Station is the station the app opens first, if this is the first collection to name one.
	Station string `json:"station,omitempty"`
	// Workpieces are what a new station and a module tried out on its own work with, if
	// this is the first collection to name them.
	Workpieces string `json:"workpieces,omitempty"`
}

// ReadCollection reads the manifest of a layer; ok is false for a layer without one.
func ReadCollection(layer fs.FS) (c CollectionDef, ok bool, err error) {
	data, err := fs.ReadFile(layer, CollectionFile)
	if errors.Is(err, fs.ErrNotExist) {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, false, fmt.Errorf("%s: %w", CollectionFile, err)
	}
	if !safeID.MatchString(c.ID) {
		return c, false, fmt.Errorf("%s: invalid collection id %q", CollectionFile, c.ID)
	}
	return c, true, nil
}

// Library finds model files in layers. Earlier layers win, so a package's own
// module shadows the catalog's module with the same id.
type Library struct {
	layers []fs.FS
	// collections are the layers' collection ids, "" for a layer of none.
	collections []string
}

// NewLibrary returns a library over the given layers.
func NewLibrary(layers ...fs.FS) *Library {
	l := &Library{layers: layers}
	for _, layer := range layers {
		c, _, _ := ReadCollection(layer)
		l.collections = append(l.collections, c.ID)
	}
	return l
}

// collectionOf is the collection of the layer a file comes from, the front one that has it.
func (l *Library) collectionOf(name string) string {
	for i, layer := range l.layers {
		if _, err := fs.Stat(layer, name); err == nil {
			return l.collections[i]
		}
	}
	return ""
}

// ReadPackage opens station package data as a library layer.
func ReadPackage(r io.ReaderAt, size int64) (fs.FS, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("not a station package: %w", err)
	}
	return z, nil
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ReadFile returns a file as the first layer that has it holds it, such as a station's
// station.json, for copying it.
func (l *Library) ReadFile(name string) ([]byte, error) {
	for _, layer := range l.layers {
		data, err := fs.ReadFile(layer, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return data, err
	}
	return nil, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
}

// Folder reads the files of a folder from the first collection that has it, such as the
// example program a collection has for a station's PLC. Layers of no collection, such as the
// user's own folder, where the projects of the programs are, are left out. It gives none for a
// folder no collection has.
func (l *Library) Folder(dir string) (map[string][]byte, error) {
	for i, layer := range l.layers {
		if l.collections[i] == "" {
			continue
		}
		items, err := fs.ReadDir(layer, dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out := map[string][]byte{}
		for _, it := range items {
			if it.IsDir() {
				continue
			}
			data, err := fs.ReadFile(layer, path.Join(dir, it.Name()))
			if err != nil {
				return nil, err
			}
			out[it.Name()] = data
		}
		return out, nil
	}
	return nil, nil
}

// With returns a library with another layer in front, such as a station being edited.
func (l *Library) With(layer fs.FS) *Library {
	return NewLibrary(append([]fs.FS{layer}, l.layers...)...)
}

func (l *Library) read(name string, into any) error {
	for _, layer := range l.layers {
		data, err := fs.ReadFile(layer, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(into); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
	return fmt.Errorf("%s: %w", name, fs.ErrNotExist)
}

// readMerged reads a JSON object of entries from every layer that has the file, an earlier
// layer's entry winning over a later one's, so that a collection adds connector types or
// materials to the others instead of hiding them.
func readMerged[T any](l *Library, name string, into *map[string]T) error {
	out := map[string]T{}
	found := false
	for i := len(l.layers) - 1; i >= 0; i-- {
		data, err := fs.ReadFile(l.layers[i], name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var entries map[string]T
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&entries); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		maps.Copy(out, entries)
		found = true
	}
	if !found {
		return fmt.Errorf("%s: %w", name, fs.ErrNotExist)
	}
	*into = out
	return nil
}

// Collection is the manifest of a collection of the library, by its id.
func (l *Library) Collection(id string) (CollectionDef, bool) {
	for _, c := range l.Collections() {
		if c.ID == id {
			return c, true
		}
	}
	return CollectionDef{}, false
}

// StationCollection is the collection a station comes from, "" for the user's own.
func (l *Library) StationCollection(id string) string {
	return l.collectionOf(path.Join("stations", id, "station.json"))
}

// ModuleCollection is the collection a module or accessory comes from, "" for the user's own.
func (l *Library) ModuleCollection(id string) string {
	if c := l.collectionOf(path.Join("modules", id, "module.json")); c != "" {
		return c
	}
	return l.collectionOf(path.Join("accessories", id, "module.json"))
}

// Collections lists the manifests of the layers that have one, the front one first. A
// manifest that cannot be read is left out.
func (l *Library) Collections() []CollectionDef {
	var out []CollectionDef
	for _, layer := range l.layers {
		if c, ok, err := ReadCollection(layer); ok && err == nil {
			out = append(out, c)
		}
	}
	return out
}

// DefaultStation is the station to open first: the first one a collection names that the
// library has, or else its first station; "" when it has none.
func (l *Library) DefaultStation() string {
	stations := l.Stations()
	for _, p := range l.Collections() {
		if slices.Contains(stations, p.Station) {
			return p.Station
		}
	}
	if len(stations) > 0 {
		return stations[0]
	}
	return ""
}

// DefaultWorkpieces are the workpieces of a new station and of a module tried out on its
// own: the first ones a collection names that the library has, or else its first; "" when
// it has none.
func (l *Library) DefaultWorkpieces() string {
	sets := l.names("workpieces", jsonFiles)
	for _, p := range l.Collections() {
		if slices.Contains(sets, p.Workpieces) {
			return p.Workpieces
		}
	}
	if len(sets) > 0 {
		return sets[0]
	}
	return ""
}

// Stations lists the ids of all stations in the library.
func (l *Library) Stations() []string {
	seen := map[string]bool{}
	for _, layer := range l.layers {
		entries, err := fs.ReadDir(layer, "stations")
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				seen[e.Name()] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Bundle is a station with everything it references.
type Bundle struct {
	Station    StationDef
	Modules    map[string]*ModuleDef
	Parts      map[string]*PartDef
	Workpieces WorkpieceSet
	Materials  map[string]Material
	Connectors map[string]ConnectorType

	// Preview is set when looking at an entry on its own, without a working station
	// around it.
	Preview bool
}

// Load reads a station and the files it references.
func (l *Library) Load(station string) (*Bundle, error) {
	if !safeID.MatchString(station) {
		return nil, fmt.Errorf("invalid station id %q", station)
	}
	b := &Bundle{Modules: map[string]*ModuleDef{}, Parts: map[string]*PartDef{}}
	if err := l.read(path.Join("stations", station, "station.json"), &b.Station); err != nil {
		return nil, err
	}
	if b.Station.Format != StationFormat {
		return nil, fmt.Errorf("station %s: format %d is not supported, this version reads %d", station, b.Station.Format, StationFormat)
	}
	if err := l.complete(b, b.Station.Modules); err != nil {
		return nil, err
	}
	return b, nil
}

// complete reads what a station needs besides its own file.
func (l *Library) complete(b *Bundle, instances []InstanceDef) error {
	if !safeID.MatchString(b.Station.Workpieces) {
		return fmt.Errorf("station %s: invalid workpieces id %q", b.Station.ID, b.Station.Workpieces)
	}
	if err := l.read(path.Join("workpieces", b.Station.Workpieces+".json"), &b.Workpieces); err != nil {
		return err
	}
	for _, k := range b.Workpieces.Kinds {
		if len(k.Size) != 0 && len(k.Size) != 2 {
			return fmt.Errorf("workpieces %s: kind %s needs its size as length and width", b.Station.Workpieces, k.ID)
		}
		if err := l.loadParts(b, k.Model, nil); err != nil {
			return fmt.Errorf("workpieces %s, kind %s: %w", b.Station.Workpieces, k.ID, err)
		}
	}
	if err := readMerged(l, "materials.json", &b.Materials); err != nil {
		return err
	}
	if err := readMerged(l, "connectors.json", &b.Connectors); err != nil {
		return err
	}
	return l.loadModules(b, instances, nil)
}

// loadModules reads the modules that instances use, and the ones those contain.
func (l *Library) loadModules(b *Bundle, instances []InstanceDef, using []string) error {
	for i := range instances {
		id := instances[i].Module
		if !safeID.MatchString(id) {
			return fmt.Errorf("invalid module id %q", id)
		}
		if slices.Contains(using, id) {
			return fmt.Errorf("module %s contains itself", id)
		}
		m := b.Modules[id]
		if m == nil {
			// Modules come first, and the accessories that go onto them.
			m = &ModuleDef{Level: "module"}
			dir := path.Join("modules", id)
			err := l.read(path.Join(dir, "module.json"), m)
			if errors.Is(err, fs.ErrNotExist) {
				m = &ModuleDef{Level: "accessory"}
				dir = path.Join("accessories", id)
				err = l.read(path.Join(dir, "module.json"), m)
			}
			if err != nil {
				return err
			}
			for k := range m.Devices {
				if d := &m.Devices[k]; d.Type == "program" {
					if err := l.compileProgram(dir, d); err != nil {
						return fmt.Errorf("module %s, program %s: %w", id, d.ID, err)
					}
				}
			}
			if m.Format != FormatVersion {
				return fmt.Errorf("module %s: format %d is not supported, this version reads %d", id, m.Format, FormatVersion)
			}
			if err := m.checkRole(); err != nil {
				return fmt.Errorf("module %s: %w", id, err)
			}
			b.Modules[id] = m
			if err := l.loadParts(b, m.Model, nil); err != nil {
				return fmt.Errorf("module %s: %w", id, err)
			}
			if slices.ContainsFunc(m.Mounts, func(m MountDef) bool { return m.Head.set }) {
				if err := l.loadParts(b, []NodeDef{{Part: ScrewPart}}, nil); err != nil {
					return fmt.Errorf("module %s: %w", id, err)
				}
			}
		}
		if err := l.loadModules(b, m.Modules, append(using, id)); err != nil {
			return err
		}
	}
	return nil
}

// loadParts reads the parts a model uses. using holds the parts being read, so a
// part that includes itself gives an error instead of never finishing.
func (l *Library) loadParts(b *Bundle, nodes []NodeDef, using []string) error {
	for i := range nodes {
		id := nodes[i].Part
		if id != "" {
			if slices.Contains(using, id) {
				return fmt.Errorf("part %s includes itself", id)
			}
			if !safeID.MatchString(id) {
				return fmt.Errorf("invalid part id %q", id)
			}
			part := b.Parts[id]
			if part == nil {
				part = &PartDef{}
				if err := l.read(path.Join("parts", id+".json"), part); err != nil {
					return err
				}
				if part.Format != FormatVersion {
					return fmt.Errorf("part %s: format %d is not supported, this version reads %d", id, part.Format, FormatVersion)
				}
				b.Parts[id] = part
			}
			if err := l.loadParts(b, part.Model, append(using, id)); err != nil {
				return err
			}
		}
		if err := l.loadParts(b, nodes[i].Children, using); err != nil {
			return err
		}
	}
	return nil
}

// checkRole reports a role the builder does not know, or a station without its size.
func (m *ModuleDef) checkRole() error {
	switch {
	case m.Role != "" && !slices.Contains(Roles, m.Role):
		return fmt.Errorf("role %q is none of %v", m.Role, Roles)
	case m.Role == RolePLC && m.Addresses == nil:
		return errors.New("a module of the plc role needs addresses, how its program names the variables of its image")
	case m.Role == RolePLC && m.Addresses.Check() != nil:
		return fmt.Errorf("addresses: %w", m.Addresses.Check())
	case m.Role != RolePLC && m.Addresses != nil:
		return errors.New("only a module of the plc role has addresses")
	case m.Role == RoleStation && (m.Floor == nil || !m.Floor.Width.set || !m.Floor.Depth.set):
		return errors.New("a module of the station role needs its floor width and depth")
	case m.Role != RoleStation && m.Floor != nil:
		return errors.New("only a module of the station role stands on the floor")
	}
	return nil
}

// Value is what a number of the module is with the given settings, which a station writes as
// JSON, and the defaults of the others.
func (m *ModuleDef) Value(n Num, given map[string]json.RawMessage) (float64, error) {
	vars := env{}
	for _, p := range m.Parameters {
		if raw, ok := given[p.ID]; ok {
			var v Num
			if err := json.Unmarshal(raw, &v); err == nil {
				if f, err := v.eval(vars); err == nil {
					vars[p.ID] = f
					continue
				}
			}
		}
		f, err := p.Default.eval(vars)
		if err != nil {
			return 0, fmt.Errorf("module %s, parameter %s: %w", m.ID, p.ID, err)
		}
		vars[p.ID] = f
	}
	return n.eval(vars)
}

// WithRole lists the modules and accessories of a role, by id: the modules first.
func (l *Library) WithRole(role string) []string {
	var out []string
	for _, folder := range []string{"modules", "accessories"} {
		for _, id := range l.names(folder, folders) {
			var m ModuleDef
			if l.read(path.Join(folder, id, "module.json"), &m) == nil && m.Role == role && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

// programName finds the name of the first PROGRAM of a source.
var programName = regexp.MustCompile(`(?im)^\s*PROGRAM\s+([A-Za-z_][A-Za-z0-9_]*)`)

// compileProgram compiles the Structured Text of a module's program device from the module's
// folder, and notes the names of its inputs and outputs.
func (l *Library) compileProgram(dir string, d *DeviceDef) error {
	if d.Source == "" || path.Base(d.Source) != d.Source || !strings.HasSuffix(strings.ToLower(d.Source), ".st") {
		return errors.New("needs its source, a .st file in the module's folder")
	}
	data, err := l.ReadFile(path.Join(dir, d.Source))
	if err != nil {
		return err
	}
	pou := d.Program
	if pou == "" {
		m := programName.FindStringSubmatch(string(data))
		if m == nil {
			return fmt.Errorf("%s has no PROGRAM", d.Source)
		}
		pou = m[1]
	}
	prog, diags := st.Compile(st.Project{Files: map[string]string{d.Source: string(data)}, Programs: []string{pou}})
	if prog == nil {
		for _, diag := range diags {
			if diag.Severity == st.SevError {
				return fmt.Errorf("%s:%d:%d: %s", diag.Pos.File, diag.Pos.Line, diag.Pos.Col, diag.Msg)
			}
		}
		return fmt.Errorf("%s does not build", d.Source)
	}
	d.compiled, d.pou, d.inputs, d.outputs = prog, pou, nil, nil
	for _, w := range st.NewRuntime(prog).Variables() {
		if w.Type != "PROGRAM" || !strings.EqualFold(w.Name, pou) {
			continue
		}
		d.pou = w.Name
		for _, v := range w.Children {
			switch v.Section {
			case "VAR_INPUT":
				d.inputs = append(d.inputs, v.Name)
			case "VAR_OUTPUT":
				d.outputs = append(d.outputs, v.Name)
			}
		}
	}
	return nil
}

// programPOU is the name of the PROGRAM a program device runs.
func programPOU(d *DeviceDef) string { return d.pou }
