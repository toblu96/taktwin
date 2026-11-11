// Package engine couples a plant to a PLC with a deterministic fixed-step clock.
// It has no UI dependency, so tests and the CLI drive the same code as the
// desktop app.
package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
	"github.com/toblu96/taktwin/internal/sim"
)

// TickStep is the fixed simulation step.
const TickStep = 2 * time.Millisecond

// maxFrame caps how much time one frame may catch up after a stall.
const maxFrame = 250 * time.Millisecond

// Snapshot is one complete state of the simulation.
type Snapshot struct {
	Time       float64             `json:"time"`
	Paused     bool                `json:"paused"`
	Speed      float64             `json:"speed"`
	Plc        string              `json:"plc"`
	Step       *plcio.StepInfo     `json:"step"`
	Inputs     plcio.Image         `json:"inputs"`
	Outputs    plcio.Image         `json:"outputs"`
	States     []float64           `json:"states"`
	Workpieces []sim.WorkpieceView `json:"workpieces"`
	Bins       [][]string          `json:"bins"`
	Parameters map[string]float64  `json:"parameters"`
	Faults     []string            `json:"faults"`
	// Collisions says what is moving into a workpiece right now, and Hits everything that ran
	// into something since the station was reset.
	Collisions []string `json:"collisions"`
	Hits       []string `json:"hits"`
	// FellOff counts workpieces lost off the station until someone acknowledges it.
	FellOff int `json:"fellOff"`
	// Analog is what the PLC's analogue inputs read, in volts.
	Analog plcio.Analog `json:"analog"`
	// Link is set while the PLC is reached over a connection. Such a PLC runs on the
	// wall clock, so the speed cannot be changed.
	Link *plcio.LinkInfo `json:"link"`
	// IOTest is set while the outputs are switched by hand instead of by the program.
	IOTest bool `json:"ioTest"`
	// PLCs are all the station's PLCs, the first one first, whose state the fields above
	// repeat.
	PLCs []PLCView `json:"plcs"`
}

// PLCView is the state of one of the station's PLCs.
type PLCView struct {
	ID      string          `json:"id"`
	Plc     string          `json:"plc"`
	Step    *plcio.StepInfo `json:"step"`
	Inputs  plcio.Image     `json:"inputs"`
	Outputs plcio.Image     `json:"outputs"`
	Analog  plcio.Analog    `json:"analog"`
	Link    *plcio.LinkInfo `json:"link"`
	IOTest  bool            `json:"ioTest"`
}

// plcRun is one PLC of the station and its program.
type plcRun struct {
	id       string
	ctl      plcio.Controller
	out      plcio.Image
	nextScan time.Duration
	ioTest   bool
	testOut  plcio.Image
}

// idle is the program of a PLC that none has been given: all its outputs stay off.
type idle struct{}

func (idle) Name() string                 { return "No PLC" }
func (idle) Cycle() time.Duration         { return 10 * time.Millisecond }
func (idle) Scan(plcio.Image) plcio.Image { return plcio.Image{} }
func (idle) Reset()                       {}

// Engine owns the simulation. All methods are safe for concurrent use. A station with
// several PLCs, as a line of stations, runs a program on each; the methods without a PLC
// act on the first one.
type Engine struct {
	mu     sync.Mutex
	bundle *model.Bundle
	plan   *model.Plan
	plant  *sim.Plant
	plcs   []*plcRun
	params map[string]float64
	now    time.Duration
	paused bool
	speed  float64
}

// New returns an engine for a loaded station and a controller.
func New(bundle *model.Bundle, ctl plcio.Controller) (*Engine, error) {
	params := map[string]float64{}
	for _, p := range bundle.Station.Parameters {
		params[p.ID] = p.Default
	}
	plan, err := resolve(bundle, params)
	if err != nil {
		return nil, err
	}
	e := &Engine{bundle: bundle, plan: plan, plant: sim.New(plan), params: params, speed: 1}
	ids := plan.PLCs
	if len(ids) == 0 {
		ids = []string{""}
	}
	for k, id := range ids {
		run := &plcRun{id: id, ctl: idle{}}
		if k == 0 {
			run.ctl = ctl
		}
		e.plcs = append(e.plcs, run)
	}
	return e, nil
}

// plc finds a PLC of the station by its module, the first one for "".
func (e *Engine) plc(id string) (*plcRun, error) {
	if id == "" {
		return e.plcs[0], nil
	}
	for _, run := range e.plcs {
		if run.id == id {
			return run, nil
		}
	}
	return nil, fmt.Errorf("the station has no PLC %s", id)
}

// PLCs lists the station's PLCs by their module, the first one first.
func (e *Engine) PLCs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.plcs))
	for k, run := range e.plcs {
		out[k] = run.id
	}
	return out
}

