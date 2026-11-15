package link

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/toblu96/taktwin/internal/plcio"
)

// signal makes a signal of an address, with a symbol.
func signal(address, symbol string) Signal {
	a, err := plcio.ParseAddress(address)
	if err != nil {
		panic(err)
	}
	s := Signal{Address: a, Symbol: symbol, Shown: "S" + a.String()}
	if !a.IsBit() {
		s.Scaling = &plcio.Scaling{Range: [2]float64{0, 10}, Raw: [2]float64{0, 27648}}
		s.Channel = (a.Byte - 8) / 2
	}
	return s
}

var station = []Signal{
	signal("%IX0.0", "B1"),
	signal("%IX0.1", "B2"),
	signal("%IW8", "H1"),
	signal("%QX0.0", "M1"),
	signal("%QX0.1", ""),
	signal("%QW8", "Y1"),
}

func parse(t *testing.T, data string) Connection {
	t.Helper()
	c, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func items(r Resolved) map[string]string {
	out := map[string]string{}
	for _, b := range r.Bindings {
		var item struct {
			NodeID string `json:"nodeId"`
		}
		_ = json.Unmarshal(b.Item, &item)
		key := b.Signal.Address.String()
		if b.Invert {
			key = "!" + key
		}
		if out[key] != "" {
			out[key] += " + " + item.NodeID
		} else {
			out[key] = item.NodeID
		}
	}
	return out
}

const fluid = `{
  "id": "fluid", "name": "Fluid", "driver": "opcua",
  "vars": { "type": "object", "required": ["project"], "properties": {
    "host": { "type": "string", "default": "127.0.0.1" },
    "port": { "type": "integer", "minimum": 1, "maximum": 65535, "default": 62555 },
    "project": { "type": "string", "minLength": 1, "default": "Task 2" }
  }},
  "settings": { "endpoint": "opc.tcp://{host}:{port}" },
  "namespaces": { "fs": "urn:fluid" },
  "bindings": [
    { "signal": "B1", "item": { "nodeId": "nsu={fs};s={project}.B1.state_man" } },
    { "address": "%QX0.0", "item": { "nodeId": "nsu={fs};s={project}.MB3.state" }, "invert": true }
  ]
}`

func TestAConnectionFillsInItsVars(t *testing.T) {
	c := parse(t, fluid)
	r, err := c.Resolve(station, map[string]any{"project": "Sorting", "port": float64(4840), "other": "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(r.Settings); got != `{"endpoint":"opc.tcp://127.0.0.1:4840"}` {
		t.Errorf("settings: %s", got)
	}
	got := items(r)
	if got["%IX0.0"] != "nsu=urn:fluid;s=Sorting.B1.state_man" || got["!%QX0.0"] != "nsu=urn:fluid;s=Sorting.MB3.state" || len(got) != 2 {
		t.Errorf("bindings: %v", got)
	}
	if len(r.Unbound) != 4 {
		t.Errorf("four signals are not exchanged: %+v", r.Unbound)
	}
	// Without values, the defaults.
	if r, err = c.Resolve(station, nil); err != nil || !strings.Contains(string(r.Settings), ":62555") {
		t.Errorf("defaults: %s %v", r.Settings, err)
	}
}

func TestValuesMustFitTheVars(t *testing.T) {
	c := parse(t, fluid)
	for _, bad := range []map[string]any{
		{"port": float64(70000)},
		{"port": "4840"},
		{"project": ""},
	} {
		if _, err := c.Resolve(station, bad); err == nil || !strings.Contains(err.Error(), "do not fit") {
			t.Errorf("%v: %v", bad, err)
		}
	}
	_, err := c.Resolve(station, map[string]any{"port": float64(0)})
	if err == nil || !strings.Contains(err.Error(), "/port") {
		t.Errorf("the message names the var: %v", err)
	}
}

func TestBrokenConnectionsAreRefused(t *testing.T) {
	for name, c := range map[string]string{
		"no id":         `{"driver": "opcua"}`,
		"no driver":     `{"id": "x"}`,
		"unknown field": `{"id": "x", "driver": "opcua", "endpoint": "opc.tcp://x"}`,
		"remote schema": `{"id": "x", "driver": "opcua", "vars": {"$ref": "https://example.com/schema.json"}}`,
		"bad schema":    `{"id": "x", "driver": "opcua", "vars": {"type": 7}}`,
	} {
		if _, err := Parse([]byte(c)); err == nil {
			t.Errorf("%s is accepted", name)
		}
	}
	for name, c := range map[string]string{
		"unknown var":      `{"id": "x", "driver": "d", "settings": {"endpoint": "{nope}"}}`,
		"unknown symbol":   `{"id": "x", "driver": "d", "bindings": [{"signal": "B9", "item": {}}]}`,
		"unknown address":  `{"id": "x", "driver": "d", "bindings": [{"address": "%IX5.0", "item": {}}]}`,
		"both":             `{"id": "x", "driver": "d", "bindings": [{"signal": "B1", "address": "%IX0.0", "item": {}}]}`,
		"no item":          `{"id": "x", "driver": "d", "bindings": [{"signal": "B1"}]}`,
		"output twice":     `{"id": "x", "driver": "d", "bindings": [{"signal": "M1", "item": {"a": 1}}, {"address": "%QX0.0", "item": {"a": 2}}]}`,
		"var as namespace": `{"id": "x", "driver": "d", "vars": {"properties": {"a": {"default": "1"}}}, "namespaces": {"a": "urn:a"}}`,
	} {
		conn, err := Parse([]byte(c))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := conn.Resolve(station, nil); err == nil {
			t.Errorf("%s resolves", name)
		}
	}
	twice := append(station, signal("%IX1.0", "B1"))
	if _, err := parse(t, `{"id": "x", "driver": "d", "bindings": [{"signal": "B1", "item": {}}]}`).Resolve(twice, nil); err == nil {
		t.Error("a symbol two signals have is no way to name one")
	}
}

// Generated bindings cover a kind of signal; explicit ones take the place of those that
// write the same item, for inputs, or the same signal, for outputs. An input may go to
// more than one item.
func TestExplicitBindingsWinOverGeneratedOnes(t *testing.T) {
	c := parse(t, `{
  "id": "list", "driver": "opcua",
  "generate": [
    { "signals": "inputs", "item": { "nodeId": "ns=4;s=IB{byte}Bit{bit:2}" } },
    { "signals": "analogInputs", "item": { "nodeId": "ns=4;s=AI{channel}" } },
    { "signals": "analogOutputs", "item": { "nodeId": "ns=4;s=AQ{channel}" } },
    { "signals": "outputs", "item": { "nodeId": "ns=4;s=QB{byte}Bit{bit:2}" } }
  ],
  "bindings": [
    { "signal": "B2", "item": { "nodeId": "ns=4;s=IB0Bit00" } },
    { "signal": "B2", "item": { "nodeId": "ns=4;s=IB0Bit05" }, "invert": true },
    { "signal": "M1", "item": { "nodeId": "ns=4;s=QB0Bit07" } }
  ]
}`)
	r, err := c.Resolve(station, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		// B1's own item is B2's now.
		"%IX0.1":  "ns=4;s=IB0Bit01 + ns=4;s=IB0Bit00",
		"!%IX0.1": "ns=4;s=IB0Bit05",
		"%IW8":    "ns=4;s=AI0",
		"%QW8":    "ns=4;s=AQ0",
		"%QX0.0":  "ns=4;s=QB0Bit07",
		"%QX0.1":  "ns=4;s=QB0Bit01",
	}
	got := items(r)
	if len(got) != len(want) {
		t.Errorf("bindings: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
	if len(r.Unbound) != 1 || r.Unbound[0].Symbol != "B1" {
		t.Errorf("B1 lost its item to B2: %+v", r.Unbound)
	}
}

func TestBindingsConvertValues(t *testing.T) {
	bit := Binding{Signal: signal("%IX0.0", "")}
	inverted := Binding{Signal: signal("%IX0.0", ""), Invert: true}
	analog := Binding{Signal: signal("%IW8", "")}
	scaled := Binding{Signal: signal("%IW10", ""), Scale: &Scale{From: [2]float64{0, 10}, To: [2]float64{0, 100}}}
	for _, c := range []struct {
		b    Binding
		v    float64
		k    Kind
		want float64
	}{
		{bit, 1, KindBool, 1}, {bit, 0, KindInt, 0}, {inverted, 1, KindBool, 0}, {inverted, 0, KindInt, 1},
		// An integer gets the PLC's raw value, a number the value on the wire, unless scaled.
		{analog, 1.5, KindFloat, 1.5}, {analog, 5, KindInt, 13824},
		{scaled, 5, KindInt, 50}, {scaled, 5, KindFloat, 50},
	} {
		if got := c.b.Outward(c.v, c.k); got != c.want {
			t.Errorf("%+v out %v as %v: %v, want %v", c.b.Signal.Address, c.v, c.k, got, c.want)
		}
	}
	if bit.Inward(7, KindInt) != 1 || bit.Inward(0, KindBool) != 0 || inverted.Inward(7, KindInt) != 0 {
		t.Error("anything but 0 is on")
	}
	out := Binding{Signal: signal("%QW8", "")}
	if out.Inward(13824, KindInt) != 5 || out.Inward(2.5, KindFloat) != 2.5 || scaled.Inward(50, KindFloat) != 5 {
		t.Error("an analogue output comes back the other way round")
	}
}
