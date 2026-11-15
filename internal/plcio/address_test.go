package plcio

import (
	"math"
	"testing"
)

func TestAddress(t *testing.T) {
	for s, want := range map[string]Address{
		"%IX0.3":  {Input: true, Size: SizeBit, Byte: 0, Bit: 3},
		"%qx7.7":  {Size: SizeBit, Byte: 7, Bit: 7},
		" %IW8 ":  {Input: true, Size: SizeWord, Byte: 8},
		"%QW100":  {Size: SizeWord, Byte: 100},
		"%IX12.0": {Input: true, Size: SizeBit, Byte: 12},
		"%IB2":    {Input: true, Size: SizeByte, Byte: 2},
		"%QD12":   {Size: SizeDouble, Byte: 12},
	} {
		got, err := ParseAddress(s)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", s, got, err)
		}
		if again, _ := ParseAddress(got.String()); again != got {
			t.Errorf("%q is written %s", s, got)
		}
	}
	for _, bad := range []string{"IX0.3", "%IX0", "%IX0.8", "%IW8.1", "%MX0.0", "%IX.3", "%IX0.3x"} {
		if _, err := ParseAddress(bad); err == nil {
			t.Errorf("%q is no address", bad)
		}
	}
	if s := BitAddress(false, Addr{Byte: 2, Bit: 5}).String(); s != "%QX2.5" {
		t.Errorf("an output bit is written %s", s)
	}
	if a, ok := BitAddress(true, Addr{Byte: 2, Bit: 1}).InImage(); !ok || a != (Addr{2, 1}) {
		t.Error("a bit in the image")
	}
	if _, ok := WordAddress(true, 8).InImage(); ok {
		t.Error("a word is no bit")
	}
	w, d := WordAddress(true, 8), Address{Input: true, Size: SizeDouble, Byte: 9}
	if !w.Overlaps(d) || w.Overlaps(WordAddress(true, 10)) || w.Overlaps(WordAddress(false, 8)) {
		t.Error("addresses overlap where they share a byte of the same direction")
	}
}

func TestTheImageHoldsWordsLowByteFirst(t *testing.T) {
	var im Image
	im.SetRaw(WordAddress(true, 8), 0x6c00)
	if im[8] != 0x00 || im[9] != 0x6c || im.Raw(WordAddress(true, 8)) != 0x6c00 {
		t.Errorf("a word: % x", im[8:10])
	}
	d := Address{Size: SizeDouble, Byte: 12}
	im.SetRaw(d, 0x01020304)
	if im[12] != 4 || im[15] != 1 || im.Raw(d) != 0x01020304 {
		t.Errorf("a double word: % x", im[12:16])
	}
	im.SetRaw(BitAddress(true, Addr{Byte: 1, Bit: 3}), 1)
	if im[1] != 8 || im.Raw(BitAddress(true, Addr{Byte: 1, Bit: 3})) != 1 {
		t.Error("a bit")
	}
	im.SetRaw(WordAddress(true, ImageBytes-1), 0xffff)
	if im[ImageBytes-1] != 0 || im.Raw(WordAddress(true, ImageBytes-1)) != 0 {
		t.Error("a word that does not fit is left alone")
	}
}

func TestScaling(t *testing.T) {
	volts := Scaling{Range: [2]float64{0, 10}, Raw: [2]float64{0, 27648}}
	var im Image
	a := WordAddress(true, 8)
	for _, c := range []struct{ v, raw, back float64 }{
		{0, 0, 0}, {10, 27648, 10}, {5, 13824, 5}, {-1, -2765, -2765.0 * 10 / 27648}, {20, math.MaxInt16, math.MaxInt16 * 10.0 / 27648},
	} {
		if got := volts.ToRaw(c.v); got != c.raw {
			t.Errorf("%v V is raw %v, not %v", c.v, got, c.raw)
		}
		volts.Write(&im, a, c.v)
		if got := volts.Read(im, a); math.Abs(got-c.back) > 1e-9 {
			t.Errorf("%v V reads back as %v, not %v", c.v, got, c.back)
		}
	}
	real := Scaling{Real: true}
	d := Address{Input: true, Size: SizeDouble, Byte: 12}
	real.Write(&im, d, 7.25)
	if got := real.Read(im, d); got != 7.25 {
		t.Errorf("a REAL is the value itself: %v", got)
	}
}

func TestFormat(t *testing.T) {
	cabinet := Naming{Inputs: "IB{byte}Bit{bit:2}", Outputs: "QB{byte}Bit{bit:2}", Analog: "AI{channel}", List: "Bits"}.Format()
	german := Format{Input: "E{byte}.{bit}", Output: "A{byte}.{bit}", InputWord: "EW{byte}"}
	for _, c := range []struct {
		f       Format
		in      string
		channel int
		want    string
	}{
		{cabinet, "%IX0.3", -1, "IB0Bit03"},
		{cabinet, "%QX1.0", -1, "QB1Bit00"},
		{cabinet, "%IW10", 1, "AI1"},
		{cabinet, "%IW10", -1, "%IW10"},
		{cabinet, "%QW8", 0, "%QW8"},
		{german, "%IX0.3", -1, "E0.3"},
		{german, "%QX2.7", -1, "A2.7"},
		{german, "%IW8", -1, "EW8"},
		{german, "%ID8", -1, "%ID8"},
		{Format{}, "%IX0.3", -1, "%IX0.3"},
	} {
		a, err := ParseAddress(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.f.ShowChannel(a, c.channel); got != c.want {
			t.Errorf("%+v shows %s as %s, not %s", c.f, c.in, got, c.want)
		}
	}
}

func TestNamingAddress(t *testing.T) {
	n := Naming{Inputs: "IB{byte}Bit{bit:2}", Outputs: "QB{byte}Bit{bit:2}", Analog: "AI{channel}", List: "Bits"}
	for s, want := range map[string]string{"IB0Bit03": "%IX0.3", "QB2Bit07": "%QX2.7", "%ix1.1": "%IX1.1"} {
		got, err := n.Address(s)
		if err != nil || got.String() != want {
			t.Errorf("%s: %s %v", s, got, err)
		}
	}
	for _, bad := range []string{"B1", "IB99Bit00", "%IX0", "AI2"} {
		if _, err := n.Address(bad); err == nil {
			t.Errorf("%s is no address of a bit", bad)
		}
	}
	if ch, ok := n.ParseAnalog("AI12"); !ok || ch != 12 {
		t.Error("an analogue input of any channel")
	}
	if _, ok := n.ParseAnalog("AI012"); ok {
		t.Error("written as the naming writes it")
	}
}
