package simapp

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	t.Cleanup(func() { s.current().Close() })
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

// A PLC can use the app's connection to CODESYS, and the connection files of the user's
// folder that fit the station.
func TestThePLCOffersItsConnections(t *testing.T) {
	s, home := newService(t)
	dir := filepath.Join(home, ConnectionsFolder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"mine.json":   `{"id": "mine", "name": "Mine", "driver": "opcua", "station": "pallet-loading"}`,
		"other.json":  `{"id": "other", "name": "Other", "driver": "opcua", "station": "elsewhere"}`,
		"plc2.json":   `{"id": "plc2", "name": "Second PLC", "driver": "opcua", "plc": "nope"}`,
		"broken.json": `{"id": `,
		"notes.txt":   `not a connection`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ConnectionsFor("")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range list {
		ids = append(ids, c.ID+"@"+c.Source)
	}
	if !slices.Equal(ids, []string{"codesys@Taktwin", "mine@Your folder"}) {
		t.Errorf("connections: %v", ids)
	}
	for _, want := range []string{`"default": "CODESYS Control Win V3 x64"`, `"default": "Application"`, `"default": "Bits"`} {
		if !strings.Contains(list[0].Vars, want) {
			t.Errorf("the CODESYS connection asks for the parts of its NodeIds: %s", list[0].Vars)
		}
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal, Connection: "nope"}); err == nil {
		t.Error("a connection that does not exist")
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal, Connection: "codesys", Values: map[string]any{"endpoint": "http://x"}}); err == nil || !strings.Contains(err.Error(), "/endpoint") {
		t.Errorf("an endpoint that does not fit: %v", err)
	}
}

// A connection file of the user's folder connects the station to a server of its own
// choosing, with the values filled in.
func TestAStationConnectsThroughAConnectionFile(t *testing.T) {
	s, home := newService(t)
	srv, err := opcsim.StartOnFreePort(context.Background(), echo{}, opcsim.Space{PLC: "plc"})
	if err != nil {
		t.Fatal(err)
	}
	// The link closes first: a server closed under a client waits for it.
	defer srv.Close()
	defer func() { _ = s.SetPLCFor("", PLCConfig{Mode: PLCNone}) }()
	dir := filepath.Join(home, ConnectionsFolder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The conveyor's run output back from its start sensor: B1 is %IX0.0, M1 %QX0.0.
	file := `{
  "id": "echo", "name": "Echo", "driver": "opcua",
  "vars": { "type": "object", "properties": { "endpoint": { "type": "string" } } },
  "settings": { "endpoint": "{endpoint}" },
  "bindings": [
    { "signal": "B1", "item": { "nodeId": "nsu=urn:taktwin:softplc;s=plc.%IX0.0" } },
    { "signal": "M1", "item": { "nodeId": "nsu=urn:taktwin:softplc;s=plc.%QX0.0" } }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "echo.json"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCExternal, Connection: "echo", Values: map[string]any{"endpoint": srv.Endpoint()}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the connection", func() bool { l := linkOf(s); return l != nil && l.State == plcio.LinkConnected })
	if l := linkOf(s); l.Variables != 2 || len(l.Unbound) == 0 || len(l.Problems) > 0 {
		t.Errorf("two signals are exchanged, the others not: %+v", l)
	}
	if got := s.PLCFor(""); got.Connection != "echo" || got.Values["endpoint"] != srv.Endpoint() {
		t.Errorf("the PLC keeps its connection: %+v", got)
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
