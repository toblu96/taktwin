// Package exercise runs the check of an exercise against a station: the steps and rules of
// its exercise.json, against whatever PLC runs the station, as the function check of a
// grading sheet does by hand.
package exercise

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/sim"
)

type engineWorkpiece = sim.WorkpieceView

// Check is the result of one line of the check.
type Check struct {
	Line string `json:"line"`
	OK   bool   `json:"ok"`
}

// Passed reports whether every line passed.
func Passed(checks []Check) bool {
	return len(checks) > 0 && !slices.ContainsFunc(checks, func(c Check) bool { return !c.OK })
}

// placed is a workpiece the run put into the station.
type placed struct {
	kind       string
	upsideDown bool
	taken      bool
}

type runner struct {
	ctx      context.Context
	progress func(Check)
	ex       *model.ExerciseDef
	e        *engine.Engine
	plan     *model.Plan
	realTime bool
	checks   []Check
	placed   map[int]*placed
	families map[string]string // kind to family
	lengths  map[string]float64
	cur      engine.Snapshot
	compiled map[string]model.Expr
	using    []string // conditions being compiled, against a condition that uses itself
}

// Options are how a check runs. With RealTime it waits on the wall clock, for a station
// that runs on its own, as in the app or with a PLC over OPC UA, and the engine has to be
// running; otherwise it advances the engine's own time. Progress hears of each line as it
// is checked, and Context stops a run, as a line that fails.
type Options struct {
	RealTime bool
	Progress func(Check)
	Context  context.Context
}

// Run checks a station that runs in the engine against an exercise, from where it is now. It
// stops at the first line that fails, as later lines would only repeat that. An error is an
// exercise that does not fit the station, such as one naming a state it does not have.
func Run(e *engine.Engine, ex *model.ExerciseDef, opts Options) ([]Check, error) {
	r := &runner{
		ctx: cmp.Or(opts.Context, context.Background()), progress: opts.Progress,
		ex: ex, e: e, plan: e.Plan(), realTime: opts.RealTime, placed: map[int]*placed{},
		families: map[string]string{}, lengths: map[string]float64{}, compiled: map[string]model.Expr{},
	}
	for _, k := range r.plan.Scene.Workpieces.Kinds {
		r.families[k.ID] = k.Family
	}
	for _, s := range r.plan.Segments {
		r.lengths[s.Name] = s.Length
	}
	// Every expression is compiled before anything runs, so that a mistake in the exercise
	// shows at once and not half way through.
	if err := r.compileAll(ex.Steps, nil); err != nil {
		return nil, err
	}
	r.steps(ex.Steps, nil)
	return r.checks, nil
}

func (r *runner) compileAll(steps []model.StepDef, round map[string]string) error {
	for _, s := range steps {
		if len(s.Repeat) > 0 {
			for _, kind := range s.Repeat {
				if err := r.compileAll(s.Steps, r.round(kind)); err != nil {
					return err
				}
			}
			continue
		}
		for _, src := range []string{s.Until, s.Holds} {
			if src == "" {
				continue
			}
			if _, err := r.compile(fill(src, round)); err != nil {
				return err
			}
		}
	}
	return nil
}

// round is what {kind} and {place} stand for in a round of a repeat.
func (r *runner) round(kind string) map[string]string {
	out := map[string]string{"kind": kind}
	if rule := r.rule(kind, false); rule != nil {
		out["place"] = rule.Place
	}
	return out
}

