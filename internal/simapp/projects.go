package simapp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/model"
)

const (
	// ProjectsFolder is where the user folder keeps projects, one folder each.
	ProjectsFolder = "projects"
	// ProjectFile is a project's file in its folder.
	ProjectFile = "project.json"
	// ProjectExtension is that of an exported project: a zip of its folder.
	ProjectExtension = ".taktwin-project"
	// ProjectFormat is the format of project files this build writes.
	ProjectFormat = 1
	// maxProjectBytes is the most an imported project may unpack to.
	maxProjectBytes = 64 << 20
)

// errNoProject is what the simulation says while no project is open.
var errNoProject = errors.New("no project is open")

// ProjectDef is the content of project.json: what a project is based on, and what it adds.
// The project's folder also holds the programs of its soft PLCs, in programs/<plc>/.
type ProjectDef struct {
	Format int    `json:"format"`
	Name   string `json:"name,omitempty"`
	Base   Base   `json:"base"`
	// PLCs are how the station's PLCs are to be connected, by module, "" for the first. A
	// project restores them without connecting.
	PLCs map[string]PLCConfig `json:"plcs,omitempty"`
	// Parameters are the station's settings as set in the simulation, by id.
	Parameters map[string]float64 `json:"parameters,omitempty"`
}

// Base is what a project is based on: a station, or an exercise and its station.
type Base struct {
	Station  string `json:"station"`
	Exercise string `json:"exercise,omitempty"`
}

// project is the project the simulation works in; its name is "" while none is open.
type project struct {
	name string
	def  ProjectDef
}

// projectDir is the folder of a project by its name.
func projectDir(name string) string { return filepath.Join(UserFolder(), ProjectsFolder, name) }

func (p project) dir() string { return projectDir(p.name) }

// ProjectInfo is a project as the app lists it.
type ProjectInfo struct {
	Name         string `json:"name"`
	Station      string `json:"station"`
	StationName  string `json:"stationName"`
	Exercise     string `json:"exercise,omitempty"`
	ExerciseName string `json:"exerciseName,omitempty"`
	// Folder is where it is kept.
	Folder string `json:"folder"`
	// Modified is when its file was last written, as RFC 3339.
	Modified string `json:"modified,omitempty"`
	// Problem says why it does not open, such as its station's collection being hidden or gone.
	Problem string `json:"problem,omitempty"`
}

func projectInfo(lib *model.Library, name string, def ProjectDef) ProjectInfo {
	info := ProjectInfo{Name: name, Station: def.Base.Station, Exercise: def.Base.Exercise, Folder: projectDir(name)}
	info.StationName = cmpOr(lib.StationName(def.Base.Station), def.Base.Station)
	if !slices.Contains(lib.Stations(), def.Base.Station) {
		info.Problem = fmt.Sprintf("Its station %s is in none of your collections.", def.Base.Station)
	}
	if def.Base.Exercise != "" {
		if ex, err := lib.Exercise(def.Base.Exercise); err == nil {
			info.ExerciseName = ex.Name
		} else {
			info.ExerciseName = def.Base.Exercise
			info.Problem = fmt.Sprintf("Its exercise %s is in none of your collections.", def.Base.Exercise)
		}
	}
	if fi, err := os.Stat(filepath.Join(projectDir(name), ProjectFile)); err == nil {
		info.Modified = fi.ModTime().UTC().Format(time.RFC3339)
	}
	return info
}

// Project says which project the simulation works in, with no name while none is open.
func (s *Service) Project() ProjectInfo {
	s.mu.Lock()
	p := s.proj
	s.mu.Unlock()
	if p.name == "" {
		return ProjectInfo{}
	}
	return projectInfo(Library(), p.name, p.def)
}

// readProject reads the file of a project's folder, or gives nothing for a folder without one.
func readProject(dir string) (ProjectDef, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, ProjectFile))
	if errors.Is(err, fs.ErrNotExist) {
		return ProjectDef{}, false, nil
	}
	if err != nil {
		return ProjectDef{}, false, err
	}
	var def ProjectDef
	if err := json.Unmarshal(data, &def); err != nil {
		return def, false, fmt.Errorf("%s: %w", ProjectFile, err)
	}
	if def.Format > ProjectFormat {
		return def, false, fmt.Errorf("the project is of a newer version of Taktwin (format %d)", def.Format)
	}
	return def, true, nil
}

// writeProject writes a project's file into its folder.
func writeProject(dir string, def ProjectDef) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	def.Format = ProjectFormat
	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ProjectFile), append(data, '\n'), 0o644)
}

