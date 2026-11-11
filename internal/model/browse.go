package model

import (
	"cmp"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/toblu96/taktwin/internal/plcio"
)

// The levels of the catalog: stations, the modules they are built from, the accessories that
// go onto modules, and shared parts.
const (
	LevelStation   = "station"
	LevelModule    = "module"
	LevelAccessory = "accessory"
	LevelPart      = "part"
)

// previewMaterial colours the parts of a part that take their colour from where it is used.
const previewMaterial = "terminalGray"

// EntryRef names another entry of the catalog.
type EntryRef struct {
	ID    string `json:"id"`
	Level string `json:"level"`
	Name  string `json:"name"`
}

// Entry is one thing in the catalog, for browsing. A file that cannot be read is
// still listed, with the reason in Error, so that one broken import hides nothing.
type Entry struct {
	EntryRef
	Group       string     `json:"group,omitempty"`
	Description string     `json:"description,omitempty"`
	Version     int        `json:"version,omitempty"`
	Contains    []EntryRef `json:"contains"`
	Error       string     `json:"error,omitempty"`
	// Mounts are the kinds of site a module can be screwed onto, such as plate-slot.
	Mounts []string `json:"mounts"`
	// Role is what the module is to the builder, such as station for what a station stands on.
	Role string `json:"role,omitempty"`
	// Collection is the collection the entry comes from, "" for the user's own.
	Collection string `json:"collection"`
}

