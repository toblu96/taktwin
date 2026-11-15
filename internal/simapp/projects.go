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
	// unsavedFolder keeps the work on a station or exercise that is in no named project yet,
	// one folder each.
	unsavedFolder = ".unsaved"
	// maxProjectBytes is the most an imported project may unpack to.
	maxProjectBytes = 64 << 20
)

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

// project is the project the simulation works in.
type project struct {
	// key is its folder below projects/: its name, or .unsaved/<station> for unsaved work.
	key string
	def ProjectDef
}

func (p project) dir() string {
	return filepath.Join(UserFolder(), ProjectsFolder, filepath.FromSlash(p.key))
}

// saved says whether the project has a name, as opposed to unsaved work.
func (p project) saved() bool { return p.key != "" && !strings.HasPrefix(p.key, unsavedFolder+"/") }

// unsavedKey is the folder of the unsaved work on a base.
func unsavedKey(b Base) string {
	if b.Exercise != "" {
		return unsavedFolder + "/exercise-" + b.Exercise
	}
	return unsavedFolder + "/" + b.Station
}

// ProjectInfo is a project as the app lists it.
type ProjectInfo struct {
	Name string `json:"name"`
	// Saved is set for a project with a name; unsaved work has none.
	Saved       bool   `json:"saved"`
	Station     string `json:"station"`
	StationName string `json:"stationName"`
	Exercise    string `json:"exercise,omitempty"`
	// Folder is where it is kept.
	Folder string `json:"folder"`
	// Modified is when its file was last written, as RFC 3339.
	Modified string `json:"modified,omitempty"`
}

func (s *Service) projectInfo(p project) ProjectInfo {
	info := ProjectInfo{Name: p.def.Name, Saved: p.saved(), Station: p.def.Base.Station, Exercise: p.def.Base.Exercise, Folder: p.dir()}
	info.StationName = Library().StationName(p.def.Base.Station)
	if fi, err := os.Stat(filepath.Join(p.dir(), ProjectFile)); err == nil {
		info.Modified = fi.ModTime().UTC().Format(time.RFC3339)
	}
	return info
}

// Project says which project the simulation works in.
func (s *Service) Project() ProjectInfo {
	s.mu.Lock()
	p := s.proj
	s.mu.Unlock()
	return s.projectInfo(p)
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

// saveProject writes the project's file, under s.mu.
func (s *Service) saveProject() error {
	if s.proj.key == "" {
		return nil
	}
	dir := s.proj.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.proj.def.Format = ProjectFormat
	data, err := json.MarshalIndent(s.proj.def, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ProjectFile), append(data, '\n'), 0o644)
}

// enter makes a project the simulation's, whose base it has just loaded, and gives the
// station its settings. Unsaved work on a station takes over the programs the station had
// before projects.
func (s *Service) enter(key string, def ProjectDef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.proj.key
	s.proj = project{key: key, def: def}
	if !s.proj.saved() && def.Base.Exercise == "" {
		if err := adoptPrograms(def.Base.Station, s.proj.dir()); err != nil {
			return err
		}
	}
	for id, v := range def.Parameters {
		// A setting the station no longer has is left out.
		_ = s.engine.SetParameter(id, v)
	}
	_ = old
	return s.saveProject()
}

// adoptPrograms copies the programs a station had before projects, in programs/<station>/,
// into the unsaved work on it, the first time.
func adoptPrograms(station, dir string) error {
	from := filepath.Join(UserFolder(), "programs", station)
	to := filepath.Join(dir, "programs")
	if _, err := os.Stat(to); err == nil {
		return nil
	}
	if _, err := os.Stat(from); err != nil {
		return nil
	}
	return copyTree(from, to)
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

// OpenProject opens a project by name: its station, its settings, and its PLCs' connections,
// ready to connect.
func (s *Service) OpenProject(name string) (model.Scene, error) {
	if !projectName(name) {
		return model.Scene{}, fmt.Errorf("%q is no project", name)
	}
	p := project{key: name}
	def, ok, err := readProject(p.dir())
	if err != nil {
		return model.Scene{}, err
	}
	if !ok {
		return model.Scene{}, fmt.Errorf("there is no project %s", name)
	}
	if _, err := s.openWork(def.Base); err != nil {
		return model.Scene{}, err
	}
	def.Name = name
	if err := s.enter(name, def); err != nil {
		return model.Scene{}, err
	}
	return s.Scene(), nil
}

// openWork loads a base into the simulation without entering its unsaved work.
func (s *Service) openWork(b Base) (model.Scene, error) {
	if b.Exercise != "" {
		bundle, _, err := Library().LoadExercise(b.Exercise)
		if err != nil {
			return model.Scene{}, err
		}
		bundle, e, err := start(bundle)
		if err != nil {
			return model.Scene{}, err
		}
		return s.replace(bundle, e, b.Exercise), nil
	}
	bundle, e, err := load(b.Station)
	if err != nil {
		return model.Scene{}, err
	}
	return s.replace(bundle, e, ""), nil
}

// enterUnsaved makes the unsaved work on what the simulation has just loaded the project.
func (s *Service) enterUnsaved(b Base) error {
	key := unsavedKey(b)
	def, _, err := readProject(project{key: key}.dir())
	if err != nil {
		return err
	}
	def.Name, def.Base = "", b
	return s.enter(key, def)
}

var forbiddenName = regexp.MustCompile(`[/\\:*?"<>|]`)

// projectName says whether a name can be that of a project's folder.
func projectName(name string) bool {
	return name != "" && len(name) <= 80 && strings.TrimSpace(name) == name && !strings.HasPrefix(name, ".") &&
		!forbiddenName.MatchString(name) && filepath.Base(name) == name
}

// SaveProjectAs keeps the project the simulation works in under a name, which it then works
// in: its programs, connections and settings.
func (s *Service) SaveProjectAs(name string) (ProjectInfo, error) {
	name = strings.TrimSpace(name)
	if !projectName(name) {
		return ProjectInfo{}, fmt.Errorf("%q cannot be a project's name: up to 80 characters, without / \\ : * ? \" < > | and not starting with a dot", name)
	}
	to := project{key: name}
	if _, err := os.Stat(to.dir()); err == nil {
		return ProjectInfo{}, fmt.Errorf("there is a project %s already", name)
	}
	s.mu.Lock()
	from := s.proj
	s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(to.dir()), 0o755); err != nil {
		return ProjectInfo{}, err
	}
	if _, err := os.Stat(from.dir()); err == nil {
		if err := copyTree(from.dir(), to.dir()); err != nil {
			return ProjectInfo{}, err
		}
	}
	s.mu.Lock()
	s.proj = project{key: name, def: from.def}
	s.proj.def.Name = name
	// The soft PLCs go on running with their programs, now the project's.
	softPLCs.Move(from.key, name)
	err := s.saveProject()
	p := s.proj
	s.mu.Unlock()
	if err != nil {
		return ProjectInfo{}, err
	}
	if !from.saved() {
		// The unsaved work is in the project now.
		_ = os.RemoveAll(from.dir())
	}
	return s.projectInfo(p), nil
}

