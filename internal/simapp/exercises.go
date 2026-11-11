package simapp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/exercise"
	"github.com/toblu96/taktwin/internal/model"
)

// CheckEvent is emitted with the CheckState each time a line of a running check is done.
const CheckEvent = "sim:check"

// ExerciseInfo is an exercise on the station in the simulation.
type ExerciseInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Changes says how the exercise sets the station up, one line each, and Open whether the
	// station in the simulation is set up so. One without changes is always open.
	Changes []string `json:"changes"`
	Open    bool     `json:"open"`
}

// CheckState is how far the check of an exercise is.
type CheckState struct {
	Exercise string           `json:"exercise"`
	Running  bool             `json:"running"`
	Lines    []exercise.Check `json:"lines"`
	Error    string           `json:"error"`
}

// checking is the check that runs, if any.
type checking struct {
	mu    sync.Mutex
	state CheckState
	stop  context.CancelFunc
}

// Exercises lists the exercises of the station in the simulation.
func (s *Service) Exercises() []ExerciseInfo {
	s.mu.Lock()
	station, open := s.bundle.Station.ID, s.exercise
	s.mu.Unlock()
	lib := Library()
	out := []ExerciseInfo{}
	for _, id := range lib.Exercises() {
		if ex, err := lib.Exercise(id); err == nil && ex.Station == station {
			changes := describeChanges(ex.Changes)
			out = append(out, ExerciseInfo{ID: ex.ID, Name: ex.Name, Description: ex.Description, Changes: changes, Open: len(changes) == 0 || open == ex.ID})
		}
	}
	return out
}

// OpenExercise puts the station of an exercise into the simulation, set up as the exercise
// changes it, with no PLC until one is connected.
func (s *Service) OpenExercise(id string) (model.Scene, error) {
	bundle, _, err := Library().LoadExercise(id)
	if err != nil {
		return model.Scene{}, err
	}
	bundle, e, err := start(bundle)
	if err != nil {
		return model.Scene{}, err
	}
	return s.replace(bundle, e, id), nil
}

// describeChanges says what an exercise changes on its station, one line each.
func describeChanges(c *model.StationChanges) []string {
	out := []string{}
	if c == nil {
		return out
	}
	refs := slices.Sorted(maps.Keys(c.Params))
	for _, ref := range refs {
		keys := slices.Sorted(maps.Keys(c.Params[ref]))
		for _, k := range keys {
			out = append(out, fmt.Sprintf("%s: %s = %s", ref, k, c.Params[ref][k].Source()))
		}
	}
	for _, p := range c.Pins {
		out = append(out, fmt.Sprintf("%s: %s and %s swapped", p.Terminal, p.From, p.To))
	}
	if c.Docks != nil {
		out = append(out, "Buttons of the dock")
	}
	return out
}

// Check returns how far the check of an exercise is.
func (s *Service) Check() CheckState {
	s.check.mu.Lock()
	defer s.check.mu.Unlock()
	return s.checkState()
}

func (s *Service) checkState() CheckState {
	st := s.check.state
	st.Lines = slices.Clone(st.Lines)
	if st.Lines == nil {
		st.Lines = []exercise.Check{}
	}
	return st
}

// StartCheck checks the station against an exercise, from its power-on state with the PLC
// as it is, line by line on the wall clock, as someone with the grading sheet would.
func (s *Service) StartCheck(id string) error {
	ex, err := Library().Exercise(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	e, station, ctx, open := s.engine, s.bundle.Station.ID, s.ctx, s.exercise
	s.mu.Unlock()
	if ex.Station != station {
		return fmt.Errorf("%s is an exercise for the %s station", ex.Name, ex.Station)
	}
	if ex.Changes != nil && open != id {
		return fmt.Errorf("%s sets the station up its own way: open it first", ex.Name)
	}
	s.check.mu.Lock()
	if s.check.state.Running {
		s.check.mu.Unlock()
		return errors.New("a check is running already")
	}
	run, stop := context.WithCancel(ctx)
	s.check.state, s.check.stop = CheckState{Exercise: id, Running: true}, stop
	s.check.mu.Unlock()
	e.Reset()
	s.emitCheck()
	go func() {
		_, err := exercise.Run(e, ex, exercise.Options{RealTime: true, Context: run, Progress: func(c exercise.Check) {
			s.check.mu.Lock()
			s.check.state.Lines = append(s.check.state.Lines, c)
			s.check.mu.Unlock()
			s.emitCheck()
		}})
		s.check.mu.Lock()
		s.check.state.Running = false
		if err != nil {
			s.check.state.Error = err.Error()
		}
		s.check.mu.Unlock()
		stop()
		s.emitCheck()
	}()
	return nil
}

// StopCheck stops a running check.
func (s *Service) StopCheck() {
	s.check.mu.Lock()
	stop := s.check.stop
	s.check.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func (s *Service) emitCheck() {
	s.check.mu.Lock()
	st := s.checkState()
	s.check.mu.Unlock()
	if app := application.Get(); app != nil {
		app.Event.Emit(CheckEvent, st)
	}
}
