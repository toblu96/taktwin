package link

import (
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// Signals are the signals of PLC k of a station: every bit its wiring reaches and every
// analogue input, with the symbols the station gives them, ordered by address.
func Signals(plan *model.Plan, k int) []Signal {
	symbols := map[string]string{}
	if k >= 0 && k < len(plan.Scene.PLCs) {
		p := plan.Scene.PLCs[k]
		for _, s := range append(append([]model.SceneSignal{}, p.Inputs...), p.Outputs...) {
			symbols[s.Address] = s.Name
		}
		for _, a := range p.Analog {
			symbols[a.Address] = a.Name
		}
	}
	signal := func(a plcio.Address) Signal {
		return Signal{Address: a, Symbol: symbols[a.String()], Shown: plan.Show(k, a)}
	}
	var out []Signal
	seen := map[plcio.Address]bool{}
	add := func(a plcio.Address) {
		if !seen[a] {
			seen[a] = true
			out = append(out, signal(a))
		}
	}
	for _, b := range plan.PLCIn {
		if b.PLC == k {
			add(plcio.BitAddress(true, b.Addr))
		}
	}
	for _, a := range plan.PLCAnalog {
		if a.PLC == k {
			add(plcio.AnalogInputAddress(a.Channel))
		}
	}
	for _, b := range plan.PLCOut {
		if b.PLC == k {
			add(plcio.BitAddress(false, b.Addr))
		}
	}
	return out
}

// Image is every signal a PLC's image can hold, without symbols: each bit of its inputs and
// outputs and each analogue input.
func Image() []Signal {
	var out []Signal
	for _, input := range []bool{true, false} {
		for b := range uint8(plcio.ImageBytes) {
			for bit := range uint8(8) {
				a := plcio.BitAddress(input, plcio.Addr{Byte: b, Bit: bit})
				out = append(out, Signal{Address: a, Shown: a.String()})
			}
		}
	}
	for ch := range plcio.AnalogInputs {
		a := plcio.AnalogInputAddress(ch)
		out = append(out, Signal{Address: a, Shown: a.String()})
	}
	return out
}
