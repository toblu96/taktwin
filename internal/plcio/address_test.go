package plcio

import "testing"

func TestAddress(t *testing.T) {
	for s, want := range map[string]Address{
		"%IX0.3":  {Input: true, Byte: 0, Bit: 3},
		"%qx7.7":  {Byte: 7, Bit: 7},
		" %IW8 ":  {Input: true, Word: true, Byte: 8},
		"%QW100":  {Word: true, Byte: 100},
		"%IX12.0": {Input: true, Byte: 12},
	} {
		got, err := ParseAddress(s)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", s, got, err)
		}
	}
	for _, bad := range []string{"IX0.3", "%IX0", "%IX0.8", "%IW8.1", "%MX0.0", "%IB0", "%IX.3", "%IX0.3x"} {
		if _, err := ParseAddress(bad); err == nil {
			t.Errorf("%q is no address", bad)
		}
	}
	if s := BitAddress(false, Addr{Byte: 2, Bit: 5}).String(); s != "%QX2.5" {
		t.Errorf("an output bit is written %s", s)
	}
	if s := AnalogInputAddress(1).String(); s != "%IW10" {
		t.Errorf("the second analogue input is %s", s)
	}
	if ch, ok := AnalogInputAddress(3).AnalogInput(); !ok || ch != 3 {
		t.Error("an analogue input address knows its channel")
	}
	for _, not := range []string{"%IW9", "%IW16", "%QW8", "%IX8.0"} {
		a, _ := ParseAddress(not)
		if _, ok := a.AnalogInput(); ok {
			t.Errorf("%s is no analogue input", not)
		}
	}
	if a, ok := (Address{Input: true, Byte: 2, Bit: 1}).InImage(); !ok || a != (Addr{2, 1}) {
		t.Error("a bit in the image")
	}
	if _, ok := (Address{Input: true, Byte: ImageBytes}).InImage(); ok {
		t.Error("a bit past the image")
	}
}

func TestFormat(t *testing.T) {
	cabinet := Naming{Inputs: "IB{byte}Bit{bit:2}", Outputs: "QB{byte}Bit{bit:2}", Analog: "AI{channel}", List: "Bits"}.Format()
	german := Format{Input: "E{byte}.{bit}", Output: "A{byte}.{bit}", InputWord: "EW{byte}"}
	for _, c := range []struct {
		f    Format
		in   string
		want string
	}{
		{cabinet, "%IX0.3", "IB0Bit03"},
		{cabinet, "%QX1.0", "QB1Bit00"},
		{cabinet, "%IW10", "AI1"},
		{cabinet, "%IW9", "%IW9"},
		{cabinet, "%QW8", "%QW8"},
		{german, "%IX0.3", "E0.3"},
		{german, "%QX2.7", "A2.7"},
		{german, "%IW8", "EW8"},
		{Format{}, "%IX0.3", "%IX0.3"},
		{german, "B1", "B1"},
	} {
		if got := c.f.ShowString(c.in); got != c.want {
			t.Errorf("%+v shows %s as %s, not %s", c.f, c.in, got, c.want)
		}
	}
}

func TestNamingAddress(t *testing.T) {
	n := Naming{Inputs: "IB{byte}Bit{bit:2}", Outputs: "QB{byte}Bit{bit:2}", Analog: "AI{channel}", List: "Bits"}
	for s, want := range map[string]string{"IB0Bit03": "%IX0.3", "QB2Bit07": "%QX2.7", "AI2": "%IW12", "%ix1.1": "%IX1.1"} {
		got, err := n.Address(s)
		if err != nil || got.String() != want {
			t.Errorf("%s: %s %v", s, got, err)
		}
	}
	for _, bad := range []string{"B1", "IB9Bit00", "%IX0"} {
		if _, err := n.Address(bad); err == nil {
			t.Errorf("%s is no address", bad)
		}
	}
}
