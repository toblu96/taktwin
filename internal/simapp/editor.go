package simapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/toblu96/taktwin/internal/link"
	"github.com/toblu96/taktwin/internal/link/opcua"
)

// SignalInfo is a signal of a PLC as the connection editor lists it.
type SignalInfo struct {
	// Address is where it is, as %IX0.3, and Shown how the PLC shows it.
	Address     string `json:"address"`
	Shown       string `json:"shown"`
	Symbol      string `json:"symbol"`
	Description string `json:"description,omitempty"`
	// Input is set for a signal the PLC reads, which a connection writes to its item.
	Input  bool   `json:"input"`
	Analog bool   `json:"analog"`
	Unit   string `json:"unit,omitempty"`
}

// SignalsFor lists the signals of one of the station's PLCs, by its module, the first one for
// "": every bit its wiring reaches and every analogue channel.
func (s *Service) SignalsFor(plc string) ([]SignalInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, _, err := s.plcOf(plc); err != nil {
		return nil, err
	}
	descriptions := map[string]string{}
	plan := s.engine.Plan()
	k := max(0, slices.Index(plan.PLCs, plc))
	if k < len(plan.Scene.PLCs) {
		p := plan.Scene.PLCs[k]
		for _, sig := range append(slices.Clone(p.Inputs), p.Outputs...) {
			descriptions[sig.Address] = sig.Description
		}
		for _, a := range append(slices.Clone(p.Analog), p.AnalogOutputs...) {
			descriptions[a.Address] = a.Description
		}
	}
	out := []SignalInfo{}
	for _, sig := range s.signals(plc) {
		info := SignalInfo{Address: sig.Address.String(), Shown: sig.Shown, Symbol: sig.Symbol, Description: descriptions[sig.Address.String()], Input: sig.Address.Input, Analog: sig.Scaling != nil}
		if sig.Scaling != nil {
			info.Unit = sig.Scaling.Unit
		}
		out = append(out, info)
	}
	return out, nil
}

// PreviewBinding is a binding of a connection being edited, as it resolves.
type PreviewBinding struct {
	Address string `json:"address"`
	// Item is the driver's item, as JSON, with the vars filled in.
	Item      string `json:"item"`
	Invert    bool   `json:"invert"`
	Generated bool   `json:"generated"`
}

// Preview is what a connection being edited exchanges.
type Preview struct {
	Bindings []PreviewBinding `json:"bindings"`
	// Unbound are the addresses of the signals it does not exchange.
	Unbound []string `json:"unbound"`
	// Error says why it does not resolve, if it does not.
	Error string `json:"error,omitempty"`
}

// PreviewConnection resolves a connection being edited, given as its file, against one of
// the station's PLCs, with the values of its form.
func (s *Service) PreviewConnection(plc, doc string, values map[string]any) Preview {
	out := Preview{Bindings: []PreviewBinding{}, Unbound: []string{}}
	c, err := parseDraft(doc)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	s.mu.Lock()
	signals := s.signals(plc)
	s.mu.Unlock()
	r, err := c.Resolve(signals, values)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	for _, b := range r.Bindings {
		out.Bindings = append(out.Bindings, PreviewBinding{Address: b.Signal.Address.String(), Item: string(b.Item), Invert: b.Invert, Generated: b.Generated})
	}
	for _, u := range r.Unbound {
		out.Unbound = append(out.Unbound, u.Address.String())
	}
	return out
}

// BrowseConnection lists what the controller of a connection being edited offers below a
// node, "" for the top, to pick the items of bindings from.
func (s *Service) BrowseConnection(doc string, values map[string]any, under string) ([]link.Node, error) {
	c, err := parseDraft(doc)
	if err != nil {
		return nil, err
	}
	d, err := link.Lookup(c.Driver)
	if err != nil {
		return nil, err
	}
	b, ok := d.(link.Browser)
	if !ok {
		return nil, fmt.Errorf("the %s driver cannot show what a controller offers", c.Driver)
	}
	settings, err := c.SettingsWith(values)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	nodes, err := b.Browse(ctx, settings, under)
	if nodes == nil {
		nodes = []link.Node{}
	}
	return nodes, err
}

// parseDraft reads a connection being edited, which may have no id yet.
func parseDraft(doc string) (link.Connection, error) {
	// The fields stay as they are written, so that the order of a form's fields in vars is kept.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &fields); err != nil {
		return link.Connection{}, err
	}
	var id string
	if json.Unmarshal(fields["id"], &id) != nil || id == "" {
		fields["id"] = json.RawMessage(`"draft"`)
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return link.Connection{}, err
	}
	return link.Parse(data)
}

// SaveConnection checks a PLC's connection, as the connection editor has it, against the
// station, with the values of its form, and keeps it in the project as the PLC's own, of the
// kind it was made from. It does not connect it.
func (s *Service) SaveConnection(plc, doc string, values map[string]any) (PLCConfig, error) {
	c, err := parseDraft(doc)
	if err != nil {
		return PLCConfig{}, err
	}
	if _, err := link.Lookup(c.Driver); err != nil {
		return PLCConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return PLCConfig{}, errNoProject
	}
	if _, err := c.Resolve(s.signals(plc), values); err != nil {
		return PLCConfig{}, err
	}
	planned := s.proj.def.PLCs[s.plcKey(plc)]
	if planned.Mode == "" {
		planned.Mode = PLCNone
	}
	kind := c.ID
	if planned.External != nil {
		kind = planned.External.Kind
	}
	planned.External = &ExternalConnection{Kind: kind, Connection: c, Values: values}
	return planned, s.remember(plc, planned)
}

// FindCODESYSList looks for the variable list of the app's CODESYS connection on the server
// its form names, and gives the form's values with where the list is filled in.
func (s *Service) FindCODESYSList(plc string, values map[string]any) (map[string]any, error) {
	s.mu.Lock()
	naming := s.naming(plc)
	s.mu.Unlock()
	if naming.Inputs == "" {
		return nil, errors.New("the PLC module does not name a variable list")
	}
	c := codesys(naming)
	filled, err := c.Values(values)
	if err != nil {
		return nil, err
	}
	endpoint, _ := filled["endpoint"].(string)
	list, _ := filled["list"].(string)
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	found, err := opcua.FindCODESYSList(ctx, endpoint, list, 3000)
	if err != nil {
		return nil, err
	}
	filled["namespace"], filled["runtime"], filled["application"] = found.Namespace, found.Runtime, found.Application
	return filled, nil
}

// genericOPCUA is the app's connection to any OPC UA server, without bindings, to start one
// of one's own from.
func genericOPCUA() link.Connection {
	return link.Connection{
		ID:          "opcua",
		Name:        "OPC UA server",
		Description: "Any OPC UA server. Bind each signal to a node with Edit bindings.",
		Driver:      "opcua",
		Vars: json.RawMessage(`{
  "type": "object",
  "required": ["endpoint"],
  "properties": {
    "endpoint": { "type": "string", "title": "Server address", "default": "opc.tcp://127.0.0.1:4840", "pattern": "^opc\\.tcp://" }
  }
}`),
		Settings: json.RawMessage(`{"endpoint": "{endpoint}"}`),
	}
}
