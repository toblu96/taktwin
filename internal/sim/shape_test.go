package sim

import (
	"math"
	"testing"

	"github.com/toblu96/taktwin/internal/model"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestFeet(t *testing.T) {
	// A pallet 1200 long and 800 wide, heading along x, and one turned to y.
	pallet := rect(model.Point{}, 1200, 800, model.Point{X: 1})
	turnedPallet := rect(model.Point{}, 1200, 800, model.Point{Y: 2})
	if !pallet.contains(590, 390, 0) || pallet.contains(610, 0, 0) || pallet.contains(0, 410, 0) {
		t.Error("a pallet covers 1200 by 800")
	}
	if !turnedPallet.contains(390, 590, 0) || turnedPallet.contains(410, 0, 0) {
		t.Error("turned, it covers 800 by 1200")
	}
	if !pallet.middle(290, 190) || pallet.middle(310, 0) {
		t.Error("its middle is half its length and width")
	}
	// Two pallets in a row along a conveyor, 100 mm apart, and touching across.
	next := rect(model.Point{X: 1300}, 1200, 800, model.Point{X: 1})
	if g := pallet.gapTo(next); !near(g, 100) {
		t.Errorf("gap along %v", g)
	}
	beside := rect(model.Point{Y: 800}, 1200, 800, model.Point{X: 1})
	if g := pallet.gapTo(beside); !near(g, 0) {
		t.Errorf("gap across %v", g)
	}
	overlapping := rect(model.Point{X: 1100}, 1200, 800, model.Point{X: 1})
	if g := pallet.gapTo(overlapping); !near(g, -100) {
		t.Errorf("overlap %v", g)
	}
	// Diagonal corners: the sides keep them apart.
	diagonal := rect(model.Point{X: 1300, Y: 900}, 1200, 800, model.Point{X: 1})
	if g := pallet.gapTo(diagonal); !near(g, 100) {
		t.Errorf("diagonal %v", g)
	}
	// A round workpiece keeps its old measures, also against a rectangle.
	a, b := round(model.Point{}, 20), round(model.Point{X: 50}, 20)
	if g := a.gapTo(b); !near(g, 10) {
		t.Errorf("round %v", g)
	}
	if g := round(model.Point{X: 650}, 20).gapTo(pallet); !near(g, 30) {
		t.Errorf("round to pallet %v", g)
	}
	if g := round(model.Point{X: 100}, 20).gapTo(pallet); !near(g, -20) {
		t.Errorf("round inside a pallet %v", g)
	}
	// A beam across the conveyor meets the pallet where its side is.
	if at, ok := pallet.enter(0, -1000, 0, 1, 2000); !ok || !near(at, 600) {
		t.Errorf("beam enters at %v %v", at, ok)
	}
	if _, ok := pallet.enter(700, -1000, 0, 1, 2000); ok {
		t.Error("a beam beside it misses")
	}
	if _, ok := pallet.enter(0, -1000, 0, 1, 500); ok {
		t.Error("a beam too short misses")
	}
	if at, ok := pallet.enter(0, 0, 1, 0, 2000); !ok || at != 0 {
		t.Errorf("a beam from inside meets it at once: %v %v", at, ok)
	}
	if s := turnedPallet.support(1, 0); !near(s, 400) {
		t.Errorf("support %v", s)
	}
	box := boxFoot(model.Point{X: -100, Y: -50}, model.Point{X: 200}, model.Point{Y: 100})
	if !box.contains(99, 49, 0) || box.contains(101, 0, 0) || !near(box.extent(), math.Hypot(100, 50)) {
		t.Errorf("box foot %+v", box)
	}
}
