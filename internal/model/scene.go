package model

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/toblu96/taktwin/internal/plcio"
)

// SceneGeometry is a resolved shape. Only the fields of its Type are set.
type SceneGeometry struct {
	RadialSegments float64        `json:"radialSegments,omitempty"`
	Points         [][2]float64   `json:"points,omitempty"`
	Holes          [][][2]float64 `json:"holes,omitempty"`
	Type           string         `json:"type"`
	Width          float64        `json:"width"`
	Height         float64        `json:"height"`
	Depth          float64        `json:"depth"`
	Radius         float64        `json:"radius"`
	From           Point          `json:"from"`
	To             Point          `json:"to"`
	Center         [2]float64     `json:"center"`

	Inner      float64 `json:"inner"`
	Outer      float64 `json:"outer"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	InnerStart float64 `json:"innerStart"`
}

// SceneLabel is resolved text on a node.
type SceneLabel struct {
	Text       string  `json:"text"`
	Size       float64 `json:"size"`
	Color      string  `json:"color,omitempty"`
	Background string  `json:"background,omitempty"`
}

// SceneJoint is a resolved joint of a node.
type SceneJoint struct {
	Type   string  `json:"type"`
	Axis   Point   `json:"axis"`
	Origin Point   `json:"origin"`
	From   float64 `json:"from"`
	To     float64 `json:"to"`
	State  int     `json:"state"`
}

// SceneLamp colours a node from a state.
type SceneLamp struct {
	State   int     `json:"state"`
	On      string  `json:"on"`
	Off     string  `json:"off"`
	Opacity float64 `json:"opacity,omitempty"`
	Glow    float64 `json:"glow,omitempty"`
}

// SceneNode is a resolved node of a module's 3D model, in the local coordinates
// of its parent. Rotation is in degrees.
type SceneNode struct {
	Name     string         `json:"name,omitempty"`
	Position Point          `json:"position"`
	Rotation Point          `json:"rotation"`
	Geometry *SceneGeometry `json:"geometry,omitempty"`
	Material string         `json:"material,omitempty"`
	Label    *SceneLabel    `json:"label,omitempty"`
	Belt     int            `json:"belt"` // state index of the travelled distance, or -1
	Press    string         `json:"press,omitempty"`
	Toggle   bool           `json:"toggle,omitempty"` // Press flips the control instead of holding it
	NoPick   bool           `json:"noPick,omitempty"`
	Joint    *SceneJoint    `json:"joint,omitempty"`
	Lamp     *SceneLamp     `json:"lamp,omitempty"`
	Children []SceneNode    `json:"children,omitempty"`
}

// SceneWidget is a resolved control. State fields are indexes into Scene.States,
// or -1. Fill is an index into Snapshot.Bins, or -1.
type SceneWidget struct {
	Type         string            `json:"type"`
	Show         string            `json:"show"`
	Label        string            `json:"label,omitempty"`
	Tag          string            `json:"tag,omitempty"`
	Tone         string            `json:"tone,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	Control      string            `json:"control,omitempty"`
	State        int               `json:"state"`
	Lamp         int               `json:"lamp"`
	Action       string            `json:"action,omitempty"`
	Args         map[string]string `json:"args,omitempty"`
	Swatch       string            `json:"swatch,omitempty"`
	DisabledWhen int               `json:"disabledWhen"`
	Fill         int               `json:"fill"`
	Capacity     int               `json:"capacity,omitempty"`
	Expects      string            `json:"expects,omitempty"`
	Widgets      []SceneWidget     `json:"widgets,omitempty"`
}

// ScenePLCMap is an address map a PLC link can use.
type ScenePLCMap struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ScenePin is one connector pin with what it is connected to.
type ScenePin struct {
	Pin      string `json:"pin"`
	Output   bool   `json:"output"` // carries a signal from the PLC
	Analog   bool   `json:"analog,omitempty"`
	Function string `json:"function,omitempty"`
	Source   string `json:"source,omitempty"` // module the function belongs to
	Spare    bool   `json:"spare,omitempty"`
	Wired    bool   `json:"wired"`
	Invert   bool   `json:"invert,omitempty"`
	State    int    `json:"state"`
	PLCName  string `json:"plcName,omitempty"`
	Address  string `json:"address,omitempty"`
	// PLC is the PLC module that Address is of, where the station has more than one.
	PLC      string `json:"plc,omitempty"`
	Forcible bool   `json:"forcible,omitempty"` // no device sets it, so a test can
}

