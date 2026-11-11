package model

import (
	"fmt"
	"path"
	"sort"
)

// ExerciseFormat is the format of exercise files this build reads.
const ExerciseFormat = 1

// ExerciseDef is the content of exercise.json: a task on a station, and how a run of the
// station is checked against it, as the function check of a grading sheet does. It holds no
// program: the station runs with whatever PLC is connected.
type ExerciseDef struct {
	Format      int    `json:"format"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Station     string `json:"station"`
	// Rules say where each workpiece belongs, by its kind, family or orientation.
	Rules []RuleDef `json:"rules,omitempty"`
	// Conditions are named expressions that steps and other conditions can use by name.
	Conditions map[string]string `json:"conditions,omitempty"`
	// Steps run in order, as the lines of a grading sheet. The first that fails ends the run.
	Steps []StepDef `json:"steps"`
	// Changes set the station up for this exercise: settings of its modules, signals moved to
	// other pins of a terminal, and the buttons of its dock.
	Changes *StationChanges `json:"changes,omitempty"`
}

// RuleDef says where workpieces that match it belong: on the transport EndsOn when they
// come to rest, as a slide, or waiting on WaitsAt, At mm along it or without At at its end,
// up to a workpiece and a half short of it where an end stop holds it, to be taken away by hand. A workpiece matches by Kind, Family and UpsideDown, where
// they are given. Place is how a sheet names that place, for {place} in a line.
type RuleDef struct {
	Kind       string   `json:"kind,omitempty"`
	Family     string   `json:"family,omitempty"`
	UpsideDown *bool    `json:"upsideDown,omitempty"`
	EndsOn     string   `json:"endsOn,omitempty"`
	WaitsAt    string   `json:"waitsAt,omitempty"`
	At         *float64 `json:"at,omitempty"`
	Place      string   `json:"place,omitempty"`
}

// StepDef is one step of an exercise's check: one of the things to do, or a check that
// becomes a Line of the result. In the steps of a Repeat, {kind} is the kind of that round
// and {place} the Place of the rule for it.
//
//   - Place runs a placeWorkpiece action with Kind, and UpsideDown for one turned over.
//   - Press presses a button or key for a moment, and Set turns one to On.
//   - Wait lets that many seconds pass.
//   - Take takes away by hand every workpiece that waits where its rule wants it.
//   - Until checks that an expression holds within Within seconds, and Holds that one keeps
//     holding for For seconds.
//   - Repeat runs Steps once for each of its kinds.
//
// Expressions know the names of the PLC's signals as out.P1 and in.B1, states such as
// gate1.solenoid.position, step (the PLC's step, or -1), lost (workpieces lost off the
// station), hits (how many different things ran into something since the start), the
// exercise's conditions by name, and the functions count("slide1.slide") and
// count("slide1.slide", "red") for how many workpieces, or of a kind, are on a transport,
// at("conveyor.surface", 65) for whether one is within a mm of that point of it,
// loads("conveyor.surface") and loads("conveyor.surface", "carton-large") for how much the
// workpieces on a transport carry, such as cartons on a pallet, and rules() for whether every
// workpiece placed so far is where its rule wants it.
type StepDef struct {
	Line string `json:"line,omitempty"`

	Place      string  `json:"place,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	UpsideDown bool    `json:"upsideDown,omitempty"`
	Press      string  `json:"press,omitempty"`
	Set        string  `json:"set,omitempty"`
	On         bool    `json:"on,omitempty"`
	Wait       float64 `json:"wait,omitempty"`
	Take       bool    `json:"take,omitempty"`

	Until  string  `json:"until,omitempty"`
	Within float64 `json:"within,omitempty"`
	Holds  string  `json:"holds,omitempty"`
	For    float64 `json:"for,omitempty"`

	Repeat []string  `json:"repeat,omitempty"`
	Steps  []StepDef `json:"steps,omitempty"`
}

// Exercises lists the ids of all exercises in the library.
func (l *Library) Exercises() []string {
	ids := l.names("exercises", folders)
	sort.Strings(ids)
	return ids
}

// Exercise reads an exercise.
func (l *Library) Exercise(id string) (*ExerciseDef, error) {
	if !safeID.MatchString(id) {
		return nil, fmt.Errorf("invalid exercise id %q", id)
	}
	ex := &ExerciseDef{}
	if err := l.read(path.Join("exercises", id, "exercise.json"), ex); err != nil {
		return nil, err
	}
	if ex.Format != ExerciseFormat {
		return nil, fmt.Errorf("exercise %s: format %d is not supported, this version reads %d", id, ex.Format, ExerciseFormat)
	}
	if !safeID.MatchString(ex.Station) {
		return nil, fmt.Errorf("exercise %s: invalid station %q", id, ex.Station)
	}
	for _, r := range ex.Rules {
		if (r.EndsOn == "") == (r.WaitsAt == "") {
			return nil, fmt.Errorf("exercise %s: a rule says either where a workpiece ends or where it waits", id)
		}
	}
	return ex, nil
}
