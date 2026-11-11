// Package simapp exposes the engine to the frontend as a Wails service. It holds
// no simulation logic.
package simapp

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/opclink"
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

	check checking
}

// The ways a station can get its PLC.
const (
	// PLCNone is no PLC: every output stays off, unless the I/O test switches it.
	PLCNone = "none"
	// PLCExternal connects to an OPC UA server such as a CODESYS soft PLC.
	PLCExternal = "external"
	// PLCSoft connects over OPC UA to the app's own soft PLC for that PLC of the station,
	// which runs the Structured Text program written in its editor.
	PLCSoft = "soft"
)

// PLCConfig says which PLC the station uses.
type PLCConfig struct {
	Mode string `json:"mode"`
	// Endpoint is the server to connect to, for PLCExternal.
	Endpoint string `json:"endpoint"`
	// Map names an address map of the station, for a program with other addresses.
	Map string `json:"map"`
}

// New loads the library's default station: the one the first collection names, such as the
// app's own reference models.
func New() (*Service, error) {
	station := defaultStation(Library())
	if station == "" {
		return nil, fmt.Errorf("there is no station to show: import a collection (a %s file) in the settings, or put one into %s", model.PackageExtension, filepath.Join(UserFolder(), CollectionsFolder))
	}
	bundle, e, err := load(station)
	if err != nil {
		return nil, err
	}
	return &Service{engine: e, bundle: bundle, ctx: context.Background(), plcs: map[string]PLCConfig{}}, nil
}

func load(station string) (*model.Bundle, *engine.Engine, error) {
	bundle, err := Library().Load(station)
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

// LoadStation replaces the station in the simulation with another one from the catalog,
// DefaultStation is the station the library opens first, "" when it has none.
func (s *Service) DefaultStation() string { return defaultStation(Library()) }

// with no PLC until one is connected. The scene it returns is the new station.
func (s *Service) LoadStation(id string) (model.Scene, error) {
	bundle, e, err := load(id)
	if err != nil {
		return model.Scene{}, err
	}
	return s.replace(bundle, e, ""), nil
}

// replace puts another station into the simulation, with no PLC.
func (s *Service) replace(bundle *model.Bundle, e *engine.Engine, exercise string) model.Scene {
	s.mu.Lock()
	old, stop := s.engine, s.stop
	s.engine, s.bundle, s.plcs, s.exercise = e, bundle, map[string]PLCConfig{}, exercise
	s.run()
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	s.StopCheck()
	old.Close()
	return e.Scene()
}

// run starts the simulation loop of the current station.
func (s *Service) run() {
	ctx, stop := context.WithCancel(s.ctx)
	s.stop = stop
	e := s.engine
	go e.Run(ctx, frame, func(snap engine.Snapshot) {
		if app := application.Get(); app != nil {
			app.Event.Emit(SnapshotEvent, snap)
		}
	})
}

// current returns the engine of the station in the simulation.
func (s *Service) current() *engine.Engine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engine
}

// ServiceStartup starts the simulation loop.
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
	s.run()
	return nil
}

// ServiceShutdown stops the simulation loop.
func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	stop := s.stop
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	s.current().Close()
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
	return cmp.Or(s.plcs[s.plcKey(plc)], PLCConfig{Mode: PLCNone})
}

// SetPLCFor connects one of the station's PLCs, by its module, and restarts the station.
func (s *Service) SetPLCFor(plc string, c PLCConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctl, err := s.controller(plc, c)
	if err != nil {
		return err
	}
	if err := s.engine.SetControllerFor(plc, ctl); err != nil {
		return err
	}
	s.plcs[s.plcKey(plc)] = c
	return nil
}