// SceneConnector is one physical connector of a module.
type SceneConnector struct {
	ID    string     `json:"id"`
	Type  string     `json:"type"`
	Label string     `json:"label"`
	Pins  []ScenePin `json:"pins"`
}

// SceneValue is a module parameter for display.
type SceneValue struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Unit  string  `json:"unit,omitempty"`
	Value float64 `json:"value"`
}

// SceneFault is a fault that can be enabled on a module.
type SceneFault struct {
	Name  string `json:"name"` // the sensor's device name
	Label string `json:"label"`
}

// SceneInstance is a placed module.
type SceneInstance struct {
	ID          string           `json:"id"`
	Module      string           `json:"module"`
	Version     int              `json:"version"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	At          Point            `json:"at"`
	RotateX     float64          `json:"rotateX"`
	RotateY     float64          `json:"rotateY"`
	RotateZ     float64          `json:"rotateZ"`
	Parent      string           `json:"parent,omitempty"`
	Level       string           `json:"level"`
	SelectAs    string           `json:"selectAs,omitempty"`
	MountedOn   string           `json:"mountedOn,omitempty"`
	IsPLC       bool             `json:"isPlc"`
	Nodes       []SceneNode      `json:"nodes"`
	Controls    []SceneWidget    `json:"controls"`
	Connectors  []SceneConnector `json:"connectors"`
	Values      []SceneValue     `json:"values"`
	Faults      []SceneFault     `json:"faults"`
	Behaviour   []string         `json:"behaviour"`
	Tests       []SceneTest      `json:"tests,omitempty"`
}

// SceneParameter is a station setting users can change.
type SceneParameter struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Unit        string    `json:"unit,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Description string    `json:"description,omitempty"`
	Options     []float64 `json:"options,omitempty"`
	Names       []string  `json:"names,omitempty"`
	Value       float64   `json:"value"`
	Min         float64   `json:"min"`
	Max         float64   `json:"max"`
	Step        float64   `json:"step,omitempty"`
	Module      string    `json:"module,omitempty"`

	Advanced bool `json:"advanced,omitempty"`
	Boolean  bool `json:"boolean,omitempty"`
}

// SceneSignal is one named bit of the PLC image.
type SceneSignal struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Variable    string `json:"variable,omitempty"`
	Description string `json:"description,omitempty"`
	Byte        uint8  `json:"byte"`
	Bit         uint8  `json:"bit"`
}

// ScenePLC is the named PLC image.
type ScenePLC struct {
	Module string `json:"module,omitempty"`
	// Naming is how the PLC's program names the variables of its image.
	Naming  plcio.Naming  `json:"naming"`
	Maps    []ScenePLCMap `json:"maps"`
	Inputs  []SceneSignal `json:"inputs"`
	Outputs []SceneSignal `json:"outputs"`
	Analog  []SceneAnalog `json:"analog"`
}

// SceneAnalog is a named analogue input of the PLC.
type SceneAnalog struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Description string `json:"description,omitempty"`
	Channel     int    `json:"channel"`
}

// SceneDock is a group of station-wide widgets.
type SceneDock struct {
	Place   string        `json:"place"`
	Widgets []SceneWidget `json:"widgets"`
}

// Scene is everything the frontend needs to show a station. It holds no
// expressions, so the frontend never has to understand the model format.
type Scene struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	States      []string            `json:"states"`
	Materials   map[string]Material `json:"materials"`
	Workpieces  WorkpieceSet        `json:"workpieces"`
	// Models are how the workpiece kinds with a model look, by kind, drawn from the middle of
	// their base.
	Models     map[string][]SceneNode `json:"models,omitempty"`
	Instances  []SceneInstance        `json:"instances"`
	Parameters []SceneParameter       `json:"parameters"`
	PLC        ScenePLC               `json:"plc"`
	// PLCs are all the station's PLCs, the first one first, which is also PLC.
	PLCs  []ScenePLC  `json:"plcs"`
	Docks []SceneDock `json:"docks"`
	// Checks are what looks wrong with how the station is built.
	Checks []SceneCheck `json:"checks"`
}

