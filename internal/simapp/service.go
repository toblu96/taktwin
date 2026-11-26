// Package simapp exposes the engine to the frontend as a Wails service. It holds
// no simulation logic.
package simapp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/link"
	"github.com/toblu96/taktwin/internal/link/opcua"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/plcio"
	"github.com/toblu96/taktwin/internal/softplc"
)

// SnapshotEvent is emitted once per frame with an engine.Snapshot.
const SnapshotEvent = "sim:snapshot"

const frame = time.Second / 30

// Service is the frontend's interface to the simulation.
type Service struct {
	engine *engine.Engine
	bundle *model.Bundle
	ctx    context.Context
	stop   context.CancelFunc

	// mu guards the fields above being replaced by another station, and plc.
	mu sync.Mutex
	// plcs is how each of the station's PLCs is connected, by its module; "" is the first.
	plcs map[string]PLCConfig
	// exercise is the exercise whose station is in the simulation, set up as it changes it,
	// or empty for the station as it is.
	exercise string
	// proj is the project the simulation works in, which keeps the PLCs' connections, the
	// station's settings and the soft PLCs' programs.
	proj project
	// opened is a file the system asked the app to open, until the window takes it.
	opened string

	check checking
}

// The ways a station can get its PLC.
const (
	// PLCNone is no PLC: every output stays off, unless the I/O test switches it.
	PLCNone = "none"
	// PLCExternal connects to a controller outside the app, such as a CODESYS PLC or
	// FluidSIM, through one of the connections the PLC can use.
	PLCExternal = "external"
	// PLCSoft connects over OPC UA to the app's own soft PLC for that PLC of the station,
	// which runs the Structured Text program written in its editor.
	PLCSoft = "soft"
)

// PLCConfig says which PLC the station uses, and keeps the PLC's own connections to a
// controller outside the app while another mode runs, so that switching back brings them back.
type PLCConfig struct {
	Mode string `json:"mode"`
	// External is the project's connection of the PLC to a controller outside the app, for
	// PLCExternal; nil until a kind of connection is chosen.
	External *ExternalConnection `json:"external,omitempty"`
	// Others are the connections of other kinds the PLC was given before, by kind, so that
	// choosing one of those kinds again brings its form and bindings back. An app that does
	// not know them reads External alone.
	Others []ExternalConnection `json:"others,omitempty"`
	// AutoConnect connects the PLC as the project opens, in its mode, without asking. A
	// connection that fails leaves the project open and the PLC not connected.
	AutoConnect bool `json:"autoConnect,omitempty"`
}

// ExternalConnection is a project's connection of one PLC to a controller outside the app:
// made from a kind, such as CODESYS or an OPC UA server, and then the project's own, with its
// bindings and what its form is filled in with.
type ExternalConnection struct {
	// Kind is the kind it was made from, by id, such as codesys or opcua.
	Kind       string          `json:"kind"`
	Connection link.Connection `json:"connection"`
	// Values are what its vars are set to, such as the server's address; the others keep
	// their defaults.
	Values map[string]any `json:"values,omitempty"`
}

// New returns the simulation with no project open: the app starts on its projects.
func New() (*Service, error) {
	return &Service{ctx: context.Background(), plcs: map[string]PLCConfig{}}, nil
}

func load(lib *model.Library, station string) (*model.Bundle, *engine.Engine, error) {
	bundle, err := lib.Load(station)
	if err != nil {
		return nil, nil, err
	}
	return start(bundle)
}

// start makes an engine for a station, with no PLC.
func start(bundle *model.Bundle) (*model.Bundle, *engine.Engine, error) {
	ctl, err := softplc.New(bundle.Station.PLC.Program)
	if err != nil {
		return nil, nil, err
	}
	e, err := engine.New(bundle, ctl)
	if err != nil {
		return nil, nil, err
	}
	return bundle, e, nil
}