// saveProject writes the file of the project the simulation works in, under s.mu.
func (s *Service) saveProject() error {
	if s.proj.name == "" {
		return nil
	}
	return writeProject(s.proj.dir(), s.proj.def)
}

// loadBase loads what a project is based on: its station, set up as its exercise changes it.
// It gives the exercise the station is set up for.
func loadBase(b Base) (*model.Bundle, *engine.Engine, string, error) {
	if b.Exercise != "" {
		bundle, _, err := Library().LoadExercise(b.Exercise)
		if err != nil {
			return nil, nil, "", err
		}
		bundle, e, err := start(bundle)
		return bundle, e, b.Exercise, err
	}
	bundle, e, err := load(b.Station)
	return bundle, e, "", err
}

// open puts a project into the simulation after closing the one before: its station, its
// settings, and its PLCs' connections, ready to connect. No PLC is connected yet.
func (s *Service) open(name string, def ProjectDef) (model.Scene, error) {
	bundle, e, exercise, err := loadBase(def.Base)
	if err != nil {
		return model.Scene{}, err
	}
	s.close()
	def.Name = name
	for id, v := range def.Parameters {
		// A setting the station no longer has is left out.
		_ = e.SetParameter(id, v)
	}
	s.mu.Lock()
	s.engine, s.bundle, s.plcs, s.exercise = e, bundle, map[string]PLCConfig{}, exercise
	s.proj = project{name: name, def: def}
	s.run()
	err = s.saveProject()
	s.mu.Unlock()
	return e.Scene(), err
}

// close leaves the project the simulation works in: a check stops, every PLC is
// disconnected, the project's soft PLCs stop, and no station is left in the simulation.
func (s *Service) close() {
	s.StopCheck()
	s.mu.Lock()
	e, stop, name := s.engine, s.stop, s.proj.name
	s.engine, s.bundle, s.stop, s.plcs, s.exercise, s.proj = nil, nil, nil, map[string]PLCConfig{}, "", project{}
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	if e != nil {
		e.Close()
	}
	if name != "" {
		softPLCs.Drop(name)
	}
}

// CloseProject leaves the project the simulation works in, which keeps everything it had:
// every PLC is disconnected and its soft PLCs stop.
func (s *Service) CloseProject() { s.close() }

// OpenProject opens a project by name: its station, its settings, and its PLCs' connections,
// ready to connect.
func (s *Service) OpenProject(name string) (model.Scene, error) {
	if !projectName(name) {
		return model.Scene{}, fmt.Errorf("%q is no project", name)
	}
	def, ok, err := readProject(projectDir(name))
	if err != nil {
		return model.Scene{}, err
	}
	if !ok {
		return model.Scene{}, fmt.Errorf("there is no project %s", name)
	}
	return s.open(name, def)
}

// NewProject makes a project of a station or an exercise and opens it, as a programming tool
// makes a new project from a device: the station's settings as it has them, no PLC
// connected, and the template program once a PLC's program is opened. Without a name it is
// named after the exercise or station, with a number where that is taken.
func (s *Service) NewProject(name string, base Base) (model.Scene, error) {
	lib := Library()
	title := ""
	if base.Exercise != "" {
		ex, err := lib.Exercise(base.Exercise)
		if err != nil {
			return model.Scene{}, err
		}
		base.Station, title = ex.Station, ex.Name
	} else {
		if !slices.Contains(lib.Stations(), base.Station) {
			return model.Scene{}, fmt.Errorf("there is no station %s", base.Station)
		}
		title = lib.StationName(base.Station)
	}
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		name = freeName(cmpOr(title, base.Station))
	case !projectName(name):
		return model.Scene{}, errName(name)
	case exists(projectDir(name)):
		return model.Scene{}, fmt.Errorf("there is a project %s already", name)
	}
	return s.open(name, ProjectDef{Name: name, Base: base})
}

var forbiddenName = regexp.MustCompile(`[/\\:*?"<>|]`)

// projectName says whether a name can be that of a project's folder.
func projectName(name string) bool {
	return name != "" && len(name) <= 80 && strings.TrimSpace(name) == name && !strings.HasPrefix(name, ".") &&
		!forbiddenName.MatchString(name) && filepath.Base(name) == name
}

