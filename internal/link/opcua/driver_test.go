package opcua

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/ua"

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

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func softServer(t *testing.T, port int) *opcsim.Server {
	t.Helper()
	s, err := opcsim.Start(context.Background(), port, echo{}, opcsim.Space{PLC: "cabinet"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func start(t *testing.T, c link.Connection, signals []link.Signal, values map[string]any) *link.Link {
	t.Helper()
	r, err := c.Resolve(signals, values)
	if err != nil {
		t.Fatal(err)
	}
	l, err := link.Start(context.Background(), link.Options{Name: "test", Connection: r})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	return l
}

func TestTheSoftPLCExchangesItsImage(t *testing.T) {
	s := softServer(t, freePort(t))
	// Cleanups run backwards: the link closes before the server, which otherwise waits for it.
	t.Cleanup(s.Close)
	l := start(t, SoftPLC(s.Endpoint(), "cabinet"), link.Image(), nil)
	until(t, "the connection", func() bool { return l.Link().State == plcio.LinkConnected })
	if got := l.Link(); got.Variables != 2*(8*8+4) || len(got.Problems) > 0 {
		t.Errorf("every signal is exchanged: %+v", got)
	}
	var in plcio.Image
	in.Set(plcio.Addr{Byte: 0, Bit: 6}, true)
	in.Set(plcio.Addr{Byte: 7, Bit: 2}, true)
	until(t, "the echo", func() bool { return l.Scan(in) == in })
	// It also clears again, so writing only what changes works both ways.
	in = plcio.Image{}
	until(t, "the echo to clear", func() bool { return l.Scan(in) == in })

	// An analogue input reaches the PLC as its raw INT, and the echo's output comes back.
	volts := plcio.Scaling{Range: [2]float64{0, 10}, Raw: [2]float64{0, 27648}}
	volts.Write(&in, plcio.WordAddress(true, 10), 7.5)
	until(t, "the analogue input", func() bool {
		l.Scan(in)
		return s.Inputs().Raw(plcio.WordAddress(true, 10)) == 20736
	})
	until(t, "the analogue output", func() bool {
		return volts.Read(l.Scan(in), plcio.WordAddress(false, 10)) == 7.5
	})
}

// A lost controller must not leave the station's outputs on, and the link comes back.
func TestALinkDropsOutputsAndReconnects(t *testing.T) {
	port := freePort(t)
	s := softServer(t, port)
	l := start(t, SoftPLC(s.Endpoint(), "cabinet"), link.Image(), nil)
	var in plcio.Image
	in.Set(plcio.Addr{Byte: 0, Bit: 0}, true)
	until(t, "the echo", func() bool { return l.Scan(in) == in })

	s.Close()
	until(t, "the outputs to drop", func() bool { return l.Scan(in) == plcio.Image{} })
	until(t, "the link to fail", func() bool { return l.Link().State == plcio.LinkFailed })
	s = softServer(t, port)
	t.Cleanup(s.Close)
	until(t, "the echo after reconnecting", func() bool { return l.Scan(in) == in })
}

func TestALinkSaysWhyItCannotConnect(t *testing.T) {
	c := SoftPLC("opc.tcp://127.0.0.1:"+strconv.Itoa(freePort(t)), "cabinet")
	l := start(t, c, link.Image(), nil)
	until(t, "the failure", func() bool { return l.Link().State == plcio.LinkFailed })
	if !strings.Contains(l.Link().Detail, "no OPC UA server") {
		t.Errorf("detail: %s", l.Link().Detail)
	}
	for settings, want := range map[string]string{
		`{"endpoint": "http://x"}`: "not an OPC UA address",
		`{"endpoint": "opc.tcp://x", "security": {"policy": "Basic256Sha256", "mode": "SignAndEncrypt"}}`: "not supported yet",
		`{"endpoint": "opc.tcp://x", "outputs": {"interval": 1}}`:                                         "outside 5 to 1000",
		`{"endpoint": "opc.tcp://x", "port": 4840}`:                                                       "unknown field",
	} {
		if _, err := (Driver{}).Open(context.Background(), []byte(settings), nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", settings, err)
		}
	}
}

// fluidServer is a server laid out as FluidSIM's: nodes named by project, component and
// variable in a namespace of its own, where state is read-only, state_man writable, and a
// valve's state an Int32.
type fluidSpace struct {
	*server.MapNamespace
}

// Name is the namespace's URI, which the map namespace inside has a variant of: the library
// registers that one too.
func (f fluidSpace) Name() string { return fluidURI }

func (f fluidSpace) Attribute(n *ua.NodeID, a ua.AttributeID) *ua.DataValue {
	f.Mu.RLock()
	defer f.Mu.RUnlock()
	if _, ok := f.Data[n.StringID()]; !ok && n.IntID() == 0 {
		// As a real server answers.
		return &ua.DataValue{EncodingMask: ua.DataValueStatusCode, Status: ua.StatusBadNodeIDUnknown}
	}
	dv := f.MapNamespace.Attribute(n, a)
	if a == ua.AttributeIDAccessLevel && strings.HasSuffix(n.StringID(), ".B1.state") {
		dv.Value = ua.MustVariant(byte(ua.AccessLevelTypeCurrentRead))
	}
	return dv
}

const fluidURI = "urn:test:fluid"

func fluidServer(t *testing.T) (*server.MapNamespace, string) {
	t.Helper()
	port := freePort(t)
	srv := server.New(
		server.EndPoint("127.0.0.1", port),
		server.EnableSecurity("None", ua.MessageSecurityModeNone),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
	)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	// A namespace first, so that the one of the nodes is not at the index a test would guess.
	server.NewMapNamespace(srv, "urn:test:other")
	ns := server.NewMapNamespace(srv, fluidURI+":inner")
	srv.AddNamespace(fluidSpace{ns})
	root, err := srv.Namespace(0)
	if err != nil {
		t.Fatal(err)
	}
	root.Objects().AddRef(ns.Objects(), id.HasComponent, true)
	ns.SetValue("Task 2.B1.state", int32(0))
	ns.SetValue("Task 2.B1.state_man", false)
	ns.SetValue("Task 2.MB3.state", int32(0))
	ns.SetValue("Task 2.Text", "a text")
	return ns, "opc.tcp://127.0.0.1:" + strconv.Itoa(port)
}

func TestAnyServerByNamespaceURI(t *testing.T) {
	ns, endpoint := fluidServer(t)
	conn, err := link.Parse([]byte(`{
  "id": "fluid", "driver": "opcua",
  "vars": { "properties": { "endpoint": {}, "project": { "default": "Task 2" } } },
  "settings": { "endpoint": "{endpoint}" },
  "namespaces": { "fs": "` + fluidURI + `" },
  "bindings": [
    { "signal": "B1", "item": { "nodeId": "nsu={fs};s={project}.B1.state_man" } },
    { "signal": "B2", "item": { "nodeId": "nsu={fs};s={project}.B1.state" } },
    { "signal": "B3", "item": { "nodeId": "nsu={fs};s={project}.B9.state" } },
    { "signal": "B4", "item": { "nodeId": "nsu={fs};s={project}.Text" } },
    { "signal": "B5", "item": { "nodeId": "nsu=urn:nope;s=x" } },
    { "signal": "MB3", "item": { "nodeId": "nsu={fs};s={project}.MB3.state" } }
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	signals := []link.Signal{
		{Address: plcio.BitAddress(true, plcio.Addr{Bit: 0}), Symbol: "B1"},
		{Address: plcio.BitAddress(true, plcio.Addr{Bit: 1}), Symbol: "B2"},
		{Address: plcio.BitAddress(true, plcio.Addr{Bit: 2}), Symbol: "B3"},
		{Address: plcio.BitAddress(true, plcio.Addr{Bit: 3}), Symbol: "B4"},
		{Address: plcio.BitAddress(true, plcio.Addr{Bit: 4}), Symbol: "B5"},
		{Address: plcio.BitAddress(false, plcio.Addr{Bit: 2}), Symbol: "MB3"},
	}
	l := start(t, conn, signals, map[string]any{"endpoint": endpoint})
	until(t, "the connection", func() bool { return l.Link().State == plcio.LinkConnected })
	info := l.Link()
	problems := strings.Join(info.Problems, "\n")
	for _, want := range []string{"s=Task 2.B1.state cannot be written", "s=Task 2.B9.state: no such node", "s=Task 2.Text holds i=12, which cannot carry a signal", "B5 (%IX0.4): the server has no namespace urn:nope"} {
		if !strings.Contains(problems, want) {
			t.Errorf("problems lack %q:\n%s", want, problems)
		}
	}
	if info.Variables != 2 {
		t.Errorf("B1 and MB3 are exchanged: %+v", info)
	}

	// A sensor of the station goes to FluidSIM as a Boolean, a valve comes back as an Int32.
	var in plcio.Image
	in.Set(plcio.Addr{Bit: 0}, true)
	until(t, "B1 on the server", func() bool {
		v, _ := ns.GetValue("Task 2.B1.state_man").(bool)
		return l.Scan(in) != (plcio.Image{1}) && v
	})
	ns.SetValue("Task 2.MB3.state", int32(1))
	until(t, "MB3 in the station", func() bool { return l.Scan(in).Get(plcio.Addr{Bit: 2}) })
	ns.SetValue("Task 2.MB3.state", int32(0))
	until(t, "MB3 off", func() bool { return !l.Scan(in).Get(plcio.Addr{Bit: 2}) })
}

// A browse path is browse names with the index of their namespace, or of the item's
// namespace without one. (The library's test server cannot follow one, so this only reads
// them.)
func TestBrowsePathsAreRead(t *testing.T) {
	names, err := parsePath("/2:Projects/Task 2/0:Objects/B1:Start", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2:Projects", "3:Task 2", "0:Objects", "3:B1:Start"}
	for i, n := range names {
		if got := fmt.Sprintf("%d:%s", n.NamespaceIndex, n.Name); i >= len(want) || got != want[i] {
			t.Errorf("step %d is %s", i, got)
		}
	}
	if _, err := parsePath("/2:a//2:b", 0); err == nil {
		t.Error("an empty step")
	}
}

func TestValuesTakeTheTypeOfTheNode(t *testing.T) {
	for _, c := range []struct {
		kind ua.TypeID
		v    float64
		want any
	}{
		{ua.TypeIDBoolean, 1, true},
		{ua.TypeIDFloat, 1.5, float32(1.5)},
		{ua.TypeIDDouble, 1.5, 1.5},
		{ua.TypeIDInt16, 1500.4, int16(1500)},
		{ua.TypeIDUint16, -3, uint16(0)},
		{ua.TypeIDInt32, 1500, int32(1500)},
		{ua.TypeIDByte, 300, uint8(255)},
	} {
		v, err := variant(c.v, c.kind)
		if err != nil {
			t.Fatal(err)
		}
		if v.Value() != c.want {
			t.Errorf("%v of %v: %v", c.kind, c.v, v.Value())
		}
	}
	if _, err := variant(1, ua.TypeIDString); err == nil {
		t.Error("a string carries no signal")
	}
}

func TestSoftPLCsShareAMachine(t *testing.T) {
	a, err := opcsim.StartOnFreePort(context.Background(), echo{}, opcsim.Space{PLC: "a"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	b, err := opcsim.StartOnFreePort(context.Background(), echo{}, opcsim.Space{PLC: "b"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	if a.Endpoint() == b.Endpoint() {
		t.Errorf("both on %s", a.Endpoint())
	}
	l := start(t, SoftPLC(b.Endpoint(), "b"), link.Image(), nil)
	until(t, "the connection", func() bool { return l.Link().State == plcio.LinkConnected && len(l.Link().Problems) == 0 })
}

// A NodeId written with its namespace's URI goes to the server as a plain NodeId with the
// namespace's index: a server may decode a request strictly and refuse one that says it
// carries a URI.
func TestANamespaceURIBecomesItsIndex(t *testing.T) {
	namespaces := []string{"http://opcfoundation.org/UA/", "urn:a", "urn:b"}
	id, err := parseNodeID("nsu=urn:b;s=Aufgabe 2 Vorlage.B1.state_man", namespaces)
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "ns=2;s=Aufgabe 2 Vorlage.B1.state_man" {
		t.Errorf("NodeId %s", id)
	}
	data, err := id.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if data[0]&0xc0 != 0 {
		t.Errorf("the encoding says it carries a URI or a server index: %08b", data[0])
	}
	for _, bad := range []string{"nsu=urn:c;s=x", "nsu=urn:b", "ns=x;s=y"} {
		if _, err := parseNodeID(bad, namespaces); err == nil {
			t.Errorf("%s", bad)
		}
	}
}