// run starts the simulation loop of the current station, under s.mu.
func (s *Service) run() {
	if s.engine == nil {
		return
	}
	ctx, stop := context.WithCancel(s.ctx)
	s.stop = stop
	e := s.engine
	go e.Run(ctx, frame, func(snap engine.Snapshot) {
		if app := application.Get(); app != nil {
			app.Event.Emit(SnapshotEvent, snap)
		}
	})
}

// current returns the engine of the station in the simulation, or errNoProject while no
// project is open.
func (s *Service) current() (*engine.Engine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return nil, errNoProject
	}
	return s.engine, nil
}

// do runs f on the station in the simulation, if a project is open.
func (s *Service) do(f func(*engine.Engine)) {
	if e, err := s.current(); err == nil {
		f(e)
	}
}

// try runs f on the station in the simulation, or says that no project is open.
func (s *Service) try(f func(*engine.Engine) error) error {
	e, err := s.current()
	if err != nil {
		return err
	}
	return f(e)
}

// ServiceStartup starts the simulation loop.
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
	s.run()
	return nil
}

// ServiceShutdown closes the project and stops every soft PLC.
func (s *Service) ServiceShutdown() error {
	s.close()
	softPLCs.Close()
	return nil
}

// PLC returns which PLC the station uses, its first one.
func (s *Service) PLC() PLCConfig { return s.PLCFor("") }

// SetPLC switches the station to another PLC and restarts it.
func (s *Service) SetPLC(c PLCConfig) error { return s.SetPLCFor("", c) }

// PLCFor returns how one of the station's PLCs is connected, by its module, the first one
// for "".
func (s *Service) PLCFor(plc string) PLCConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return PLCConfig{Mode: PLCNone}
	}
	if c, ok := s.plcs[s.plcKey(plc)]; ok {
		return c
	}
	return PLCConfig{Mode: PLCNone}
}

// SetPLCFor connects one of the station's PLCs, by its module, and restarts the station.
// Without a connection of its own, the config uses the one the project keeps for the PLC, and
// the project keeps the connections of other kinds it has.
func (s *Service) SetPLCFor(plc string, c PLCConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return errNoProject
	}
	c, err := s.connect(plc, c)
	if err != nil {
		return err
	}
	return s.remember(plc, c)
}

// ConnectPLCFor connects one of the station's PLCs, by its module, as the project means it to
// be connected, and restarts the station, as the PLC's Connect does without its panel.
func (s *Service) ConnectPLCFor(plc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return errNoProject
	}
	planned := s.proj.def.PLCs[s.plcKey(plc)]
	if planned.Mode == "" || planned.Mode == PLCNone {
		return errors.New("choose how the PLC connects first")
	}
	_, err := s.connect(plc, PLCConfig{Mode: planned.Mode})
	return err
}

// PlanPLCFor keeps how the project means one of the station's PLCs, by its module, to be
// connected, without connecting it: the mode Connect then connects in. It gives the PLC's
// config as the project now has it.
func (s *Service) PlanPLCFor(plc, mode string) (PLCConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return PLCConfig{}, errNoProject
	}
	if mode != PLCNone && mode != PLCSoft && mode != PLCExternal {
		return PLCConfig{}, fmt.Errorf("there is no PLC mode %q", mode)
	}
	planned := s.proj.def.PLCs[s.plcKey(plc)]
	planned.Mode = mode
	return planned, s.remember(plc, planned)
}

// SetAutoConnect says whether one of the station's PLCs, by its module, connects as the
// project opens, and gives how the project now means to connect it.
func (s *Service) SetAutoConnect(plc string, on bool) (PLCConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return PLCConfig{}, errNoProject
	}
	planned := s.proj.def.PLCs[s.plcKey(plc)]
	if planned.Mode == "" {
		planned.Mode = PLCNone
	}
	planned.AutoConnect = on
	return planned, s.remember(plc, planned)
}

