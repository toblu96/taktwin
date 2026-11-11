// Package plcio defines the bit image Taktwin exchanges with a PLC.
//
// The image mirrors a variable list of a PLC program, as a CODESYS project offers it over
// OPC UA, so the soft PLC and a real one see the same addresses. Its variables are named as
// the PLC module says (see Naming).
package plcio

import "time"

// ImageBytes is the size of each direction of the image, 8 bytes of inputs and 8 of outputs. A station
// uses as many of them as its PLC's connectors have bytes; a PLC program only needs to offer
// those.
const ImageBytes = 8

// Image is one direction of the image: index 0 is the first byte of inputs or outputs.
type Image [ImageBytes]byte

// AnalogInputs is how many analogue inputs a PLC has, channels 0 to 3.
const AnalogInputs = 4

// Analog is the value of each analogue input, in volts.
type Analog [AnalogInputs]float64

// Addr is the position of one bit in an Image.
type Addr struct {
	Byte uint8
	Bit  uint8
}

// Get reads the bit at a.
func (im Image) Get(a Addr) bool {
	return im[a.Byte]&(1<<a.Bit) != 0
}

// Set writes the bit at a.
func (im *Image) Set(a Addr, v bool) {
	if v {
		im[a.Byte] |= 1 << a.Bit
	} else {
		im[a.Byte] &^= 1 << a.Bit
	}
}

// Controller is the PLC side of the link. The simulation only depends on this
// interface, so the built-in PLC and a remote soft PLC are interchangeable.
type Controller interface {
	// Name is shown to the user as the active PLC source.
	Name() string
	// Cycle is the time between two calls of Scan.
	Cycle() time.Duration
	// Scan runs one PLC cycle: it receives station inputs and returns outputs.
	Scan(in Image) Image
	// Reset returns the controller to its power-on state.
	Reset()
}

// AnalogReceiver is implemented by controllers that pass analogue inputs on to a PLC. It
// is called before each Scan.
type AnalogReceiver interface {
	SetAnalog(Analog)
}

// StepInfo is the active step of a sequence.
type StepInfo struct {
	Number uint16 `json:"number"`
	Label  string `json:"label"`
}

// StepReporter is implemented by controllers that can expose their active step.
// A remote soft PLC cannot, so callers must treat it as optional.
type StepReporter interface {
	Step() StepInfo
}
