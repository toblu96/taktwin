package simapp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing/fstest"
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
	// StationFile is the project's own station, which it has once it is built or changed in
	// the project. Without it, the project's station is that of a collection.
	StationFile = "station.json"
	// ThumbnailFile is a picture of the project's station as it was last closed.
	ThumbnailFile = "thumbnail.jpg"
	// maxThumbnailBytes is the largest thumbnail a project keeps.
	maxThumbnailBytes = 1 << 20
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
	// LastCheck is how the last check of an exercise that ran to its end went.
	LastCheck *LastCheck `json:"lastCheck,omitempty"`
	// Handout marks a project file written to hand an exercise out: it brings its station and
	// exercise, and opening it again offers to go on with the project made from it before.
	Handout bool `json:"handout,omitempty"`
	// FromHandout marks a project made from such a file.
	FromHandout bool `json:"fromHandout,omitempty"`
	// Requires lists the collections the station is built from, as they were when the file
	// was written.
	Requires []Requirement `json:"requires,omitempty"`
}

// Requirement is a collection a project needs, with the version it was made with.
type Requirement struct {
	Collection string `json:"collection"`
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	// Installed is the version here, if one is; Hidden says that it is hidden in the settings.
	Installed string `json:"installed,omitempty"`
	Hidden    bool   `json:"hidden,omitempty"`
}

// LastCheck is how a check of an exercise went: passed or not, and when, as RFC 3339.
type LastCheck struct {
	Exercise string `json:"exercise"`
	Passed   bool   `json:"passed"`
	At       string `json:"at"`
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
	// Own says that the project has a station of its own, which it builds on, rather than
	// that of a collection, which follows the collection's updates.
	Own bool `json:"own,omitempty"`
	// Folder is where it is kept.
	Folder string `json:"folder"`
	// Modified is when its file was last written, as RFC 3339.
	Modified string `json:"modified,omitempty"`
	// Problem says why it does not open, such as its station's collection being hidden or gone.
	Problem string `json:"problem,omitempty"`
	// Thumbnail is a picture of its station as it was last closed, as a data URL.
	Thumbnail string     `json:"thumbnail,omitempty"`
	LastCheck *LastCheck `json:"lastCheck,omitempty"`
}

func projectInfo(lib *model.Library, name string, def ProjectDef) ProjectInfo {
	info := ProjectInfo{Name: name, Station: def.Base.Station, Exercise: def.Base.Exercise, Folder: projectDir(name)}
	_, info.Own = ownStation(projectDir(name))
	lib = projectLibraryFrom(lib, name, def.Base)
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
	if data, err := os.ReadFile(filepath.Join(projectDir(name), ThumbnailFile)); err == nil && len(data) <= maxThumbnailBytes {
		info.Thumbnail = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
	}
	info.LastCheck = def.LastCheck
	return info
}

// SaveThumbnail keeps a picture of the open project's station, a JPEG as a data URL, to
// show the project by.
func (s *Service) SaveThumbnail(dataURL string) error {
	data, ok := strings.CutPrefix(dataURL, "data:image/jpeg;base64,")
	if !ok {
		return errors.New("a thumbnail is a JPEG data URL")
	}
	img, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return err
	}
	if len(img) > maxThumbnailBytes {
		return errors.New("the thumbnail is too large")
	}
	s.mu.Lock()
	name := s.proj.name
	s.mu.Unlock()
	if name == "" {
		return errNoProject
	}
	return os.WriteFile(filepath.Join(projectDir(name), ThumbnailFile), img, 0o644)
}