// names lists the entries in a folder across all layers.
func (l *Library) names(dir string, wanted func(fs.DirEntry) (string, bool)) []string {
	seen := map[string]bool{}
	for _, layer := range l.layers {
		items, err := fs.ReadDir(layer, dir)
		if err != nil {
			continue
		}
		for _, item := range items {
			if name, ok := wanted(item); ok {
				seen[name] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func folders(item fs.DirEntry) (string, bool) { return item.Name(), item.IsDir() }

func jsonFiles(item fs.DirEntry) (string, bool) {
	name, ok := strings.CutSuffix(item.Name(), ".json")
	return name, ok && !item.IsDir()
}

// partsIn finds the parts a model tree uses.
func partsIn(nodes []NodeDef, found map[string]bool) {
	for i := range nodes {
		if nodes[i].Part != "" {
			found[nodes[i].Part] = true
		}
		partsIn(nodes[i].Children, found)
	}
}

// Entries lists the whole catalog, with what each entry contains and is used in.
func (l *Library) Entries() []Entry {
	type raw struct {
		entry   Entry
		modules []string
		parts   []string
	}
	var all []raw
	add := func(id, level, group, name, description string, version int, modules []string, parts map[string]bool, err error) {
		e := Entry{EntryRef: EntryRef{ID: id, Level: level, Name: cmp.Or(name, id)}, Group: group, Description: description, Version: version, Contains: []EntryRef{}, Mounts: []string{}}
		if err != nil {
			e.Error = err.Error()
		}
		all = append(all, raw{entry: e, modules: modules, parts: slices.Sorted(maps.Keys(parts))})
	}
	for _, id := range l.names("stations", folders) {
		var st StationDef
		err := l.read(path.Join("stations", id, "station.json"), &st)
		if err == nil {
			// A station that reads fine can still not fit together, such as a screw beside its slot.
			var b *Bundle
			if b, err = l.Load(id); err == nil {
				_, err = b.Resolve(nil)
			}
		}
		modules := make([]string, 0, len(st.Modules))
		for i := range st.Modules {
			modules = append(modules, st.Modules[i].Module)
		}
		add(id, LevelStation, "", st.Name, st.Description, 0, modules, nil, err)
		all[len(all)-1].entry.Collection = l.collectionOf(path.Join("stations", id, "station.json"))
	}
	for _, kind := range []struct{ level, folder string }{{LevelModule, "modules"}, {LevelAccessory, "accessories"}} {
		level := kind.level
		for _, id := range l.names(kind.folder, folders) {
			// A module's own folder wins over an accessory with the same name.
			if level == LevelAccessory && slices.ContainsFunc(all, func(r raw) bool { return r.entry.Level == LevelModule && r.entry.ID == id }) {
				continue
			}
			var m ModuleDef
			err := l.read(path.Join(kind.folder, id, "module.json"), &m)
			if err == nil {
				err = m.checkRole()
			}
			modules := make([]string, 0, len(m.Modules))
			for i := range m.Modules {
				modules = append(modules, m.Modules[i].Module)
			}
			parts := map[string]bool{}
			partsIn(m.Model, parts)
			add(id, level, m.Group, m.Name, m.Description, m.Version, modules, parts, err)
			all[len(all)-1].entry.Collection = l.collectionOf(path.Join(kind.folder, id, "module.json"))
			all[len(all)-1].entry.Role = m.Role
			for _, mount := range m.Mounts {
				if e := &all[len(all)-1].entry; !slices.Contains(e.Mounts, mount.Type) {
					e.Mounts = append(e.Mounts, mount.Type)
				}
			}
		}
	}
	for _, id := range l.names("parts", jsonFiles) {
		var p PartDef
		err := l.read(path.Join("parts", id+".json"), &p)
		parts := map[string]bool{}
		partsIn(p.Model, parts)
		add(id, LevelPart, "", p.Name, "", 0, nil, parts, err)
		all[len(all)-1].entry.Collection = l.collectionOf(path.Join("parts", id+".json"))
	}

	find := func(id string, part bool) *raw {
		for i := range all {
			if all[i].entry.ID == id && (all[i].entry.Level == LevelPart) == part {
				return &all[i]
			}
		}
		return nil
	}
	link := func(from *raw, to *raw) {
		if to == nil || slices.Contains(from.entry.Contains, to.entry.EntryRef) {
			return
		}
		from.entry.Contains = append(from.entry.Contains, to.entry.EntryRef)
	}
	for i := range all {
		for _, id := range all[i].modules {
			link(&all[i], find(id, false))
		}
		for _, id := range all[i].parts {
			link(&all[i], find(id, true))
		}
	}
	out := make([]Entry, 0, len(all))
	for i := range all {
		out = append(out, all[i].entry)
	}
	order := []string{LevelStation, LevelModule, LevelAccessory, LevelPart}
	slices.SortStableFunc(out, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(slices.Index(order, a.Level), slices.Index(order, b.Level)), cmp.Compare(a.Name, b.Name))
	})
	return out
}

// Preview loads one entry on its own, with no PLC around it, to look at and try out.
// A single module becomes a small station in which all its parameters can be changed.
func (l *Library) Preview(level, id string) (*Bundle, error) {
	if !safeID.MatchString(id) {
		return nil, fmt.Errorf("invalid id %q", id)
	}
	if level == LevelStation {
		b, err := l.Load(id)
		if err != nil {
			return nil, err
		}
		b.Preview = true
		return b, nil
	}
	b := &Bundle{Modules: map[string]*ModuleDef{}, Parts: map[string]*PartDef{}, Preview: true}
	st := &b.Station
	// A single module is tried out with the library's default workpieces.
	st.Format, st.ID, st.Workpieces = StationFormat, id, l.DefaultWorkpieces()
	if level == LevelPart {
		p := &PartDef{}
		if err := l.read(path.Join("parts", id+".json"), p); err != nil {
			return nil, err
		}
		b.Modules[id] = &ModuleDef{Format: FormatVersion, ID: id, Name: cmp.Or(p.Name, id), Level: LevelPart, Model: []NodeDef{{Part: id, Material: previewMaterial}}}
		if err := l.loadParts(b, b.Modules[id].Model, nil); err != nil {
			return nil, err
		}
	}
	if err := l.complete(b, []InstanceDef{{ID: id, Module: id}}); err != nil {
		return nil, err
	}
	m := b.Modules[id]
	st.Name, st.Description = m.Name, m.Description
	st.Modules = []InstanceDef{{ID: id, Module: id, Params: map[string]Num{}}}
	// Only what people may change is offered: the settings with a label.
	for _, p := range m.Parameters {
		if p.Label == "" {
			continue
		}
		value, err := m.Value(p.Default, nil)
		if err != nil {
			continue
		}
		st.Parameters = append(st.Parameters, previewParameter(p, value, id))
		st.Modules[0].Params[p.ID] = Num{src: p.ID, set: true}
	}
	return b, nil
}

// previewParameter makes a module's parameter editable, with a range around its default.
func previewParameter(p ParameterDef, value float64, module string) StationParameterDef {
	s := StationParameterDef{
		ID: p.ID, Label: cmp.Or(p.Label, p.ID), Unit: p.Unit, Kind: p.Kind, Description: p.Description, Options: p.Options, Names: p.Names,
		Default: value, Module: module, Advanced: p.Label == "", Boolean: p.Default.flag,
	}
	if p.Min != nil && p.Max != nil {
		s.Min, s.Max, s.Step = *p.Min, *p.Max, cmp.Or(p.Step, previewStep(*p.Max-*p.Min))
		return s
	}
	if len(p.Options) > 0 {
		s.Min, s.Max = slices.Min(p.Options), slices.Max(p.Options)
		return s
	}
	switch {
	case p.Default.flag:
		s.Max, s.Step = 1, 1
	case p.Unit == "%":
		s.Max, s.Step = 100, 1
	case value == 0:
		s.Min, s.Max, s.Step = -500, 500, 1
	case value > 0:
		s.Max, s.Step = value*4, previewStep(value)
	default:
		s.Min, s.Step = value*4, previewStep(-value)
	}
	return s
}

func previewStep(value float64) float64 {
	if value >= 10 {
		return 1
	}
	return 0.1
}

// SceneTest places a workpiece to see how a module reacts, while previewing it.
type SceneTest struct {
	Label string `json:"label"`
	Place string `json:"place"`           // action that takes the kind of workpiece
	Clear string `json:"clear"`           // action that removes it again
	Holds bool   `json:"holds,omitempty"` // one held in place, not a queue on a transport
}

// addPreviewTests adds the ways to try out one module: a workpiece held in front of
// each sensor, and one placed on each transport of its own.
func (r *resolver) addPreviewTests() {
	r.tests = map[string][]SceneTest{}
	if !r.bundle.Preview || len(r.bundle.Station.Docks) > 0 {
		return
	}
	radius := r.bundle.Workpieces.Diameter / 2
	add := func(instance, id, label string, seg int, at float64, holds bool) {
		name := instance + "." + id
		r.plan.Actions[name+".test"] = ActionPlan{Type: "placeWorkpiece", Segment: seg, At: at, BlockedBy: -1}
		r.plan.Actions[name+".testClear"] = ActionPlan{Type: "clearTransport", Segment: seg, BlockedBy: -1}
		r.tests[instance] = append(r.tests[instance], SceneTest{Label: label, Place: name + ".test", Clear: name + ".testClear", Holds: holds})
	}
	for _, in := range r.order {
		for i := range in.mod.Transport {
			name := in.def.ID + "." + in.mod.Transport[i].ID
			if seg, ok := r.segs[name]; ok && !in.mod.Transport[i].Shared {
				t := &in.mod.Transport[i]
				add(in.def.ID, t.ID, "Place on "+cmp.Or(t.Label, t.ID), seg, min(radius, r.plan.Segments[seg].Length/2), false)
			}
		}
	}
	for i := range r.plan.Sensors {
		s := &r.plan.Sensors[i]
		instance, id, _ := cutLast(s.Name, ".")
		// Far enough along the beam that it does not touch the sensor.
		at := max(s.Reach/2, radius+3)
		from := Point{s.Origin.X, s.Origin.Y, s.Origin.Z - r.bundle.Workpieces.Height/2}
		seg := SegmentPlan{
			Name: s.Name + ".test", From: from, To: Point{from.X + s.Dir.X*(at+1), from.Y + s.Dir.Y*(at+1), from.Z},
			Length: at + 1, Drive: -1, Reverse: -1, Factor: -1, FreeFrom: -1, Next: -1, Prev: -1, Travel: r.state(s.Name + ".test.travel"), Count: r.state(s.Name + ".test.count"), Before: r.state(s.Name + ".test.before"),
		}
		r.plan.Segments = append(r.plan.Segments, seg)
		add(instance, id, "Hold in front of the sensor", len(r.plan.Segments)-1, at, true)
	}
}

func cutLast(s, sep string) (string, string, bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// behaviour says in words what the devices and transports of a module do, from the
// values it is resolved with.
func (r *resolver) behaviour(in *instance) []string {
	out := []string{}
	mm := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	for i := range in.mod.Devices {
		d := &in.mod.Devices[i]
		if !r.enabled(d.When) {
			continue
		}
		switch d.Type {
		case "cylinder":
			how := "A spring returns it when the valve switches off."
			if d.Valve == "bistable" {
				how = "It keeps its position until the other valve coil switches."
			}
			out = append(out, fmt.Sprintf("Cylinder, at rest %s: moves %s mm in %s s. %s", d.Rest, mm(r.num(d.Stroke)), mm(round2(r.num(d.Stroke)/r.num(d.Speed))), how))
		case "rotarySolenoid":
			out = append(out, fmt.Sprintf("Rotary solenoid: turns %s° in %s s while switched on and returns when switched off.", mm(r.num(d.Stroke)), mm(round2(r.num(d.Stroke)/r.num(d.Speed)))))
		case "rotaryDrive":
			out = append(out, fmt.Sprintf("Semi-rotary drive: swivels %s° in %s s while switched on and swivels back when switched off.", mm(r.num(d.Stroke)), mm(round2(r.num(d.Stroke)/r.num(d.Speed)))))
		case "distanceSensor":
			out = append(out, fmt.Sprintf("Measures straight down from %s to %s mm as 0 to 10 V, and switches on when something is within %s mm.", mm(r.num(d.Min)), mm(r.num(d.Reach)), mm(r.num(d.Threshold))))
		case "gripper":
			switch {
			case d.Cylinder != "":
				out = append(out, "Picks up the workpiece between its jaws when they close, carries it, and sets it down on what is below when they open.")
			case d.Detect == "metal":
				out = append(out, "Magnet: holds a metal workpiece it touches from above while switched on, and sets it down on what is below when switched off.")
			default:
				out = append(out, "Suction cup: holds a workpiece it touches from above while the vacuum is on, and sets it down on what is below when it goes off.")
			}
		case "axis":
			how := fmt.Sprintf("Moves %s mm in %s s while its motor runs, and stops wherever the motor stops.", mm(r.num(d.Stroke)), mm(round2(r.num(d.Stroke)/r.num(d.Speed))))
			if d.Wrap {
				how = fmt.Sprintf("Turns a full turn in %s s while its motor runs, on and on, and stops wherever the motor stops.", mm(round2(r.num(d.Stroke)/r.num(d.Speed))))
			}
			out = append(out, "Axis: "+how)
		case "logic":
			what := d.Expr
			if what == "" {
				what = fmt.Sprintf("on from %s until %s", d.Set, d.Reset)
			}
			out = append(out, fmt.Sprintf("Logic %s: %s.", d.ID, what))
		case "pusher":
			out = append(out, "Pushes the lowest workpiece of the stack out with its cylinder, and leaves it on what lies below at the end of the stroke.")
		case "solid":
			if text := "Solid: workpieces cannot pass through it, and it cannot move into them."; !slices.Contains(out, text) {
				out = append(out, text)
			}
		case "reedSwitch":
			out = append(out, fmt.Sprintf("Position sensor: on when %s is %s.", d.Cylinder, d.At))
		case "sensor":
			var what string
			switch d.Detect {
			case "metal":
				what = fmt.Sprintf("Switches on for metal workpieces within %s mm, and no others.", mm(r.num(d.Reach)))
			case "reflection":
				what = fmt.Sprintf("Switches on when a workpiece within %s mm reflects at least %s%% of the light.", mm(r.num(d.Reach)), mm(r.num(d.Threshold)))
			default:
				what = fmt.Sprintf("Switches on when a workpiece in its %s mm beam lets less than %s%% of the light through.", mm(r.num(d.Reach)), mm(r.num(d.Threshold)))
			}
			if delay := r.num(d.OffDelay); delay > 0 {
				what += fmt.Sprintf(" Stays on for %s s after it passes.", mm(delay))
			}
			if d.Mirror != nil && r.enabled(d.Mirror.When) {
				what += fmt.Sprintf(" A mirror folds the beam, which runs on for another %s mm.", mm(r.num(d.Mirror.Reach)))
			}
			out = append(out, what)
		case "motorController":
			out = append(out, "Motor controller: one direction input runs the motor, both or none stop it, and creep halves the speed.")
		case "button":
			out = append(out, fmt.Sprintf("Button %s: on while pressed.", d.ID))
		case "key":
			out = append(out, fmt.Sprintf("Key switch %s: stays where it is turned.", d.ID))
		}
	}
	for i := range in.mod.Transport {
		t := &in.mod.Transport[i]
		idx, ok := r.segs[in.def.ID+"."+t.ID]
		if !ok {
			continue
		}
		s := &r.plan.Segments[idx]
		switch {
		case t.Static:
			out = append(out, "A surface to put workpieces on.")
		case t.Shared:
			out = append(out, fmt.Sprintf("Guides workpieces along a %s mm curve at the speed of the transport before it.", mm(round2(s.Length))))
		case t.Drive != "":
			out = append(out, fmt.Sprintf("Moves workpieces %s mm at %s mm/s while its drive runs.", mm(round2(s.Length)), mm(round2(s.Speed))))
		default:
			out = append(out, fmt.Sprintf("Workpieces slide %s mm by gravity at %s mm/s and queue at the end.", mm(round2(s.Length)), mm(round2(s.Speed))))
		}
	}
	// Many places of one kind, as on a rack, say so once.
	return slices.Compact(out)
}

func round2(v float64) float64 {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 2, 64), 64)
	return f
}

// AddressMap returns a PLC address map of the station by id. An empty id is the
// station's own addresses.
func (p *Plan) AddressMap(mapID string) (plcio.AddressMap, error) {
	var out plcio.AddressMap
	if mapID == "" {
		return out, nil
	}
	for i := range p.maps {
		if p.maps[i].ID != mapID {
			continue
		}
		convert := func(defs []PLCBindingDef, find func(string) (plcio.Addr, bool), wantInput bool) ([]plcio.Binding, error) {
			var list []plcio.Binding
			for _, b := range defs {
				local, ok := find(b.Signal)
				if !ok {
					return nil, fmt.Errorf("PLC map %s: no signal %q", mapID, b.Signal)
				}
				if len(p.Naming) == 0 {
					return nil, fmt.Errorf("PLC map %s: the station has no PLC", mapID)
				}
				if _, isInput, err := p.Naming[0].Parse(b.Variable); err != nil {
					return nil, fmt.Errorf("PLC map %s: %w", mapID, err)
				} else if isInput != wantInput {
					return nil, fmt.Errorf("PLC map %s: %s is on the wrong side for %s", mapID, b.Variable, b.Signal)
				}
				list = append(list, plcio.Binding{Local: local, Variable: b.Variable, Invert: b.Invert})
			}
			return list, nil
		}
		var err error
		if out.Inputs, err = convert(p.maps[i].Inputs, p.PLCInput, true); err != nil {
			return out, err
		}
		out.Outputs, err = convert(p.maps[i].Outputs, p.PLCOutput, false)
		return out, err
	}
	return out, fmt.Errorf("the station has no PLC map %q", mapID)
}