func errName(name string) error {
	return fmt.Errorf("%q cannot be a project's name: up to 80 characters, without / \\ : * ? \" < > | and not starting with a dot", name)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// freeName makes a name a project can have and no project has yet: the name, made fit for a
// folder, with a number where it is taken.
func freeName(name string) string {
	name = strings.TrimLeft(strings.TrimSpace(forbiddenName.ReplaceAllString(name, "-")), ".")
	if len(name) > 70 {
		name = strings.TrimSpace(name[:70])
	}
	if !projectName(name) {
		name = "Project"
	}
	free := name
	for n := 2; exists(projectDir(free)); n++ {
		free = fmt.Sprintf("%s (%d)", name, n)
	}
	return free
}

// Projects lists the projects of the user folder, the latest first.
func (s *Service) Projects() []ProjectInfo {
	out := []ProjectInfo{}
	items, err := os.ReadDir(filepath.Join(UserFolder(), ProjectsFolder))
	if err != nil {
		return out
	}
	lib := Library()
	for _, it := range items {
		if !it.IsDir() || !projectName(it.Name()) {
			continue
		}
		def, ok, err := readProject(projectDir(it.Name()))
		if err != nil || !ok {
			continue
		}
		out = append(out, projectInfo(lib, it.Name(), def))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out
}

// Example is a station a collection opens with, offered to start a project from.
type Example struct {
	Station     string `json:"station"`
	StationName string `json:"stationName"`
	Collection  string `json:"collection"`
	Description string `json:"description"`
}

// Examples lists the stations the collections open with, those hidden in the settings left
// out: the app's own reference station first.
func (s *Service) Examples() []Example {
	lib := Library()
	stations := lib.Stations()
	out := []Example{}
	seen := map[string]bool{}
	for _, c := range lib.Collections() {
		if c.Station == "" || seen[c.Station] || !slices.Contains(stations, c.Station) {
			continue
		}
		seen[c.Station] = true
		out = append(out, Example{Station: c.Station, StationName: cmpOr(lib.StationName(c.Station), c.Station), Collection: cmpOr(c.Name, c.ID), Description: c.Description})
	}
	return out
}

// closed checks that a project exists and is not the one open.
func (s *Service) closed(name string) error {
	if !projectName(name) {
		return fmt.Errorf("%q is no project", name)
	}
	if _, ok, _ := readProject(projectDir(name)); !ok {
		return fmt.Errorf("there is no project %s", name)
	}
	s.mu.Lock()
	open := s.proj.name
	s.mu.Unlock()
	if open == name {
		return fmt.Errorf("the project %s is open: close it first", name)
	}
	return nil
}

// RenameProject gives a project that is not open another name.
func (s *Service) RenameProject(name, to string) (ProjectInfo, error) {
	to = strings.TrimSpace(to)
	if err := s.closed(name); err != nil {
		return ProjectInfo{}, err
	}
	if !projectName(to) {
		return ProjectInfo{}, errName(to)
	}
	if to == name {
		def, _, err := readProject(projectDir(name))
		return projectInfo(Library(), name, def), err
	}
	// A name that differs only in case is the same folder on macOS and Windows.
	if exists(projectDir(to)) && !strings.EqualFold(to, name) {
		return ProjectInfo{}, fmt.Errorf("there is a project %s already", to)
	}
	if err := os.Rename(projectDir(name), projectDir(to)); err != nil {
		return ProjectInfo{}, err
	}
	def, _, err := readProject(projectDir(to))
	if err != nil {
		return ProjectInfo{}, err
	}
	def.Name = to
	return projectInfo(Library(), to, def), writeProject(projectDir(to), def)
}

// DuplicateProject copies a project, with its programs, under a name of its own.
func (s *Service) DuplicateProject(name string) (ProjectInfo, error) {
	if !projectName(name) {
		return ProjectInfo{}, fmt.Errorf("%q is no project", name)
	}
	def, ok, err := readProject(projectDir(name))
	if err != nil {
		return ProjectInfo{}, err
	}
	if !ok {
		return ProjectInfo{}, fmt.Errorf("there is no project %s", name)
	}
	to := freeName(name + " (copy)")
	if err := copyTree(projectDir(name), projectDir(to)); err != nil {
		return ProjectInfo{}, err
	}
	def.Name = to
	return projectInfo(Library(), to, def), writeProject(projectDir(to), def)
}

// DeleteProject removes a project that is not open.
func (s *Service) DeleteProject(name string) error {
	if err := s.closed(name); err != nil {
		return err
	}
	return os.RemoveAll(projectDir(name))
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// PlannedPLC is how the project means one of the station's PLCs, by its module, to be
// connected, which the PLC panel offers to connect.
func (s *Service) PlannedPLC(plc string) PLCConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return PLCConfig{Mode: PLCNone}
	}
	if c, ok := s.proj.def.PLCs[s.plcKey(plc)]; ok {
		return c
	}
	return PLCConfig{Mode: PLCNone}
}

// remember keeps how a PLC is connected in the project, under s.mu.
func (s *Service) remember(plc string, c PLCConfig) error {
	if s.proj.def.PLCs == nil {
		s.proj.def.PLCs = map[string]PLCConfig{}
	}
	s.proj.def.PLCs[s.plcKey(plc)] = c
	return s.saveProject()
}

// ExportProject writes a project into a .taktwin-project file the user picks in the system's
// dialog. Cancelling gives an empty path.
func (s *Service) ExportProject(name string) (string, error) {
	if !projectName(name) || !exists(filepath.Join(projectDir(name), ProjectFile)) {
		return "", fmt.Errorf("there is no project %s", name)
	}
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to choose a file in")
	}
	to, err := app.Dialog.SaveFile().SetMessage("Export the project").SetFilename(name+ProjectExtension).
		AddFilter("Taktwin project", "*"+ProjectExtension).PromptForSingleSelection()
	if err != nil || to == "" {
		return "", err
	}
	return to, ExportProjectTo(projectDir(name), to)
}

