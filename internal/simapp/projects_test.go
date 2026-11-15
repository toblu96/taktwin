package simapp

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The app starts on the unsaved work on its station, which takes over the programs the
// station had before projects, and keeps what is changed: how the PLC is connected, without
// connecting it again, and the station's settings.
func TestUnsavedWorkIsKept(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TAKTWIN_HOME", home)
	t.Setenv("TAKTWIN_CONFIG", t.TempDir())
	t.Setenv("TAKTWIN_COLLECTIONS", "")
	old := filepath.Join(home, "programs", "pallet-loading", "cabinet")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "Mine.st"), []byte("PROGRAM Mine\nEND_PROGRAM\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.current().Close() })
	p := s.Project()
	if p.Saved || p.Station != "pallet-loading" || p.StationName == "" {
		t.Fatalf("project: %+v", p)
	}
	unsaved := filepath.Join(home, ProjectsFolder, unsavedFolder, "pallet-loading")
	if _, err := os.Stat(filepath.Join(unsaved, "programs", "cabinet", "Mine.st")); err != nil {
		t.Errorf("the station's programs are taken over: %v", err)
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
	_ = s.SetPLCFor("", PLCConfig{Mode: PLCNone})
	_ = s.SetPLCFor("", config)

	again, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { again.current().Close() })
	if got := again.PlannedPLC(""); got.Mode != PLCExternal || got.Connection != "opcua" || got.Values["endpoint"] != "opc.tcp://127.0.0.1:1" {
		t.Errorf("the PLC's connection is kept: %+v", got)
	}
	if got := again.PLCFor(""); got.Mode != PLCNone {
		t.Errorf("but not connected again: %+v", got)
	}
	if got := again.Snapshot().Parameters[param.ID]; got != value {
		t.Errorf("the setting is %v, not %v", got, value)
	}
}

// Save as keeps the work under a name, with its programs; the soft PLC's programs are then
// the project's, and opening another station leaves it for that station's unsaved work.
func TestAProjectIsSavedUnderAName(t *testing.T) {
	s, home := newService(t)
	prog := NewProgramService(s)
	if _, err := prog.Open(""); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", ".hidden", "a/b", "x:y", strings.Repeat("x", 81)} {
		if _, err := s.SaveProjectAs(bad); err == nil {
			t.Errorf("%q is no name", bad)
		}
	}
	info, err := s.SaveProjectAs("Line 1")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ProjectsFolder, "Line 1")
	if !info.Saved || info.Name != "Line 1" || info.Folder != dir {
		t.Errorf("saved: %+v", info)
	}
	if _, err := os.Stat(filepath.Join(dir, "programs", "cabinet", "PLC_PRG.st")); err != nil {
		t.Errorf("the programs are in the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ProjectsFolder, unsavedFolder, "pallet-loading")); !os.IsNotExist(err) {
		t.Errorf("the unsaved work is gone: %v", err)
	}
	if _, err := s.SaveProjectAs("Line 1"); err == nil {
		t.Error("a name that is taken")
	}

	// The programming tool works in the project now, and saves back to where it opened.
	opened, err := prog.Open("")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Project != "Line 1" || !strings.HasPrefix(opened.Folder, dir) {
		t.Errorf("the program is the project's: %+v", opened)
	}
	if err := prog.SaveFile(opened.Project, "cabinet", "Extra.st", "PROGRAM Extra\nEND_PROGRAM\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "programs", "cabinet", "Extra.st")); err != nil {
		t.Error(err)
	}
	for _, key := range []string{"../x", ".unsaved/../../x", "a/b", "", "pallet-loading", ".unsaved/nope"} {
		if err := prog.SaveFile(key, "cabinet", "X.st", ""); err == nil {
			t.Errorf("no project %q", key)
		}
	}

	if err := s.DeleteProject("Line 1"); err == nil {
		t.Error("the open project cannot be deleted")
	}
	if _, err := s.CloseProject(); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Saved || p.Station != "pallet-loading" {
		t.Errorf("closed: %+v", p)
	}
	if _, err := s.NewProject("Line 2", "pallet-loading"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewProject("Line 2", "pallet-loading"); err == nil {
		t.Error("a name that is taken")
	}
	var names []string
	for _, p := range s.Projects() {
		names = append(names, p.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"Line 1", "Line 2"}) {
		t.Errorf("projects: %v", names)
	}
	if _, err := s.OpenProject("Line 1"); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Name != "Line 1" {
		t.Errorf("opened: %+v", p)
	}
	if err := s.DeleteProject("Line 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenProject("Line 2"); err == nil {
		t.Error("it is gone")
	}
}

// A project travels as a file and comes back as a project of its own.
func TestAProjectIsExportedAndImported(t *testing.T) {
	s, home := newService(t)
	if _, err := NewProgramService(s).Open(""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveProjectAs("Line 1"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "Line 1"+ProjectExtension)
	if err := ExportProjectTo(s.Project().Folder, file); err != nil {
		t.Fatal(err)
	}
	name, err := ImportProjectFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Line 1 (2)" {
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