// SceneCheck is one thing wrong with how a station is built: in words, the top-level
// modules it is about, and the spot in station coordinates where it happens, if there is one.
type SceneCheck struct {
	Text      string   `json:"text"`
	Instances []string `json:"instances"`
	At        *Point   `json:"at,omitempty"`
}

// unresolvedText matches a {name} no instance gave a value for.
var unresolvedText = regexp.MustCompile(`\{[A-Za-z0-9_]+\}`)

func (r *resolver) sceneInstance(in *instance) SceneInstance {
	s := SceneInstance{
		ID: in.def.ID, Module: in.mod.ID, Version: in.mod.Version, Name: in.def.Name, Description: in.mod.Description,
		At: in.at, RotateX: in.rotX, RotateY: in.rotY, RotateZ: in.rotZ, Level: in.mod.Level, SelectAs: in.def.SelectAs,
		Controls: []SceneWidget{}, Connectors: []SceneConnector{}, Values: []SceneValue{}, Faults: []SceneFault{},
	}
	if s.Name == "" {
		s.Name = in.mod.Name
	}
	if in.parent != nil {
		s.Parent = in.parent.def.ID
	}
	if on := in.def.MountedOn; on != "" {
		moves := in.parent != nil && slices.ContainsFunc(in.parent.mod.Joints, func(j JointDef) bool { return j.Node == on })
		if !moves {
			r.fail("%s is mounted on %q, which is no moving node of the module around it", in.def.ID, on)
		}
		s.MountedOn = on
	}
	for i := range in.mod.Devices {
		d := &in.mod.Devices[i]
		if !r.enabled(d.When) {
			continue
		}
		switch d.Type {
		case "plc":
			s.IsPLC = true
		case "sensor":
			if d.Fault != "" {
				s.Faults = append(s.Faults, SceneFault{Name: in.def.ID + "." + d.ID, Label: d.Fault})
			}
		}
	}
	for _, p := range in.mod.Parameters {
		if p.Label != "" {
			s.Values = append(s.Values, SceneValue{ID: p.ID, Label: p.Label, Unit: p.Unit, Value: in.vars[p.ID]})
		}
	}
	s.Behaviour = r.behaviour(in)
	s.Tests = r.tests[in.def.ID]
	s.Connectors = r.sceneConnectors(in)
	s.Nodes = r.nodes(in)
	s.Controls = r.widgets(in.mod.Controls, in.def.ID+".")
	return s
}

// netInfo describes a net by the pin that gives it a function.
func (r *resolver) netInfo() map[string]*pin {
	info := map[string]*pin{}
	for _, in := range r.order {
		for _, p := range in.pins {
			if p.def.Function != "" && (p.def.Reads != "" || p.def.Drives != "" || p.def.Constant != nil) {
				info[r.find(p.key)] = p
			}
		}
	}
	return info
}

