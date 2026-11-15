// Package plcio defines the process image Taktwin exchanges with a PLC.
//
// Each direction of the image is a row of bytes, as a PLC's: bits, bytes, words and double
// words in it are reached by their IEC 61131-3 addresses, %IX0.3, %IB2, %IW8, %ID12 (see
// Address). Words and double words are stored with their low byte first, as CODESYS on a PC
// stores them. A PLC module says how much of the image it uses.
package plcio

import (
	"encoding/binary"
	"math"
	"time"
)

// ImageBytes is the most bytes each direction of the image can have. A PLC module uses
// as many of them as it says.
const ImageBytes = 64

// Image is one direction of the image: index 0 is the first byte of inputs or outputs.
type Image [ImageBytes]byte

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

// Raw reads the value at an address as the unsigned number its bits make: 0 or 1 for a bit.
// An address outside the image reads 0.
func (im Image) Raw(a Address) uint32 {
	n := a.Bytes()
	if a.Byte < 0 || a.Byte+n > ImageBytes {
		return 0
	}
	switch a.Size {
	case SizeByte:
		return uint32(im[a.Byte])
	case SizeWord:
		return uint32(binary.LittleEndian.Uint16(im[a.Byte:]))
	case SizeDouble:
		return binary.LittleEndian.Uint32(im[a.Byte:])
	}
	if im[a.Byte]&(1<<a.Bit) != 0 {
		return 1
	}
	return 0
}

// SetRaw writes the value at an address, cut to its size. An address outside the image is
// left alone.
func (im *Image) SetRaw(a Address, v uint32) {
	n := a.Bytes()
	if a.Byte < 0 || a.Byte+n > ImageBytes {
		return
	}
	switch a.Size {
	case SizeByte:
		im[a.Byte] = byte(v)
	case SizeWord:
		binary.LittleEndian.PutUint16(im[a.Byte:], uint16(v))
	case SizeDouble:
		binary.LittleEndian.PutUint32(im[a.Byte:], v)
	default:
		im.Set(Addr{Byte: uint8(a.Byte), Bit: a.Bit}, v != 0)
	}
}

// Scaling is how an analogue value appears in the image: as an INT in a word, whose Raw
// range stands for the Range of the physical value, such as 0 to 27648 for 0 to 10 V; or,
// with Real, as the physical value itself, a REAL in a double word.
type Scaling struct {
	Range [2]float64
	Raw   [2]float64
	Unit  string
	Real  bool
}

// ToRaw gives the raw value of a physical one, as a number: the integer, rounded and held to
// the range of an INT, or the value itself for a REAL.
func (s Scaling) ToRaw(v float64) float64 {
	if s.Real {
		return v
	}
	span := s.Range[1] - s.Range[0]
	raw := s.Raw[0]
	if span != 0 {
		raw += (v - s.Range[0]) * (s.Raw[1] - s.Raw[0]) / span
	}
	return math.Max(math.MinInt16, math.Min(math.MaxInt16, math.Round(raw)))
}

// FromRaw gives the physical value of a raw one.
func (s Scaling) FromRaw(raw float64) float64 {
	if s.Real {
		return raw
	}
	span := s.Raw[1] - s.Raw[0]
	if span == 0 {
		return s.Range[0]
	}
	return s.Range[0] + (raw-s.Raw[0])*(s.Range[1]-s.Range[0])/span
}

// Write puts a physical value into the image at an address.
func (s Scaling) Write(im *Image, a Address, v float64) {
	if s.Real {
		im.SetRaw(a, math.Float32bits(float32(v)))
		return
	}
	im.SetRaw(a, uint32(uint16(int16(s.ToRaw(v)))))
}

// Read takes a physical value out of the image at an address.
func (s Scaling) Read(im Image, a Address) float64 {
	raw := im.Raw(a)
	if s.Real {
		return float64(math.Float32frombits(raw))
	}
	return s.FromRaw(float64(int16(uint16(raw))))
}
