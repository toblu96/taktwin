package simapp

import (
	"testing"
)

// A workpiece put onto the station can be taken off again by hand, by its id, and then it is
// gone; one that is not there cannot.
func TestAWorkpieceIsTakenOffByHand(t *testing.T) {
	s, _ := newService(t)
	if err := s.RunAction("cartons.put", map[string]string{"kind": "carton-large"}); err != nil {
		t.Fatal(err)
	}
	var id int
	waitFor(t, "the carton on the belt", func() bool {
		for _, w := range s.Snapshot().Workpieces {
			if w.Kind == "carton-large" {
				id = w.ID
				return true
			}
		}
		return false
	})
	if err := s.TakeWorkpiece(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the carton gone", func() bool {
		for _, w := range s.Snapshot().Workpieces {
			if w.ID == id {
				return false
			}
		}
		return true
	})
	if err := s.TakeWorkpiece(id); err == nil {
		t.Error("a workpiece that is gone cannot be taken again")
	}
}

// While a check runs, it has the station to itself: no workpiece is taken by hand.
func TestNoWorkpieceIsTakenDuringACheck(t *testing.T) {
	s, _ := newService(t)
	s.check.mu.Lock()
	s.check.state.Running = true
	s.check.mu.Unlock()
	t.Cleanup(func() {
		s.check.mu.Lock()
		s.check.state.Running = false
		s.check.mu.Unlock()
	})
	if err := s.TakeWorkpiece(1); err == nil {
		t.Error("it is refused while the check runs")
	}
}
