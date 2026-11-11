package model

import (
	"fmt"
	"sort"
	"strings"
)

// StationChanges set a station up for an exercise, as a teacher would on the real one: the
// settings of its modules, the wiring at the I/O terminals, and the buttons of its dock.
type StationChanges struct {
	// Params sets settings, by instance: a module of the station such as "magazine", or a part
	// of one such as "belt/mid", and by the setting's id.
	Params map[string]map[string]Num `json:"params,omitempty"`
	// Pins swaps the wires at two pins of the same kind on a terminal, as the builder's
	// wiring view does, so that a signal reaches the PLC at another address. The names of the
	// PLC's signals follow their sensors and actuators.
	Pins []PinSwap `json:"pins,omitempty"`
	// Docks, where given, replace the station's docks, as the buttons for caps instead of
	// workpieces.
	Docks []DockDef `json:"docks,omitempty"`
}

// PinSwap swaps what is wired to two pins of a terminal, such as Q0 and Q7 of "io.T" or of
// "belt/io.T", a part's terminal.
type PinSwap struct {
	Terminal string `json:"terminal"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// LoadExercise reads an exercise and its station, set up as the exercise changes it.
func (l *Library) LoadExercise(id string) (*Bundle, *ExerciseDef, error) {
	ex, err := l.Exercise(id)
	if err != nil {
		return nil, nil, err
	}
	b, err := l.Load(ex.Station)
	if err != nil {
		return nil, nil, err
	}
	if ex.Changes != nil {
		if err := b.Change(*ex.Changes); err != nil {
			return nil, nil, fmt.Errorf("exercise %s: %w", id, err)
		}
	}
	return b, ex, nil
}

// Change sets the station of a bundle up as the changes say.
func (b *Bundle) Change(c StationChanges) error {
	st := &b.Station
	var before map[string]string
	if len(c.Pins) > 0 {
		plan, err := b.Resolve(nil)
		if err != nil {
			return err
		}
		before = deviceAddresses(plan)
	}
	refs := make([]string, 0, len(c.Params))
	for ref := range c.Params {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		parent, part, isPart := strings.Cut(ref, "/")
		i := st.instance(parent)
		if i < 0 {
			return fmt.Errorf("the station has no module %s", parent)
		}
		m := &st.Modules[i]
		target := &m.Params
		if isPart {
			if m.Parts == nil {
				m.Parts = map[string]PartChange{}
			}
			pc := m.Parts[part]
			if pc.Params == nil {
				pc.Params = map[string]Num{}
			}
			for k, v := range c.Params[ref] {
				pc.Params[k] = v
			}
			m.Parts[part] = pc
			continue
		}
		if *target == nil {
			*target = map[string]Num{}
		}
		for k, v := range c.Params[ref] {
			(*target)[k] = v
		}
	}
	for _, p := range c.Pins {
		if err := b.swapPins(p); err != nil {
			return err
		}
	}
	if c.Docks != nil {
		st.Docks = c.Docks
	}
	plan, err := b.Resolve(nil)
	if err != nil {
		return err
	}
	if before != nil {
		// The names of the PLC's signals move with the pins of the devices they belong to.
		after := deviceAddresses(plan)
		moved := map[string]string{}
		for key, address := range before {
			if now, ok := after[key]; ok && now != address {
				moved[address] = now
			}
		}
		for i := range st.PLC.Signals {
			if now, ok := moved[st.PLC.Signals[i].Address]; ok {
				st.PLC.Signals[i].Address = now
			}
		}
	}
	return nil
}

func (st *StationDef) instance(id string) int {
	for i := range st.Modules {
		if st.Modules[i].ID == id {
			return i
		}
	}
	return -1
}

// pinKind is whether a pin is a digital input, a digital output or something else.
func pinKind(pin string) string {
	switch {
	case len(pin) > 1 && pin[0] == 'I' && pin[1] >= '0' && pin[1] <= '9':
		return "input"
	case len(pin) > 1 && pin[0] == 'Q' && pin[1] >= '0' && pin[1] <= '9':
		return "output"
	}
	return pin
}

// swapPins swaps the wires at two pins of a terminal: the station's own, and those inside the
// module the terminal is a part of.
func (b *Bundle) swapPins(p PinSwap) error {
	if p.From == p.To {
		return nil
	}
	if pinKind(p.From) != pinKind(p.To) {
		return fmt.Errorf("%s and %s of %s are not of the same kind", p.From, p.To, p.Terminal)
	}
	st := &b.Station
	a, z := p.Terminal+"."+p.From, p.Terminal+"."+p.To
	swap := func(end string) string {
		switch end {
		case a:
			return z
		case z:
			return a
		}
		return end
	}
	for i := range st.Wires {
		st.Wires[i].From, st.Wires[i].To = swap(st.Wires[i].From), swap(st.Wires[i].To)
	}
	parent, local, ok := strings.Cut(p.Terminal, "/")
	if !ok {
		return nil
	}
	i := st.instance(parent)
	if i < 0 || b.Modules[st.Modules[i].Module] == nil {
		return fmt.Errorf("the station has no module %s", parent)
	}
	m := &st.Modules[i]
	la, lz := local+"."+p.From, local+"."+p.To
	next := map[string]string{}
	for _, w := range b.Modules[m.Module].Wires {
		for _, end := range []string{w.From, w.To} {
			now := end
			if v, ok := m.Rewire[end]; ok {
				now = v
			}
			switch now {
			case la:
				now = lz
			case lz:
				now = la
			}
			if now != end {
				next[end] = now
			}
		}
	}
	m.Rewire = next
	if len(m.Rewire) == 0 {
		m.Rewire = nil
	}
	return nil
}

// deviceAddresses is the PLC address each pin of a sensor or actuator reaches.
func deviceAddresses(plan *Plan) map[string]string {
	out := map[string]string{}
	for _, in := range plan.Scene.Instances {
		for _, c := range in.Connectors {
			for _, p := range c.Pins {
				if p.Address != "" && p.Source == "" && p.Function != "" {
					out[in.ID+"."+c.ID+"."+p.Pin] = p.Address
				}
			}
		}
	}
	return out
}
