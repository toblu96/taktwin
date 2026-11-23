package simapp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

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
		if !fs.ValidPath(name) || strings.Contains(f.Name, `\`) || !allowed(name) {
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
	if pf.def, err = parseProject(raw); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(from), err)
	}
	return pf, nil
}

// allowed says whether a project file may hold a file of that path.
func allowed(name string) bool {
	switch name {
	case ProjectFile, ThumbnailFile, StationFile, ExerciseFile, InstructionsFile:
		return true
	}
	if rest, ok := strings.CutPrefix(name, collectionsDir); ok {
		return !strings.Contains(rest, "/") && strings.HasSuffix(rest, model.PackageExtension)
	}
	return strings.HasPrefix(name, "programs/")
}

// all are the file's files by their paths in it, the collections it brings among them.
func (pf *projectFile) all() map[string][]byte {
	out := maps.Clone(pf.files)
	for name, data := range pf.collections {
		out[collectionsDir+name] = data
	}
	return out
}

// ProjectFileInfo is what opening a project file would do, for the app to ask about first.
type ProjectFileInfo struct {
	Name         string `json:"name"`
	ExerciseName string `json:"exerciseName,omitempty"`
	// Installs are the collections the file brings that are missing here, or older.
	Installs []Requirement `json:"installs"`
	// Missing are the collections it needs that are neither here, shown, nor in the file.
	Missing []Requirement `json:"missing"`
	// Unsaved is when a working copy was last changed that has changes the file lacks, such
	// as one left by a crash, as RFC 3339; "" without one.
	Unsaved string `json:"unsaved,omitempty"`
	// HasUnsaved says that there is such a working copy, also when its time is not known.
	HasUnsaved bool `json:"hasUnsaved,omitempty"`
}

// InspectProjectFile says what opening a project file would do, without doing it.
func (s *Service) InspectProjectFile(from string) (ProjectFileInfo, error) {
	pf, err := readProjectFileAt(from)
	if err != nil {
		return ProjectFileInfo{}, err
	}
	info := ProjectFileInfo{Name: nameOf(from)}
	if raw, ok := pf.files[ExerciseFile]; ok {
		var ex struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(raw, &ex)
		info.ExerciseName = ex.Name
	}
	info.Installs, info.Missing = pf.needs()
	s.mu.Lock()
	open := s.proj.key() != "" && sameFile(s.proj.key(), from)
	s.mu.Unlock()
	if !open {
		info.Unsaved, info.HasUnsaved = leftOver(pf.def.ID)
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
	// Restore goes on with the changes a working copy has that the file lacks; without it,
	// they are dropped.
	Restore bool `json:"restore"`
}

// OpenProjectFile opens a .taktwin-project file where it is, such as one in the user
// folder's projects, one dropped onto the window or one double-clicked, after installing
// the collections it brings when asked to. Its changes are saved into it. A file that does
// not open, such as one whose station is missing, is listed all the same.
func (s *Service) OpenProjectFile(from string, opts OpenOptions) (ProjectInfo, error) {
	pf, err := readProjectFileAt(from)
	if err != nil {
		return ProjectInfo{}, err
	}
	if opts.Install {
		if err := pf.install(); err != nil {
			return ProjectInfo{}, err
		}
	}
	if _, err := s.openFile(from, pf, opts.Restore); err != nil {
		return ProjectInfo{Name: nameOf(from), Path: from}, err
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
func (s *Service) ExportExercise(file string, programs bool) (string, error) {
	files, err := s.projectFiles(file)
	if err != nil {
		return "", err
	}
	def, err := parseProject(files[ProjectFile])
	if err != nil {
		return "", err
	}
	name := nameOf(file)
	if ex, err := projectLibrary(mapReader(files), def.Base).Exercise(def.Base.Exercise); err == nil && def.Base.Exercise != "" {
		name = cmpOr(freeFileName(ex.Name), name)
	}
	to, err := chooseSaveFile("Hand out the exercise", name)
	if err != nil || to == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(to), ProjectExtension) {
		to += ProjectExtension
	}
	return to, exportExercise(files, to, programs)
}

// freeFileName makes a name fit for a file, or "" if nothing of it is.
func freeFileName(name string) string {
	name = strings.TrimLeft(strings.TrimSpace(forbiddenName.ReplaceAllString(name, "-")), ".")
	if !projectName(name) {
		return ""
	}
	return name
}

// ExportExerciseTo writes a project file on an exercise into a file to hand out as one: its
// station and exercise as copies, the instructions, the station's settings, the programs if
// asked to as a start, and the collections it is built from that students may not have. It
// leaves out how the PLCs are connected and how checks went.
func ExportExerciseTo(file, to string, programs bool) error {
	pf, err := readProjectFileAt(file)
	if err != nil {
		return err
	}
	return exportExercise(pf.all(), to, programs)
}

func exportExercise(files map[string][]byte, to string, programs bool) error {
	def, err := parseProject(files[ProjectFile])
	if err != nil {
		return err
	}
	id := def.Base.Exercise
	if id == "" {
		return errors.New("only a project on an exercise can be handed out")
	}
	read := mapReader(files)
	lib := projectLibrary(read, def.Base)
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
	raw, err := projectJSON(ProjectDef{ID: newID(), Base: def.Base, Parameters: def.Parameters, Requires: requires})
	if err != nil {
		return err
	}
	out := map[string][]byte{ProjectFile: raw, StationFile: station, ExerciseFile: exercise}
	if data, err := read(InstructionsFile); err == nil {
		out[InstructionsFile] = data
	} else if data, err := lib.ReadFile(path.Join("exercises", id, InstructionsFile)); err == nil {
		out[InstructionsFile] = data
	}
	if data, err := read(ThumbnailFile); err == nil {
		out[ThumbnailFile] = data
	}
	if programs {
		for name, data := range files {
			if strings.HasPrefix(name, "programs/") {
				out[name] = data
			}
		}
	}
	for _, r := range requires {
		data, ok, err := collectionPackage(r.Collection)
		if err != nil {
			return err
		}
		if ok {
			out[collectionsDir+r.Collection+model.PackageExtension] = data
		}
	}
	return writeZipOver(to, out)
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
		data, err := zipped(files)
		return data, err == nil, err
	}
	return nil, false, nil
}

// Instructions are what the open project's exercise asks for, in Markdown: the project's own
// instructions.md, or else the one beside the exercise in its collection; "" without.
func (s *Service) Instructions() string {
	s.mu.Lock()
	dir, b := s.proj.dir(), s.proj.def.Base
	s.mu.Unlock()
	if dir == "" {
		return ""
	}
	if data, err := os.ReadFile(filepath.Join(dir, InstructionsFile)); err == nil {
		return string(data)
	}
	if b.Exercise != "" {
		if data, err := projectLibrary(dirReader(dir), b).ReadFile(path.Join("exercises", b.Exercise, InstructionsFile)); err == nil {
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
