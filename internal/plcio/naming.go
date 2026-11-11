package plcio

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Naming is how a PLC names the variables of its image in its program, which a PLC module
// declares: templates for its inputs, outputs and analogue inputs, with {byte}, {bit} (or
// {bit:2} for two digits) and {channel}, and the variable list they are in, which a PLC
// offers over OPC UA, such as "IB{byte}Bit{bit:2}", "QB{byte}Bit{bit:2}", "AI{channel}" and
// "Bits".
type Naming struct {
	// Inputs names an input bit, with {byte} and {bit} or {bit:2}, such as "IB{byte}Bit{bit:2}".
	Inputs string `json:"inputs"`
	// Outputs names an output bit, with {byte} and {bit} or {bit:2}, such as "QB{byte}Bit{bit:2}".
	Outputs string `json:"outputs"`
	// Analog names an analogue input, with {channel}, such as "AI{channel}"; without it the
	// PLC has none.
	Analog string `json:"analog,omitempty"`
	// List is the variable list the bits are in, which the PLC offers over OPC UA, such as "Bits".
	List string `json:"list"`
}

// Check reports a naming that cannot tell its variables apart.
func (n Naming) Check() error {
	switch {
	case n.List == "" || strings.ContainsAny(n.List, ". {}"):
		return fmt.Errorf("the variable list needs a name, not %q", n.List)
	case !strings.Contains(n.Inputs, "{byte}") || !strings.Contains(n.Inputs, "{bit") || !strings.Contains(n.Outputs, "{byte}") || !strings.Contains(n.Outputs, "{bit"):
		return errors.New("the names of inputs and outputs need {byte} and {bit}")
	case n.Analog != "" && !strings.Contains(n.Analog, "{channel}"):
		return errors.New("the names of analogue inputs need {channel}")
	}
	in, out := n.Input(Addr{}), n.Output(Addr{})
	if in == out {
		return fmt.Errorf("inputs and outputs are both named %s", in)
	}
	if _, isIn, err := n.Parse(in); err != nil || !isIn {
		return fmt.Errorf("%s does not read back as an input", in)
	}
	if _, isIn, err := n.Parse(out); err != nil || isIn {
		return fmt.Errorf("%s does not read back as an output", out)
	}
	return nil
}

func (n Naming) fill(template string, a Addr) string {
	s := strings.ReplaceAll(template, "{byte}", strconv.Itoa(int(a.Byte)))
	s = strings.ReplaceAll(s, "{bit:2}", fmt.Sprintf("%02d", a.Bit))
	return strings.ReplaceAll(s, "{bit}", strconv.Itoa(int(a.Bit)))
}

// Input names the input at a.
func (n Naming) Input(a Addr) string { return n.fill(n.Inputs, a) }

// Output names the output at a.
func (n Naming) Output(a Addr) string { return n.fill(n.Outputs, a) }

// AnalogInput names an analogue input, or gives "" for a PLC without them.
func (n Naming) AnalogInput(channel int) string {
	if n.Analog == "" {
		return ""
	}
	return strings.ReplaceAll(n.Analog, "{channel}", strconv.Itoa(channel))
}

// ParseAnalog reads the name of an analogue input.
func (n Naming) ParseAnalog(name string) (int, bool) {
	for channel := range AnalogInputs {
		if name != "" && name == n.AnalogInput(channel) {
			return channel, true
		}
	}
	return 0, false
}

// pattern turns a template into a pattern that matches its names, with the byte and bit.
func pattern(template string) *regexp.Regexp {
	p := regexp.QuoteMeta(template)
	p = strings.ReplaceAll(p, regexp.QuoteMeta("{byte}"), `(?P<byte>\d+)`)
	p = strings.ReplaceAll(p, regexp.QuoteMeta("{bit:2}"), `(?P<bit>\d\d)`)
	p = strings.ReplaceAll(p, regexp.QuoteMeta("{bit}"), `(?P<bit>\d+)`)
	return regexp.MustCompile("^" + p + "$")
}

// Parse reads a variable's name: where it is in the image, and whether it is an input.
func (n Naming) Parse(name string) (Addr, bool, error) {
	for _, side := range []struct {
		template string
		input    bool
	}{{n.Inputs, true}, {n.Outputs, false}} {
		if side.template == "" {
			continue
		}
		re := pattern(side.template)
		m := re.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		b, _ := strconv.Atoi(m[re.SubexpIndex("byte")])
		bit, _ := strconv.Atoi(m[re.SubexpIndex("bit")])
		a := Addr{Byte: uint8(b), Bit: uint8(bit)}
		if b >= ImageBytes || bit > 7 || n.fill(side.template, a) != name {
			return Addr{}, false, fmt.Errorf("%q is outside the image", name)
		}
		return a, side.input, nil
	}
	return Addr{}, false, fmt.Errorf("%q is no variable of the image, named as %s or %s", name, n.Input(Addr{}), n.Output(Addr{}))
}
