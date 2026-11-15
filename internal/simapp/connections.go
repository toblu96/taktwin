package simapp

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/toblu96/taktwin/internal/link"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/opcsim"
	"github.com/toblu96/taktwin/internal/plcio"
)

// ConnectionsFolder is where collections and the user's folder keep connection files.
const ConnectionsFolder = "connections"

// ConnectionInfo is a connection a PLC of the station can use, for the PLC panel.
type ConnectionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Source is where it comes from: the app, a collection by name, or the user's folder.
	Source string `json:"source"`
	Driver string `json:"driver"`
	// Vars is the JSON Schema of the values someone fills in, as JSON; "" for none.
	Vars string `json:"vars"`
}

// ConnectionsFor lists the connections one of the station's PLCs can use, by its module,
// the first one for "": the app's own for CODESYS, one for each address map of the station,
// then the files of the user's folder and of the collections.
func (s *Service) ConnectionsFor(plc string) ([]ConnectionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.connections(plc)
	if err != nil {
		return nil, err
	}
	out := []ConnectionInfo{}
	for _, c := range list {
		out = append(out, ConnectionInfo{ID: c.conn.ID, Name: c.conn.Name, Description: c.conn.Description, Source: c.source, Driver: c.conn.Driver, Vars: string(c.conn.Vars)})
	}
	return out, nil
}

type offered struct {
	conn   link.Connection
	source string
}

// connections are those a PLC can use, under s.mu.
func (s *Service) connections(plc string) ([]offered, error) {
	station, id, err := s.plcOf(plc)
	if err != nil {
		return nil, err
	}
	var out []offered
	first := s.plcKey(id) == ""
	if naming := s.naming(plc); naming.Inputs != "" {
		out = append(out, offered{codesys(naming, nil), "Taktwin"})
		if first {
			for _, m := range s.engine.Plan().Maps() {
				out = append(out, offered{codesys(naming, &m), "Taktwin"})
			}
		}
	}
	lib := Library()
	names := map[string]string{}
	for _, c := range lib.Collections() {
		names[c.ID] = c.Name
	}
	seen := map[string]bool{}
	for _, o := range out {
		seen[o.conn.ID] = true
	}
	for _, f := range lib.Files(ConnectionsFolder) {
		if !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		c, err := link.Parse(f.Data)
		if err != nil || seen[c.ID] {
			continue
		}
		if c.Station != "" && c.Station != station || c.PLC != "" && c.PLC != id || c.PLC == "" && !first {
			continue
		}
		seen[c.ID] = true
		out = append(out, offered{c, cmp.Or(names[f.Collection], "Your folder")})
	}
	return out, nil
}

// codesys is the app's connection to a CODESYS PLC that offers its image as a variable
// list, named as the PLC module says, through its symbol configuration. With a map, the
// variables it names stand for other signals, of a program written for another wiring.
func codesys(naming plcio.Naming, m *model.PLCMapDef) link.Connection {
	c := link.Connection{
		ID:          "codesys",
		Name:        "CODESYS: variable list " + naming.List,
		Description: fmt.Sprintf("A CODESYS PLC with the variable list %s in its symbol configuration, with %s for inputs and %s for outputs, and no security.", naming.List, naming.Input(plcio.Addr{}), naming.Output(plcio.Addr{})),
		Driver:      "opcua",
		Vars: json.RawMessage(`{
  "type": "object",
  "required": ["endpoint", "runtime", "application", "list", "namespace"],
  "properties": {
    "endpoint": { "type": "string", "title": "Server address", "description": "opc.tcp:// and the computer the PLC runs on, with port 4840 unless its OPC UA server is set to another", "default": "opc.tcp://127.0.0.1:4840", "pattern": "^opc\\.tcp://" },
    "runtime": { "type": "string", "title": "PLC name", "description": "How the PLC names itself in its variables, the part after |var|: the name of its runtime, such as CODESYS Control Win V3 x64, unless its configuration sets another", "default": "CODESYS Control Win V3 x64", "minLength": 1 },
    "application": { "type": "string", "title": "Application", "description": "The application in the device tree of the CODESYS project", "default": "Application", "minLength": 1 },
    "list": { "type": "string", "title": "Variable list", "description": "The global variable list with the image, as named in the CODESYS project", "default": "` + naming.List + `", "minLength": 1 },
    "namespace": { "type": "string", "title": "Namespace", "description": "The namespace of the PLC's variables on its server, as ns=<index> or nsu=<URI>: ns=4 for CODESYS", "default": "ns=4", "pattern": "^nsu?=" }
  }
}`),
		Settings: json.RawMessage(`{"endpoint": "{endpoint}"}`),
	}
	item := func(name string) json.RawMessage {
		data, _ := json.Marshal(map[string]string{"nodeId": "{namespace};s=|var|{runtime}.{application}.{list}." + name})
		return data
	}
	c.Generate = []link.GenerateDef{{Signals: "inputs", Item: item(naming.Inputs)}, {Signals: "outputs", Item: item(naming.Outputs)}}
	if naming.Analog != "" {
		c.Generate = append(c.Generate, link.GenerateDef{Signals: "analogInputs", Item: item(naming.Analog)})
	}
	if m != nil {
		c.ID, c.Name = "codesys-"+m.ID, "CODESYS: "+m.Name
		c.Description = fmt.Sprintf("As %s, for a program written for other addresses.", naming.List)
		for _, list := range [][]model.PLCBindingDef{m.Inputs, m.Outputs} {
			for _, b := range list {
				c.Bindings = append(c.Bindings, link.BindingDef{Signal: b.Signal, Item: item(b.Variable), Invert: b.Invert})
			}
		}
	}
	return c
}

// signals are the signals of one of the station's PLCs, by its module, under s.mu.
func (s *Service) signals(plc string) []link.Signal {
	plan := s.engine.Plan()
	k := slices.Index(plan.PLCs, plc)
	if plc == "" {
		k = 0
	}
	return link.Signals(plan, k)
}

// space is how the soft PLC of one of the station's PLCs names its nodes, under s.mu.
func (s *Service) space(plc, id string) opcsim.Space {
	labels := map[string]string{}
	var addresses []plcio.Address
	for _, sig := range s.signals(plc) {
		labels[sig.Address.String()] = strings.TrimSpace(sig.Shown + " " + sig.Symbol)
		addresses = append(addresses, sig.Address)
	}
	return opcsim.Space{PLC: id, Labels: labels, Addresses: addresses}
}

// connection finds a connection by id for one of the station's PLCs, under s.mu.
func (s *Service) connection(plc, id string) (link.Connection, error) {
	list, err := s.connections(plc)
	if err != nil {
		return link.Connection{}, err
	}
	for _, o := range list {
		if o.conn.ID == id {
			return o.conn, nil
		}
	}
	if id == "" {
		return link.Connection{}, fmt.Errorf("choose a connection")
	}
	return link.Connection{}, fmt.Errorf("there is no connection %q for this PLC", id)
}
