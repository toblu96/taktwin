package simapp

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The app starts with no project: there is no station in the simulation, and nothing to
// connect, check or program until one is opened.
func TestTheAppStartsWithNoProject(t *testing.T) {
	t.Setenv("TAKTWIN_HOME", t.TempDir())
	t.Setenv("TAKTWIN_CONFIG", t.TempDir())
	t.Setenv("TAKTWIN_COLLECTIONS", "")
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Name != "" {
		t.Errorf("a project is open: %+v", p)
	}
	if scene := s.Scene(); scene.ID != "" {
		t.Errorf("a station is in the simulation: %s", scene.ID)
	}
	if err := s.SetPLCFor("", PLCConfig{Mode: PLCSoft}); err == nil {
		t.Error("a PLC is connected with no project")
	}
	if err := s.StartCheck("load-a-pallet"); err == nil {
		t.Error("an exercise is checked with no project")
	}
	if got := s.Exercises(); len(got) != 0 {
		t.Errorf("exercises: %+v", got)
	}
	if _, err := NewProgramService(s).Open(""); err == nil {
		t.Error("a program is opened with no project")
	}
	s.SetPaused(true)
	s.CloseProject()

	examples := s.Examples()
	if len(examples) != 1 || examples[0].Station != "pallet-loading" || examples[0].StationName != "Pallet loading" {
		t.Errorf("the reference station is the example: %+v", examples)
	}
	all := s.AllExercises()
	if len(all) != 1 || all[0].ID != "load-a-pallet" || all[0].Station != "pallet-loading" || all[0].Collection != "taktwin" {
		t.Errorf("every exercise is offered: %+v", all)
	}
}

// A project keeps what is changed in it: how the PLC is connected, without connecting it
// again, and the station's settings. Closing it disconnects the PLC and stops its soft PLC.
func TestAProjectKeepsItsWork(t *testing.T) {
	s, home := newService(t)
	p := s.Project()
	if p.Name != "Test" || p.Station != "pallet-loading" || p.StationName != "Pallet loading" || p.Folder != filepath.Join(home, ProjectsFolder, "Test") || p.Problem != "" {
		t.Fatalf("project: %+v", p)
	}
	// The reference station has no settings of its own; the engine keeps one by any id.
	param := struct{ ID string }{"speed"}
	value := 0.75
	if err := s.SetParameter(param.ID, value); err != nil {
		t.Fatal(err)
	}
	config := PLCConfig{Mode: PLCExternal, Connection: "opcua", Values: map[string]any{"endpoint": "opc.tcp://127.0.0.1:1"}}
	if err := s.SetPLCFor("", config); err != nil {
		t.Fatal(err)
	}
	prog := NewProgramService(s)
	if _, err := prog.Open(""); err != nil {
		t.Fatal(err)
	}
	if res, err := prog.Download(""); err != nil || !res.OK {
		t.Fatalf("the template downloads: %+v %v", res, err)
	}

	s.CloseProject()
	if p := s.Project(); p.Name != "" || s.Scene().ID != "" {
		t.Fatalf("closed: %+v", p)
	}
	if _, err := s.OpenProject("Test"); err != nil {
		t.Fatal(err)
	}
	if got := s.PlannedPLC(""); got.Mode != PLCExternal || got.Connection != "opcua" || got.Values["endpoint"] != "opc.tcp://127.0.0.1:1" {
		t.Errorf("the PLC's connection is kept: %+v", got)
	}
	if got := s.PLCFor(""); got.Mode != PLCNone {
		t.Errorf("but not connected again: %+v", got)
	}
	if got := s.Snapshot().Parameters[param.ID]; got != value {
		t.Errorf("the setting is %v, not %v", got, value)
	}
	if st, err := prog.Status(""); err != nil || st.Loaded {
		t.Errorf("the soft PLC stopped with the project and starts empty: %+v %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(home, ProjectsFolder, "Test", "programs", "cabinet", "PLC_PRG.st")); err != nil {
		t.Errorf("the program is kept: %v", err)
	}
}

// A new project is named after its station or exercise unless it is given a name; projects
// that are not open can be renamed, duplicated and deleted.
func TestProjectsAreMadeAndManaged(t *testing.T) {
	s, home := newService(t)
	for _, bad := range []string{".hidden", "a/b", "x:y", strings.Repeat("x", 81)} {
		if _, err := s.NewProject(bad, Base{Station: "pallet-loading"}); err == nil {
			t.Errorf("%q is no name", bad)
		}
	}
	if _, err := s.NewProject("Test", Base{Station: "pallet-loading"}); err == nil {
		t.Error("a name that is taken")
	}
	if _, err := s.NewProject("", Base{Station: "nope"}); err == nil {
		t.Error("a station that does not exist")
	}
	if _, err := s.NewProject("", Base{Station: "pallet-loading"}); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Name != "Pallet loading" {
		t.Errorf("named after the station: %+v", p)
	}
	if _, err := s.NewProject("", Base{Station: "pallet-loading"}); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Name != "Pallet loading (2)" {
		t.Errorf("with a number where the name is taken: %+v", p)
	}
	if _, err := s.NewProject("", Base{Exercise: "load-a-pallet"}); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Name != "Load a pallet" || p.Exercise != "load-a-pallet" || p.ExerciseName != "Load a pallet" || p.Station != "pallet-loading" {
		t.Errorf("named after the exercise, on its station: %+v", p)
	}
	if got := s.Exercises(); len(got) != 1 || !got[0].Open {
		t.Errorf("the exercise is set up: %+v", got)
	}

	if err := s.DeleteProject("Load a pallet"); err == nil {
		t.Error("the open project cannot be deleted")
	}
	if _, err := s.RenameProject("Load a pallet", "Mine"); err == nil {
		t.Error("the open project cannot be renamed")
	}
	if _, err := s.RenameProject("Test", "Pallet loading"); err == nil {
		t.Error("renamed onto another project")
	}
	renamed, err := s.RenameProject("Test", "Line 1")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Line 1" || !exists(filepath.Join(home, ProjectsFolder, "Line 1", ProjectFile)) || exists(filepath.Join(home, ProjectsFolder, "Test")) {
		t.Errorf("renamed: %+v", renamed)
	}
	if def, _, _ := readProject(projectDir("Line 1")); def.Name != "Line 1" {
		t.Errorf("its file has the new name: %+v", def)
	}
	copied, err := s.DuplicateProject("Line 1")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Name != "Line 1 (copy)" || copied.Station != "pallet-loading" {
		t.Errorf("duplicated: %+v", copied)
	}
	if err := s.DeleteProject("Pallet loading (2)"); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range s.Projects() {
		names = append(names, p.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"Line 1", "Line 1 (copy)", "Load a pallet", "Pallet loading"}) {
		t.Errorf("projects: %v", names)
	}
	if _, err := s.OpenProject("Pallet loading (2)"); err == nil {
		t.Error("it is gone")
	}

	// A program is saved back into the project it was opened from, which must exist.
	prog := NewProgramService(s)
	opened, err := prog.Open("")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Project != "Load a pallet" || !strings.HasPrefix(opened.Folder, projectDir("Load a pallet")) {
		t.Errorf("the program is the project's: %+v", opened)
	}
	if err := prog.SaveFile(opened.Project, "cabinet", "Extra.st", "PROGRAM Extra\nEND_PROGRAM\n"); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(projectDir("Load a pallet"), "programs", "cabinet", "Extra.st")) {
		t.Error("saved into the project it came from")
	}
	for _, name := range []string{"../x", "a/b", "", "Test", ".unsaved/pallet-loading"} {
		if err := prog.SaveFile(name, "cabinet", "X.st", ""); err == nil {
			t.Errorf("no project %q", name)
		}
	}

	// A project whose station is gone is listed with the problem, and does not open.
	if err := writeProject(projectDir("Gone"), ProjectDef{Base: Base{Station: "gone"}}); err != nil {
		t.Fatal(err)
	}
	for _, p := range s.Projects() {
		if p.Name == "Gone" && p.Problem == "" {
			t.Errorf("no problem: %+v", p)
		}
	}
	if _, err := s.OpenProject("Gone"); err == nil {
		t.Error("it opens")
	}
	if p := s.Project(); p.Name != "Load a pallet" {
		t.Errorf("the open project stays open: %+v", p)
	}
}

