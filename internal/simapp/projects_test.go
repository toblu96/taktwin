package simapp

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/toblu96/taktwin/internal/model"
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

// zipFiles reads the files of a zip file by their names.
func zipFiles(t *testing.T, file string) map[string]string {
	t.Helper()
	z, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	out := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(r)
		r.Close()
		out[f.Name] = string(data)
	}
	return out
}

// defIn reads the project.json of a project file.
func defIn(t *testing.T, file string) ProjectDef {
	t.Helper()
	def, err := parseProject([]byte(zipFiles(t, file)[ProjectFile]))
	if err != nil {
		t.Fatal(err)
	}
	return def
}

// A project keeps what is changed in it: how the PLC is connected, without connecting it
// again, and the station's settings. Closing it writes its file, disconnects the PLC and
// stops its soft PLC, and leaves no working copy behind.
func TestAProjectKeepsItsWork(t *testing.T) {
	s, home := newService(t)
	p := s.Project()
	file := filepath.Join(home, ProjectsFolder, "Test"+ProjectExtension)
	if p.Name != "Test" || p.Path != file || !p.InFolder || p.Station != "pallet-loading" || p.StationName != "Pallet loading" || p.Problem != "" {
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
	id := defIn(t, file).ID

	s.CloseProject()
	if p := s.Project(); p.Name != "" || s.Scene().ID != "" {
		t.Fatalf("closed: %+v", p)
	}
	if exists(filepath.Join(workingDir(), id)) || exists(metaFile(id)) {
		t.Error("a working copy is left behind")
	}
	if _, ok := zipFiles(t, file)["programs/cabinet/PLC_PRG.st"]; !ok {
		t.Error("the program is in the file")
	}
	if _, err := s.OpenProjectFile(file, OpenOptions{}); err != nil {
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
	if got := defIn(t, file).ID; got != id {
		t.Errorf("the id changed from %s to %s", id, got)
	}
}

// A new project is a file in the user folder's projects, named after its station or exercise
// unless it is given a name; project files that are not open can be renamed, copied and
// deleted.
func TestProjectsAreMadeAndManaged(t *testing.T) {
	s, home := newService(t)
	in := func(name string) string { return filepath.Join(home, ProjectsFolder, name+ProjectExtension) }
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
	if p := s.Project(); p.Name != "Test" {
		t.Errorf("the open project stays open: %+v", p)
	}
	if _, err := s.NewProject("", Base{Station: "pallet-loading"}); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Name != "Pallet loading" || p.Path != in("Pallet loading") {
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

	if err := s.DeleteProject(in("Load a pallet")); err == nil {
		t.Error("the open project cannot be deleted")
	}
	if _, err := s.RenameProject(in("Load a pallet"), "Mine"); err == nil {
		t.Error("the open project cannot be renamed")
	}
	if _, err := s.RenameProject(in("Test"), "Pallet loading"); err == nil {
		t.Error("renamed onto another project")
	}
	renamed, err := s.RenameProject(in("Test"), "Line 1")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Line 1" || renamed.Path != in("Line 1") || exists(in("Test")) {
		t.Errorf("renamed: %+v", renamed)
	}
	if err := s.copyProject(in("Line 1"), in("Line 1 (copy)")); err != nil {
		t.Fatal(err)
	}
	if a, b := defIn(t, in("Line 1")), defIn(t, in("Line 1 (copy)")); a.ID == b.ID || b.Base.Station != "pallet-loading" {
		t.Errorf("a copy has an id of its own: %+v %+v", a, b)
	}
	if err := s.DeleteProject(in("Pallet loading (2)")); err != nil {
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
	if _, err := s.OpenProjectFile(in("Pallet loading (2)"), OpenOptions{}); err == nil {
		t.Error("it is gone")
	}

	// A program is saved back into the project it was opened from, while it is open.
	prog := NewProgramService(s)
	opened, err := prog.Open("")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Project != in("Load a pallet") {
		t.Errorf("the program is the project's: %+v", opened)
	}
	if err := prog.SaveFile(opened.Project, "cabinet", "Extra.st", "PROGRAM Extra\nEND_PROGRAM\n"); err != nil {
		t.Fatal(err)
	}
	if st := s.SaveProject(); st.State != SaveSaved {
		t.Fatalf("saved: %+v", st)
	}
	if _, ok := zipFiles(t, in("Load a pallet"))["programs/cabinet/Extra.st"]; !ok {
		t.Error("saved into the project it came from")
	}
	for _, name := range []string{"../x", "a/b", "", "Test", in("Line 1")} {
		if err := prog.SaveFile(name, "cabinet", "X.st", ""); err == nil {
			t.Errorf("no open project %q", name)
		}
	}

	// A project whose station is gone is listed with the problem, and does not open.
	gone, err := newProjectFile("Gone", ProjectDef{Base: Base{Station: "gone"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(s.Projects(), func(p ProjectInfo) bool { return p.Name == "Gone" }); i < 0 || s.Projects()[i].Problem == "" {
		t.Errorf("listed with no problem: %+v", s.Projects())
	}
	if _, err := s.OpenProjectFile(gone, OpenOptions{}); err == nil {
		t.Error("it opens")
	}
	if p := s.Project(); p.Name != "Load a pallet" {
		t.Errorf("the open project stays open: %+v", p)
	}
}

// A project file opened from anywhere, such as the downloads folder, is the project: its
// changes are saved into it, nothing is copied into the user folder, and the projects page
// lists it until it is taken off the list.
func TestAProjectIsSavedWhereItIs(t *testing.T) {
	s, home := newService(t)
	downloads := t.TempDir()
	file := filepath.Join(downloads, "Mine"+ProjectExtension)
	if err := s.copyProject(filepath.Join(home, ProjectsFolder, "Test"+ProjectExtension), file); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenProjectFile(file, OpenOptions{}); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.Name != "Mine" || p.Path != file || p.InFolder {
		t.Errorf("opened where it is: %+v", p)
	}
	if err := s.SetParameter("speed", 0.5); err != nil {
		t.Fatal(err)
	}
	if st := s.SaveState(); st.State != SaveSaved && st.State != SavePending {
		t.Errorf("state: %+v", st)
	}
	if st := s.SaveProject(); st.State != SaveSaved {
		t.Fatalf("saved: %+v", st)
	}
	if got := defIn(t, file).Parameters["speed"]; got != 0.5 {
		t.Errorf("the file has the setting: %v", got)
	}
	s.CloseProject()
	items, _ := os.ReadDir(filepath.Join(home, ProjectsFolder))
	if len(items) != 1 {
		t.Errorf("only the user folder's own project is there: %v", items)
	}
	listed := func() *ProjectInfo {
		for _, p := range s.Projects() {
			if p.Name == "Mine" {
				return &p
			}
		}
		return nil
	}
	if p := listed(); p == nil || p.Path != file || p.InFolder || p.Missing {
		t.Fatalf("listed: %+v", p)
	}

	// A file moved away is listed as missing, until it is taken off the list.
	if err := os.Rename(file, file+".moved"); err != nil {
		t.Fatal(err)
	}
	if p := listed(); p == nil || !p.Missing {
		t.Errorf("not missing: %+v", p)
	}
	if got := s.Projects(); !got[len(got)-1].Missing {
		t.Errorf("a missing file is listed last: %+v", got)
	}
	s.ForgetProject(file)
	if p := listed(); p != nil {
		t.Errorf("still listed: %+v", p)
	}
}

// A change is written into the file shortly after it is made, without closing the project.
func TestAChangeIsSavedShortlyAfter(t *testing.T) {
	saveEvery = 10 * time.Millisecond
	t.Cleanup(func() { saveEvery = time.Second })
	s, _ := newService(t)
	file := s.Project().Path
	if err := s.SetParameter("speed", 0.25); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for defIn(t, file).Parameters["speed"] != 0.25 {
		if time.Now().After(deadline) {
			t.Fatalf("not saved: %+v", s.SaveState())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := s.SaveState(); st.State != SaveSaved || st.Saved == "" {
		t.Errorf("state: %+v", st)
	}
}

// A file changed from elsewhere while the project is open is not overwritten: the project
// can be written over it, the file opened as it is now, or the project saved as a copy.
func TestAFileChangedElsewhereIsNotOverwritten(t *testing.T) {
	s, _ := newService(t)
	file := s.Project().Path
	theirs := func(speed float64) {
		t.Helper()
		files := map[string][]byte{}
		for name, data := range zipFiles(t, file) {
			files[name] = []byte(data)
		}
		def := defIn(t, file)
		def.Parameters = map[string]float64{"speed": speed}
		files[ProjectFile], _ = projectJSON(def)
		later := time.Now().Add(2 * time.Second)
		if err := writeZipOver(file, files); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(file, later, later)
	}
	theirs(0.9)
	if err := s.SetParameter("speed", 0.1); err != nil {
		t.Fatal(err)
	}
	if st := s.SaveProject(); st.State != SaveConflict || st.Problem == "" {
		t.Fatalf("not a conflict: %+v", st)
	}
	if got := defIn(t, file).Parameters["speed"]; got != 0.9 {
		t.Errorf("their change was overwritten: %v", got)
	}
	if st := s.KeepMine(); st.State != SaveSaved {
		t.Fatalf("kept: %+v", st)
	}
	if got := defIn(t, file).Parameters["speed"]; got != 0.1 {
		t.Errorf("mine is not written: %v", got)
	}

	theirs(0.8)
	if _, err := s.OpenTheirs(); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Parameters["speed"]; got != 0.8 {
		t.Errorf("theirs is not open: %v", got)
	}

	theirs(0.7)
	if err := s.SetParameter("speed", 0.2); err != nil {
		t.Fatal(err)
	}
	if st := s.SaveProject(); st.State != SaveConflict {
		t.Fatalf("not a conflict: %+v", st)
	}
	id := defIn(t, file).ID
	copied := filepath.Join(t.TempDir(), "Mine")
	if err := s.saveProjectAs(copied); err != nil {
		t.Fatal(err)
	}
	copied += ProjectExtension
	if p := s.Project(); p.Path != copied || p.Name != "Mine" || s.SaveState().State != SaveSaved {
		t.Errorf("working in the copy: %+v %+v", p, s.SaveState())
	}
	if def := defIn(t, copied); def.ID == id || def.Parameters["speed"] != 0.2 {
		t.Errorf("the copy: %+v", def)
	}
	if got := defIn(t, file).Parameters["speed"]; got != 0.7 {
		t.Errorf("their file was changed: %v", got)
	}
}

// A file that cannot be written opens read only: its changes stay in the working copy until
// the project is saved as a copy.
func TestAFileThatCannotBeWrittenOpensReadOnly(t *testing.T) {
	s, _ := newService(t)
	file := s.Project().Path
	s.CloseProject()
	if err := os.Chmod(file, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0o644) })
	if writable(file) {
		t.Skip("the file can be written all the same, as by an administrator")
	}
	if _, err := s.OpenProjectFile(file, OpenOptions{}); err != nil {
		t.Fatal(err)
	}
	if st := s.SaveState(); st.State != SaveReadOnly {
		t.Errorf("state: %+v", st)
	}
	if err := s.SetParameter("speed", 0.3); err != nil {
		t.Fatal(err)
	}
	if st := s.SaveProject(); st.State != SaveReadOnly || !st.Unsaved {
		t.Errorf("state: %+v", st)
	}
	if _, ok := defIn(t, file).Parameters["speed"]; ok {
		t.Error("the file was written")
	}
	copied := filepath.Join(t.TempDir(), "Mine"+ProjectExtension)
	if err := s.saveProjectAs(copied); err != nil {
		t.Fatal(err)
	}
	if st := s.SaveState(); st.State != SaveSaved {
		t.Errorf("the copy can be written: %+v", st)
	}
	if got := defIn(t, copied).Parameters["speed"]; got != 0.3 {
		t.Errorf("the copy has the change: %v", got)
	}
}

// Changes a crash left in a working copy are offered back when the file is opened again,
// and dropped if they are not wanted.
func TestChangesLeftByACrashAreOfferedBack(t *testing.T) {
	s, _ := newService(t)
	file := s.Project().Path
	crash := func() {
		t.Helper()
		s.mu.Lock()
		doc := s.proj.doc
		s.proj.doc = nil
		s.mu.Unlock()
		doc.halt()
		s.CloseProject()
	}
	if err := s.SetParameter("speed", 0.4); err != nil {
		t.Fatal(err)
	}
	crash()
	info, err := s.InspectProjectFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasUnsaved {
		t.Fatalf("not offered back: %+v", info)
	}
	if _, err := s.OpenProjectFile(file, OpenOptions{Restore: true}); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Parameters["speed"]; got != 0.4 {
		t.Errorf("the change is not back: %v", got)
	}
	if info, _ := s.InspectProjectFile(file); info.HasUnsaved {
		t.Errorf("the open project offers itself back: %+v", info)
	}
	if st := s.SaveProject(); st.State != SaveSaved || defIn(t, file).Parameters["speed"] != 0.4 {
		t.Errorf("not saved: %+v", st)
	}

	if err := s.SetParameter("speed", 0.6); err != nil {
		t.Fatal(err)
	}
	crash()
	if _, err := s.OpenProjectFile(file, OpenOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Parameters["speed"]; got != 0.4 {
		t.Errorf("the change is not dropped: %v", got)
	}
	s.CloseProject()
	if info, _ := s.InspectProjectFile(file); info.HasUnsaved {
		t.Errorf("still offered: %+v", info)
	}
}

// A project file holds data only, of this format: anything else is refused.
func TestOnlyProjectFilesOpen(t *testing.T) {
	s, _ := newService(t)
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
	project := `{"format":2,"id":"x","base":{"station":"pallet-loading"}}`
	for what, files := range map[string]map[string]string{
		"a way out":         {ProjectFile: project, "../evil.st": "x"},
		"something else":    {ProjectFile: project, "run.sh": "x"},
		"no project":        {"programs/cabinet/PLC_PRG.st": "x"},
		"a newer format":    {ProjectFile: `{"format":99}`},
		"an earlier format": {ProjectFile: `{"format":1,"base":{"station":"pallet-loading"}}`},
		"a broken project":  {ProjectFile: `{`},
	} {
		if _, err := s.OpenProjectFile(bad(files), OpenOptions{}); err == nil {
			t.Errorf("%s opens", what)
		}
	}
	if _, err := s.OpenProjectFile(filepath.Join(t.TempDir(), "gone"+ProjectExtension), OpenOptions{}); err == nil {
		t.Error("no file")
	}
	if _, err := s.OpenProjectFile(filepath.Join(t.TempDir(), "notes.txt"), OpenOptions{}); err == nil {
		t.Error("a file that is no project")
	}
	// A file without an id gets one.
	file := bad(map[string]string{ProjectFile: `{"format":2,"base":{"station":"pallet-loading"}}`})
	if _, err := s.OpenProjectFile(file, OpenOptions{}); err != nil {
		t.Fatal(err)
	}
	s.CloseProject()
	if id := defIn(t, file).ID; !validID(id) {
		t.Errorf("no id: %q", id)
	}
}

// A project keeps a picture of its station and how its last check went, lists them, and
// keeps the picture in its file.
func TestAProjectShowsItsPictureAndLastCheck(t *testing.T) {
	s, _ := newService(t)
	file := s.Project().Path
	jpeg := "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQ=="
	for _, bad := range []string{"", "data:image/png;base64,AAAA", "data:image/jpeg;base64,***"} {
		if err := s.SaveThumbnail(bad); err == nil {
			t.Errorf("%q is kept", bad)
		}
	}
	if err := s.SaveThumbnail(jpeg); err != nil {
		t.Fatal(err)
	}
	s.keepCheck("Other", LastCheck{Exercise: "load-a-pallet", Passed: true})
	s.keepCheck(file, LastCheck{Exercise: "load-a-pallet", Passed: true, At: "2026-11-21T10:00:00Z"})
	p := s.Project()
	if p.Thumbnail != jpeg || p.LastCheck == nil || !p.LastCheck.Passed || p.LastCheck.At != "2026-11-21T10:00:00Z" {
		t.Errorf("project: %+v", p)
	}
	s.CloseProject()
	if err := s.SaveThumbnail(jpeg); err == nil {
		t.Error("a picture with no project open")
	}
	listed := s.Projects()
	if len(listed) != 1 || listed[0].Thumbnail != jpeg || listed[0].LastCheck == nil {
		t.Errorf("listed with its picture: %+v", listed)
	}
}

// A project can have a station of its own: an empty one to build, or a copy of its
// collection's. The builder keeps every change in the project, the simulation loads it again
// on request, and the project file keeps it. An exercise's station stays as it is.
func TestAProjectBuildsAStationOfItsOwn(t *testing.T) {
	s, _ := newService(t)
	b := NewBuilderService(s)
	t.Cleanup(b.Close)
	if p := s.Project(); p.Own {
		t.Fatalf("a collection's station is not the project's own: %+v", p)
	}
	if _, err := b.Edit(); err == nil {
		t.Error("the builder builds on a collection's station")
	}
	p, err := s.CustomizeStation()
	if err != nil {
		t.Fatal(err)
	}
	if !p.Own || p.StationName != "Pallet loading" {
		t.Fatalf("customized: %+v", p)
	}
	st, err := b.Edit()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SetStation("My line", "Mine"); err != nil {
		t.Fatal(err)
	}
	if p := s.Project(); p.StationName != "My line" || p.Station != "pallet-loading" {
		t.Errorf("the builder keeps its changes in the project, under the station's id: %+v", p)
	}
	before := len(st.Modules)
	if scene, err := s.ReloadStation(); err != nil || scene.Name != "My line" || len(scene.Instances) == 0 {
		t.Fatalf("reloaded: %s %v", scene.Name, err)
	}
	b.Close()

	// A new project with an empty station opens with it, to be built on.
	if _, err := s.NewEmptyProject("", "taktwin"); err != nil {
		t.Fatal(err)
	}
	empty := s.Project()
	if empty.Name != "New station" || !empty.Own || empty.Station != "new-station" || empty.Problem != "" {
		t.Fatalf("empty: %+v", empty)
	}
	st, err = b.Edit()
	if err != nil {
		t.Fatal(err)
	}
	if module(st, "base-plate") == nil || len(st.Modules) >= before {
		t.Errorf("an empty station stands on its base: %+v", st.Modules)
	}
	if _, err := b.Add("pallet-conveyor"); err != nil {
		t.Fatal(err)
	}
	scene, err := s.ReloadStation()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(scene.Instances, func(i model.SceneInstance) bool { return i.Module == "pallet-conveyor" }) {
		t.Error("the simulation has the conveyor that was built")
	}

	// The builder stops keeping changes once the project is closed.
	s.CloseProject()
	if _, err := b.Add("pallet-conveyor"); err == nil {
		t.Error("a change kept in a closed project")
	}

	// The project file keeps the station.
	if _, ok := zipFiles(t, empty.Path)[StationFile]; !ok {
		t.Error("the file has no station")
	}
	opened, err := s.OpenProjectFile(empty.Path, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Own || opened.Name != "New station" {
		t.Errorf("opened: %+v", opened)
	}

	if _, err := s.NewProject("", Base{Exercise: "load-a-pallet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CustomizeStation(); err == nil {
		t.Error("an exercise's station is customized")
	}
}
