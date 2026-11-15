package simapp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/ua"

	"github.com/toblu96/taktwin/internal/opcsim"
)

// connectionDraft is a connection being edited, from the app's generic one.
func connectionDraft(t *testing.T, s *Service, edit func(map[string]any)) string {
	t.Helper()
	doc, err := s.ConnectionFor("", "opcua")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Yours || doc.Info.Source != "Taktwin" {
		t.Errorf("the generic connection is the app's: %+v", doc)
	}
	var c map[string]any
	if err := json.Unmarshal([]byte(doc.JSON), &c); err != nil {
		t.Fatal(err)
	}
	c["id"], c["name"] = "mine", "Mine"
	edit(c)
	data, _ := json.Marshal(c)
	return string(data)
}

// The editor sees what a connection exchanges as it is edited, keeps it in the user's folder,
// and removes it again.
func TestAConnectionIsEditedAndKept(t *testing.T) {
	s, home := newService(t)
	signals, err := s.SignalsFor("")
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) == 0 || signals[0].Address != "%IX0.0" || signals[0].Symbol != "B1" || !signals[0].Input || signals[0].Description == "" {
		t.Fatalf("signals: %+v", signals)
	}
	doc := connectionDraft(t, s, func(c map[string]any) {
		c["bindings"] = []any{map[string]any{"signal": "M1", "item": map[string]any{"nodeId": "ns=2;s=run"}, "invert": true}}
		c["generate"] = []any{map[string]any{"signals": "inputs", "item": map[string]any{"nodeId": "ns=2;s=in.{shown}"}}}
	})
	values := map[string]any{"endpoint": "opc.tcp://127.0.0.1:4840"}
	p := s.PreviewConnection("", doc, values)
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	byAddress := map[string]PreviewBinding{}
	for _, b := range p.Bindings {
		byAddress[b.Address] = b
	}
	if b := byAddress["%IX0.0"]; !b.Generated || b.Item != `{"nodeId":"ns=2;s=in.IB0Bit00"}` {
		t.Errorf("B1 by the template: %+v", b)
	}
	if b := byAddress["%QX0.0"]; b.Generated || !b.Invert || b.Item != `{"nodeId":"ns=2;s=run"}` {
		t.Errorf("M1 bound by hand: %+v", b)
	}
	if !slices.Contains(p.Unbound, "%QX0.1") {
		t.Errorf("Y1 is not exchanged: %v", p.Unbound)
	}
	if p := s.PreviewConnection("", strings.Replace(doc, `"signal":"M1"`, `"signal":"M9"`, 1), values); !strings.Contains(p.Error, "M9") {
		t.Errorf("an unknown symbol: %+v", p)
	}

	info, err := s.SaveConnection("", doc, values)
	if err != nil {
		t.Fatal(err)
	}
	if info.Source != "Your folder" {
		t.Errorf("saved into the user's folder: %+v", info)
	}
	if _, err := os.Stat(filepath.Join(home, ConnectionsFolder, "mine.json")); err != nil {
		t.Fatal(err)
	}
	if again, err := s.ConnectionFor("", "mine"); err != nil || !again.Yours {
		t.Errorf("listed as the user's: %+v %v", again, err)
	}
	for id, want := range map[string]string{"codesys": "of the app", "../x": "no id", "opcua": "of the app"} {
		bad := strings.Replace(doc, `"id":"mine"`, `"id":"`+id+`"`, 1)
		if _, err := s.SaveConnection("", bad, values); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("id %s: %v", id, err)
		}
	}
	if err := s.DeleteConnection("mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConnectionFor("", "mine"); err == nil {
		t.Error("it is gone")
	}
}

// The editor shows the nodes of the soft PLC's server, each with its NodeId by namespace URI,
// its type and what a binding can do with it.
func TestTheEditorBrowsesAServer(t *testing.T) {
	s, _ := newService(t)
	srv, err := opcsim.StartOnFreePort(context.Background(), echo{}, opcsim.Space{PLC: "plc", Labels: map[string]string{"%IX0.0": "IB0Bit00 B1"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	doc := connectionDraft(t, s, func(map[string]any) {})
	top, err := s.BrowseConnection(doc, map[string]any{"endpoint": srv.Endpoint()}, "")
	if err != nil {
		t.Fatal(err)
	}
	// The objects of the server and the soft PLC's namespace below Objects.
	var found bool
	var below []string
	for _, n := range top {
		if n.Below && !strings.HasPrefix(n.ID, "i=") {
			below = append(below, n.Under)
		}
	}
	for _, under := range below {
		nodes, err := s.BrowseConnection(doc, map[string]any{"endpoint": srv.Endpoint()}, under)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range nodes {
			if n.ID == "nsu=urn:taktwin:softplc;s=plc.%IX0.0" {
				found = true
				if n.Type != "Boolean" || !n.Readable || !n.Writable || n.Name != "IB0Bit00 B1" || string(n.Item) != `{"nodeId":"nsu=urn:taktwin:softplc;s=plc.%IX0.0"}` {
					t.Errorf("node: %+v", n)
				}
			}
		}
	}
	if !found {
		t.Errorf("the soft PLC's nodes are not below %v", below)
	}
	if _, err := s.BrowseConnection(doc, map[string]any{"endpoint": "opc.tcp://127.0.0.1:1"}, ""); err == nil || !strings.Contains(err.Error(), "no OPC UA server") {
		t.Errorf("no server: %v", err)
	}
}

// Find fills in the CODESYS form from the server, by the name of the list.
func TestFindFillsInTheCODESYSForm(t *testing.T) {
	s, _ := newService(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	srv := server.New(server.EndPoint("127.0.0.1", port), server.EnableSecurity("None", ua.MessageSecurityModeNone), server.EnableAuthMode(ua.UserTokenTypeAnonymous))
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	server.NewMapNamespace(srv, "urn:test:other")
	ns := server.NewMapNamespace(srv, "urn:test:codesys")
	root, _ := srv.Namespace(0)
	root.Objects().AddRef(ns.Objects(), id.HasComponent, true)
	ns.SetValue("|var|My PLC.Line.Bits", int32(0))
	ns.SetValue("|var|My PLC.Line.Bits.IB0Bit00", false)
	endpoint := "opc.tcp://127.0.0.1:" + strconv.Itoa(port)
	values, err := s.FindCODESYSList("", map[string]any{"endpoint": endpoint})
	if err != nil {
		t.Fatal(err)
	}
	if values["runtime"] != "My PLC" || values["application"] != "Line" || values["namespace"] != "nsu=urn:test:codesys" || values["list"] != "Bits" || values["endpoint"] != endpoint {
		t.Errorf("values: %v", values)
	}
	if _, err := s.FindCODESYSList("", map[string]any{"endpoint": endpoint, "list": "Nope"}); err == nil || !strings.Contains(err.Error(), "symbol configuration") {
		t.Errorf("a list the server has not: %v", err)
	}
}
