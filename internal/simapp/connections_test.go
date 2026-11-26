package simapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/toblu96/taktwin/internal/link"
	"github.com/toblu96/taktwin/internal/opcsim"
	"github.com/toblu96/taktwin/internal/plcio"
)

// echo drives each output from the input with the same address.
type echo struct{}

func (echo) Name() string                    { return "echo" }
func (echo) Cycle() time.Duration            { return 5 * time.Millisecond }
func (echo) Scan(in plcio.Image) plcio.Image { return in }
func (echo) Reset()                          {}

func newService(t *testing.T) (*Service, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("TAKTWIN_HOME", home)
	t.Setenv("TAKTWIN_CONFIG", t.TempDir())
	t.Setenv("TAKTWIN_COLLECTIONS", "")
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewProject("Test", Base{Station: "pallet-loading"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.CloseProject)
	return s, home
}

func linkOf(s *Service) *plcio.LinkInfo {
	snap := s.Snapshot()
	if len(snap.PLCs) > 0 && snap.PLCs[0].Link != nil {
		return snap.PLCs[0].Link
	}
	return snap.Link
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A PLC is given a kind of connection: the app's own for CODESYS and for any OPC UA server,
// and those of the collections that name no station. A collection's connection for one
// station is a mapping, no kind, and the user's folder offers none.
func TestThePLCOffersKindsOfConnection(t *testing.T) {
	s, home := newService(t)
	dev := t.TempDir()
	t.Setenv("TAKTWIN_COLLECTIONS", dev)
	files := map[string]string{
		"collection.json":         `{"id": "kinds", "name": "Kinds", "version": "1.0.0"}`,
		"connections/ports.json":  `{"id": "ports", "name": "Ports", "driver": "opcua"}`,
		"connections/mine.json":   `{"id": "mine", "name": "Mine", "driver": "opcua", "station": "pallet-loading"}`,
		"connections/plc2.json":   `{"id": "plc2", "name": "Second PLC", "driver": "opcua", "plc": "nope"}`,
		"connections/broken.json": `{"id": `,
		"connections/notes.txt":   `not a connection`,
	}
	files["../home/connections/yours.json"] = `{"id": "yours", "name": "Yours", "driver": "opcua"}`
	for name, data := range files {
		p := filepath.Join(dev, name)
		if strings.HasPrefix(name, "../home/") {
			p = filepath.Join(home, strings.TrimPrefix(name, "../home/"))
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ConnectionKinds("")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range list {
		ids = append(ids, c.ID+"@"+c.Source)
	}
	if !slices.Equal(ids, []string{"codesys@Taktwin", "fluidsim-ports@Basics", "ports@Kinds", "opcua@Taktwin"}) {
		t.Errorf("kinds: %v", ids)
	}
	for _, want := range []string{`"default": "CODESYS Control Win V3 x64"`, `"default": "Application"`, `"default": "Bits"`} {
		if !strings.Contains(list[0].Vars, want) {
			t.Errorf("the CODESYS connection asks for the parts of its NodeIds: %s", list[0].Vars)
		}
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal}); err == nil {
		t.Error("connected without a kind of connection")
	}
	if _, err := s.ChooseConnection("", "mine"); err == nil {
		t.Error("a mapping is a kind")
	}
	planned, err := s.ChooseConnection("", "codesys")
	if err != nil {
		t.Fatal(err)
	}
	if planned.Mode != PLCNone || planned.External == nil || planned.External.Kind != "codesys" || len(planned.External.Connection.Generate) == 0 {
		t.Errorf("planned: %+v", planned)
	}
	if _, err := s.SetConnectionValues("", map[string]any{"endpoint": "http://x"}); err != nil {
		t.Fatal(err)
	}
	if got := s.PlannedPLC(""); got.External == nil || got.External.Values["endpoint"] != "http://x" {
		t.Errorf("the address is kept in the project: %+v", got)
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal}); err == nil || !strings.Contains(err.Error(), "/endpoint") {
		t.Errorf("an endpoint that does not fit: %v", err)
	}
}

// A PLC's own connection, with bindings saved from the editor, connects the station to a
// server; switching to no PLC and back brings the same connection back.
func TestAStationConnectsThroughItsOwnConnection(t *testing.T) {
	s, _ := newService(t)
	srv, err := opcsim.StartOnFreePort(context.Background(), echo{}, opcsim.Space{PLC: "plc"})
	if err != nil {
		t.Fatal(err)
	}
	// The link closes first: a server closed under a client waits for it.
	defer srv.Close()
	defer func() { _ = s.SetPLCFor("", PLCConfig{Mode: PLCNone}) }()
	planned, err := s.ChooseConnection("", "opcua")
	if err != nil {
		t.Fatal(err)
	}
	c := planned.External.Connection
	// The conveyor's run output back from its start sensor: B1 is %IX0.0, M1 %QX0.0.
	c.Bindings = []link.BindingDef{
		{Signal: "B1", Item: json.RawMessage(`{"nodeId": "nsu=urn:taktwin:softplc;s=plc.%IX0.0"}`)},
		{Signal: "M1", Item: json.RawMessage(`{"nodeId": "nsu=urn:taktwin:softplc;s=plc.%QX0.0"}`)},
	}
	doc, _ := json.Marshal(c)
	if _, err := s.SaveConnection("", string(doc), map[string]any{"endpoint": srv.Endpoint()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the connection", func() bool { l := linkOf(s); return l != nil && l.State == plcio.LinkConnected })
	if l := linkOf(s); l.Variables != 2 || len(l.Unbound) == 0 || len(l.Problems) > 0 {
		t.Errorf("two signals are exchanged, the others not: %+v", l)
	}
	if got := s.PLCFor(""); got.External == nil || got.External.Kind != "opcua" || got.External.Values["endpoint"] != srv.Endpoint() || len(got.External.Connection.Bindings) != 2 {
		t.Errorf("the PLC runs its connection: %+v", got)
	}

	if err := s.SetPLCFor("", PLCConfig{Mode: PLCNone}); err != nil {
		t.Fatal(err)
	}
	if got := s.PlannedPLC(""); got.Mode != PLCNone || got.External == nil || len(got.External.Connection.Bindings) != 2 {
		t.Errorf("the connection is kept while no PLC runs: %+v", got)
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the connection again", func() bool { l := linkOf(s); return l != nil && l.State == plcio.LinkConnected && l.Variables == 2 })
}

// A template's binding for a signal nothing of the station uses, without a symbol, which the
// controller has no node for, is left out without a word; one for a signal in use is reported.
func TestATemplateLeavesOutWhatNothingUses(t *testing.T) {
	s, _ := newService(t)
	srv, err := opcsim.StartOnFreePort(context.Background(), echo{}, opcsim.Space{PLC: "plc"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	defer func() { _ = s.SetPLCFor("", PLCConfig{Mode: PLCNone}) }()
	planned, err := s.ChooseConnection("", "opcua")
	if err != nil {
		t.Fatal(err)
	}
	c := planned.External.Connection
	c.Generate = []link.GenerateDef{{Signals: "inputs", Item: json.RawMessage(`{"nodeId": "nsu=urn:taktwin:softplc;s=nowhere.{address}"}`)}}
	doc, _ := json.Marshal(c)
	if _, err := s.SaveConnection("", string(doc), map[string]any{"endpoint": srv.Endpoint()}); err != nil {
		t.Fatal(err)
	}
	signals, err := s.SignalsFor("")
	if err != nil {
		t.Fatal(err)
	}
	named, unnamed := 0, 0
	for _, sig := range signals {
		if sig.Input && !sig.Analog {
			if sig.Symbol != "" {
				named++
			} else {
				unnamed++
			}
		}
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the connection", func() bool { l := linkOf(s); return l != nil && l.State == plcio.LinkConnected })
	l := linkOf(s)
	t.Logf("%d inputs in use, %d not", named, unnamed)
	if len(l.Problems) != named || l.Variables != 0 {
		t.Errorf("%d signals in use are reported, %d not: %+v", named, unnamed, l)
	}
	if outputs := len(signals) - named - unnamed; len(l.Unbound) < outputs+unnamed {
		t.Errorf("the unused inputs are listed as not exchanged: %+v", l.Unbound)
	}
}

// The soft PLC is reached through its server, as any other.
func TestTheSoftPLCIsReachedThroughItsServer(t *testing.T) {
	s, _ := newService(t)
	t.Cleanup(softPLCs.Close)
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCSoft}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.SetPLCFor("", PLCConfig{Mode: PLCNone}) }()
	waitFor(t, "the connection", func() bool { l := linkOf(s); return l != nil && l.State == plcio.LinkConnected })
	l := linkOf(s)
	if len(l.Problems) > 0 || l.Variables == 0 || len(l.Unbound) > 0 || !strings.HasPrefix(l.Endpoint, "opc.tcp://127.0.0.1:") {
		t.Errorf("every signal of the station is exchanged: %+v", l)
	}
}

// A PLC connected to a controller that cannot be reached can be let go of: the link ends and the
// station runs without it, while the project keeps the connection and its values, ready to
// connect again.
func TestAConnectionThatCannotBeReachedIsLetGoOf(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.ChooseConnection("", "opcua"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetConnectionValues("", map[string]any{"endpoint": "opc.tcp://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal}); err != nil {
		t.Fatal(err)
	}
	if linkOf(s) == nil {
		t.Fatal("it tries to connect")
	}
	if err := s.DisconnectPLCFor(""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the link to end", func() bool { return linkOf(s) == nil })
	if got := s.PLCFor(""); got.Mode != PLCNone {
		t.Errorf("no PLC runs: %+v", got)
	}
	if got := s.PlannedPLC(""); got.External == nil || got.External.Kind != "opcua" || got.External.Values["endpoint"] != "opc.tcp://127.0.0.1:1" {
		t.Errorf("the project keeps the connection to connect again: %+v", got)
	}
}

// Trying another kind of connection loses nothing: choosing the first kind again brings its
// form back, and connecting keeps the others in the project.
func TestAPLCKeepsAConnectionOfEachKind(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.ChooseConnection("", "codesys"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetConnectionValues("", map[string]any{"endpoint": "opc.tcp://plc:4840"}); err != nil {
		t.Fatal(err)
	}
	planned, err := s.ChooseConnection("", "opcua")
	if err != nil {
		t.Fatal(err)
	}
	if planned.External.Kind != "opcua" || len(planned.External.Values) > 0 || len(planned.Others) != 1 || planned.Others[0].Kind != "codesys" {
		t.Errorf("a new connection of the other kind, the first kept: %+v", planned)
	}
	if _, err := s.SetConnectionValues("", map[string]any{"endpoint": "opc.tcp://server:4840"}); err != nil {
		t.Fatal(err)
	}
	planned, err = s.ChooseConnection("", "codesys")
	if err != nil {
		t.Fatal(err)
	}
	if planned.External.Values["endpoint"] != "opc.tcp://plc:4840" || len(planned.Others) != 1 || planned.Others[0].Values["endpoint"] != "opc.tcp://server:4840" {
		t.Errorf("each kind comes back with its form: %+v", planned)
	}
	if again, _ := s.ChooseConnection("", "codesys"); !samePLC(again, planned) {
		t.Errorf("choosing the kind it has changes nothing: %+v", again)
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCSoft}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.SetPLCFor("", PLCConfig{Mode: PLCNone}) }()
	if got := s.PlannedPLC(""); got.Mode != PLCSoft || got.External == nil || got.External.Kind != "codesys" || len(got.Others) != 1 {
		t.Errorf("connecting keeps every connection: %+v", got)
	}
}
