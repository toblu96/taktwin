package simapp

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/toblu96/taktwin/internal/model"
)

// The wiring of a station is shown and changed as boxes, one for each module that wiring
// goes through: the PLC, the interface modules, the I/O terminals such as a conveyor's I/O block,
// and a stand's console. Each box belongs to the station (stand) it stands on.

// WiringState is everything that can be wired in the station being built.
type WiringState struct {
	Stations []WiringStation `json:"stations"`
	// Cables join two connectors, as the station names them.
	Cables []WiringCable `json:"cables"`
}

// WiringStation is a stand and the boxes on it.
type WiringStation struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Boxes []WiringBox `json:"boxes"`
}

// WiringBox is a module that wiring goes through. Kind is plc, interface, terminal or console.
type WiringBox struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Of         string            `json:"of"` // the module it belongs to, such as Conveyor
	Connectors []WiringConnector `json:"connectors"`
}

// WiringConnector is a connector of a box. Ref is how cables name it, which for the I/O block
// inside a conveyor is the conveyor's own D-Sub. A terminal's pins are listed with what they
// are wired to and the PLC address and name they reach.
type WiringConnector struct {
	Ref   string      `json:"ref"`
	ID    string      `json:"id"`
	Type  string      `json:"type"`
	Label string      `json:"label"`
	Color string      `json:"color,omitempty"`
	Byte  *int        `json:"byte,omitempty"`
	Pins  []WiringPin `json:"pins"`
}

// WiringPin is a pin of a terminal: what it carries, from which module, and the PLC signal.
type WiringPin struct {
	Pin      string `json:"pin"`
	Output   bool   `json:"output"`
	Analog   bool   `json:"analog"`
	Function string `json:"function"`
	Source   string `json:"source"`
	Address  string `json:"address"`
	Name     string `json:"name"`
	// PLC is the PLC the address is of, where the station has more than one.
	PLC string `json:"plc"`
}

// WiringCable is a cable of the station.
type WiringCable struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// wiringKinds is how the wiring view shows a module of a role.
var wiringKinds = map[string]string{model.RolePLC: "plc", model.RoleInterface: "interface", model.RoleTerminal: "terminal", model.RoleConsole: "console"}

// Wiring returns the wiring of the station being built.
func (s *BuilderService) Wiring() (WiringState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return WiringState{}, errors.New("no station is being built")
	}
	return s.wiring(), nil
}

func (s *BuilderService) wiring() WiringState {
	d, scene := s.draft, s.plan.Scene
	out := WiringState{Stations: []WiringStation{}, Cables: []WiringCable{}}
	for _, c := range d.Cables {
		out.Cables = append(out.Cables, WiringCable{From: c.From, To: c.To})
	}
	// The names of the signals, by PLC and address.
	names := map[string]string{}
	for k, plc := range scene.PLCs {
		id := ""
		if k > 0 || len(scene.PLCs) > 1 {
			id = plc.Module
		}
		for _, sig := range append(slices.Clone(plc.Inputs), plc.Outputs...) {
			names[id+":"+sig.Address] = sig.Name
		}
		for _, a := range plc.Analog {
			names[id+":"+a.Address] = a.Name
		}
	}
	stations := map[string]int{}
	for _, m := range d.Modules {
		if s.standing(m) {
			stations[m.ID] = len(out.Stations)
			out.Stations = append(out.Stations, WiringStation{ID: m.ID, Name: s.nameIn(d, m.ID), Boxes: []WiringBox{}})
		}
	}
	for _, in := range scene.Instances {
		def := s.bundle.Modules[in.Module]
		if def == nil {
			continue
		}
		kind, ok := wiringKinds[def.Role]
		if !ok {
			continue
		}
		box := WiringBox{ID: in.ID, Kind: kind, Name: cmp.Or(in.Name, def.Name), Connectors: []WiringConnector{}}
		root, _, contained := strings.Cut(in.ID, "/")
		if contained {
			box.Of = s.nameIn(d, root)
		}
		shown := map[string]model.SceneConnector{}
		for _, c := range in.Connectors {
			shown[c.ID] = c
		}
		for _, c := range def.Terminal.Connectors {
			sc, visible := shown[c.ID]
			if !visible && !c.Internal {
				continue // switched off, such as the console of a stand without one
			}
			wc := WiringConnector{Ref: s.cableRef(in.ID, c.ID), ID: c.ID, Type: c.Type, Label: s.bundle.Connectors[c.Type].Label, Color: s.bundle.Connectors[c.Type].Color, Byte: c.Byte, Pins: []WiringPin{}}
			if kind == "terminal" && !c.Internal {
				for _, p := range sc.Pins {
					wc.Pins = append(wc.Pins, WiringPin{Pin: p.Pin, Output: p.Output, Analog: p.Analog, Function: p.Function, Source: p.Source, Address: p.Address, Name: names[p.PLC+":"+p.Address], PLC: p.PLC})
				}
			}
			box.Connectors = append(box.Connectors, wc)
		}
		if kind == "console" && len(box.Connectors) == 0 {
			continue
		}
		i, ok := stations[s.stationOf(d, in.ID)]
		if !ok {
			// Not on any stand: shown with the first.
			if len(out.Stations) == 0 {
				out.Stations = append(out.Stations, WiringStation{ID: "", Name: "Station", Boxes: []WiringBox{}})
			}
			i = 0
		}
		out.Stations[i].Boxes = append(out.Stations[i].Boxes, box)
	}
	return out
}