// connect gives one of the station's PLCs the controller a config says, with the connections
// and the setting the project keeps for it, under s.mu, and gives the config as it now is.
func (s *Service) connect(plc string, c PLCConfig) (PLCConfig, error) {
	kept := s.proj.def.PLCs[s.plcKey(plc)]
	if c.External == nil {
		c.External = kept.External
	}
	c.Others, c.AutoConnect = kept.Others, kept.AutoConnect
	ctl, err := s.controller(plc, c)
	if err != nil {
		return c, err
	}
	if err := s.engine.SetControllerFor(plc, ctl); err != nil {
		return c, err
	}
	s.plcs[s.plcKey(plc)] = c
	return c, nil
}

// autoConnect connects the PLCs the project says to connect as it opens, under s.mu. What
// fails is left not connected, for the PLC's panel to connect again.
func (s *Service) autoConnect() {
	for _, key := range slices.Sorted(maps.Keys(s.proj.def.PLCs)) {
		c := s.proj.def.PLCs[key]
		if !c.AutoConnect || c.Mode == "" || c.Mode == PLCNone {
			continue
		}
		_, _ = s.connect(key, PLCConfig{Mode: c.Mode})
	}
}

// DisconnectPLCFor ends the connection of one of the station's PLCs, by its module, and restarts
// the station without it. The project keeps how it connects the PLC, so it is offered again,
// ready to connect: a controller that cannot be reached is let go of without losing its form.
func (s *Service) DisconnectPLCFor(plc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return errNoProject
	}
	none := PLCConfig{Mode: PLCNone}
	ctl, err := s.controller(plc, none)
	if err != nil {
		return err
	}
	if err := s.engine.SetControllerFor(plc, ctl); err != nil {
		return err
	}
	s.plcs[s.plcKey(plc)] = none
	return nil
}

// plcOf names a PLC of the station in the simulation by the station and its module, the
// first one for "".
func (s *Service) plcOf(plc string) (station, id string, err error) {
	if s.engine == nil {
		return "", "", errNoProject
	}
	ids := s.engine.Plan().PLCs
	switch {
	case plc == "" && len(ids) > 0:
		plc = ids[0]
	case plc == "":
		return "", "", fmt.Errorf("the station has no PLC")
	case !slices.Contains(ids, plc):
		return "", "", fmt.Errorf("the station has no PLC %s", plc)
	}
	return s.bundle.Station.ID, plc, nil
}

// plcKey is how the configs are kept: "" for the station's first PLC.
func (s *Service) plcKey(plc string) string {
	if ids := s.engine.Plan().PLCs; len(ids) > 0 && ids[0] == plc {
		return ""
	}
	return plc
}

func (s *Service) controller(plc string, c PLCConfig) (plcio.Controller, error) {
	switch c.Mode {
	case PLCNone, "":
		return softplc.Idle{}, nil
	case PLCSoft:
		_, id, err := s.plcOf(plc)
		if err != nil {
			return nil, err
		}
		soft := softPLCs.Get(s.proj.key(), id)
		soft.SetSpace(s.space(plc, id))
		if err := soft.Serve(s.ctx); err != nil {
			return nil, err
		}
		resolved, err := opcua.SoftPLC(soft.Endpoint(), id).Resolve(s.signals(plc), nil)
		if err != nil {
			return nil, err
		}
		return link.Start(s.ctx, link.Options{Name: soft.Name(), Connection: resolved, Program: soft.LinkProgram()})
	case PLCExternal:
		if c.External == nil {
			return nil, errors.New("choose the kind of connection first")
		}
		conn := c.External.Connection
		resolved, err := conn.Resolve(s.signals(plc), c.External.Values)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", conn.Name, err)
		}
		return link.Start(s.ctx, link.Options{Name: conn.Name, Connection: resolved})
	}
	return nil, fmt.Errorf("unknown PLC mode %q", c.Mode)
}

// naming is how a PLC of the station names the variables of its image, as its module says.
func (s *Service) naming(plc string) plcio.Naming {
	plan := s.engine.Plan()
	k := slices.Index(plan.PLCs, plc)
	if plc == "" {
		k = 0
	}
	if k < 0 || k >= len(plan.Naming) {
		return plcio.Naming{}
	}
	return plan.Naming[k]
}

