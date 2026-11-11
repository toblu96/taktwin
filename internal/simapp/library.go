package simapp

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/softplc"
)

// LibraryEvent is emitted once per frame with the preview's engine.Snapshot.
const LibraryEvent = "library:snapshot"

// LibraryService browses the catalog and runs one entry at a time on its own engine,
// without a PLC, so that it can be looked at and tried out. The station in the
// simulation is not affected.
type LibraryService struct {
	mu     sync.Mutex
	lib    *model.Library
	ctx    context.Context
	emit   func(engine.Snapshot)
	bundle *model.Bundle
	engine *engine.Engine
	stop   context.CancelFunc
}

// NewLibraryService browses the built-in catalog and the user's own stations.
func NewLibraryService() *LibraryService {
	return &LibraryService{
		lib: Library(),
		ctx: context.Background(),
		emit: func(snap engine.Snapshot) {
			application.Get().Event.Emit(LibraryEvent, snap)
		},
	}
}

// ServiceStartup keeps the context that ends previews with the app.
func (s *LibraryService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.ctx = ctx
	return nil
}

// ServiceShutdown stops the preview.
func (s *LibraryService) ServiceShutdown() error {
	s.Close()
	return nil
}

// Entries lists the catalog and the user's own stations, reading the user folder again.
func (s *LibraryService) Entries() []model.Entry {
	lib := Library()
	s.mu.Lock()
	s.lib = lib
	s.mu.Unlock()
	return lib.Entries()
}

// Open starts previewing an entry and returns its scene.
func (s *LibraryService) Open(level, id string) (model.Scene, error) {
	bundle, err := s.lib.Preview(level, id)
	if err != nil {
		return model.Scene{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.start(bundle)
}

// start runs a new engine for the bundle. If that fails, the old one keeps running.
func (s *LibraryService) start(bundle *model.Bundle) (model.Scene, error) {
	e, err := engine.New(bundle, softplc.Idle{})
	if err != nil {
		return model.Scene{}, err
	}
	s.halt()
	ctx, stop := context.WithCancel(s.ctx)
	s.bundle, s.engine, s.stop = bundle, e, stop
	go e.Run(ctx, frame, s.emit)
	return e.Scene(), nil
}

func (s *LibraryService) halt() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	s.engine = nil
}

// Close stops the preview.
func (s *LibraryService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.halt()
}

// running returns the engine of the open preview.
func (s *LibraryService) running() (*engine.Engine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return nil, errors.New("nothing is open in the library")
	}
	return s.engine, nil
}

// SetParameter changes a parameter. This can change the shape of the module, so it
// restarts the preview and returns the new scene.
func (s *LibraryService) SetParameter(id string, value float64) (model.Scene, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bundle == nil {
		return model.Scene{}, errors.New("nothing is open in the library")
	}
	for i := range s.bundle.Station.Parameters {
		p := &s.bundle.Station.Parameters[i]
		if p.ID != id {
			continue
		}
		old := p.Default
		p.Default = value
		scene, err := s.start(s.bundle)
		if err != nil {
			p.Default = old
		}
		return scene, err
	}
	return model.Scene{}, errors.New("unknown parameter " + id)
}

// SetPin holds a signal on or off.
func (s *LibraryService) SetPin(state int, on bool) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.SetPin(state, on)
}

// SetControl presses or releases a button, or turns a key switch.
func (s *LibraryService) SetControl(name string, on bool) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.SetControl(name, on)
}

// SetFault enables or clears a fault.
func (s *LibraryService) SetFault(name string, on bool) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.SetFault(name, on)
}

// RunAction triggers a function of the module, or places a test workpiece.
func (s *LibraryService) RunAction(name string, args map[string]string) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.RunAction(name, args)
}

// Reset puts everything back to how it started.
func (s *LibraryService) Reset() error {
	e, err := s.running()
	if err != nil {
		return err
	}
	e.Reset()
	return nil
}
