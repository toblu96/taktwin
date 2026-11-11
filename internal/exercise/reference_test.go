package exercise_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/exercise"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
	"github.com/toblu96/taktwin/internal/plcprog"
	"github.com/toblu96/taktwin/internal/softplc"
)

// clocked runs a soft PLC on the simulation's time: each scan is one task cycle later.
type clocked struct {
	*plcprog.Soft
	scans int64
}

func (c *clocked) Scan(in plcio.Image) plcio.Image {
	c.scans++
	return c.Soft.Scan(in)
}

// lagging delivers some of the PLC's outputs later than the others, as a link can when it
// reads a server's variables while the program is changing them.
type lagging struct {
	*clocked
	slow  plcio.Image
	delay int
	past  []plcio.Image
}

func (l *lagging) Scan(in plcio.Image) plcio.Image {
	out := l.clocked.Scan(in)
	l.past = append(l.past, out)
	if len(l.past) <= l.delay {
		return plcio.Image{}
	}
	old := l.past[len(l.past)-1-l.delay]
	for b := range out {
		out[b] = out[b]&^l.slow[b] | old[b]&l.slow[b]
	}
	return out
}

// runExample runs the pallet loading station with its example program on the soft PLC,
// through the exercise "Load a pallet". With lag, the axes' position numbers reach them that
// many PLC cycles after the rest.
func runExample(t *testing.T, stepChain []byte, lag int) []exercise.Check {
	t.Helper()
	lib := model.NewLibrary(catalog.Reference, catalog.Base)
	bundle, err := lib.Load("pallet-loading")
	if err != nil {
		t.Fatal(err)
	}
	probe, err := engine.New(bundle, softplc.Idle{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	proj, err := plcprog.Open(dir, plcprog.Signals(probe.Scene().PLC))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "StepChain.st"), stepChain, 0o644); err != nil {
		t.Fatal(err)
	}
	prog, diags, err := proj.Build()
	if err != nil {
		t.Fatal(err)
	}
	if prog == nil {
		t.Fatalf("the example does not build: %v", diags)
	}
	soft := &clocked{Soft: plcprog.NewSoft("Soft PLC")}
	soft.UseClock(func() time.Duration { return time.Duration(soft.scans) * prog.Cycle() })
	if err := soft.Download(prog, proj.Meta.Step); err != nil {
		t.Fatal(err)
	}
	if err := soft.Start(); err != nil {
		t.Fatal(err)
	}
	ctl := &lagging{clocked: soft, delay: lag}
	for _, name := range []string{"A1_R0", "A1_R1", "A2_R0", "A2_R1", "A3_R0", "A3_R1"} {
		a, ok := probe.Plan().PLCOutput(name)
		if !ok {
			t.Fatalf("no output %s", name)
		}
		ctl.slow.Set(a, true)
	}
	e, err := engine.New(bundle, ctl)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := lib.Exercise("load-a-pallet")
	if err != nil {
		t.Fatal(err)
	}
	checks, err := exercise.Run(e, ex, exercise.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !exercise.Passed(checks) {
		for _, w := range e.Snapshot().Workpieces {
			t.Logf("%s on %s at %v", w.Kind, w.On, w.S)
		}
		t.Logf("hits: %v; step %+v", e.Snapshot().Hits, soft.Step())
	}
	return checks
}

func exampleStepChain(t *testing.T) []byte {
	t.Helper()
	data, err := fs.ReadFile(catalog.Reference, "programs/pallet-loading/cabinet/StepChain.st")
	if err != nil {
		t.Fatalf("the collection has an example program: %v", err)
	}
	return data
}

// The example program of the pallet loading station, the project template with the step
// chain the collection ships, passes the station's exercise on the soft PLC: also when the
// positions of the axes reach them 60 ms after the rest, as over a link that reads the
// variables one by one, because it sets them before it starts an axis.
func TestTheExampleProgramLoadsAPallet(t *testing.T) {
	for _, lag := range []int{0, 3} {
		checks := runExample(t, exampleStepChain(t), lag)
		if !exercise.Passed(checks) {
			for _, c := range checks {
				t.Log(c)
			}
			t.Fatalf("with the positions %d cycles late, the example does not load a pallet", lag)
		}
	}
}