// AcknowledgeFellOff clears the warning about workpieces lost off the station.
func (s *Service) AcknowledgeFellOff() { s.do(func(e *engine.Engine) { e.AcknowledgeFellOff() }) }

// SetIOTest cuts the programs of all the station's PLCs off from the I/O, so that their
// outputs are switched by hand, or connects them again.
func (s *Service) SetIOTest(on bool) { s.do(func(e *engine.Engine) { e.SetIOTest(on) }) }

// SetTestOutput switches one output by hand during an I/O test.
func (s *Service) SetTestOutput(variable string, on bool) error {
	return s.try(func(e *engine.Engine) error { return e.SetTestOutput(variable, on) })
}

// ClearTestOutputs switches the outputs of all the station's PLCs off during an I/O test.
func (s *Service) ClearTestOutputs() { s.do(func(e *engine.Engine) { e.ClearTestOutputs() }) }

// SetTestOutputFor switches one output of one of the station's PLCs by hand during the I/O
// test.
func (s *Service) SetTestOutputFor(plc, variable string, on bool) error {
	return s.try(func(e *engine.Engine) error { return e.SetTestOutputFor(plc, variable, on) })
}

// SetTestAnalogFor sets an analogue output of one of the station's PLCs by hand during the
// I/O test, to a value on the wire such as volts.
func (s *Service) SetTestAnalogFor(plc, address string, value float64) error {
	return s.try(func(e *engine.Engine) error { return e.SetTestAnalogFor(plc, address, value) })
}

// Scene returns everything the frontend needs to show the station, an empty one while no
// project is open.
func (s *Service) Scene() model.Scene {
	e, err := s.current()
	if err != nil {
		return model.Scene{}
	}
	return e.Scene()
}

// Snapshot returns the current simulation state, an empty one while no project is open.
func (s *Service) Snapshot() engine.Snapshot {
	e, err := s.current()
	if err != nil {
		return engine.Snapshot{}
	}
	return e.Snapshot()
}

// RunAction triggers a function of a module or the station.
func (s *Service) RunAction(name string, args map[string]string) error {
	return s.try(func(e *engine.Engine) error { return e.RunAction(name, args) })
}

// TakeWorkpiece takes a workpiece off the station by hand, by its id, as someone lifts it away.
// A check that runs has the station to itself, so it is refused then.
func (s *Service) TakeWorkpiece(id int) error {
	s.check.mu.Lock()
	checking := s.check.state.Running
	s.check.mu.Unlock()
	if checking {
		return errors.New("the check is running: workpieces stay where they are until it ends")
	}
	return s.try(func(e *engine.Engine) error { return e.TakeWorkpiece(id) })
}

// SetControl presses or releases a button, or turns a key switch.
func (s *Service) SetControl(name string, on bool) error {
	return s.try(func(e *engine.Engine) error { return e.SetControl(name, on) })
}

// SetFault enables or clears a fault.
func (s *Service) SetFault(name string, on bool) error {
	return s.try(func(e *engine.Engine) error { return e.SetFault(name, on) })
}

// SetParameter changes a station parameter while the station runs, and keeps it in the
// project.
func (s *Service) SetParameter(id string, value float64) error {
	if err := s.try(func(e *engine.Engine) error { return e.SetParameter(id, value) }); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proj.def.Parameters == nil {
		s.proj.def.Parameters = map[string]float64{}
	}
	s.proj.def.Parameters[id] = value
	return s.saveProject()
}

// SetPaused pauses or resumes the simulation.
func (s *Service) SetPaused(paused bool) { s.do(func(e *engine.Engine) { e.SetPaused(paused) }) }

// SetSpeed sets the simulation speed factor.
func (s *Service) SetSpeed(speed float64) { s.do(func(e *engine.Engine) { e.SetSpeed(speed) }) }

// Reset restarts the station and the PLC.
func (s *Service) Reset() { s.do(func(e *engine.Engine) { e.Reset() }) }
