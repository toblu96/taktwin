package plcprog

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/toblu96/taktwin/internal/opcsim"
	"github.com/toblu96/taktwin/internal/plcio"
	"github.com/toblu96/taktwin/st"
)

// Soft is a soft PLC inside the app: it runs a compiled Structured Text program on its
// task's cycle, and serves its Bits list over OPC UA as CODESYS does, so that the
// simulation connects to it like to any other PLC. A programming tool downloads, starts,
// watches and forces it.
type Soft struct {
	mu     sync.Mutex
	name   string
	prog   *st.Program
	rt     *st.Runtime
	step   string
	start  time.Time
	cycle  time.Duration
	server *opcsim.Server
	ctx    context.Context
	// naming is how the station's PLC module names the variables the server offers.
	naming plcio.Naming
	// clock gives the time for the timers; nil is the wall clock since the start.
	clock func() time.Duration
	// loaded is when the program was downloaded.
	loaded time.Time
}

// NewSoft makes a soft PLC without a program, named as the simulation shows it.
func NewSoft(name string) *Soft {
	return &Soft{name: name, start: time.Now(), cycle: 20 * time.Millisecond}
}

// Name implements plcio.Controller.
func (s *Soft) Name() string { return s.name }

// Cycle implements plcio.Controller.
func (s *Soft) Cycle() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cycle
}

// UseClock makes the timers run on a clock of the caller's, such as the simulation's time
// when it drives the PLC itself instead of over OPC UA.
func (s *Soft) UseClock(clock func() time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = clock
}

func (s *Soft) now() time.Duration {
	if s.clock != nil {
		return s.clock()
	}
	return time.Since(s.start)
}

// Scan implements plcio.Controller: one cycle of the program.
func (s *Soft) Scan(in plcio.Image) plcio.Image {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out plcio.Image
	if s.rt == nil {
		return out
	}
	copy(out[:], s.rt.Scan(in[:], s.now()))
	return out
}

// Reset implements plcio.Controller: a cold reset, after which a program that ran runs on,
// as when the station restarts.
func (s *Soft) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt == nil {
		return
	}
	state, _ := s.rt.State()
	s.rt.ResetCold()
	if state != st.Stopped {
		_ = s.rt.Start()
	}
}

// Step implements plcio.StepReporter, from the variable the project names as its step.
func (s *Soft) Step() plcio.StepInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt == nil || s.step == "" {
		return plcio.StepInfo{}
	}
	v, err := s.rt.Read(s.step)
	if err != nil {
		return plcio.StepInfo{}
	}
	var n int
	if _, err := fmt.Sscan(v, &n); err != nil || n < 0 || n > 65535 {
		return plcio.StepInfo{Label: v}
	}
	return plcio.StepInfo{Number: uint16(n), Label: s.step}
}

// Download loads a compiled program, stopped and with every variable at its initial value,
// as a programming tool's login with download does. The step is the path of the variable
// that holds the sequence's step, or empty.
func (s *Soft) Download(prog *st.Program, step string) error {
	s.mu.Lock()
	s.prog, s.rt, s.step, s.loaded = prog, st.NewRuntime(prog), step, time.Now()
	restart := s.server != nil && prog.Cycle() != s.cycle
	s.cycle = prog.Cycle()
	ctx := s.ctx
	s.mu.Unlock()
	// The OPC UA server scans on the task's interval, so a new interval needs a new server.
	if restart {
		s.mu.Lock()
		old := s.server
		s.server = nil
		s.mu.Unlock()
		old.Close()
		return s.Serve(ctx)
	}
	return nil
}

// SetNaming says how the PLC module names the variables of its image, which the OPC UA
// server offers by those names. A running server starts again with new names.
func (s *Soft) SetNaming(n plcio.Naming) {
	s.mu.Lock()
	changed := s.naming != n
	s.naming = n
	srv, ctx := s.server, s.ctx
	if changed {
		s.server = nil
	}
	s.mu.Unlock()
	if changed && srv != nil {
		srv.Close()
		_ = s.Serve(ctx)
	}
}

