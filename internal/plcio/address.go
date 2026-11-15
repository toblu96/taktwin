package plcio

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Address is where a signal is in a PLC's image, as an IEC 61131-3 direct address: %IX0.3
// and %QX0.3 for bits, %IW8 and %QW8 for 16-bit words. It is how stations store and the
// simulation knows a signal, whatever the PLC module calls it on screen or a protocol calls
// it outside (see Format).
type Address struct {
	Input bool
	// Word is a 16-bit word at Byte; otherwise the address is the bit Bit of Byte.
	Word bool
	Byte int
	Bit  uint8
}

// BitAddress is the address of a bit of the inputs or outputs.
func BitAddress(input bool, a Addr) Address {
	return Address{Input: input, Byte: int(a.Byte), Bit: a.Bit}
}

// AnalogInputAddress is the address of an analogue input channel: a word each, after the
// bytes of bits, as %IW8 for the first.
func AnalogInputAddress(channel int) Address {
	return Address{Input: true, Word: true, Byte: ImageBytes + 2*channel}
}

var directAddress = regexp.MustCompile(`^%([IQ])([XW])(\d+)(?:\.(\d+))?$`)

// ParseAddress reads an address such as %IX0.3 or %IW8, in capitals or not.
func ParseAddress(s string) (Address, error) {
	m := directAddress.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil {
		return Address{}, fmt.Errorf("%q is not an address such as %%IX0.3, %%QX0.3 or %%IW8", s)
	}
	b, err := strconv.Atoi(m[3])
	if err != nil || b > 0xffff {
		return Address{}, fmt.Errorf("%q has no byte number", s)
	}
	a := Address{Input: m[1] == "I", Word: m[2] == "W", Byte: b}
	switch {
	case a.Word && m[4] != "":
		return Address{}, fmt.Errorf("%q is a word and has no bit", s)
	case !a.Word && m[4] == "":
		return Address{}, fmt.Errorf("%q needs a bit, as %%IX0.3", s)
	case !a.Word:
		bit, err := strconv.Atoi(m[4])
		if err != nil || bit > 7 {
			return Address{}, fmt.Errorf("%q has a bit outside 0 to 7", s)
		}
		a.Bit = uint8(bit)
	}
	return a, nil
}

// IsAddress says whether s is written as an address, right or wrong, rather than as a name.
func IsAddress(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "%") }

// String writes the address as %IX0.3 or %IW8.
func (a Address) String() string {
	side := "Q"
	if a.Input {
		side = "I"
	}
	if a.Word {
		return fmt.Sprintf("%%%sW%d", side, a.Byte)
	}
	return fmt.Sprintf("%%%sX%d.%d", side, a.Byte, a.Bit)
}

// InImage is the position of a bit address in an Image, if it is one there.
func (a Address) InImage() (Addr, bool) {
	if a.Word || a.Byte >= ImageBytes {
		return Addr{}, false
	}
	return Addr{Byte: uint8(a.Byte), Bit: a.Bit}, true
}

// AnalogInput is the channel of an analogue input address, if it is one.
func (a Address) AnalogInput() (int, bool) {
	channel := (a.Byte - ImageBytes) / 2
	if !a.Input || !a.Word || a.Byte < ImageBytes || (a.Byte-ImageBytes)%2 != 0 || channel >= AnalogInputs {
		return 0, false
	}
	return channel, true
}

// Format is how a PLC module shows the addresses of its image: a template for bits of the
// inputs and of the outputs, with {byte}, {bit} (or {bit:2} for two digits), and for words,
// with {byte} and, for an analogue input, {channel}. Without a template, an address is shown
// as it is, such as %IX0.3.
type Format struct {
	// Input shows an input bit, such as "I{byte}.{bit}" for I0.3.
	Input string `json:"input,omitempty"`
	// Output shows an output bit, such as "Q{byte}.{bit}" for Q0.3.
	Output string `json:"output,omitempty"`
	// InputWord shows an input word, such as "IW{byte}" or, for analogue inputs, "AI{channel}".
	InputWord string `json:"inputWord,omitempty"`
	// OutputWord shows an output word, such as "QW{byte}".
	OutputWord string `json:"outputWord,omitempty"`
}

// Show writes an address as the format says.
func (f Format) Show(a Address) string {
	template := f.Output
	switch {
	case a.Input && a.Word:
		template = f.InputWord
	case a.Word:
		template = f.OutputWord
	case a.Input:
		template = f.Input
	}
	if template == "" {
		return a.String()
	}
	if strings.Contains(template, "{channel}") {
		channel, ok := a.AnalogInput()
		if !ok {
			return a.String()
		}
		template = strings.ReplaceAll(template, "{channel}", strconv.Itoa(channel))
	}
	s := strings.ReplaceAll(template, "{byte}", strconv.Itoa(a.Byte))
	s = strings.ReplaceAll(s, "{bit:2}", fmt.Sprintf("%02d", a.Bit))
	return strings.ReplaceAll(s, "{bit}", strconv.Itoa(int(a.Bit)))
}

// ShowString writes an address given as text as the format says, or gives it back as it is
// when it is not one.
func (f Format) ShowString(s string) string {
	a, err := ParseAddress(s)
	if err != nil {
		return s
	}
	return f.Show(a)
}

// Format is how the naming's names show the addresses they stand for.
func (n Naming) Format() Format {
	return Format{Input: n.Inputs, Output: n.Outputs, InputWord: n.Analog}
}

// Address reads an address, written as one or, as older station files do, as the name of a
// variable of the naming, such as IB0Bit03 or AI0.
func (n Naming) Address(s string) (Address, error) {
	if IsAddress(s) {
		return ParseAddress(s)
	}
	if channel, ok := n.ParseAnalog(s); ok {
		return AnalogInputAddress(channel), nil
	}
	a, input, err := n.Parse(s)
	if err != nil {
		return Address{}, err
	}
	return BitAddress(input, a), nil
}