// Projects lists the projects of the user folder, the latest first.
func (s *Service) Projects() []ProjectInfo {
	out := []ProjectInfo{}
	items, err := os.ReadDir(filepath.Join(UserFolder(), ProjectsFolder))
	if err != nil {
		return out
	}
	for _, it := range items {
		if !it.IsDir() || !projectName(it.Name()) {
			continue
		}
		p := project{key: it.Name()}
		def, ok, err := readProject(p.dir())
		if err != nil || !ok {
			continue
		}
		p.def = def
		p.def.Name = it.Name()
		out = append(out, s.projectInfo(p))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out
}

// DeleteProject removes a project by name. The one the simulation works in cannot be.
func (s *Service) DeleteProject(name string) error {
	if !projectName(name) {
		return fmt.Errorf("%q is no project", name)
	}
	s.mu.Lock()
	current := s.proj.key
	s.mu.Unlock()
	if current == name {
		return errors.New("the project is open: open another station or project first")
	}
	p := project{key: name}
	if _, ok, _ := readProject(p.dir()); !ok {
		return fmt.Errorf("there is no project %s", name)
	}
	return os.RemoveAll(p.dir())
}

// PlannedPLC is how the project means one of the station's PLCs, by its module, to be
// connected, which the PLC panel offers to connect.
func (s *Service) PlannedPLC(plc string) PLCConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
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

// ExportProject writes the project the simulation works in into a .taktwin-project file the
// user picks in the system's dialog. Cancelling gives an empty path.
func (s *Service) ExportProject() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to choose a file in")
	}
	s.mu.Lock()
	p := s.proj
	s.mu.Unlock()
	name := cmpOr(p.def.Name, p.def.Base.Station)
	to, err := app.Dialog.SaveFile().SetMessage("Export the project").SetFilename(name+ProjectExtension).
		AddFilter("Taktwin project", "*"+ProjectExtension).PromptForSingleSelection()
	if err != nil || to == "" {
		return "", err
	}
	return to, ExportProjectTo(p.dir(), to)
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

// ImportProject lets the user pick a .taktwin-project file in the system's dialog, unpacks it
// into a project of its own and opens it. Cancelling gives no name.
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
		return ProjectInfo{}, err
	}
	return s.Project(), nil
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
	base := cmpOr(strings.TrimSpace(def.Name), strings.TrimSuffix(filepath.Base(from), ProjectExtension))
	if !projectName(base) {
		base = "Imported project"
	}
	name := base
	for n := 2; ; n++ {
		if _, err := os.Stat(project{key: name}.dir()); errors.Is(err, fs.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s (%d)", base, n)
	}
	dir := project{key: name}.dir()
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

// NewProject makes a project of a station under a name and opens it, as a programming tool
// makes a new project from a device: its settings as the station has them, no PLC
// connected, and the template program once a PLC's program is opened.
func (s *Service) NewProject(name, station string) (model.Scene, error) {
	name = strings.TrimSpace(name)
	if !projectName(name) {
		return model.Scene{}, fmt.Errorf("%q cannot be a project's name: up to 80 characters, without / \\ : * ? \" < > | and not starting with a dot", name)
	}
	if _, err := os.Stat(project{key: name}.dir()); err == nil {
		return model.Scene{}, fmt.Errorf("there is a project %s already", name)
	}
	if _, err := s.openWork(Base{Station: station}); err != nil {
		return model.Scene{}, err
	}
	if err := s.enter(name, ProjectDef{Name: name, Base: Base{Station: station}}); err != nil {
		return model.Scene{}, err
	}
	return s.Scene(), nil
}

// CloseProject leaves a named project for the unsaved work on its station or exercise.
func (s *Service) CloseProject() (model.Scene, error) {
	s.mu.Lock()
	base := s.proj.def.Base
	s.mu.Unlock()
	if _, err := s.openWork(base); err != nil {
		return model.Scene{}, err
	}
	if err := s.enterUnsaved(base); err != nil {
		return model.Scene{}, err
	}
	return s.Scene(), nil
}

// ProjectCommandEvent carries a command of the app's File menu to the frontend: new, open,
// save, saveAs, close, export or import.
const ProjectCommandEvent = "project:command"
