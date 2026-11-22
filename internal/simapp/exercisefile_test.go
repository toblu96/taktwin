package simapp

import (
	"archive/zip"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/toblu96/taktwin/catalog"
)

// zipped lists the files of a zip file.
func zipped(t *testing.T, file string) []string {
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
	file := filepath.Join(t.TempDir(), "lab.taktwin")
	if err := writeZip(file, files); err != nil {
		t.Fatal(err)
	}
	return file
}

// An exercise is handed out as one file, with its station and exercise, the instructions
// and the collections students may not have, but not the teacher's programs unless asked.
// Students open it as a project of their own and go on with it when they open it again; what
// they hand in comes back as a project named after its file.
func TestAnExerciseIsHandedOutAsOneFile(t *testing.T) {
	s, home := newService(t)
	if _, err := InstallCollection(labCollection(t, "1.0.0")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewProject("", Base{Exercise: "load-a-pallet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProgramService(s).Open(""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir("Load a pallet"), InstructionsFile), []byte("# Load a pallet\n\nWrite the program.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExportExerciseTo("Test", filepath.Join(t.TempDir(), "x"+ProjectExtension), false); err == nil {
		t.Error("a project on a station is handed out")
	}
	handout := filepath.Join(t.TempDir(), "Load a pallet"+ProjectExtension)
	if err := ExportExerciseTo("Load a pallet", handout, false); err != nil {
		t.Fatal(err)
	}
	names := zipped(t, handout)
	for _, want := range []string{ProjectFile, StationFile, ExerciseFile, InstructionsFile, "collections/lab.taktwin"} {
		if !slices.Contains(names, want) {
			t.Errorf("the file has no %s: %v", want, names)
		}
	}
	if slices.ContainsFunc(names, func(n string) bool { return strings.HasPrefix(n, "programs/") }) {
		t.Errorf("the teacher's programs are handed out: %v", names)
	}
	withPrograms := filepath.Join(t.TempDir(), "start"+ProjectExtension)
	if err := ExportExerciseTo("Load a pallet", withPrograms, true); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(zipped(t, withPrograms), "programs/cabinet/PLC_PRG.st") {
		t.Error("the programs are handed out when asked")
	}
	s.CloseProject()

	// A student without the lab collection is asked to install it.
	if err := NewLibraryService().RemoveCollection("lab"); err != nil {
		t.Fatal(err)
	}
	info, err := s.InspectProjectFile(handout)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Handout || info.ExerciseName != "Load a pallet" || info.Existing != "" || len(info.Missing) != 0 ||
		len(info.Installs) != 1 || info.Installs[0].Collection != "lab" || info.Installs[0].Version != "1.0.0" {
		t.Fatalf("inspected: %+v", info)
	}
	opened, err := s.OpenProjectFile(handout, OpenOptions{Install: true})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Name != "Load a pallet (2)" || opened.Exercise != "load-a-pallet" || !opened.Own || opened.Problem != "" {
		t.Errorf("opened: %+v", opened)
	}
	if got := s.Instructions(); !strings.Contains(got, "Write the program.") {
		t.Errorf("instructions: %q", got)
	}
	if got := s.Exercises(); len(got) != 1 || !got[0].Open {
		t.Errorf("the exercise is there to check: %+v", got)
	}
	if _, err := s.CustomizeStation(); err == nil {
		t.Error("the exercise's station is changed")
	}
	if info, _ := s.InspectProjectFile(handout); len(info.Installs) != 0 {
		t.Errorf("the collection is installed: %+v", info.Installs)
	}
	s.CloseProject()

	// Opening it again goes on with that project.
	info, _ = s.InspectProjectFile(handout)
	if info.Existing != "Load a pallet (2)" {
		t.Fatalf("existing: %+v", info)
	}
	if p, err := s.OpenProjectFile(handout, OpenOptions{Continue: true}); err != nil || p.Name != "Load a pallet (2)" {
		t.Fatalf("continued: %+v %v", p, err)
	}
	s.CloseProject()
	if p, err := s.OpenProjectFile(handout, OpenOptions{}); err != nil || p.Name != "Load a pallet (3)" {
		t.Fatalf("started again: %+v %v", p, err)
	}
	s.CloseProject()

	// What a student hands in is named after its file, and is no handout.
	handIn := filepath.Join(t.TempDir(), "Anna"+ProjectExtension)
	if err := ExportProjectTo(projectDir("Load a pallet (2)"), handIn); err != nil {
		t.Fatal(err)
	}
	if info, _ := s.InspectProjectFile(handIn); info.Handout || info.Existing != "" || info.Name != "Anna" {
		t.Errorf("hand-in: %+v", info)
	}
	if p, err := s.OpenProjectFile(handIn, OpenOptions{}); err != nil || p.Name != "Anna" || p.Exercise != "load-a-pallet" {
		t.Errorf("handed in: %+v %v", p, err)
	}
	if def, _, _ := readProject(projectDir("Anna")); def.FromHandout || def.Handout {
		t.Errorf("a hand-in is no handout: %+v", def)
	}

	// A collection that is neither here nor in the file is missing.
	def := ProjectDef{Format: ProjectFormat, Name: "Odd", Base: Base{Station: "pallet-loading"}, Handout: true, Requires: []Requirement{{Collection: "nope", Name: "Nope", Version: "2.0.0"}}}
	raw, _ := json.Marshal(def)
	odd := filepath.Join(home, "odd"+ProjectExtension)
	if err := writeZip(odd, map[string][]byte{ProjectFile: raw}); err != nil {
		t.Fatal(err)
	}
	if info, _ := s.InspectProjectFile(odd); len(info.Missing) != 1 || info.Missing[0].Collection != "nope" {
		t.Errorf("missing: %+v", info)
	}
}

func TestNewerVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		newer bool
	}{{"1.10.0", "1.9.2", true}, {"1.0.0", "1.0.0", false}, {"1.0", "1.0.1", false}, {"2", "", true}, {"", "1.0.0", false}} {
		if newer(c.a, c.b) != c.newer {
			t.Errorf("%s after %s: %v", c.a, c.b, !c.newer)
		}
	}
}
