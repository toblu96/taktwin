package plcio

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Address is where a signal is in a PLC's image, as an IEC 61131-3 direct address: %IX0.3
// and %QX0.3 for bits, %IB2 for a byte, %IW8 for a 16-bit word and %ID12 for a 32-bit
// double word, each at its first byte. It is how stations store and the simulation knows a
// signal, whatever the PLC module calls it on screen or a protocol calls it outside (see
// Format).
type Address struct {
	Input bool
	// Size is SizeBit, SizeByte, SizeWord or SizeDouble.
	Size byte
	Byte int
	// Bit is the bit of Byte, for a bit.
	Bit uint8
}

// The sizes of an address, as the letter after %I or %Q.
const (
	SizeBit    = 'X'
	SizeByte   = 'B'
	SizeWord   = 'W'
	SizeDouble = 'D'
)

// BitAddress is the address of a bit of the inputs or outputs.
func BitAddress(input bool, a Addr) Address {
	return Address{Input: input, Size: SizeBit, Byte: int(a.Byte), Bit: a.Bit}
}

// WordAddress is the address of a word of the inputs or outputs, at its first byte.
func WordAddress(input bool, b int) Address {
	return Address{Input: input, Size: SizeWord, Byte: b}
}

var directAddress = regexp.MustCompile(`^%([IQ])([XBWD])(\d+)(?:\.(\d+))?$`)

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
	a := Address{Input: m[1] == "I", Size: m[2][0], Byte: b}
	switch {
	case a.Size != SizeBit && m[4] != "":
		return Address{}, fmt.Errorf("%q is no bit and has no bit number", s)
	case a.Size == SizeBit && m[4] == "":
		return Address{}, fmt.Errorf("%q needs a bit, as %%IX0.3", s)
	case a.Size == SizeBit:
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
	if a.IsBit() {
		return fmt.Sprintf("%%%sX%d.%d", side, a.Byte, a.Bit)
	}
	return fmt.Sprintf("%%%s%c%d", side, a.Size, a.Byte)
}

// Bytes is how many bytes of the image the address takes: 1 for a bit or a byte.
func (a Address) Bytes() int {
	switch a.Size {
	case SizeWord:
		return 2
	case SizeDouble:
		return 4
	}
	return 1
}

// IsBit says whether the address is that of a bit.
func (a Address) IsBit() bool { return a.Size == SizeBit || a.Size == 0 }

// Overlaps says whether two addresses of the same direction share a byte.
func (a Address) Overlaps(b Address) bool {
	return a.Input == b.Input && a.Byte < b.Byte+b.Bytes() && b.Byte < a.Byte+a.Bytes()
}

// InImage is the position of a bit address in an Image, if it is one there.
func (a Address) InImage() (Addr, bool) {
	if !a.IsBit() || a.Byte < 0 || a.Byte >= ImageBytes {
		return Addr{}, false
	}
	return Addr{Byte: uint8(a.Byte), Bit: a.Bit}, true
}

// Format is how a PLC module shows the addresses of its image: a template for bits of the
// inputs and of the outputs, with {byte}, {bit} (or {bit:2} for two digits), and for words,
// with {byte} and, for an analogue channel, {channel}. Without a template, an address is
// shown as it is, such as %IX0.3.
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
func (f Format) Show(a Address) string { return f.ShowChannel(a, -1) }

// ShowChannel writes an address of an analogue channel, 0 for the first, as the format
// says; -1 is none.
func (f Format) ShowChannel(a Address, channel int) string {
	template := ""
	switch {
	case a.IsBit() && a.Input:
		template = f.Input
	case a.IsBit():
		template = f.Output
	case a.Size == SizeWord && a.Input:
		template = f.InputWord
	case a.Size == SizeWord:
		template = f.OutputWord
	}
	if template == "" {
		return a.String()
	}
	if strings.Contains(template, "{channel}") {
		if channel < 0 {
			return a.String()
		}
		template = strings.ReplaceAll(template, "{channel}", strconv.Itoa(channel))
	}
	s := strings.ReplaceAll(template, "{byte}", strconv.Itoa(a.Byte))
	s = strings.ReplaceAll(s, "{bit:2}", fmt.Sprintf("%02d", a.Bit))
	return strings.ReplaceAll(s, "{bit}", strconv.Itoa(int(a.Bit)))
}

// Format is how the naming's names show the addresses they stand for.
func (n Naming) Format() Format {
	return Format{Input: n.Inputs, Output: n.Outputs, InputWord: n.Analog}
}

// Address reads the address of a bit, written as one or, as older station files do, as the
// name of a variable of the naming, such as IB0Bit03. The names of analogue inputs, such as
// AI0, are channels (see ParseAnalog), whose addresses the PLC module places.
func (n Naming) Address(s string) (Address, error) {
	if IsAddress(s) {
		return ParseAddress(s)
	}
	a, input, err := n.Parse(s)
	if err != nil {
		return Address{}, err
	}
	return BitAddress(input, a), nil
}