// stationOf is the stand a module stands on, through what it is screwed onto.
func (s *BuilderService) stationOf(d *draft, id string) string {
	id, _, _ = strings.Cut(id, "/")
	for range len(d.Modules) + 1 {
		i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == id })
		if i < 0 {
			return ""
		}
		m := d.Modules[i]
		if m.Place == nil {
			if s.standing(m) {
				return m.ID
			}
			return ""
		}
		id = owner(m.Place.On)
	}
	return ""
}

// cableRef is how a station's cables name a connector: for a module's own I/O block, the
// connector of the module that its cable inside the module leads to.
func (s *BuilderService) cableRef(instance, connector string) string {
	parent, child, contained := strings.Cut(instance, "/")
	if !contained || strings.Contains(child, "/") {
		return instance + "." + connector
	}
	i := slices.IndexFunc(s.draft.Modules, func(m draftModule) bool { return m.ID == parent })
	if i < 0 {
		return instance + "." + connector
	}
	if def := s.bundle.Modules[s.draft.Modules[i].Module]; def != nil {
		for _, c := range def.Cables {
			switch child + "." + connector {
			case c.To:
				return parent + "." + c.From
			case c.From:
				return parent + "." + c.To
			}
		}
	}
	return instance + "." + connector
}

// Plug joins two connectors of the same kind with a cable. Where one of them is cabled
// already, that cable moves to the other; where both are, their cables swap, as when the
// I/O blocks of two modules change places on an interface module.
func (s *BuilderService) Plug(a, b string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	if a == b {
		return s.state(), nil
	}
	d := s.draft.clone()
	end := func(ref string) (int, bool) {
		for i, c := range d.Cables {
			if c.From == ref {
				return i, true
			}
			if c.To == ref {
				return i, false
			}
		}
		return -1, false
	}
	set := func(i int, from bool, ref string) {
		if from {
			d.Cables[i].From = ref
		} else {
			d.Cables[i].To = ref
		}
	}
	ia, fa := end(a)
	ib, fb := end(b)
	switch {
	case ia >= 0 && ia == ib:
		return s.state(), nil
	case ia < 0 && ib < 0:
		d.Cables = append(d.Cables, draftJoin{From: a, To: b})
	case ib < 0:
		set(ia, fa, b)
	case ia < 0:
		set(ib, fb, a)
	default:
		set(ia, fa, b)
		set(ib, fb, a)
	}
	return s.rewired(d)
}

// Unplug takes the cable off a connector.
func (s *BuilderService) Unplug(ref string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	d := s.draft.clone()
	n := len(d.Cables)
	d.Cables = slices.DeleteFunc(d.Cables, func(c draftJoin) bool { return c.From == ref || c.To == ref })
	if len(d.Cables) == n {
		return BuilderState{}, fmt.Errorf("nothing is plugged into %s", ref)
	}
	return s.rewired(d)
}

// pinClass tells inputs, outputs and analogue pins apart, which only change places with
// their own kind.
func pinClass(pin string) string {
	switch {
	case strings.HasPrefix(pin, "AI"), strings.HasPrefix(pin, "UE"):
		return "analogue in"
	case strings.HasPrefix(pin, "AQ"), strings.HasPrefix(pin, "UA"):
		return "analogue out"
	case strings.HasPrefix(pin, "I"):
		return "input"
	case strings.HasPrefix(pin, "Q"):
		return "output"
	}
	return pin
}