func (r *resolver) sceneConnectors(in *instance) []SceneConnector {
	info := r.netInfo()
	// The names of the signals, by PLC and address.
	byAddress := map[string]string{}
	for k, id := range r.plan.PLCs {
		def := r.bundle.Station.PLC
		if k > 0 {
			def = StationPLCDef{}
			if i := slices.IndexFunc(r.bundle.Station.PLCs, func(d StationPLCDef) bool { return d.Module == id }); i >= 0 {
				def = r.bundle.Station.PLCs[i]
			}
		}
		for _, s := range def.Signals {
			byAddress[fmt.Sprint(k, s.Address)] = s.Name
		}
	}
	plcOf := func(root string) string {
		if len(r.plan.PLCs) < 2 {
			return ""
		}
		return r.plan.PLCs[r.netPLCOf[root]]
	}
	out := []SceneConnector{}
	for i := range in.mod.Terminal.Connectors {
		c := &in.mod.Terminal.Connectors[i]
		if in.conns[c.ID] == nil || c.Internal {
			continue
		}
		sc := SceneConnector{ID: c.ID, Type: c.Type, Label: r.bundle.Connectors[c.Type].Label, Pins: []ScenePin{}}
		for _, p := range in.pins {
			if p.conn != c {
				continue
			}
			root := r.find(p.key)
			analog := p.def.Analog || slices.Contains(r.bundle.Connectors[c.Type].Analog, p.name)
			output := p.def.Drives != "" || (p.def.Reads == "" && p.def.Constant == nil &&
				(strings.HasPrefix(p.name, "Q") || (analog && (strings.HasPrefix(p.name, "AQ") || strings.HasPrefix(p.name, "UA")))))
			sp := ScenePin{
				Pin: p.name, Output: output, Analog: analog, Function: p.def.Function, Spare: p.def.Spare, Invert: p.def.Invert, State: r.state(p.key),
				Address: r.netPLC[root], PLCName: byAddress[fmt.Sprint(r.netPLCOf[root], r.netPLC[root])],
			}
			if sp.Address != "" {
				// A pin on a PLC's net is an output where the PLC drives the net.
				sp.PLC = plcOf(root)
				sp.Output = r.netPLCOut[root]
			}
			if owner := info[root]; owner != nil {
				sp.Function = owner.def.Function
				sp.Invert = owner.def.Invert
				if owner.inst != in {
					sp.Source = owner.inst.def.Name
					if sp.Source == "" {
						sp.Source = owner.inst.mod.Name
					}
				}
			}
			sp.Wired = info[root] != nil
			sp.Forcible = !analog && p.def.Constant == nil && p.def.Reads == "" && (info[root] == nil || info[root].def.Reads == "")
			sc.Pins = append(sc.Pins, sp)
		}
		out = append(out, sc)
	}
	return out
}

// nodeBindings is what the module's logic attaches to named nodes.
type nodeBindings struct {
	joints map[string]*SceneJoint
	lamps  map[string]*SceneLamp
	press  map[string]string
	toggle map[string]bool
	used   map[string]bool
}

// workpieceModels resolves the models of the workpiece kinds that have one.
func (r *resolver) workpieceModels() map[string][]SceneNode {
	var out map[string][]SceneNode
	for _, k := range r.bundle.Workpieces.Kinds {
		if len(k.Model) == 0 {
			continue
		}
		if out == nil {
			out = map[string][]SceneNode{}
		}
		r.where = "workpiece " + k.ID
		b := nodeBindings{joints: map[string]*SceneJoint{}, lamps: map[string]*SceneLamp{}, press: map[string]string{}, toggle: map[string]bool{}, used: map[string]bool{}}
		out[k.ID] = r.resolveNodes(k.Model, &b)
	}
	return out
}

func (r *resolver) nodes(in *instance) []SceneNode {
	b := nodeBindings{joints: map[string]*SceneJoint{}, lamps: map[string]*SceneLamp{}, press: map[string]string{}, toggle: map[string]bool{}, used: map[string]bool{}}
	for i := range in.mod.Joints {
		j := &in.mod.Joints[i]
		if !r.enabled(j.When) {
			continue
		}
		if j.Type != "revolute" && j.Type != "prismatic" {
			r.fail("joint on %s has unknown type %q", j.Node, j.Type)
			continue
		}
		b.joints[j.Node] = &SceneJoint{
			Type: j.Type, Axis: r.vec(j.Axis), Origin: r.vec(j.Origin),
			From: r.num(j.Range[0]), To: r.num(j.Range[1]), State: r.local(j.Driver),
		}
	}
	for i := range in.mod.Devices {
		d := &in.mod.Devices[i]
		if !r.enabled(d.When) || d.Node == "" {
			continue
		}
		name := in.def.ID + "." + d.ID
		switch d.Type {
		case "lamp":
			state := -1 // always lit
			if !d.AlwaysOn {
				state = r.state(name + ".on")
			}
			b.lamps[d.Node] = &SceneLamp{State: state, On: d.On, Off: d.Off, Opacity: d.Opacity, Glow: r.num(d.Glow)}
		case "button", "key":
			b.press[d.Node] = name
			b.toggle[d.Node] = d.Type == "key"
		}
	}
	nodes := r.resolveNodes(in.mod.Model, &b)
	for i := range in.mod.Mounts {
		m := &in.mod.Mounts[i]
		if !r.enabled(m.When) || !m.Head.set {
			continue
		}
		for _, s := range m.Screws {
			n := r.resolveNode(&NodeDef{Part: ScrewPart}, &b)
			n.Position = r.vec(s)
			n.Position.Z += r.num(m.Head)
			nodes = append(nodes, n)
		}
	}
	for name := range b.joints {
		if !b.used[name] {
			r.fail("joint refers to unknown node %q", name)
		}
	}
	for name := range b.lamps {
		if !b.used[name] {
			r.fail("lamp refers to unknown node %q", name)
		}
	}
	for name := range b.press {
		if !b.used[name] {
			r.fail("control refers to unknown node %q", name)
		}
	}
	return nodes
}

