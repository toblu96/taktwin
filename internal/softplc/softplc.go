// Package softplc is the PLC that runs inside Taktwin when no other one is connected. It has
// no program: stations are general, and what runs them comes from outside, over OPC UA.
package softplc

import (
	"fmt"
	"time"

	"github.com/toblu96/taktwin/internal/plcio"
)

// New returns the PLC a station file names. Only a station without a program is known: the
// programs that once were built in belonged to particular exercises.
func New(program string) (plcio.Controller, error) {
	if program == "" {
		return Idle{}, nil
	}
	return nil, fmt.Errorf("the PLC program %q is not built in: connect a PLC over OPC UA instead", program)
}

// Idle is a PLC that keeps all outputs off.
type Idle struct{}

// Name implements plcio.Controller.
func (Idle) Name() string { return "No PLC" }

// Cycle implements plcio.Controller.
func (Idle) Cycle() time.Duration { return 20 * time.Millisecond }

// Scan implements plcio.Controller.
func (Idle) Scan(plcio.Image) plcio.Image { return plcio.Image{} }

// Reset implements plcio.Controller.
func (Idle) Reset() {}