// MovePin moves what is wired to a pin of a terminal onto another of its pins, and what was
// there onto the first, such as a conveyor's forward from output Q0 of its I/O block to Q3.
// The terminal is named as "belt/io.T". The PLC names of the signals go along.
func (s *BuilderService) MovePin(terminal, from, to string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	if from == to {
		return s.state(), nil
	}
	if pinClass(from) != pinClass(to) {
		return BuilderState{}, fmt.Errorf("%s is an %s and %s an %s: a pin only changes places with one of its kind", from, pinClass(from), to, pinClass(to))
	}
	d := s.draft.clone()
	a, b := terminal+"."+from, terminal+"."+to
	swap := func(end string) string {
		switch end {
		case a:
			return b
		case b:
			return a
		}
		return end
	}
	// The station's own wires, such as those of a sensor that was added.
	for i := range d.Wires {
		d.Wires[i].From, d.Wires[i].To = swap(d.Wires[i].From), swap(d.Wires[i].To)
	}
	// The wires inside the module the terminal belongs to.
	if parent, local, ok := strings.Cut(terminal, "/"); ok {
		i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == parent })
		if i < 0 || s.bundle.Modules[d.Modules[i].Module] == nil {
			return BuilderState{}, fmt.Errorf("the station has no module %s", parent)
		}
		def := s.bundle.Modules[d.Modules[i].Module]
		m := &d.Modules[i]
		la, lb := local+"."+from, local+"."+to
		if m.Rewire == nil {
			m.Rewire = map[string]string{}
		}
		ends := map[string]bool{}
		for _, w := range def.Wires {
			ends[w.From], ends[w.To] = true, true
		}
		next := map[string]string{}
		for end := range ends {
			now := end
			if v, ok := m.Rewire[end]; ok {
				now = v
			}
			switch now {
			case la:
				now = lb
			case lb:
				now = la
			}
			if now != end {
				next[end] = now
			}
		}
		m.Rewire = next
		if len(m.Rewire) == 0 {
			m.Rewire = nil
		}
	}
	return s.rewired(d)
}

var signalName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SetSignal names the signal at an address, such as IB0Bit00, of a PLC by its module, the
// station's first for "". An empty name goes back to the name it gets from the module pin it
// reaches.
func (s *BuilderService) SetSignal(plc, address, name string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	name = strings.TrimSpace(name)
	if name != "" && !signalName.MatchString(name) {
		return BuilderState{}, fmt.Errorf("%q is not a name a PLC program can use: letters, digits and _, not starting with a digit", name)
	}
	d := s.draft.clone()
	def := d.plcDef(plc)
	if def == nil {
		return BuilderState{}, fmt.Errorf("the station has no PLC %s", plc)
	}
	k := 0
	if plc != "" {
		k = slices.Index(s.plan.PLCs, plc)
	}
	if k < 0 || k >= len(s.plan.Naming) {
		return BuilderState{}, fmt.Errorf("the station has no PLC %s", plc)
	}
	if _, _, err := s.plan.Naming[k].Parse(address); err != nil {
		if _, ok := s.plan.Naming[k].ParseAnalog(address); !ok {
			return BuilderState{}, fmt.Errorf("the PLC has no signal %s", address)
		}
	}
	i := slices.IndexFunc(def.Signals, func(sig draftSignal) bool { return sig.Address == address })
	switch {
	case name == "" && i >= 0:
		def.Signals = slices.Delete(def.Signals, i, i+1)
	case name == "":
	case i >= 0:
		def.Signals[i].Name = name
	default:
		def.Signals = append(def.Signals, draftSignal{Address: address, Name: name})
	}
	st, err := s.commit(d)
	return st, plain(err)
}

// rewired makes a changed wiring the station's, with each named PLC signal moved to the
// address its device now reaches, so that names follow the sensors and actuators.
func (s *BuilderService) rewired(d *draft) (BuilderState, error) {
	before := deviceAddresses(s.plan)
	_, plan, err := s.resolve(d)
	if err != nil {
		return BuilderState{}, plain(err)
	}
	after := deviceAddresses(plan)
	moved := map[string]string{}
	for key, address := range before {
		if now, ok := after[key]; ok && now != address {
			moved[address] = now
		}
	}
	// Moved addresses, as PLC:address.
	if len(moved) > 0 {
		for _, def := range append([]*draftPLC{&d.PLC}, pointers(d.PLCs)...) {
			plc := def.Module
			if len(d.PLCs) == 0 {
				plc = ""
			}
			for i := range def.Signals {
				if now, ok := moved[plc+":"+def.Signals[i].Address]; ok {
					_, def.Signals[i].Address, _ = strings.Cut(now, ":")
				}
			}
		}
	}
	st, err := s.commit(d)
	return st, plain(err)
}

// deviceAddresses is the PLC address each pin of a sensor or actuator reaches.
func deviceAddresses(plan *model.Plan) map[string]string {
	out := map[string]string{}
	for _, in := range plan.Scene.Instances {
		for _, c := range in.Connectors {
			for _, p := range c.Pins {
				if p.Address != "" && p.Source == "" && p.Function != "" {
					out[in.ID+"."+c.ID+"."+p.Pin] = p.PLC + ":" + p.Address
				}
			}
		}
	}
	return out
}

func pointers[T any](list []T) []*T {
	out := make([]*T, len(list))
	for i := range list {
		out[i] = &list[i]
	}
	return out
}
