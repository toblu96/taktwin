package simapp

import (
	"context"
	"encoding/json"
	"maps"
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

// connectionDraft is the PLC's own connection being edited, made from the app's generic one.
func connectionDraft(t *testing.T, s *Service, edit func(map[string]any)) string {
	t.Helper()
	planned, err := s.ChooseConnection("", "opcua")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(planned.External.Connection)
	var c map[string]any
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	edit(c)
	data, _ = json.Marshal(c)
	return string(data)
}

// The editor sees what a connection exchanges as it is edited, and keeps it in the project as
// the PLC's own, not in the user's folder.
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

	if _, err := s.SaveConnection("", strings.Replace(doc, `"signal":"M1"`, `"signal":"M9"`, 1), values); err == nil {
		t.Error("a connection that does not resolve is kept")
	}
	planned, err := s.SaveConnection("", doc, values)
	if err != nil {
		t.Fatal(err)
	}
	if planned.External == nil || planned.External.Kind != "opcua" || len(planned.External.Connection.Bindings) != 1 || planned.External.Values["endpoint"] != "opc.tcp://127.0.0.1:4840" {
		t.Errorf("kept as the PLC's own: %+v", planned)
	}
	if got := s.PlannedPLC(""); got.External == nil || len(got.External.Connection.Generate) != 1 {
		t.Errorf("the project keeps it: %+v", got)
	}
	s.SaveProject()
	if def := defIn(t, s.Project().Path); def.PLCs[""].External == nil || len(def.PLCs[""].External.Connection.Bindings) != 1 {
		t.Errorf("the file has it: %+v", def.PLCs)
	}
	if items, _ := os.ReadDir(filepath.Join(home, ConnectionsFolder)); len(items) != 0 {
		t.Errorf("a connection file was written into the user's folder: %v", items)
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

// The programming tool checks the files being typed without saving them, formats a file as
// CODESYS does, and remembers where it was left per project on this computer, not in the
// project file.
func TestTheProgrammingToolChecksFormatsAndRemembers(t *testing.T) {
	s, _ := newService(t)
	prog := NewProgramService(s)
	opened, err := prog.Open("")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := prog.ReadFile("", "PLC_PRG.st")
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(saved, "END_PROGRAM", "x := ;\nEND_PROGRAM", 1)
	if !strings.Contains(saved, "END_PROGRAM") {
		broken = saved + "\nx := ;\n"
	}
	res, err := prog.Check("", map[string]string{"PLC_PRG.st": broken})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || len(res.Diagnostics) == 0 {
		t.Errorf("the mistake is not found: %+v", res)
	}
	if again, _ := prog.ReadFile("", "PLC_PRG.st"); again != saved {
		t.Error("checking saved the file")
	}
	if res, _ := prog.Check("", nil); !res.OK {
		t.Errorf("the saved project has mistakes: %+v", res.Diagnostics)
	}

	if out, err := prog.Format("x.st", "program x\nvar a:int; end_var\na:=1;\nend_program\n"); err != nil || !strings.Contains(out, "a := 1;") || !strings.Contains(out, "PROGRAM x") {
		t.Errorf("formatted: %q %v", out, err)
	}

	if st := prog.EditorState(""); len(st.Open) != 0 {
		t.Errorf("a new project remembers editors: %+v", st)
	}
	want := EditorState{Open: []string{"PLC_PRG.st", "StepChain.st"}, Active: "StepChain.st", Places: map[string]EditorPlace{"StepChain.st": {Cursor: 120, Top: 40}}}
	if err := prog.SetEditorState("", want); err != nil {
		t.Fatal(err)
	}
	file := s.Project().Path
	s.CloseProject()
	if _, err := s.OpenProjectFile(file, OpenOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := prog.EditorState(""); got.Active != "StepChain.st" || len(got.Open) != 2 || got.Places["StepChain.st"].Cursor != 120 {
		t.Errorf("not remembered: %+v", got)
	}
	if strings.Contains(strings.Join(slices.Collect(maps.Keys(zipFiles(t, file))), " "), "editor") {
		t.Error("the editors are in the project file")
	}
	if _, err := s.NewProject("", Base{Station: "pallet-loading"}); err != nil {
		t.Fatal(err)
	}
	if got := prog.EditorState(""); len(got.Open) != 0 {
		t.Errorf("another project has them: %+v", got)
	}
	_ = opened
}

// Saving a connection from the editor keeps its form's fields in the order they were written,
// so that the form asks for the server's address first, as the kind does.
func TestSavingAConnectionKeepsTheOrderOfItsForm(t *testing.T) {
	s, _ := newService(t)
	planned, err := s.ChooseConnection("", "codesys")
	if err != nil {
		t.Fatal(err)
	}
	before := string(planned.External.Connection.Vars)
	doc, _ := json.Marshal(planned.External.Connection)
	saved, err := s.SaveConnection("", string(doc), map[string]any{"endpoint": "opc.tcp://127.0.0.1:4840"})
	if err != nil {
		t.Fatal(err)
	}
	if a, b := strings.Index(before, `"endpoint"`), strings.Index(before, `"application"`); a < 0 || b < 0 || a > b {
		t.Fatalf("the kind asks for the address first: %s", before)
	}
	after := string(saved.External.Connection.Vars)
	if a, b := strings.Index(after, `"endpoint"`), strings.Index(after, `"application"`); a < 0 || a > b {
		t.Errorf("the saved form asks for the address first: %s", after)
	}
}