func (r *resolver) resolveNodes(defs []NodeDef, b *nodeBindings) []SceneNode {
	out := make([]SceneNode, 0, len(defs))
	for i := range defs {
		d := &defs[i]
		if !r.enabled(d.When) {
			continue
		}
		n := r.resolveNode(d, b)
		if d.Repeat == nil {
			out = append(out, n)
			continue
		}
		step := r.vec(d.Repeat.Step)
		for c := 0; c < int(r.num(d.Repeat.Count)); c++ {
			f := float64(c)
			out = append(out, SceneNode{Belt: -1, Position: Point{step.X * f, step.Y * f, step.Z * f}, Children: []SceneNode{n}})
		}
	}
	return out
}

func (r *resolver) resolveNode(d *NodeDef, b *nodeBindings) SceneNode {
	material := d.Material
	if material == "inherit" && r.partMaterial != "" {
		material = r.partMaterial
	}
	n := SceneNode{
		Name: d.Name, Position: r.vec(d.Position), Rotation: r.vec(d.Rotation), Material: material,
		NoPick: d.NoPick, Belt: -1,
	}
	if d.Name != "" {
		if b.used[d.Name] && (b.joints[d.Name] != nil || b.lamps[d.Name] != nil || b.press[d.Name] != "") {
			r.fail("node name %q is used twice", d.Name)
		}
		b.used[d.Name] = true
		n.Joint, n.Lamp, n.Press, n.Toggle = b.joints[d.Name], b.lamps[d.Name], b.press[d.Name], b.toggle[d.Name]
	}
	if g := d.Geometry; g != nil {
		n.Geometry = r.geometry(g)
		if n.Lamp == nil && d.Belt == "" {
			if _, ok := r.bundle.Materials[material]; !ok {
				r.fail("node %s uses unknown material %q", d.Name, material)
			}
		}
	}
	if d.Label != nil {
		n.Label = &SceneLabel{Text: r.text(d.Label.Text), Size: r.num(d.Label.Size), Color: d.Label.Color, Background: d.Label.Background}
	}
	if d.Belt != "" {
		n.Belt = r.local(d.Belt + ".travel")
	}
	n.Children = r.resolveNodes(d.Children, b)
	if d.Part != "" {
		outer := r.partMaterial
		r.partMaterial = material
		n.Children = append(n.Children, r.resolveNodes(r.bundle.Parts[d.Part].Model, b)...)
		r.partMaterial = outer
	}
	return n
}

