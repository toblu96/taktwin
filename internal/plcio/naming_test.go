package plcio

import "testing"

func TestNaming(t *testing.T) {
	english := Naming{Inputs: "IB{byte}Bit{bit:2}", Outputs: "QB{byte}Bit{bit:2}", Analog: "AI{channel}", List: "Bits"}
	if err := english.Check(); err != nil {
		t.Fatal(err)
	}
	a := Addr{Byte: 2, Bit: 5}
	if english.Input(a) != "IB2Bit05" || english.Output(a) != "QB2Bit05" || english.AnalogInput(3) != "AI3" {
		t.Errorf("names: %s %s %s", english.Input(a), english.Output(a), english.AnalogInput(3))
	}
	if got, in, err := english.Parse("QB7Bit07"); err != nil || in || got != (Addr{7, 7}) {
		t.Errorf("parse an output: %v %v %v", got, in, err)
	}
	for _, bad := range []string{"QB8Bit00", "IB0Bit8", "IB0Bit5", "EB0Bit00", "IB0Bit00x"} {
		if _, _, err := english.Parse(bad); err == nil {
			t.Errorf("%s is no variable", bad)
		}
	}
	if ch, ok := english.ParseAnalog("AI2"); !ok || ch != 2 {
		t.Error("analogue inputs by name")
	}
	iec := Naming{Inputs: "I{byte}_{bit}", Outputs: "Q{byte}_{bit}", List: "IO"}
	if err := iec.Check(); err != nil || iec.Input(a) != "I2_5" {
		t.Errorf("another naming: %v %s", err, iec.Input(a))
	}
	if _, ok := iec.ParseAnalog(""); ok {
		t.Error("without analogue inputs, no name is one")
	}
	for _, bad := range []Naming{
		{Inputs: "X{byte}", Outputs: "Y{byte}{bit}", List: "B"},
		{Inputs: "{byte}.{bit}", Outputs: "{byte}.{bit}", List: "B"},
		{Inputs: "I{byte}{bit}", Outputs: "Q{byte}{bit}", List: ""},
	} {
		if bad.Check() == nil {
			t.Errorf("%+v cannot tell its variables apart", bad)
		}
	}
}
