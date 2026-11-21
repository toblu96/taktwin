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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/model"
)

const (
	// ExerciseFile is a project's own copy of its exercise, which a file handed out brings.
	ExerciseFile = "exercise.json"
	// InstructionsFile is what a project's exercise asks for, in Markdown: the project's own,
	// or else the one beside the exercise in its collection.
	InstructionsFile = "instructions.md"
	// collectionsDir is where a project file keeps the collections it brings, each as a
	// .taktwin file.
	collectionsDir = "collections/"
)

// projectFile is what a .taktwin-project file holds.
type projectFile struct {
	def ProjectDef
	// files are those of the project's folder, by their path in it.
	files map[string][]byte
	// collections are the collection files it brings, by name.
	collections map[string][]byte
}

// readProjectFile reads a .taktwin-project file. It holds data only: project.json, the
// project's station, exercise, instructions and picture, the sources of programs, and the
// collections it brings. Anything else, a path out of it, or too much data is refused.
func readProjectFile(from string) (*projectFile, error) {
	data, err := os.ReadFile(from)
	if err != nil {
		return nil, err
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%s is no project file: %w", filepath.Base(from), err)
	}
	pf := &projectFile{files: map[string][]byte{}, collections: map[string][]byte{}}
	var total int64
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(f.Name)
		if !fs.ValidPath(name) || strings.Contains(f.Name, `\`) || !projectPath(name) {
			return nil, fmt.Errorf("%s holds %s, which no project has", filepath.Base(from), f.Name)
		}
		total += int64(f.UncompressedSize64)
		if total > maxProjectBytes {
			return nil, fmt.Errorf("%s is larger than a project can be", filepath.Base(from))
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(io.LimitReader(r, maxProjectBytes+1))
		r.Close()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(name, collectionsDir) {
			pf.collections[strings.TrimPrefix(name, collectionsDir)] = content
		} else {
			pf.files[name] = content
		}
	}
	raw, ok := pf.files[ProjectFile]
	if !ok {
		return nil, fmt.Errorf("%s is no project: it has no %s", filepath.Base(from), ProjectFile)
	}
	if err := json.Unmarshal(raw, &pf.def); err != nil {
		return nil, fmt.Errorf("%s: %w", ProjectFile, err)
	}
	if pf.def.Format > ProjectFormat {
		return nil, fmt.Errorf("the project is of a newer version of Taktwin (format %d)", pf.def.Format)
	}
	return pf, nil
}

// projectPath says whether a project file may hold a file of that path.
func projectPath(name string) bool {
	switch name {
	case ProjectFile, ThumbnailFile, StationFile, ExerciseFile, InstructionsFile:
		return true
	}
	if rest, ok := strings.CutPrefix(name, collectionsDir); ok {
		return !strings.Contains(rest, "/") && strings.HasSuffix(rest, model.PackageExtension)
	}
	return strings.HasPrefix(name, "programs/")
}

// name is what a project from the file is called: one handed out after its project, one
// handed in after its file, which is how a teacher tells students' work apart.
func (pf *projectFile) name(from string) string {
	file := strings.TrimSuffix(filepath.Base(from), filepath.Ext(from))
	if pf.def.Handout {
		return cmpOr(strings.TrimSpace(pf.def.Name), file)
	}
	return cmpOr(file, strings.TrimSpace(pf.def.Name))
}

// importAs unpacks the file into a project of the user folder of its own, with a number
// where its name is taken, and gives the name. The collections it brings are not part of it.
func (pf *projectFile) importAs(from string) (string, error) {
	name := freeName(pf.name(from))
	dir := projectDir(name)
	for _, n := range slices.Sorted(func(yield func(string) bool) {
		for n := range pf.files {
			if !yield(n) {
				return
			}
		}
	}) {
		target := filepath.Join(dir, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, pf.files[n], 0o644); err != nil {
			return "", err
		}
	}
	def := pf.def
	def.Name, def.FromHandout, def.Handout = name, pf.def.Handout, false
	return name, writeProject(dir, def)
}

// ImportProjectFile unpacks a .taktwin-project file into a project of the user folder of its
// own, and gives its name.
func ImportProjectFile(from string) (string, error) {
	pf, err := readProjectFile(from)
	if err != nil {
		return "", err
	}
	return pf.importAs(from)
}

// ProjectFileInfo is what opening a project file would do, for the app to ask about first.
type ProjectFileInfo struct {
	Name         string `json:"name"`
	Handout      bool   `json:"handout"`
	ExerciseName string `json:"exerciseName,omitempty"`
	// Existing is the project made from the same exercise handed out before, which can be
	// gone on with instead.
	Existing string `json:"existing,omitempty"`
	// Installs are the collections the file brings that are missing here, or older.
	Installs []Requirement `json:"installs"`
	// Missing are the collections it needs that are neither here, shown, nor in the file.
	Missing []Requirement `json:"missing"`
}

// InspectProjectFile says what opening a project file would do, without doing it.
func (s *Service) InspectProjectFile(from string) (ProjectFileInfo, error) {
	pf, err := readProjectFileAt(from)
	if err != nil {
		return ProjectFileInfo{}, err
	}
	info := ProjectFileInfo{Name: pf.name(from), Handout: pf.def.Handout}
	if raw, ok := pf.files[ExerciseFile]; ok {
		var ex struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(raw, &ex)
		info.ExerciseName = ex.Name
	}
	info.Installs, info.Missing = pf.needs()
	if pf.def.Handout {
		info.Existing = handedOut(pf.def.Base.Exercise)
	}
	return info, nil
}

func readProjectFileAt(from string) (*projectFile, error) {
	if !strings.EqualFold(filepath.Ext(from), ProjectExtension) {
		return nil, fmt.Errorf("%s is no project file: those end in %s", filepath.Base(from), ProjectExtension)
	}
	return readProjectFile(from)
}

// OpenOptions are the answers to what InspectProjectFile asks.
type OpenOptions struct {
	// Install installs the collections the file brings that are missing here, or older.
	Install bool `json:"install"`
	// Continue opens the project made from the same exercise handed out before instead.
	Continue bool `json:"continue"`
}

// OpenProjectFile opens a .taktwin-project file, such as one dropped onto the window or
// double-clicked: as a project of its own, after installing the collections it brings when
// asked to, or as the project made from the same handout before. A project that is imported
// but does not open, such as one whose station is missing, is listed all the same.
func (s *Service) OpenProjectFile(from string, opts OpenOptions) (ProjectInfo, error) {
	pf, err := readProjectFileAt(from)
	if err != nil {
		return ProjectInfo{}, err
	}
	if existing := handedOut(pf.def.Base.Exercise); opts.Continue && pf.def.Handout && existing != "" {
		_, err := s.OpenProject(existing)
		return s.Project(), err
	}
	if opts.Install {
		if err := pf.install(); err != nil {
			return ProjectInfo{}, err
		}
	}
	name, err := pf.importAs(from)
	if err != nil {
		return ProjectInfo{}, err
	}
	if _, err := s.OpenProject(name); err != nil {
		return ProjectInfo{Name: name}, fmt.Errorf("imported as %s, but it does not open: %w", name, err)
	}
	return s.Project(), nil
}

// ChooseProjectFile lets the user pick a .taktwin-project file in the system's dialog, to
// open with OpenProjectFile. Cancelling gives an empty path.
func (s *Service) ChooseProjectFile() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to choose a file in")
	}
	return app.Dialog.OpenFile().SetTitle("Open a project").AddFilter("Taktwin project", "*"+ProjectExtension).
		CanChooseFiles(true).PromptForSingleSelection()
}

// handedOut is the project made from an exercise handed out, the latest if there are more.
func handedOut(exercise string) string {
	if exercise == "" {
		return ""
	}
	items, _ := os.ReadDir(filepath.Join(UserFolder(), ProjectsFolder))
	best, latest := "", time.Time{}
	for _, it := range items {
		if !it.IsDir() || !projectName(it.Name()) {
			continue
		}
		def, ok, _ := readProject(projectDir(it.Name()))
		fi, err := os.Stat(filepath.Join(projectDir(it.Name()), ProjectFile))
		if ok && err == nil && def.FromHandout && def.Base.Exercise == exercise && fi.ModTime().After(latest) {
			best, latest = it.Name(), fi.ModTime()
		}
	}
	return best
}

// installed are the collections here by id, shown or hidden: the version of the first of
// each, and whether it is hidden.
func installed() map[string]Requirement {
	hidden := readSettings().Hidden
	out := map[string]Requirement{}
	for _, src := range sources() {
		id := src.collection.ID
		if src.err != nil || id == "" {
			continue
		}
		if _, seen := out[id]; !seen {
			out[id] = Requirement{Collection: id, Name: src.collection.Name, Installed: src.collection.Version, Hidden: src.kind != SourceBase && slices.Contains(hidden, id)}
		}
	}
	return out
}

// needs says which collections the file brings that are missing here or older, and which it
// needs that are neither here and shown nor in the file.
func (pf *projectFile) needs() (installs, missing []Requirement) {
	here := installed()
	installs, missing = []Requirement{}, []Requirement{}
	brought := map[string]bool{}
	for _, file := range slices.Sorted(func(yield func(string) bool) {
		for n := range pf.collections {
			if !yield(n) {
				return
			}
		}
	}) {
		c, err := collectionOf(pf.collections[file])
		if err != nil {
			continue
		}
		brought[c.ID] = true
		have, ok := here[c.ID]
		if !ok || newer(c.Version, have.Installed) {
			installs = append(installs, Requirement{Collection: c.ID, Name: c.Name, Version: c.Version, Installed: have.Installed})
		}
	}
	for _, r := range pf.def.Requires {
		have, ok := here[r.Collection]
		switch {
		case brought[r.Collection] && !have.Hidden:
		case !ok:
			missing = append(missing, r)
		case have.Hidden:
			r.Installed, r.Hidden = have.Installed, true
			missing = append(missing, r)
		}
	}
	return installs, missing
}

// install installs the collections the file brings that are missing here, or older.
func (pf *projectFile) install() error {
	installs, _ := pf.needs()
	for _, r := range installs {
		for _, data := range pf.collections {
			if c, err := collectionOf(data); err != nil || c.ID != r.Collection {
				continue
			}
			tmp, err := os.CreateTemp("", "taktwin-*"+model.PackageExtension)
			if err != nil {
				return err
			}
			_, err = tmp.Write(data)
			tmp.Close()
			if err == nil {
				_, err = InstallCollection(tmp.Name())
			}
			os.Remove(tmp.Name())
			if err != nil {
				return fmt.Errorf("installing %s: %w", cmpOr(r.Name, r.Collection), err)
			}
			break
		}
	}
	return nil
}

func collectionOf(data []byte) (model.CollectionDef, error) {
	layer, err := model.ReadPackage(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return model.CollectionDef{}, err
	}
	c, ok, err := model.ReadCollection(layer)
	if err == nil && !ok {
		err = errors.New("no collection")
	}
	return c, err
}

// newer says whether version a is newer than b, comparing their numbers one by one, such as
// 1.10.0 after 1.9.2. One without a version is not newer.
func newer(a, b string) bool {
	if a == "" {
		return false
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// ExportExercise writes a project on an exercise into a file to hand out, which the user
// picks in the system's dialog; see ExportExerciseTo. Cancelling gives an empty path.
func (s *Service) ExportExercise(name string, programs bool) (string, error) {
	if !projectName(name) || !exists(filepath.Join(projectDir(name), ProjectFile)) {
		return "", fmt.Errorf("there is no project %s", name)
	}
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to choose a file in")
	}
	to, err := app.Dialog.SaveFile().SetMessage("Hand out the exercise").SetFilename(name+ProjectExtension).
		AddFilter("Taktwin project", "*"+ProjectExtension).PromptForSingleSelection()
	if err != nil || to == "" {
		return "", err
	}
	return to, ExportExerciseTo(name, to, programs)
}

// ExportExerciseTo writes a project on an exercise into a file to hand out as one: its
// station and exercise as copies, the instructions, the station's settings, the programs if
// asked to as a start, and the collections it is built from that students may not have. It
// leaves out how the PLCs are connected and how checks went.
func ExportExerciseTo(name, to string, programs bool) error {
	def, ok, err := readProject(projectDir(name))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("there is no project %s", name)
	}
	id := def.Base.Exercise
	if id == "" {
		return errors.New("only a project on an exercise can be handed out")
	}
	lib := projectLibrary(name, def.Base)
	exercise, err := lib.ReadFile(path.Join("exercises", id, "exercise.json"))
	if err != nil {
		return err
	}
	station, err := lib.ReadFile(path.Join("stations", def.Base.Station, "station.json"))
	if err != nil {
		return err
	}
	bundle, _, err := lib.LoadExercise(id)
	if err != nil {
		return err
	}
	requires, own := requirements(lib, bundle)
	if len(own) > 0 {
		return fmt.Errorf("the station uses %s from your own folder, which students do not have: put them into a collection first", strings.Join(own, ", "))
	}
	out := ProjectDef{Format: ProjectFormat, Name: name, Base: def.Base, Parameters: def.Parameters, Handout: true, Requires: requires}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	files := map[string][]byte{ProjectFile: append(raw, '\n'), StationFile: station, ExerciseFile: exercise}
	if data, err := os.ReadFile(filepath.Join(projectDir(name), InstructionsFile)); err == nil {
		files[InstructionsFile] = data
	} else if data, err := lib.ReadFile(path.Join("exercises", id, InstructionsFile)); err == nil {
		files[InstructionsFile] = data
	}
	if data, err := os.ReadFile(filepath.Join(projectDir(name), ThumbnailFile)); err == nil {
		files[ThumbnailFile] = data
	}
	if programs {
		dir := filepath.Join(projectDir(name), "programs")
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err
			}
			rel, _ := filepath.Rel(projectDir(name), p)
			data, err := os.ReadFile(p)
			files[filepath.ToSlash(rel)] = data
			return err
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	for _, r := range requires {
		data, ok, err := collectionPackage(r.Collection)
		if err != nil {
			return err
		}
		if ok {
			files[collectionsDir+r.Collection+model.PackageExtension] = data
		}
	}
	return writeZip(to, files)
}

// requirements are the collections a station is built from, besides the base every app
// has, with their names and versions; own lists what comes from the user's own folder.
func requirements(lib *model.Library, bundle *model.Bundle) (out []Requirement, own []string) {
	ids := map[string]bool{}
	add := func(collection, what string) {
		switch collection {
		case "":
			if !slices.Contains(own, what) {
				own = append(own, what)
			}
		case "base":
		default:
			ids[collection] = true
		}
	}
	for id := range bundle.Modules {
		add(lib.ModuleCollection(id), id)
	}
	for id := range bundle.Parts {
		add(lib.FileCollection(path.Join("parts", id+".json")), id)
	}
	if w := bundle.Station.Workpieces; w != "" {
		add(lib.FileCollection(path.Join("workpieces", w+".json")), w)
	}
	for _, id := range slices.Sorted(func(yield func(string) bool) {
		for id := range ids {
			if !yield(id) {
				return
			}
		}
	}) {
		c, _ := lib.Collection(id)
		out = append(out, Requirement{Collection: id, Name: c.Name, Version: c.Version})
	}
	slices.Sort(own)
	return out, own
}

// collectionPackage is a collection as a .taktwin file, to hand out with an exercise: one
// imported or given for development. The app's own needs none, as every app has it.
func collectionPackage(id string) ([]byte, bool, error) {
	for _, src := range sources() {
		if src.collection.ID != id || src.err != nil || src.path == "" || (src.kind != SourceInstalled && src.kind != SourceDevelopment) {
			continue
		}
		info, err := os.Stat(src.path)
		if err != nil {
			return nil, false, err
		}
		if !info.IsDir() {
			data, err := os.ReadFile(src.path)
			return data, err == nil, err
		}
		files := map[string][]byte{}
		err = filepath.WalkDir(src.path, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".") {
				return err
			}
			rel, _ := filepath.Rel(src.path, p)
			data, err := os.ReadFile(p)
			files[filepath.ToSlash(rel)] = data
			return err
		})
		if err != nil {
			return nil, false, err
		}
		tmp, err := os.CreateTemp("", "taktwin-*"+model.PackageExtension)
		if err != nil {
			return nil, false, err
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		if err := writeZip(tmp.Name(), files); err != nil {
			return nil, false, err
		}
		data, err := os.ReadFile(tmp.Name())
		return data, err == nil, err
	}
	return nil, false, nil
}

// Instructions are what the open project's exercise asks for, in Markdown: the project's own
// instructions.md, or else the one beside the exercise in its collection; "" without.
func (s *Service) Instructions() string {
	s.mu.Lock()
	name, b := s.proj.name, s.proj.def.Base
	s.mu.Unlock()
	if name == "" {
		return ""
	}
	if data, err := os.ReadFile(filepath.Join(projectDir(name), InstructionsFile)); err == nil {
		return string(data)
	}
	if b.Exercise != "" {
		if data, err := projectLibrary(name, b).ReadFile(path.Join("exercises", b.Exercise, InstructionsFile)); err == nil {
			return string(data)
		}
	}
	return ""
}

// Opened keeps a file the system asked the app to open, such as a project file that was
// double-clicked, until the window takes it: it may come before the window is there.
func (s *Service) Opened(file string) {
	s.mu.Lock()
	s.opened = file
	s.mu.Unlock()
	if app := application.Get(); app != nil {
		app.Event.Emit(FileOpenedEvent, file)
	}
}

// TakeOpenedFile gives the file the system asked the app to open, once, or "".
func (s *Service) TakeOpenedFile() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.opened
	s.opened = ""
	return file
}