func (r *resolver) geometry(g *GeometryDef) *SceneGeometry {
	s := &SceneGeometry{Type: g.Type}
	switch g.Type {
	case "BoxGeometry":
		s.Width, s.Height, s.Depth = r.num(g.Width), r.num(g.Height), r.num(g.Depth)
	case "PlaneGeometry":
		s.Width, s.Height = r.num(g.Width), r.num(g.Height)
	case "CylinderGeometry":
		s.From, s.To, s.Radius, s.RadialSegments = r.vec(g.From), r.vec(g.To), r.num(g.Radius), r.num(g.RadialSegments)
	case "ExtrudeGeometry":
		if len(g.Points) < 3 {
			r.fail("an ExtrudeGeometry needs at least 3 points")
		}
		s.Points, s.Depth = r.outline(g.Points), r.num(g.Depth)
		for _, hole := range g.Holes {
			s.Holes = append(s.Holes, r.outline(hole))
		}
	case "SphereGeometry":
		s.Radius = r.num(g.Radius)
	case "SectorGeometry":
		s.Center = [2]float64{r.num(g.Center[0]), r.num(g.Center[1])}
		s.Inner, s.Outer, s.Start, s.End = r.num(g.Inner), r.num(g.Outer), r.num(g.Start), r.num(g.End)
		s.InnerStart = r.numOr(g.InnerStart, s.Start)
		s.Height = r.num(g.Height)
	default:
		r.fail("unknown geometry type %q", g.Type)
	}
	return s
}

func (r *resolver) outline(points [][2]Num) [][2]float64 {
	out := make([][2]float64, len(points))
	for i, p := range points {
		out[i] = [2]float64{r.num(p[0]), r.num(p[1])}
	}
	return out
}

// widgets resolves controls. prefix is "instance." on a module and empty in a
// station, whose references are already complete.
func (r *resolver) widgets(defs []WidgetDef, prefix string) []SceneWidget {
	out := make([]SceneWidget, 0, len(defs))
	onModule := prefix != ""
	state := func(name string) int {
		if onModule {
			return r.local(name)
		}
		return r.global(name)
	}
	for i := range defs {
		d := &defs[i]
		if !r.enabled(d.When) {
			continue
		}
		w := SceneWidget{
			Type: d.Type, Show: d.Show, Label: r.text(d.Label), Tag: r.text(d.Tag), Tone: d.Tone, Labels: d.Labels,
			State: -1, Lamp: -1, DisabledWhen: -1, Fill: -1, Args: d.Args, Swatch: d.Swatch,
		}
		if w.Show == "" {
			w.Show = "popup"
		}
		switch d.Type {
		case "push", "switch":
			w.Control = prefix + d.Device
			if _, ok := r.plan.Controls[w.Control]; !ok {
				r.fail("widget refers to unknown control %q", w.Control)
				continue
			}
			w.State = r.plan.Controls[w.Control]
			if d.Type == "push" && d.State != "" {
				w.Lamp = state(d.State)
			}
		case "lamp":
			w.State = state(d.State)
		case "button":
			w.Action = prefix + d.Action
			if _, ok := r.plan.Actions[w.Action]; !ok {
				r.fail("widget refers to unknown action %q", w.Action)
				continue
			}
			if d.DisabledWhen != "" {
				w.DisabledWhen = state(d.DisabledWhen)
			}
		case "fill":
			w.Fill = r.binOf(prefix + d.Transport)
			w.Capacity = int(r.num(d.Capacity))
			w.Expects = unresolvedText.ReplaceAllString(r.text(d.Expects), "")
			if w.Expects != "" && !r.isKind(w.Expects) {
				r.fail("fill expects unknown workpiece kind %q", w.Expects)
			}
		case "group":
			w.Widgets = r.widgets(d.Widgets, prefix)
		default:
			r.fail("unknown widget type %q", d.Type)
			continue
		}
		out = append(out, w)
	}
	return out
}

func (r *resolver) isKind(id string) bool {
	for _, k := range r.bundle.Workpieces.Kinds {
		if k.ID == id {
			return true
		}
	}
	return false
}

func (r *resolver) binOf(transport string) int {
	if i := r.plan.Bin(transport); i >= 0 {
		return i
	}
	seg := r.segmentRef(transport)
	r.plan.Bins = append(r.plan.Bins, seg)
	return len(r.plan.Bins) - 1
}