// resolve evaluates the station and checks how it is built.
func resolve(bundle *model.Bundle, params map[string]float64) (*model.Plan, error) {
	plan, err := bundle.Resolve(params)
	if err != nil {
		return nil, err
	}
	plan.Scene.Checks = sim.Check(plan)
	return plan, nil
}

// Scene returns the station description for the frontend.
func (e *Engine) Scene() model.Scene {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plan.Scene
}

// Plan returns the resolved station, for looking up states and addresses.
func (e *Engine) Plan() *model.Plan {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plan
}

// Advance runs the simulation for d of simulated time, ignoring pause and speed.
func (e *Engine) Advance(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.advance(d)
}

func (e *Engine) advance(d time.Duration) {
	outs := make([]plcio.Image, len(e.plcs))
	for end := e.now + d; e.now+TickStep <= end; e.now += TickStep {
		for k, run := range e.plcs {
			if run.ioTest {
				run.out = run.testOut
			} else if e.now >= run.nextScan {
				if r, ok := run.ctl.(plcio.AnalogReceiver); ok {
					r.SetAnalog(e.plant.AnalogOf(k))
				}
				run.out = run.ctl.Scan(e.plant.InputsOf(k))
				run.nextScan += run.ctl.Cycle()
			}
			outs[k] = run.out
		}
		e.plant.StepAll(TickStep.Seconds(), outs)
	}
}

// AdvanceUntil runs until cond holds or timeout passes, and reports whether cond
// was met. It must not call Engine methods other than through the snapshot.
func (e *Engine) AdvanceUntil(cond func(Snapshot) bool, timeout time.Duration) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for end := e.now + timeout; e.now < end; {
		if cond(e.snapshot()) {
			return true
		}
		e.advance(10 * TickStep)
	}
	return cond(e.snapshot())
}

// Snapshot returns the current state.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot()
}

func (e *Engine) snapshot() Snapshot {
	first := e.plcs[0]
	s := Snapshot{
		Time:       e.now.Seconds(),
		Paused:     e.paused,
		Speed:      e.speed,
		Plc:        first.ctl.Name(),
		Inputs:     e.plant.Inputs(),
		Outputs:    first.out,
		States:     e.plant.States(),
		Workpieces: e.plant.Workpieces(),
		Bins:       e.plant.Bins(),
		Parameters: e.params,
		Faults:     e.plant.Faults(),
		Collisions: e.plant.Collisions(),
		Hits:       e.plant.Hits(),
		FellOff:    e.plant.FellOff(),
		Analog:     e.plant.Analog(),
		IOTest:     first.ioTest,
	}
	for k, run := range e.plcs {
		view := PLCView{ID: run.id, Plc: run.ctl.Name(), Inputs: e.plant.InputsOf(k), Outputs: run.out, Analog: e.plant.AnalogOf(k), IOTest: run.ioTest}
		if r, ok := run.ctl.(plcio.StepReporter); ok && !run.ioTest {
			if step := r.Step(); step.Label != "" || step.Number != 0 {
				view.Step = &step
			}
		}
		if r, ok := run.ctl.(plcio.LinkReporter); ok {
			link := r.Link()
			view.Link = &link
		}
		s.PLCs = append(s.PLCs, view)
	}
	s.Step, s.Link = s.PLCs[0].Step, s.PLCs[0].Link
	return s
}

// linked reports whether a PLC is reached over a connection, on the wall clock.
func (e *Engine) linked() bool {
	for _, run := range e.plcs {
		if _, ok := run.ctl.(plcio.LinkReporter); ok {
			return true
		}
	}
	return false
}

// Run advances in real time and calls emit once per frame until ctx ends.
func (e *Engine) Run(ctx context.Context, frame time.Duration, emit func(Snapshot)) {
	ticker := time.NewTicker(frame)
	defer ticker.Stop()
	last := time.Now()
	var debt time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := min(now.Sub(last), maxFrame)
			last = now
			e.mu.Lock()
			if !e.paused {
				debt += time.Duration(float64(elapsed) * e.speed)
				whole := debt - debt%TickStep
				e.advance(whole)
				debt -= whole
			}
			snap := e.snapshot()
			e.mu.Unlock()
			emit(snap)
		}
	}
}

// SetPaused pauses or resumes Run.
func (e *Engine) SetPaused(paused bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.paused = paused
}

// SetSpeed sets the simulated seconds per real second used by Run.
func (e *Engine) SetSpeed(speed float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.linked() {
		return
	}
	e.speed = speed
}

// SetController swaps the PLC and restarts the station with it, as nothing the old PLC
// was doing carries over. It shuts the old one down if it needs that.
func (e *Engine) SetController(ctl plcio.Controller) { _ = e.SetControllerFor("", ctl) }

