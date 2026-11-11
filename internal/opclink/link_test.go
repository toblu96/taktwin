package opclink

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"

	"github.com/toblu96/taktwin/internal/opcsim"
	"github.com/toblu96/taktwin/internal/plcio"
)

// echo drives each output from the input with the same address.
type echo struct{ resets int }

// naming is how the server of these tests names its variables.
var naming = plcio.Naming{Inputs: "IB{byte}Bit{bit:2}", Outputs: "QB{byte}Bit{bit:2}", Analog: "AI{channel}", List: "Bits"}

func (*echo) Name() string                    { return "echo" }
func (*echo) Cycle() time.Duration            { return 5 * time.Millisecond }
func (*echo) Scan(in plcio.Image) plcio.Image { return in }
func (e *echo) Reset()                        { e.resets++ }

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func server(t *testing.T) *opcsim.Server {
	t.Helper()
	s, err := opcsim.Start(context.Background(), freePort(t), &echo{}, naming)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
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

func TestLinkExchangesTheImageWithAServer(t *testing.T) {
	s := server(t)
	l := Start(context.Background(), Options{Endpoint: s.Endpoint(), Naming: naming, Name: "test"})
	defer l.Close()
	until(t, "the connection", func() bool { return l.Link().State == plcio.LinkConnected })
	if got := l.Link().Variables; got != plcio.ImageBytes*8*2 {
		t.Errorf("found %d variables", got)
	}
	var in plcio.Image
	in.Set(plcio.Addr{Byte: 0, Bit: 6}, true)
	in.Set(plcio.Addr{Byte: 1, Bit: 2}, true)
	until(t, "the echo", func() bool { return l.Scan(in) == in })

	// It also clears again, so only writing changes works in both directions.
	in = plcio.Image{}
	until(t, "the echo to clear", func() bool { return l.Scan(in) == in })
}

func TestAddressMapLetsAProgramUseOtherBits(t *testing.T) {
	s := server(t)
	local := plcio.Addr{Byte: 0, Bit: 6}
	back := plcio.Addr{Byte: 0, Bit: 1}
	l := Start(context.Background(), Options{Endpoint: s.Endpoint(), Naming: naming, Map: plcio.AddressMap{
		Inputs: []plcio.Binding{{Local: local, Variable: "IB0Bit01"}},
	}})
	defer l.Close()
	var in plcio.Image
	in.Set(local, true)
	// The echo returns IB0Bit01 on QB0Bit01.
	until(t, "the mapped bit", func() bool { return l.Scan(in).Get(back) })
}

func TestLinkSaysWhyItCannotConnect(t *testing.T) {
	l := Start(context.Background(), Options{Endpoint: "opc.tcp://127.0.0.1:" + itoa(freePort(t)), Naming: naming})
	defer l.Close()
	until(t, "the failure", func() bool { return l.Link().State == plcio.LinkFailed })
	if !strings.Contains(l.Link().Detail, "no OPC UA server") {
		t.Errorf("detail: %s", l.Link().Detail)
	}
}

func TestLinkSaysWhenTheVariableListIsMissing(t *testing.T) {
	s := server(t)
	l := Start(context.Background(), Options{Endpoint: s.Endpoint(), Naming: plcio.Naming{Inputs: naming.Inputs, Outputs: naming.Outputs, List: "Nope"}})
	defer l.Close()
	until(t, "the failure", func() bool { return l.Link().State == plcio.LinkFailed })
	if !strings.Contains(l.Link().Detail, "symbol configuration") {
		t.Errorf("detail: %s", l.Link().Detail)
	}
}

// A lost PLC must not leave the station's outputs on, and the link must come back.
func TestLinkDropsOutputsAndReconnects(t *testing.T) {
	port := freePort(t)
	s, err := opcsim.Start(context.Background(), port, &echo{}, naming)
	if err != nil {
		t.Fatal(err)
	}
	l := Start(context.Background(), Options{Endpoint: s.Endpoint(), Naming: naming})
	defer l.Close()
	var in plcio.Image
	in.Set(plcio.Addr{Byte: 0, Bit: 0}, true)
	until(t, "the echo", func() bool { return l.Scan(in) == in })

	s.Close()
	until(t, "the outputs to drop", func() bool { return l.Scan(in) == plcio.Image{} })
	if l.Link().State != plcio.LinkFailed {
		t.Errorf("state: %s", l.Link().State)
	}

	s, err = opcsim.Start(context.Background(), port, &echo{}, naming)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	until(t, "the echo after reconnecting", func() bool { return l.Scan(in) == in })
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestSimulatedServersShareAMachine(t *testing.T) {
	a, err := opcsim.StartOnFreePort(context.Background(), &echo{}, naming)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := opcsim.StartOnFreePort(context.Background(), &echo{}, naming)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if a.Endpoint() == b.Endpoint() {
		t.Errorf("both on %s", a.Endpoint())
	}
	l := Start(context.Background(), Options{Endpoint: b.Endpoint(), Naming: naming})
	defer l.Close()
	until(t, "the connection", func() bool { return l.Link().State == plcio.LinkConnected })
}

func TestAnalogValuesTakeTheTypeOfTheVariable(t *testing.T) {
	for _, c := range []struct {
		kind ua.TypeID
		want any
	}{
		{ua.TypeIDFloat, float32(1.5)},
		{ua.TypeIDDouble, 1.5},
		{ua.TypeIDInt16, int16(1500)},
		{ua.TypeIDUint16, uint16(1500)},
		{ua.TypeIDInt32, int32(1500)},
		{ua.TypeIDUint32, uint32(1500)},
	} {
		v, err := analogVariant(1.5, c.kind)
		if err != nil {
			t.Fatal(err)
		}
		if v.Value() != c.want {
			t.Errorf("%v: %v", c.kind, v.Value())
		}
	}
	if _, err := analogVariant(1.5, ua.TypeIDBoolean); err == nil {
		t.Error("a BOOL cannot hold an analogue value")
	}
}

func TestLinkSaysWhenAnAnalogInputIsMissing(t *testing.T) {
	s := server(t)
	l := Start(context.Background(), Options{Endpoint: s.Endpoint(), Naming: naming, Name: "test", Analog: []string{"AI9"}})
	defer l.Close()
	until(t, "the connection", func() bool { return l.Link().State == plcio.LinkConnected })
	if got := l.Link().Detail; !strings.Contains(got, "AI9 is not on the server") {
		t.Errorf("detail: %q", got)
	}
}