// scenePLC is the I/O table of the station's PLC k, as def names its signals.
func (r *resolver) scenePLC(st *StationDef, def StationPLCDef, k int) ScenePLC {
	p := ScenePLC{Module: def.Module, Maps: []ScenePLCMap{}, Inputs: []SceneSignal{}, Outputs: []SceneSignal{}}
	naming := plcio.Naming{}
	if k < len(r.plan.Naming) {
		naming = r.plan.Naming[k]
	}
	p.Naming = naming
	for i := range def.Maps {
		p.Maps = append(p.Maps, ScenePLCMap{ID: def.Maps[i].ID, Name: def.Maps[i].Name})
	}
	seen := map[string]bool{}
	bits := map[string]bool{}
	for _, b := range r.plan.PLCIn {
		if b.PLC == k {
			bits[naming.Input(b.Addr)] = true
		}
	}
	for _, b := range r.plan.PLCOut {
		if b.PLC == k {
			bits[naming.Output(b.Addr)] = true
		}
	}
	list := "plc"
	if k > 0 {
		list = "plcs " + def.Module
	}
	for i := range def.Signals {
		s := &def.Signals[i]
		r.where = fmt.Sprintf("station %s, %s.signals[%d]", st.ID, list, i)
		if channel, ok := naming.ParseAnalog(s.Address); ok {
			if !slices.ContainsFunc(r.plan.PLCAnalog, func(a PLCAnalogPlan) bool { return a.Channel == channel && a.PLC == k }) {
				r.fail("address %s is not on a PLC connector", s.Address)
				continue
			}
			p.Analog = append(p.Analog, SceneAnalog{Name: s.Name, Address: s.Address, Description: s.Description, Channel: channel})
			continue
		}
		input, a, err := r.parseAddress(k, s.Address)
		if err != nil {
			r.fail("%v", err)
			continue
		}
		if seen[s.Address] {
			r.fail("address %s is named twice", s.Address)
			continue
		}
		seen[s.Address] = true
		if !bits[s.Address] {
			r.fail("address %s is not on a PLC connector", s.Address)
			continue
		}
		sig := SceneSignal{Name: s.Name, Address: s.Address, Variable: s.Variable, Description: s.Description, Byte: a.Byte, Bit: a.Bit}
		if input {
			p.Inputs = append(p.Inputs, sig)
		} else {
			p.Outputs = append(p.Outputs, sig)
		}
	}
	r.where = "station " + st.ID
	r.defaultSignals(&p, seen, k)
	return p
}

// defaultSignals names the bits a station leaves unnamed after the module pin they reach,
// so that a station put together from modules has an I/O table without writing one. Only
// a bit that reaches a device counts, and the pin of the outermost module on its way wins,
// as the magazine's "Slide retracted" over its switch's.
func (r *resolver) defaultSignals(p *ScenePLC, named map[string]bool, k int) {
	found := map[string]*pin{}
	depth := func(q *pin) int { return strings.Count(q.inst.def.ID, "/") }
	// Only a net with a device on it means something, not a spare socket.
	device := map[string]bool{}
	for _, in := range r.order {
		for _, q := range in.pins {
			if q.def.Reads != "" || q.def.Drives != "" {
				device[r.find(q.key)] = true
			}
		}
	}
	for _, in := range r.order {
		for _, q := range in.pins {
			if !q.declared || q.def.Function == "" || !device[r.find(q.key)] {
				continue
			}
			addr, ok := r.netPLC[r.find(q.key)]
			if !ok || named[addr] || r.netPLCOf[r.find(q.key)] != k {
				continue
			}
			if best := found[addr]; best == nil || depth(q) < depth(best) {
				found[addr] = q
			}
		}
	}
	var inputs, outputs []SceneSignal
	for addr, q := range found {
		input, a, err := r.parseAddress(k, addr)
		if err != nil {
			continue // an analogue input
		}
		name := cmp.Or(q.inst.def.Name, q.inst.mod.Name)
		sig := SceneSignal{Name: q.inst.def.ID + "." + q.name, Address: addr, Description: name + ": " + q.def.Function, Byte: a.Byte, Bit: a.Bit}
		if input {
			inputs = append(inputs, sig)
		} else {
			outputs = append(outputs, sig)
		}
	}
	order := func(a, b SceneSignal) int { return cmp.Or(cmp.Compare(a.Byte, b.Byte), cmp.Compare(a.Bit, b.Bit)) }
	slices.SortFunc(inputs, order)
	slices.SortFunc(outputs, order)
	p.Inputs = append(p.Inputs, inputs...)
	p.Outputs = append(p.Outputs, outputs...)
}
