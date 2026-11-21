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

	"github.com/toblu96/taktwin/internal/exercise"
	"github.com/toblu96/taktwin/internal/model"
)

// CheckEvent is emitted with the CheckState each time a line of a running check is done.
const CheckEvent = "sim:check"

// ExerciseInfo is an exercise and the station it is on.
type ExerciseInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Station     string `json:"station"`
	StationName string `json:"stationName"`
	// Collection is the collection it comes from, "" for the user's own.
	Collection string `json:"collection,omitempty"`
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
	if s.bundle == nil {
		s.mu.Unlock()
		return []ExerciseInfo{}
	}
	station, open := s.bundle.Station.ID, s.exercise
	s.mu.Unlock()
	out := []ExerciseInfo{}
	for _, ex := range exercises(Library()) {
		if ex.Station == station {
			ex.Open = len(ex.Changes) == 0 || open == ex.ID
			out = append(out, ex)
		}
	}
	return out
}

// AllExercises lists every exercise of the library, to start a project from.
func (s *Service) AllExercises() []ExerciseInfo { return exercises(Library()) }

func exercises(lib *model.Library) []ExerciseInfo {
	out := []ExerciseInfo{}
	for _, id := range lib.Exercises() {
		if ex, err := lib.Exercise(id); err == nil {
			out = append(out, ExerciseInfo{ID: ex.ID, Name: ex.Name, Description: ex.Description, Station: ex.Station, StationName: cmpOr(lib.StationName(ex.Station), ex.Station),
				Collection: lib.ExerciseCollection(ex.ID), Changes: describeChanges(ex.Changes)})
		}
	}
	return out
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
	if s.engine == nil {
		s.mu.Unlock()
		return errNoProject
	}
	e, station, ctx, open, project := s.engine, s.bundle.Station.ID, s.ctx, s.exercise, s.proj.name
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
		passed := passed(s.check.state)
		s.check.mu.Unlock()
		// A check that was stopped says nothing about the program.
		if run.Err() == nil {
			s.keepCheck(project, LastCheck{Exercise: id, Passed: passed, At: time.Now().UTC().Format(time.RFC3339)})
		}
		stop()
		s.emitCheck()
	}()
	return nil
}

// passed says whether a check that ran to its end passed every line.
func passed(st CheckState) bool {
	if st.Error != "" || len(st.Lines) == 0 {
		return false
	}
	for _, l := range st.Lines {
		if !l.OK {
			return false
		}
	}
	return true
}

// keepCheck keeps how a check went in the project it ran in, if that is still open.
func (s *Service) keepCheck(project string, c LastCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if project == "" || s.proj.name != project {
		return
	}
	s.proj.def.LastCheck = &c
	_ = s.saveProject()
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
