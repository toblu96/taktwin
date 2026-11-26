package simapp

import (
	"cmp"
	"encoding/json"
	"errors"
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

// ConnectionInfo is a kind of connection a PLC of the station can be given, for the PLC
// panel: a technology, such as CODESYS or an OPC UA server, not a mapping of one station.
type ConnectionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Source is where it comes from: the app, or a collection by name.
	Source string `json:"source"`
	Driver string `json:"driver"`
	// Vars is the JSON Schema of the values someone fills in, as JSON; "" for none.
	Vars string `json:"vars"`
}

// ConnectionKinds lists the kinds of connection one of the station's PLCs, by its module, the
// first one for "", can be given: the app's own for CODESYS, those of the collections, which
// name no station, and one to any OPC UA server.
func (s *Service) ConnectionKinds(plc string) ([]ConnectionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.kinds(plc)
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

// kinds are the kinds of connection a PLC can be given, under s.mu. A connection file of a
// collection that names a station is a mapping of that station, which a project keeps as its
// own connection, and is no kind.
//
// A kind is only where a connection starts: once chosen, the project keeps a copy of its own.
// So the kinds come from the collections installed here, the built-in ones as this version of
// the app has them, and only those that are not installed from the copies the project carries,
// such as a teacher's collection the student does not have.
func (s *Service) kinds(plc string) ([]offered, error) {
	_, id, err := s.plcOf(plc)
	if err != nil {
		return nil, err
	}
	var out []offered
	if naming := s.naming(plc); naming.Inputs != "" {
		out = append(out, offered{codesys(naming), "Taktwin"})
	}
	installed := map[string]offered{}
	collect(Library(), id, func(o offered) { installed[o.conn.ID] = o })
	seen := map[string]bool{"codesys": true, genericOPCUA().ID: true}
	collect(libraryOf(s.proj), id, func(o offered) {
		if seen[o.conn.ID] {
			return
		}
		seen[o.conn.ID] = true
		if newer, ok := installed[o.conn.ID]; ok {
			o = newer
		}
		out = append(out, o)
	})
	return append(out, offered{genericOPCUA(), "Taktwin"}), nil
}

// collect gives each kind of connection a library has for the PLC id, in the library's order:
// its connection files that name no station, and no other PLC.
func collect(lib *model.Library, plc string, each func(offered)) {
	names := map[string]string{}
	for _, c := range lib.Collections() {
		names[c.ID] = c.Name
	}
	for _, f := range lib.Files(ConnectionsFolder) {
		if !strings.HasSuffix(f.Name, ".json") || f.Collection == "" {
			continue
		}
		c, err := link.Parse(f.Data)
		if err != nil || c.Station != "" || c.PLC != "" && c.PLC != plc {
			continue
		}
		each(offered{c, cmp.Or(names[f.Collection], f.Collection)})
	}
}

// ChooseConnection gives one of the station's PLCs, by its module, its connection of a kind:
// the one the project kept of that kind, with its form and bindings, or else a new one. The
// connection it had is kept among the others, so that choosing its kind again brings it back,
// and nothing is lost by trying another. It gives how the project now means to connect the PLC,
// and does not connect it.
func (s *Service) ChooseConnection(plc, kind string) (PLCConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.kinds(plc)
	if err != nil {
		return PLCConfig{}, err
	}
	i := slices.IndexFunc(list, func(o offered) bool { return o.conn.ID == kind })
	if i < 0 {
		return PLCConfig{}, fmt.Errorf("there is no kind of connection %q for this PLC", kind)
	}
	planned := s.proj.def.PLCs[s.plcKey(plc)]
	if planned.Mode == "" {
		planned.Mode = PLCNone
	}
	if planned.External != nil && planned.External.Kind == kind {
		return planned, nil
	}
	others := slices.Clone(planned.Others)
	next := ExternalConnection{Kind: kind, Connection: list[i].conn}
	if j := slices.IndexFunc(others, func(e ExternalConnection) bool { return e.Kind == kind }); j >= 0 {
		next = others[j]
		others = slices.Delete(others, j, j+1)
	}
	if planned.External != nil {
		others = append(others, *planned.External)
	}
	slices.SortFunc(others, func(a, b ExternalConnection) int { return strings.Compare(a.Kind, b.Kind) })
	planned.External, planned.Others = &next, others
	if len(others) == 0 {
		planned.Others = nil
	}
	return planned, s.remember(plc, planned)
}

// SetConnectionValues keeps what the form of a PLC's connection is filled in with, such as
// the server's address, in the project, without connecting.
func (s *Service) SetConnectionValues(plc string, values map[string]any) (PLCConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return PLCConfig{}, errNoProject
	}
	planned := s.proj.def.PLCs[s.plcKey(plc)]
	if planned.External == nil {
		return PLCConfig{}, errors.New("choose the kind of connection first")
	}
	ext := *planned.External
	ext.Values = values
	planned.External = &ext
	return planned, s.remember(plc, planned)
}

// codesys is the app's connection to a CODESYS PLC that offers its image as a variable
// list, named as the PLC module says, through its symbol configuration. A program written for
// another wiring gets bindings of its own in the project's connection.
func codesys(naming plcio.Naming) link.Connection {
	c := link.Connection{
		ID:          "codesys",
		Name:        "CODESYS: variable list " + naming.List,
		Description: fmt.Sprintf("A CODESYS PLC with the variable list %s in its symbol configuration.", naming.List),
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