// SetControllerFor gives one of the station's PLCs, by its module, another program, and
// restarts the station with it.
func (e *Engine) SetControllerFor(plc string, ctl plcio.Controller) error {
	e.mu.Lock()
	run, err := e.plc(plc)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	old := run.ctl
	run.ctl = ctl
	run.ioTest, run.testOut = false, plcio.Image{}
	if _, linked := ctl.(plcio.LinkReporter); linked {
		e.speed = 1
	}
	e.plant.Reset()
	e.now = 0
	for _, r := range e.plcs {
		r.nextScan, r.out = 0, plcio.Image{}
		if r != run {
			r.ctl.Reset()
		}
	}
	e.mu.Unlock()
	// Outside the lock: closing a link can take as long as its network.
	if c, ok := old.(plcio.Closer); ok {
		c.Close()
	}
	return nil
}

// SetIOTest cuts the program off from the I/O, or connects it again. While it is cut off,
// its outputs do nothing and it gets no new inputs, and the outputs are switched with
// SetTestOutput. All outputs go off in both directions and the station stays as it is,
// so a workpiece can be positioned for teaching a sensor. Ending it restarts a program
// this app runs, since its steps no longer match the station.
func (e *Engine) SetIOTest(on bool) { _ = e.SetIOTestFor("", on) }

// SetIOTestFor starts or ends the I/O test of one of the station's PLCs.
func (e *Engine) SetIOTestFor(plc string, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	run, err := e.plc(plc)
	if err != nil || on == run.ioTest {
		return err
	}
	run.ioTest, run.testOut, run.out = on, plcio.Image{}, plcio.Image{}
	if !on {
		run.ctl.Reset()
		run.nextScan = e.now
	}
	return nil
}

// SetTestOutput switches one output by hand during an I/O test. The variable is a name of
// the PLC's image, as its module names them.
func (e *Engine) SetTestOutput(variable string, on bool) error {
	return e.SetTestOutputFor("", variable, on)
}

// SetTestOutputFor switches one output of one of the station's PLCs by hand.
func (e *Engine) SetTestOutputFor(plc, variable string, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	run, err := e.plc(plc)
	if err != nil {
		return err
	}
	k := slices.Index(e.plcs, run)
	if k >= len(e.plan.Naming) {
		return errors.New("the station has no PLC")
	}
	a, isInput, err := e.plan.Naming[k].Parse(variable)
	if err != nil {
		return err
	}
	if isInput {
		return fmt.Errorf("%s is an input, and comes from the station", variable)
	}
	if !run.ioTest {
		return errors.New("outputs can only be switched by hand during an I/O test")
	}
	run.testOut.Set(a, on)
	return nil
}

// ClearTestOutputs switches all outputs off during an I/O test.
func (e *Engine) ClearTestOutputs() { _ = e.ClearTestOutputsFor("") }

// ClearTestOutputsFor switches all outputs of one of the station's PLCs off during its I/O
// test.
func (e *Engine) ClearTestOutputsFor(plc string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	run, err := e.plc(plc)
	if err == nil {
		run.testOut = plcio.Image{}
	}
	return err
}

// Close shuts down the PLCs that need that.
func (e *Engine) Close() {
	e.mu.Lock()
	var ctls []plcio.Controller
	for _, run := range e.plcs {
		ctls = append(ctls, run.ctl)
	}
	e.mu.Unlock()
	for _, ctl := range ctls {
		if c, ok := ctl.(plcio.Closer); ok {
			c.Close()
		}
	}
}

// AcknowledgeFellOff clears the count of workpieces lost off the station.
func (e *Engine) AcknowledgeFellOff() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.plant.AcknowledgeFellOff()
}

// Reset restarts the plant and the PLC. Pause, speed, parameters and faults are kept.
func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.plant.Reset()
	e.now = 0
	for _, run := range e.plcs {
		run.ctl.Reset()
		run.nextScan, run.out, run.testOut = 0, plcio.Image{}, plcio.Image{}
	}
}

// RunAction triggers a function of a module or the station.
func (e *Engine) RunAction(name string, args map[string]string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plant.RunAction(name, args)
}

// TakeWorkpiece takes a workpiece away by hand, by its id.
func (e *Engine) TakeWorkpiece(id int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plant.Take(id)
}

// SetControl presses or releases a control.
func (e *Engine) SetControl(name string, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plant.SetControl(name, on)
}

// SetPin holds a signal on or off, to try out a module without a PLC.
func (e *Engine) SetPin(state int, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plant.SetPin(state, on)
}

// SetFault enables or clears a fault.
func (e *Engine) SetFault(name string, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plant.SetFault(name, on)
}

// SetParameter changes a station parameter while the station runs.
func (e *Engine) SetParameter(id string, value float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	values := make(map[string]float64, len(e.params))
	for k, v := range e.params {
		values[k] = v
	}
	values[id] = value
	plan, err := resolve(e.bundle, values)
	if err != nil {
		return err
	}
	e.params, e.plan = values, plan
	e.plant.ApplyPlan(plan)
	return nil
}