// ImportProject lets the user pick a .taktwin-project file in the system's dialog, unpacks it
// into a project of its own and opens it. Cancelling gives no name. A project that is
// imported but does not open, such as one whose station is missing, is listed all the same.
func (s *Service) ImportProject() (ProjectInfo, error) {
	app := application.Get()
	if app == nil {
		return ProjectInfo{}, errors.New("no window to choose a file in")
	}
	from, err := app.Dialog.OpenFile().SetTitle("Import a project").AddFilter("Taktwin project", "*"+ProjectExtension).
		CanChooseFiles(true).PromptForSingleSelection()
	if err != nil || from == "" {
		return ProjectInfo{}, err
	}
	name, err := ImportProjectFile(from)
	if err != nil {
		return ProjectInfo{}, err
	}
	if _, err := s.OpenProject(name); err != nil {
		return ProjectInfo{Name: name}, fmt.Errorf("imported as %s, but it does not open: %w", name, err)
	}
	return s.Project(), nil
}

// ExportProjectTo writes a project's folder into a zip file: its project.json and programs.
func ExportProjectTo(dir, to string) error {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		w, err := z.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	return os.WriteFile(to, buf.Bytes(), 0o644)
}

// ImportProjectFile unpacks a .taktwin-project file into a project of the user folder, named
// as it says, or after the file, with a number where the name is taken. It holds data only:
// a project.json and the sources of programs.
func ImportProjectFile(from string) (string, error) {
	data, err := os.ReadFile(from)
	if err != nil {
		return "", err
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("%s is no project file: %w", filepath.Base(from), err)
	}
	files := map[string][]byte{}
	var total int64
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(f.Name)
		if !fs.ValidPath(name) || strings.Contains(f.Name, `\`) || (name != ProjectFile && !strings.HasPrefix(name, "programs/")) {
			return "", fmt.Errorf("%s holds %s, which no project has", filepath.Base(from), f.Name)
		}
		total += int64(f.UncompressedSize64)
		if total > maxProjectBytes {
			return "", fmt.Errorf("%s is larger than a project can be", filepath.Base(from))
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		content, err := io.ReadAll(io.LimitReader(r, maxProjectBytes+1))
		r.Close()
		if err != nil {
			return "", err
		}
		files[name] = content
	}
	raw, ok := files[ProjectFile]
	if !ok {
		return "", fmt.Errorf("%s is no project: it has no %s", filepath.Base(from), ProjectFile)
	}
	var def ProjectDef
	if err := json.Unmarshal(raw, &def); err != nil {
		return "", fmt.Errorf("%s: %w", ProjectFile, err)
	}
	if def.Format > ProjectFormat {
		return "", fmt.Errorf("the project is of a newer version of Taktwin (format %d)", def.Format)
	}
	name := freeName(cmpOr(strings.TrimSpace(def.Name), strings.TrimSuffix(filepath.Base(from), ProjectExtension)))
	dir := projectDir(name)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		target := filepath.Join(dir, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, files[n], 0o644); err != nil {
			return "", err
		}
	}
	return name, nil
}

// MenuCommandEvent carries a command of the app's menu on a Mac to the frontend: import or
// close a project, or show the settings.
const MenuCommandEvent = "menu:command"
