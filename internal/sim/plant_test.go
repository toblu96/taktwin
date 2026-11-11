package sim

import (
	"testing"

	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
)

const dt = 0.002

func outputs(t *testing.T, plan *model.Plan, names ...string) plcio.Image {
	t.Helper()
	var im plcio.Image
	for _, n := range names {
		a, ok := plan.PLCOutput(n)
		if !ok {
			t.Fatalf("no output %s", n)
		}
		im.Set(a, true)
	}
	return im
}

func input(t *testing.T, p *Plant, plan *model.Plan, name string) bool {
	t.Helper()
	a, ok := plan.PLCInput(name)
	if !ok {
		t.Fatalf("no input %s", name)
	}
	return p.Inputs().Get(a)
}

func run(p *Plant, seconds float64, out plcio.Image) {
	for i := 0; i < int(seconds/dt); i++ {
		p.Step(dt, out)
	}
}

func place(p *Plant, kind string) error {
	return p.RunAction("place", map[string]string{"kind": kind})
}
