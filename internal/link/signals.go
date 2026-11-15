package link

import (
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

// Signals are the signals of PLC k of a station: every bit its wiring reaches and every
// analogue channel, with the symbols the station gives them.
func Signals(plan *model.Plan, k int) []Signal {
	symbols := map[string]string{}
	if k >= 0 && k < len(plan.Scene.PLCs) {
		p := plan.Scene.PLCs[k]
		for _, s := range append(append([]model.SceneSignal{}, p.Inputs...), p.Outputs...) {
			symbols[s.Address] = s.Name
		}
		for _, a := range append(append([]model.SceneAnalog{}, p.Analog...), p.AnalogOutputs...) {
			symbols[a.Address] = a.Name
		}
	}
	var out []Signal
	seen := map[plcio.Address]bool{}
	add := func(a plcio.Address, analog *model.PLCAnalogPlan) {
		if seen[a] {
			return
		}
		seen[a] = true
		s := Signal{Address: a, Symbol: symbols[a.String()], Shown: plan.Show(k, a)}
		if analog != nil {
			s.Scaling, s.Channel = &analog.Scaling, analog.Channel
		}
		out = append(out, s)
	}
	for _, b := range plan.PLCIn {
		if b.PLC == k {
			add(plcio.BitAddress(true, b.Addr), nil)
		}
	}
	for i, a := range plan.PLCAnalog {
		if a.PLC == k {
			add(a.Address, &plan.PLCAnalog[i])
		}
	}
	for _, b := range plan.PLCOut {
		if b.PLC == k {
			add(plcio.BitAddress(false, b.Addr), nil)
		}
	}
	for i, a := range plan.PLCAnalogOut {
		if a.PLC == k {
			add(a.Address, &plan.PLCAnalogOut[i])
		}
	}
	return out
}

// Image is every signal of a PLC with an image as a PLC module has it without saying: each
// bit of 8 bytes of inputs and of outputs, and 4 analogue inputs and outputs as INTs of 0 to
// 27648 for 0 to 10 V in the words from byte 8 on. It has no symbols.
func Image() []Signal {
	var out []Signal
	scaling := &plcio.Scaling{Range: [2]float64{0, 10}, Raw: [2]float64{0, 27648}, Unit: "V"}
	for _, input := range []bool{true, false} {
		for b := range uint8(8) {
			for bit := range uint8(8) {
				a := plcio.BitAddress(input, plcio.Addr{Byte: b, Bit: bit})
				out = append(out, Signal{Address: a, Shown: a.String()})
			}
		}
		for ch := range 4 {
			a := plcio.WordAddress(input, 8+2*ch)
			out = append(out, Signal{Address: a, Shown: a.String(), Scaling: scaling, Channel: ch})
		}
	}
	return out
}