func fill(s string, round map[string]string) string {
	for k, v := range round {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// steps runs steps and reports whether all passed.
func (r *runner) steps(steps []model.StepDef, round map[string]string) bool {
	for _, s := range steps {
		if !r.step(s, round) {
			return false
		}
	}
	return true
}

func (r *runner) step(s model.StepDef, round map[string]string) bool {
	if r.ctx.Err() != nil {
		return false
	}
	line := fill(s.Line, round)
	switch {
	case len(s.Repeat) > 0:
		for _, kind := range s.Repeat {
			if !r.steps(s.Steps, r.round(kind)) {
				return false
			}
		}
	case s.Place != "":
		return r.put(fill(s.Place, round), fill(s.Kind, round), s.UpsideDown, line)
	case s.Press != "":
		_ = r.e.SetControl(s.Press, true)
		r.advance(100 * time.Millisecond)
		_ = r.e.SetControl(s.Press, false)
		r.advance(100 * time.Millisecond)
	case s.Set != "":
		_ = r.e.SetControl(s.Set, s.On)
		r.advance(100 * time.Millisecond)
	case s.Wait > 0:
		r.advance(seconds(s.Wait))
	case s.Take:
		r.take()
	case s.Until != "":
		f, _ := r.compile(fill(s.Until, round))
		return r.record(cmpOr(line, fill(s.Until, round)), r.until(r.cond(f), seconds(s.Within)))
	case s.Holds != "":
		f, _ := r.compile(fill(s.Holds, round))
		return r.record(cmpOr(line, fill(s.Holds, round)), r.holds(r.cond(f), seconds(s.For)))
	}
	return true
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

func (r *runner) record(line string, ok bool) bool {
	ok = ok && r.ctx.Err() == nil
	r.checks = append(r.checks, Check{Line: line, OK: ok})
	if r.progress != nil {
		r.progress(r.checks[len(r.checks)-1])
	}
	return ok
}

func (r *runner) cond(f model.Expr) func(engine.Snapshot) bool {
	return func(s engine.Snapshot) bool {
		r.cur = s
		return f(nil) >= 0.5
	}
}

// put places a workpiece with an action, and remembers it.
func (r *runner) put(action, kind string, upsideDown bool, line string) bool {
	before := map[int]bool{}
	for _, w := range r.e.Snapshot().Workpieces {
		before[w.ID] = true
	}
	args := map[string]string{"kind": kind}
	if upsideDown {
		args["upsideDown"] = "true"
	}
	if err := r.e.RunAction(action, args); err != nil {
		return r.record(fmt.Sprintf("%s: %v", cmpOr(line, "Place "+kind), err), false)
	}
	for _, w := range r.e.Snapshot().Workpieces {
		if !before[w.ID] {
			r.placed[w.ID] = &placed{kind: kind, upsideDown: upsideDown}
		}
	}
	if line != "" {
		return r.record(line, true)
	}
	return true
}

// take takes away by hand what waits where its rule wants it.
func (r *runner) take() {
	for _, w := range r.e.Snapshot().Workpieces {
		p := r.placed[w.ID]
		if p == nil || p.taken {
			continue
		}
		if rule := r.rule(p.kind, p.upsideDown); rule != nil && rule.WaitsAt != "" && r.waits(w, rule) {
			if r.e.TakeWorkpiece(w.ID) == nil {
				p.taken = true
			}
		}
	}
}

// rule is the first rule a workpiece matches, or nil.
func (r *runner) rule(kind string, upsideDown bool) *model.RuleDef {
	for i := range r.ex.Rules {
		rule := &r.ex.Rules[i]
		if (rule.Kind == "" || rule.Kind == kind) && (rule.Family == "" || rule.Family == r.families[kind]) &&
			(rule.UpsideDown == nil || *rule.UpsideDown == upsideDown) {
			return rule
		}
	}
	return nil
}

// waits reports whether a workpiece waits where a rule wants it: At mm along the transport,
// or without At at its end: where an end stop holds it, up to a workpiece and a half short of
// it, as where the belt stops once the sensor at its end sees a low cap.
func (r *runner) waits(w engineWorkpiece, rule *model.RuleDef) bool {
	if w.On != rule.WaitsAt {
		return false
	}
	if rule.At != nil {
		return math.Abs(w.S-*rule.At) <= 1
	}
	// At its end: within one and a half workpieces of it, by their length where they have one.
	along := r.plan.Scene.Workpieces.Diameter
	for _, k := range r.plan.Scene.Workpieces.Kinds {
		if k.ID == w.Kind && len(k.Size) == 2 {
			along = k.Size[0]
		}
	}
	return w.S >= r.lengths[rule.WaitsAt]-1.5*along-1
}

// rulesHold reports whether every workpiece placed so far is where its rule wants it: those
// to take by hand waiting or taken, and the others on where they end.
func (r *runner) rulesHold() bool {
	for id, p := range r.placed {
		if p.taken {
			continue
		}
		rule := r.rule(p.kind, p.upsideDown)
		if rule == nil {
			continue
		}
		i := slices.IndexFunc(r.cur.Workpieces, func(w engineWorkpiece) bool { return w.ID == id })
		if i < 0 {
			return false // lost
		}
		w := r.cur.Workpieces[i]
		if rule.WaitsAt != "" && !r.waits(w, rule) || rule.EndsOn != "" && w.On != rule.EndsOn {
			return false
		}
	}
	return true
}

// until and advance take the engine's time, unless the run is on the wall clock.
func (r *runner) until(cond func(engine.Snapshot) bool, timeout time.Duration) bool {
	if !r.realTime {
		return r.e.AdvanceUntil(cond, timeout)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) && r.ctx.Err() == nil {
		if cond(r.e.Snapshot()) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond(r.e.Snapshot())
}

// holds reports whether a condition holds all through a time.
func (r *runner) holds(cond func(engine.Snapshot) bool, d time.Duration) bool {
	const step = 20 * time.Millisecond
	for left := d; ; left -= step {
		if !cond(r.e.Snapshot()) {
			return false
		}
		if left <= 0 {
			return true
		}
		r.advance(min(step, left))
	}
}

func (r *runner) advance(d time.Duration) {
	if r.realTime {
		select {
		case <-time.After(d):
		case <-r.ctx.Done():
		}
		return
	}
	r.e.Advance(d)
}

// compile compiles an expression against the current snapshot.
func (r *runner) compile(src string) (model.Expr, error) {
	if f, ok := r.compiled[src]; ok {
		return f, nil
	}
	f, err := model.CompileExpr(src, model.ExprEnv{Name: r.name, Call: r.call})
	if err != nil {
		return nil, fmt.Errorf("exercise %s: %w", r.ex.ID, err)
	}
	r.compiled[src] = f
	return f, nil
}

func (r *runner) name(name string) (model.Expr, error) {
	switch name {
	case "step":
		return func([]float64) float64 {
			if r.cur.Step == nil {
				return -1
			}
			return float64(r.cur.Step.Number)
		}, nil
	case "lost":
		return func([]float64) float64 { return float64(r.cur.FellOff) }, nil
	case "hits":
		return func([]float64) float64 { return float64(len(r.cur.Hits)) }, nil
	}
	if src, ok := r.ex.Conditions[name]; ok {
		if slices.Contains(r.using, name) {
			return nil, fmt.Errorf("condition %s uses itself", name)
		}
		r.using = append(r.using, name)
		defer func() { r.using = r.using[:len(r.using)-1] }()
		f, err := model.CompileExpr(src, model.ExprEnv{Name: r.name, Call: r.call})
		if err != nil {
			return nil, fmt.Errorf("condition %s: %w", name, err)
		}
		return f, nil
	}
	if signal, ok := strings.CutPrefix(name, "out."); ok {
		addr, ok := r.plan.PLCOutput(signal)
		if !ok {
			return nil, fmt.Errorf("the PLC has no output %s", signal)
		}
		return func([]float64) float64 { return flag(r.cur.Outputs.Get(addr)) }, nil
	}
	if signal, ok := strings.CutPrefix(name, "in."); ok {
		addr, ok := r.plan.PLCInput(signal)
		if !ok {
			return nil, fmt.Errorf("the PLC has no input %s", signal)
		}
		return func([]float64) float64 { return flag(r.cur.Inputs.Get(addr)) }, nil
	}
	if i := r.plan.State(name); i >= 0 {
		return func([]float64) float64 { return r.cur.States[i] }, nil
	}
	return nil, fmt.Errorf("the station has no state, signal or condition %s", name)
}

func (r *runner) call(fn string, args []any) (model.Expr, error) {
	text := func(i int) (string, bool) {
		if i >= len(args) {
			return "", false
		}
		s, ok := args[i].(string)
		return s, ok
	}
	number := func(i int) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		v, ok := args[i].(float64)
		return v, ok
	}
	switch fn {
	case "rules":
		if len(args) != 0 {
			return nil, errors.New("rules() takes nothing")
		}
		return func([]float64) float64 { return flag(r.rulesHold()) }, nil
	case "count":
		transport, ok := text(0)
		kind, hasKind := text(1)
		if !ok || len(args) > 2 || len(args) == 2 && !hasKind {
			return nil, errors.New(`count takes a transport and maybe a kind, as count("slide1.slide", "red")`)
		}
		if _, known := r.lengths[transport]; !known {
			return nil, fmt.Errorf("the station has no transport %s", transport)
		}
		return func([]float64) float64 {
			n := 0
			for _, w := range r.cur.Workpieces {
				if w.On == transport && (!hasKind || w.Kind == kind) {
					n++
				}
			}
			return float64(n)
		}, nil
	case "loads":
		transport, ok := text(0)
		kind, hasKind := text(1)
		if !ok || len(args) > 2 || len(args) == 2 && !hasKind {
			return nil, errors.New(`loads takes a transport and maybe a kind, as loads("conveyor.surface", "carton-large")`)
		}
		if _, known := r.lengths[transport]; !known {
			return nil, fmt.Errorf("the station has no transport %s", transport)
		}
		return func([]float64) float64 {
			n := 0
			for _, w := range r.cur.Workpieces {
				if w.On != transport {
					continue
				}
				for _, l := range w.Loads {
					if !hasKind || l.Kind == kind {
						n++
					}
				}
			}
			return float64(n)
		}, nil
	case "at":
		transport, ok := text(0)
		at, ok2 := number(1)
		tolerance, given := number(2)
		if !ok || !ok2 || len(args) > 3 || len(args) == 3 && !given {
			return nil, errors.New(`at takes a transport, mm along it and maybe a tolerance, as at("conveyor.surface", 65)`)
		}
		if _, known := r.lengths[transport]; !known {
			return nil, fmt.Errorf("the station has no transport %s", transport)
		}
		if !given {
			tolerance = 1
		}
		return func([]float64) float64 {
			return flag(slices.ContainsFunc(r.cur.Workpieces, func(w engineWorkpiece) bool {
				return w.On == transport && math.Abs(w.S-at) <= tolerance
			}))
		}, nil
	}
	return nil, fmt.Errorf("unknown function %s", fn)
}

func flag(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
