package simapp

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/plcio"
)

// zipNames lists the files of a zip file.
func zipNames(t *testing.T, file string) []string {
	t.Helper()
	z, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	return names
}

// labCollection writes the app's reference collection as another one, lab, of a version.
func labCollection(t *testing.T, version string) string {
	t.Helper()
	return labCollectionWith(t, version, nil)
}

// labCollectionWith writes lab with its files changed by edit.
func labCollectionWith(t *testing.T, version string, edit func(files map[string][]byte)) string {
	t.Helper()
	files := map[string][]byte{}
	err := fs.WalkDir(catalog.Reference, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(catalog.Reference, p)
		files[p] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	files["collection.json"] = []byte(`{"id":"lab","name":"Lab","version":"` + version + `"}`)
	if edit != nil {
		edit(files)
	}
	file := filepath.Join(t.TempDir(), "lab.taktwin")
	if err := writeZipOver(file, files); err != nil {
		t.Fatal(err)
	}
	return file
}

// An exercise is handed out as one file, with its station and exercise, the instructions
// and the collections students may not have, but not the teacher's programs unless asked.
// Students open it where it is and work in it; what they hand in is that file, named as
// they like.
func TestAnExerciseIsHandedOutAsOneFile(t *testing.T) {
	s, home := newService(t)
	if _, err := InstallCollection(labCollection(t, "1.0.0")); err != nil {
		t.Fatal(err)
	}
	station := s.Project().Path
	if _, err := s.NewProject("", Base{Exercise: "load-a-pallet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProgramService(s).Open(""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.proj.dir(), InstructionsFile), []byte("# Load a pallet\n\nWrite the program.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stationFiles, err := s.projectFiles(station)
	if err != nil {
		t.Fatal(err)
	}
	if err := handOut(stationFiles, filepath.Join(t.TempDir(), "x"+ProjectExtension), false); err != nil {
		t.Errorf("a project on a station is handed out too: %v", err)
	}
	files, err := s.projectFiles(s.Project().Path)
	if err != nil {
		t.Fatal(err)
	}
	handout := filepath.Join(t.TempDir(), "Load a pallet"+ProjectExtension)
	if err := handOut(files, handout, false); err != nil {
		t.Fatal(err)
	}
	names := zipNames(t, handout)
	for _, want := range []string{ProjectFile, StationFile, ExerciseFile, InstructionsFile, "collections/lab.taktwin", "original/" + ProjectFile, "original/" + StationFile, "original/" + ExerciseFile} {
		if !slices.Contains(names, want) {
			t.Errorf("the file has no %s: %v", want, names)
		}
	}
	if slices.ContainsFunc(names, func(n string) bool { return strings.Contains(n, "programs/") }) {
		t.Errorf("the teacher's programs are handed out: %v", names)
	}
	if defIn(t, handout).ID == defIn(t, s.Project().Path).ID {
		t.Error("the handout has the teacher's project's id")
	}
	withPrograms := filepath.Join(t.TempDir(), "start"+ProjectExtension)
	if err := handOut(files, withPrograms, true); err != nil {
		t.Fatal(err)
	}
	if names := zipNames(t, withPrograms); !slices.Contains(names, "programs/cabinet/PLC_PRG.st") || !slices.Contains(names, "original/programs/cabinet/PLC_PRG.st") {
		t.Error("the programs are handed out when asked, as the copy's original")
	}
	s.CloseProject()

	// A student without the lab collection opens it all the same: the file carries it.
	if err := NewLibraryService().RemoveCollection("lab"); err != nil {
		t.Fatal(err)
	}
	info, err := s.InspectProjectFile(handout)
	if err != nil {
		t.Fatal(err)
	}
	if info.ExerciseName != "Load a pallet" || info.HasUnsaved || len(info.Missing) != 0 {
		t.Fatalf("inspected: %+v", info)
	}
	opened, err := s.OpenProjectFile(handout, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Name != "Load a pallet" || opened.Path != handout || opened.Exercise != "load-a-pallet" || opened.Problem != "" {
		t.Errorf("opened: %+v", opened)
	}
	if got := s.Instructions(); !strings.Contains(got, "Write the program.") {
		t.Errorf("instructions: %q", got)
	}
	if got := s.Exercises(); len(got) != 1 || !got[0].Open {
		t.Errorf("the exercise is there to check: %+v", got)
	}
	if _, err := NewBuilderService(s).Edit(); err == nil {
		t.Error("the exercise's station is built on")
	}
	if _, ok := installed()["lab"]; ok {
		t.Error("opening the file installed the collection")
	}
	if _, err := NewProgramService(s).Open(""); err != nil {
		t.Fatal(err)
	}
	s.CloseProject()
	if !slices.Contains(zipNames(t, handout), "programs/cabinet/PLC_PRG.st") {
		t.Error("the student's work is in the file")
	}

	// What a student hands in is the file, named after them.
	handIn := filepath.Join(t.TempDir(), "Anna"+ProjectExtension)
	if err := os.Rename(handout, handIn); err != nil {
		t.Fatal(err)
	}
	if p, err := s.OpenProjectFile(handIn, OpenOptions{}); err != nil || p.Name != "Anna" || p.Exercise != "load-a-pallet" {
		t.Errorf("handed in: %+v %v", p, err)
	}

	// A collection that is neither here nor in the file is missing.
	raw, _ := projectJSON(ProjectDef{ID: newID(), Base: Base{Station: "pallet-loading"}, Collections: []Requirement{{Collection: "nope", Name: "Nope", Version: "2.0.0"}}})
	odd := filepath.Join(home, "odd"+ProjectExtension)
	if err := writeZipOver(odd, map[string][]byte{ProjectFile: raw}); err != nil {
		t.Fatal(err)
	}
	if info, _ := s.InspectProjectFile(odd); len(info.Missing) != 1 || info.Missing[0].Collection != "nope" {
		t.Errorf("missing: %+v", info)
	}
}

func TestNewerVersions(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{{"1.10.0", "1.9.2", true}, {"1.0.0", "1.0.0", false}, {"1.0", "1.0.1", false}, {"2", "", true}, {"", "1.0.0", false}} {
		if newer(c.a, c.b) != c.newer {
			t.Errorf("%s after %s: %v", c.a, c.b, !c.newer)
		}
	}
}

// An exercise knows when it or its station was changed since its original, such as since
// it was handed out, and putting everything back makes it as it was.
func TestAnExerciseKnowsWhenItWasChanged(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.NewProject("", Base{Exercise: "load-a-pallet"}); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.ExerciseChanged {
		t.Fatalf("a new exercise is changed: %+v", p)
	}
	if def := s.proj.def; def.ExerciseHash == "" {
		t.Fatal("no hash of the exercise")
	}
	file := filepath.Join(s.proj.dir(), ExerciseFile)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(data), `"name": "Load a pallet"`, `"name": "Easy"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); !p.ExerciseChanged {
		t.Error("the changed exercise is not noticed")
	}
	if _, err := s.PutBack(PartEverything, ""); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.ExerciseChanged {
		t.Error("put back, it is still changed")
	}
	// A project on a station has nothing to compare.
	if _, err := s.NewProject("", Base{Station: "pallet-loading"}); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.ExerciseChanged || s.proj.def.ExerciseHash != "" {
		t.Errorf("a station's project: %+v", p)
	}
}

// A teacher in expert mode checks a hand-in whose exercise was changed with the exercise as
// their own collections have it, and the result says so; the hand-in's file stays as it was.
// Without expert mode, or with copies as the teacher's, the project's own are used.
func TestATeacherChecksWithTheirOwnExercise(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.NewProject("", Base{Exercise: "load-a-pallet"}); err != nil {
		t.Fatal(err)
	}
	check := func() CheckState {
		t.Helper()
		if err := s.StartCheck("load-a-pallet"); err != nil {
			t.Fatal(err)
		}
		st := s.Check()
		s.StopCheck()
		waitFor(t, "the check to stop", func() bool { return !s.Check().Running })
		return st
	}
	if err := (SettingsService{}).SetExpertMode(true); err != nil {
		t.Fatal(err)
	}
	if st := check(); st.Note != "" {
		t.Errorf("the copies are the teacher's: %+v", st)
	}
	file := filepath.Join(s.proj.dir(), ExerciseFile)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	easy := strings.Replace(string(data), `"name": "Load a pallet"`, `"name": "Easy"`, 1)
	if err := os.WriteFile(file, []byte(easy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (SettingsService{}).SetExpertMode(false); err != nil {
		t.Fatal(err)
	}
	if st := check(); st.Note != "" {
		t.Errorf("a student checks with the project's own: %+v", st)
	}
	if ex, _ := s.library().Exercise("load-a-pallet"); ex.Name != "Easy" {
		t.Errorf("the project's own: %s", ex.Name)
	}
	if err := (SettingsService{}).SetExpertMode(true); err != nil {
		t.Fatal(err)
	}
	// The PLC stays connected while the teacher's station loads.
	t.Cleanup(softPLCs.Close)
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCSoft}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.SetPLCFor("", PLCConfig{Mode: PLCNone}) }()
	st := check()
	if got := s.PLCFor(""); got.Mode != PLCSoft {
		t.Errorf("the PLC was let go of: %+v", got)
	}
	waitFor(t, "the soft PLC", func() bool { l := linkOf(s); return l != nil && l.State == plcio.LinkConnected })
	if !strings.Contains(st.Note, "exercise") {
		t.Errorf("the result does not say so: %+v", st)
	}
	if ex, _ := s.library().Exercise("load-a-pallet"); ex.Name != "Load a pallet" {
		t.Errorf("checked with: %s", ex.Name)
	}
	if got, _ := os.ReadFile(file); string(got) != easy {
		t.Error("the hand-in's exercise was changed")
	}
}