// A project travels as a file and comes back as a project of its own.
func TestAProjectIsExportedAndImported(t *testing.T) {
	s, home := newService(t)
	if _, err := NewProgramService(s).Open(""); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "Test"+ProjectExtension)
	if err := ExportProjectTo(s.Project().Folder, file); err != nil {
		t.Fatal(err)
	}
	name, err := ImportProjectFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Test (2)" {
		t.Errorf("a name that is taken gets a number: %s", name)
	}
	if _, err := os.Stat(filepath.Join(home, ProjectsFolder, name, "programs", "cabinet", "PLC_PRG.st")); err != nil {
		t.Error(err)
	}
	if _, err := s.OpenProject(name); err != nil {
		t.Fatal(err)
	}

	bad := func(files map[string]string) string {
		path := filepath.Join(t.TempDir(), "bad"+ProjectExtension)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		z := zip.NewWriter(f)
		for n, content := range files {
			w, _ := z.Create(n)
			_, _ = w.Write([]byte(content))
		}
		z.Close()
		f.Close()
		return path
	}
	for what, files := range map[string]map[string]string{
		"a way out":      {ProjectFile: `{"format":1,"base":{"station":"pallet-loading"}}`, "../evil.st": "x"},
		"something else": {ProjectFile: `{"format":1,"base":{"station":"pallet-loading"}}`, "run.sh": "x"},
		"no project":     {"programs/cabinet/PLC_PRG.st": "x"},
		"a newer format": {ProjectFile: `{"format":99}`},
	} {
		if _, err := ImportProjectFile(bad(files)); err == nil {
			t.Errorf("%s is imported", what)
		}
	}
	if _, err := ImportProjectFile(file + ".missing"); err == nil {
		t.Error("no file")
	}
}
