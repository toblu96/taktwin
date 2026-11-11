package simapp

import (
	"strings"
	"testing"
)

// The reference station comes with an example program, which the programming tool offers and
// copies into the project, where it builds.
func TestTheExampleProgramOfTheReferenceStation(t *testing.T) {
	t.Setenv("TAKTWIN_HOME", t.TempDir())
	t.Setenv("TAKTWIN_CONFIG", t.TempDir())
	t.Setenv("TAKTWIN_COLLECTIONS", "")
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if s.bundle.Station.ID != "pallet-loading" {
		t.Fatalf("the app opens on its own reference station, not %s", s.bundle.Station.ID)
	}
	p := NewProgramService(s)
	proj, err := p.Open("")
	if err != nil {
		t.Fatal(err)
	}
	if proj.PLC != "cabinet" || len(proj.Example) != 1 || proj.Example[0] != "StepChain.st" {
		t.Fatalf("the project offers the example: %+v", proj)
	}
	if res, err := p.Build(""); err != nil || !res.OK {
		t.Fatalf("the template builds: %+v %v", res, err)
	}
	written, err := p.LoadExample("")
	if err != nil || len(written) != 1 {
		t.Fatalf("loading the example: %v %v", written, err)
	}
	src, err := p.ReadFile("", "StepChain.st")
	if err != nil || !strings.Contains(src, "two layers of four") {
		t.Fatalf("the step chain is the example's: %v", err)
	}
	if res, err := p.Build(""); err != nil || !res.OK {
		t.Fatalf("the example builds: %+v %v", res, err)
	}
}