// Serve starts the OPC UA server, if it does not run yet.
func (s *Soft) Serve(ctx context.Context) error {
	s.mu.Lock()
	if s.server != nil {
		s.mu.Unlock()
		return nil
	}
	s.ctx = ctx
	naming := s.naming
	s.mu.Unlock()
	srv, err := opcsim.StartOnFreePort(ctx, s, naming)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.server = srv
	s.mu.Unlock()
	return nil
}

// Endpoint is where the OPC UA server is reached, or empty while it does not run.
func (s *Soft) Endpoint() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return ""
	}
	return s.server.Endpoint()
}

// Close stops the OPC UA server.
func (s *Soft) Close() {
	s.mu.Lock()
	srv := s.server
	s.server = nil
	s.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
}

var errNoProgram = errors.New("the PLC has no program yet: build it and download it first")

// Start runs the program.
func (s *Soft) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt == nil {
		return errNoProgram
	}
	return s.rt.Start()
}

// Stop holds the program.
func (s *Soft) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt != nil {
		s.rt.Stop()
	}
}

// ResetProgram resets the variables, cold or keeping the RETAIN ones, and stops.
func (s *Soft) ResetProgram(cold bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt == nil {
		return
	}
	if cold {
		s.rt.ResetCold()
	} else {
		s.rt.ResetWarm()
	}
}

// Status is what a programming tool shows of a PLC.
type Status struct {
	Loaded   bool             `json:"loaded"`
	State    string           `json:"state"`
	Error    *st.RuntimeError `json:"error"`
	Cycles   int64            `json:"cycles"`
	Cycle    string           `json:"cycle"`
	Endpoint string           `json:"endpoint"`
	Forced   []string         `json:"forced"`
	LoadedAt string           `json:"loadedAt"`
}

// Status reports the PLC's state.
func (s *Soft) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Status{State: "empty", Cycle: s.cycle.String(), Forced: []string{}}
	if s.server != nil {
		out.Endpoint = s.server.Endpoint()
	}
	if s.rt == nil {
		return out
	}
	state, err := s.rt.State()
	out.Loaded, out.State, out.Error, out.Cycles = true, string(state), err, s.rt.Cycles()
	out.Forced = append(out.Forced, s.rt.Forced()...)
	out.LoadedAt = s.loaded.Format(time.TimeOnly)
	return out
}

// Variables lists the program's variables with their values.
func (s *Soft) Variables() []st.WatchVar {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt == nil {
		return []st.WatchVar{}
	}
	return s.rt.Variables()
}

// Write sets a variable once.
func (s *Soft) Write(path, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt == nil {
		return errNoProgram
	}
	return s.rt.Write(path, value)
}

// Force holds a variable at a value.
func (s *Soft) Force(path, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt == nil {
		return errNoProgram
	}
	return s.rt.Force(path, value)
}

// Release lets a forced variable go; "" lets all go.
func (s *Soft) Release(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rt != nil {
		s.rt.Release(path)
	}
}

// LinkProgram is what the simulation's OPC UA link may do with this PLC: show its step and
// reset it along with the station. Closing the link leaves the PLC running, as the
// programming tool owns it.
func (s *Soft) LinkProgram() interface {
	Step() (plcio.StepInfo, bool)
	Reset()
	Close()
} {
	return linkProgram{s}
}

type linkProgram struct{ s *Soft }

func (l linkProgram) Step() (plcio.StepInfo, bool) {
	info := l.s.Step()
	return info, info.Label != ""
}
func (l linkProgram) Reset() { l.s.Reset() }
func (l linkProgram) Close() {}

// Manager keeps the soft PLCs of the app, one for each PLC of each station.
type Manager struct {
	mu   sync.Mutex
	plcs map[string]*Soft
}

// Get gives the soft PLC for a station's PLC, making it on first use.
func (m *Manager) Get(station, plc string) *Soft {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plcs == nil {
		m.plcs = map[string]*Soft{}
	}
	key := station + "/" + plc
	s := m.plcs[key]
	if s == nil {
		s = NewSoft(fmt.Sprintf("Soft PLC %s (Structured Text)", plc))
		m.plcs[key] = s
	}
	return s
}

// Close stops every soft PLC's server.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.plcs {
		s.Close()
	}
}