// plcOf names a PLC of the station in the simulation by the station and its module, the
// first one for "".
func (s *Service) plcOf(plc string) (station, id string, err error) {
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
		station, id, err := s.plcOf(plc)
		if err != nil {
			return nil, err
		}
		soft := softPLCs.Get(station, id)
		naming := s.naming(plc)
		soft.SetNaming(naming)
		if err := soft.Serve(s.ctx); err != nil {
			return nil, err
		}
		return opclink.Start(s.ctx, opclink.Options{Endpoint: soft.Endpoint(), Name: soft.Name(), Naming: naming, Analog: s.analogNames(plc), Bytes: s.imageBytes(plc), Program: soft.LinkProgram()}), nil
	case PLCExternal:
		if !strings.HasPrefix(c.Endpoint, "opc.tcp://") {
			return nil, fmt.Errorf("%q is not an OPC UA address: it starts with opc.tcp://", c.Endpoint)
		}
		var addresses plcio.AddressMap
		if s.plcKey(plc) == "" {
			var err error
			if addresses, err = s.engine.Plan().AddressMap(c.Map); err != nil {
				return nil, err
			}
		}
		return opclink.Start(s.ctx, opclink.Options{Endpoint: c.Endpoint, Name: "OPC UA server", Naming: s.naming(plc), Map: addresses, Analog: s.analogNames(plc), Bytes: s.imageBytes(plc)}), nil
	}
	return nil, fmt.Errorf("unknown PLC mode %q", c.Mode)
}

// analogNames lists the analogue inputs a PLC of the station uses.
func (s *Service) analogNames(plc string) []string {
	scene := s.engine.Plan().Scene
	var names []string
	for _, p := range scene.PLCs {
		if p.Module == plc || plc == "" && p.Module == scene.PLC.Module {
			for _, a := range p.Analog {
				names = append(names, a.Address)
			}
		}
	}
	return names
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

// imageBytes is how many bytes of the image a PLC of the station uses: up to the highest
// byte of its signals.
func (s *Service) imageBytes(plc string) int {
	plan := s.engine.Plan()
	k := slices.Index(plan.PLCs, plc)
	if plc == "" {
		k = 0
	}
	bytes := 0
	for _, list := range [][]model.PLCBitPlan{plan.PLCIn, plan.PLCOut} {
		for _, b := range list {
			if b.PLC == k {
				bytes = max(bytes, int(b.Addr.Byte)+1)
			}
		}
	}
	return bytes
}

// AcknowledgeFellOff clears the warning about workpieces lost off the station.
func (s *Service) AcknowledgeFellOff() { s.current().AcknowledgeFellOff() }

// SetIOTest cuts the program off from the I/O, or connects it again.
func (s *Service) SetIOTest(on bool) { s.current().SetIOTest(on) }

// SetTestOutput switches one output by hand during an I/O test.
func (s *Service) SetTestOutput(variable string, on bool) error {
	return s.current().SetTestOutput(variable, on)
}

// ClearTestOutputs switches all outputs off during an I/O test.
func (s *Service) ClearTestOutputs() { s.current().ClearTestOutputs() }

// SetIOTestFor starts or ends the I/O test of one of the station's PLCs, by its module.
func (s *Service) SetIOTestFor(plc string, on bool) error { return s.current().SetIOTestFor(plc, on) }

// SetTestOutputFor switches one output of one of the station's PLCs by hand during its I/O
// test.
func (s *Service) SetTestOutputFor(plc, variable string, on bool) error {
	return s.current().SetTestOutputFor(plc, variable, on)
}

// ClearTestOutputsFor switches all outputs of one of the station's PLCs off during its I/O
// test.
func (s *Service) ClearTestOutputsFor(plc string) error { return s.current().ClearTestOutputsFor(plc) }

// Scene returns everything the frontend needs to show the station.
func (s *Service) Scene() model.Scene { return s.current().Scene() }

// Snapshot returns the current simulation state.
func (s *Service) Snapshot() engine.Snapshot { return s.current().Snapshot() }

// RunAction triggers a function of a module or the station.
func (s *Service) RunAction(name string, args map[string]string) error {
	return s.current().RunAction(name, args)
}

// SetControl presses or releases a button, or turns a key switch.
func (s *Service) SetControl(name string, on bool) error { return s.current().SetControl(name, on) }

// SetFault enables or clears a fault.
func (s *Service) SetFault(name string, on bool) error { return s.current().SetFault(name, on) }

// SetParameter changes a station parameter while the station runs.
func (s *Service) SetParameter(id string, value float64) error {
	return s.current().SetParameter(id, value)
}

// SetPaused pauses or resumes the simulation.
func (s *Service) SetPaused(paused bool) { s.current().SetPaused(paused) }

// SetSpeed sets the simulation speed factor.
func (s *Service) SetSpeed(speed float64) { s.current().SetSpeed(speed) }

// Reset restarts the station and the PLC.
func (s *Service) Reset() { s.current().Reset() }