// RevealProject shows a project's folder in the system's file manager.
func (s *Service) RevealProject(name string) error {
	if !projectName(name) || !exists(filepath.Join(projectDir(name), ProjectFile)) {
		return fmt.Errorf("there is no project %s", name)
	}
	app := application.Get()
	if app == nil {
		return errors.New("no file manager to show it in")
	}
	return app.Env.OpenFileManager(projectDir(name), false)
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

// ownStation reads a project's own station, if it has one.
func ownStation(dir string) ([]byte, bool) {
	data, err := os.ReadFile(filepath.Join(dir, StationFile))
	return data, err == nil
}

// projectLibrary is the library a project's station loads from: the collections, with the
// project's own station and exercise in front of them if it has them.
func projectLibrary(name string, b Base) *model.Library { return projectLibraryFrom(Library(), name, b) }

func projectLibraryFrom(lib *model.Library, name string, b Base) *model.Library {
	layer := fstest.MapFS{}
	if data, ok := ownStation(projectDir(name)); ok {
		layer[path.Join("stations", b.Station, "station.json")] = &fstest.MapFile{Data: data}
	}
	if data, err := os.ReadFile(filepath.Join(projectDir(name), ExerciseFile)); err == nil && b.Exercise != "" {
		layer[path.Join("exercises", b.Exercise, "exercise.json")] = &fstest.MapFile{Data: data}
	}
	if len(layer) == 0 {
		return lib
	}
	return lib.With(layer)
}

// library is the library of the open project, or the collections while none is open.
func (s *Service) library() *model.Library {
	s.mu.Lock()
	name, b := s.proj.name, s.proj.def.Base
	s.mu.Unlock()
	if name == "" {
		return Library()
	}
	return projectLibrary(name, b)
}

// loadBase loads what a project is based on: its station, set up as its exercise changes it.
// It gives the exercise the station is set up for.
func loadBase(lib *model.Library, b Base) (*model.Bundle, *engine.Engine, string, error) {
	if b.Exercise != "" {
		bundle, _, err := lib.LoadExercise(b.Exercise)
		if err != nil {
			return nil, nil, "", err
		}
		bundle, e, err := start(bundle)
		return bundle, e, b.Exercise, err
	}
	bundle, e, err := load(lib, b.Station)
	return bundle, e, "", err
}

// open puts a project into the simulation after closing the one before: its station, its
// settings, and its PLCs' connections, ready to connect. No PLC is connected yet.
func (s *Service) open(name string, def ProjectDef) (model.Scene, error) {
	bundle, e, exercise, err := loadBase(projectLibrary(name, def.Base), def.Base)
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

// ReloadStation loads the open project's station again, as its files have it now, such as
// after it was built on: every PLC is disconnected, as on a restart, while the soft PLCs go
// on with their programs, ready to connect again.
func (s *Service) ReloadStation() (model.Scene, error) {
	s.mu.Lock()
	name, def := s.proj.name, s.proj.def
	s.mu.Unlock()
	if name == "" {
		return model.Scene{}, errNoProject
	}
	bundle, e, exercise, err := loadBase(projectLibrary(name, def.Base), def.Base)
	if err != nil {
		return model.Scene{}, err
	}
	for id, v := range def.Parameters {
		_ = e.SetParameter(id, v)
	}
	s.StopCheck()
	s.mu.Lock()
	if s.proj.name != name {
		s.mu.Unlock()
		e.Close()
		return model.Scene{}, errors.New("the project was closed")
	}
	old, stop := s.engine, s.stop
	s.engine, s.bundle, s.plcs, s.exercise = e, bundle, map[string]PLCConfig{}, exercise
	s.run()
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	if old != nil {
		old.Close()
	}
	return e.Scene(), nil
}

// CustomizeStation gives the open project a station of its own, a copy of its collection's,
// to build on. It no longer follows the collection's updates. The station of an exercise
// stays as the exercise has it.
func (s *Service) CustomizeStation() (ProjectInfo, error) {
	s.mu.Lock()
	name, b := s.proj.name, s.proj.def.Base
	s.mu.Unlock()
	switch {
	case name == "":
		return ProjectInfo{}, errNoProject
	case b.Exercise != "":
		return ProjectInfo{}, errors.New("the station of an exercise stays as the exercise has it")
	}
	if _, ok := ownStation(projectDir(name)); !ok {
		data, err := Library().ReadFile(path.Join("stations", b.Station, "station.json"))
		if err != nil {
			return ProjectInfo{}, err
		}
		if err := os.WriteFile(filepath.Join(projectDir(name), StationFile), data, 0o644); err != nil {
			return ProjectInfo{}, err
		}
	}
	return s.Project(), nil
}

// editedStation gives the open project's own station, to build on, with the project's name.
func (s *Service) editedStation() (string, []byte, error) {
	s.mu.Lock()
	name := s.proj.name
	s.mu.Unlock()
	if name == "" {
		return "", nil, errNoProject
	}
	data, ok := ownStation(projectDir(name))
	if !ok {
		return "", nil, errors.New("the project has no station of its own to build on")
	}
	return name, data, nil
}

// runningStation is the station the simulation runs: its file, as the project has it or its
// collection, and the station as it was loaded and resolved, with what the project's exercise
// set up, such as pins swapped.
func (s *Service) runningStation() ([]byte, *model.Bundle, *model.Plan, error) {
	s.mu.Lock()
	name, b, bundle, e := s.proj.name, s.proj.def.Base, s.bundle, s.engine
	s.mu.Unlock()
	if name == "" || bundle == nil || e == nil {
		return nil, nil, nil, errNoProject
	}
	data, err := projectLibrary(name, b).ReadFile(path.Join("stations", bundle.Station.ID, "station.json"))
	return data, bundle, e.Plan(), err
}

// writeStation keeps a station built in a project as the project's own, while that project
// is open.
func (s *Service) writeStation(project string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proj.name != project {
		return fmt.Errorf("the project %s is no longer open", project)
	}
	return os.WriteFile(filepath.Join(projectDir(project), StationFile), data, 0o644)
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

// NewEmptyProject makes a project with an empty station of its own, standing on what a
// collection has to stand on, and opens it to be built on. Without a name it is called New
// station, with a number where that is taken.
func (s *Service) NewEmptyProject(name, collection string) (model.Scene, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		name = freeName("New station")
	case !projectName(name):
		return model.Scene{}, errName(name)
	case exists(projectDir(name)):
		return model.Scene{}, fmt.Errorf("there is a project %s already", name)
	}
	b := &BuilderService{ctx: context.Background(), emit: func(engine.Snapshot) {}, lib: Library()}
	if _, err := b.startEmpty(collection); err != nil {
		return model.Scene{}, err
	}
	d := b.draft.clone()
	b.halt()
	d.ID, d.Name = cmpOr(slug(name), "station"), name
	data, err := d.bytes()
	if err != nil {
		return model.Scene{}, err
	}
	if err := os.MkdirAll(projectDir(name), 0o755); err != nil {
		return model.Scene{}, err
	}
	if err := os.WriteFile(filepath.Join(projectDir(name), StationFile), data, 0o644); err != nil {
		return model.Scene{}, err
	}
	scene, err := s.open(name, ProjectDef{Name: name, Base: Base{Station: d.ID}})
	if err != nil {
		_ = os.RemoveAll(projectDir(name))
	}
	return scene, err
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
	// The app's own comes first, as it is the one to start with, unless it is hidden.
	if first := defaultStation(lib); first != "" {
		slices.SortStableFunc(out, func(a, b Example) int {
			switch {
			case a.Station == first && b.Station != first:
				return -1
			case b.Station == first && a.Station != first:
				return 1
			}
			return 0
		})
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

// ExportProjectTo writes a project's folder into a zip file: its project.json, station,
// exercise, picture and programs.
func ExportProjectTo(dir, to string) error {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		return err
	}
	return writeZip(to, files)
}

// writeZip writes files into a zip file, in the order of their names.
func writeZip(to string, files map[string][]byte) error {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write(files[name]); err != nil {
			return err
		}
	}
	if err := z.Close(); err != nil {
		return err
	}
	return os.WriteFile(to, buf.Bytes(), 0o644)
}

// FileOpenedEvent says that the system asked the app to open a file, such as a project file
// that was double-clicked; TakeOpenedFile gives it.
const FileOpenedEvent = "files:opened"

// FilesDroppedEvent carries the paths of files dropped onto the window to the frontend.
const FilesDroppedEvent = "files:dropped"

// MenuCommandEvent carries a command of the app's menu on a Mac to the frontend: import or
// close a project, or show the settings.
const MenuCommandEvent = "menu:command"
