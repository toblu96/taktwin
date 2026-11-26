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
	// ProjectsFolder is where the user folder keeps new projects, as files, and the settings
	// folder the working copies.
	ProjectsFolder = "projects"
	// ProjectFile is a project's own file inside it.
	ProjectFile = "project.json"
	// ProjectExtension is that of a project file: a zip of the project's files.
	ProjectExtension = ".taktwin-project"
	// ProjectFormat is the format of project files this build reads and writes.
	ProjectFormat = 2
	// StationFile is the project's own station, which it has once it is built or changed in
	// the project. Without it, the project's station is that of a collection.
	StationFile = "station.json"
	// ThumbnailFile is a picture of the project's station as it was last closed.
	ThumbnailFile = "thumbnail.jpg"
	// maxThumbnailBytes is the largest thumbnail a project keeps.
	maxThumbnailBytes = 1 << 20
	// maxProjectBytes is the most a project file may unpack to.
	maxProjectBytes = 64 << 20
	// maxRecent is how many project files the recent list keeps.
	maxRecent = 50
)

// errNoProject is what the simulation says while no project is open.
var errNoProject = errors.New("no project is open")

// ProjectDef is the content of project.json: what a project is based on, and what it adds.
// The project file also holds the programs of its soft PLCs, in programs/<plc>/.
type ProjectDef struct {
	Format int `json:"format"`
	// ID is made with the project and kept when its file is renamed or moved; a copy saved by
	// the app gets one of its own. It names the project's working copy.
	ID   string `json:"id"`
	Base Base   `json:"base"`
	// PLCs are how the station's PLCs are to be connected, by module, "" for the first. A
	// project restores them without connecting.
	PLCs map[string]PLCConfig `json:"plcs,omitempty"`
	// Parameters are the station's settings as set in the simulation, by id.
	Parameters map[string]float64 `json:"parameters,omitempty"`
	// LastCheck is how the last check of an exercise that ran to its end went.
	LastCheck *LastCheck `json:"lastCheck,omitempty"`
	// Collections are those the project carries, each as a .taktwin file in collections/,
	// in the version it was made with or last updated to: every collection its station,
	// exercise and their example programs come from, the app's own among them.
	Collections []Requirement `json:"collections,omitempty"`
	// Original is when the project's original, under original/, was set, as RFC 3339.
	Original string `json:"original,omitempty"`
	// ExerciseHash sums the exercise and its station as the original has them, for a project
	// on an exercise, to tell when they were changed since.
	ExerciseHash string `json:"exerciseHash,omitempty"`
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
	// name is that of its file, without the extension.
	name string
	def  ProjectDef
	doc  *document
	// teacher is the exercise and its station as the teacher's collections have them, in front
	// of the project's own while a teacher checks a hand-in whose copies differ; never saved.
	teacher fs.FS
}

// dir is the project's working copy.
func (p project) dir() string {
	if p.doc == nil {
		return ""
	}
	return p.doc.dir
}

// key tells the project apart from any other: its file.
func (p project) key() string {
	if p.doc == nil {
		return ""
	}
	return p.doc.path
}

// projectsDir is where new projects are made.
func projectsDir() string { return filepath.Join(UserFolder(), ProjectsFolder) }

// nameOf is a project's name: that of its file, without the extension.
func nameOf(file string) string {
	base := filepath.Base(file)
	return base[:len(base)-len(filepath.Ext(base))]
}

// ProjectInfo is a project as the app lists it.
type ProjectInfo struct {
	// Name is that of its file, without the extension.
	Name string `json:"name"`
	// Path is its file.
	Path         string `json:"path"`
	Station      string `json:"station"`
	StationName  string `json:"stationName"`
	Exercise     string `json:"exercise,omitempty"`
	ExerciseName string `json:"exerciseName,omitempty"`
	// Modified is when its file was last written, as RFC 3339.
	Modified string `json:"modified,omitempty"`
	// Problem says why it does not open, such as its station's collection being hidden or gone.
	Problem string `json:"problem,omitempty"`
	// Thumbnail is a picture of its station as it was last closed, as a data URL.
	Thumbnail string     `json:"thumbnail,omitempty"`
	LastCheck *LastCheck `json:"lastCheck,omitempty"`
	// InFolder says that the file is in the user folder's projects, where new ones are made;
	// the others are listed because they were opened.
	InFolder bool `json:"inFolder,omitempty"`
	// Missing says that the file is no longer where it was opened from.
	Missing bool `json:"missing,omitempty"`
	// Updates are the collections it carries of which a newer version is installed here,
	// with the version it has and the one installed.
	Updates []Requirement `json:"updates,omitempty"`
	// ExerciseChanged says that its exercise or the exercise's station is no longer as its
	// original has them, such as when it was handed out.
	ExerciseChanged bool `json:"exerciseChanged,omitempty"`
}

// reader reads a project's files by their paths in it.
type reader func(name string) ([]byte, error)

func dirReader(dir string) reader {
	return func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))) }
}

func mapReader(files map[string][]byte) reader {
	return func(name string) ([]byte, error) {
		if data, ok := files[name]; ok {
			return data, nil
		}
		return nil, fs.ErrNotExist
	}
}

func projectInfo(srcs []source, file string, read reader, def ProjectDef) ProjectInfo {
	info := ProjectInfo{Name: nameOf(file), Path: file, Station: def.Base.Station, Exercise: def.Base.Exercise, InFolder: inProjectsDir(file)}
	lib := libraryFrom(srcs, projectLayers(read, def))
	info.Updates = updatesOf(srcs, def)
	info.ExerciseChanged = def.ExerciseHash != "" && exerciseHash(read) != def.ExerciseHash
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
	if fi, err := os.Stat(file); err == nil {
		info.Modified = fi.ModTime().UTC().Format(time.RFC3339)
	}
	if data, err := read(ThumbnailFile); err == nil && len(data) <= maxThumbnailBytes {
		info.Thumbnail = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
	}
	info.LastCheck = def.LastCheck
	return info
}

// inProjectsDir says whether a file is in the user folder's projects.
func inProjectsDir(file string) bool {
	fi, err := os.Stat(filepath.Dir(file))
	if err != nil {
		return false
	}
	dir, err := os.Stat(projectsDir())
	return err == nil && os.SameFile(fi, dir)
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
	dir := s.proj.dir()
	s.mu.Unlock()
	if dir == "" {
		return errNoProject
	}
	return writeIfChanged(filepath.Join(dir, ThumbnailFile), img)
}

// writeIfChanged writes a file unless it has that content already, so that the project's
// file is not written again for nothing.
func writeIfChanged(p string, data []byte) error {
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// RevealProject shows a project's file in the system's file manager.
func (s *Service) RevealProject(file string) error {
	if !exists(file) {
		return fmt.Errorf("%s is no longer there", file)
	}
	app := application.Get()
	if app == nil {
		return errors.New("no file manager to show it in")
	}
	return app.Env.OpenFileManager(file, true)
}

// Project says which project the simulation works in, with no name while none is open.
func (s *Service) Project() ProjectInfo {
	s.mu.Lock()
	p := s.proj
	s.mu.Unlock()
	if p.name == "" {
		return ProjectInfo{}
	}
	return projectInfo(sources(), p.key(), dirReader(p.dir()), p.def)
}

// SaveState says how the open project's file stands, with no state while none is open.
func (s *Service) SaveState() SaveState {
	s.mu.Lock()
	doc := s.proj.doc
	s.mu.Unlock()
	if doc == nil {
		return SaveState{}
	}
	return doc.saveState()
}

// SaveProject writes the open project's file now, if it can, and says how it stands.
func (s *Service) SaveProject() SaveState {
	s.mu.Lock()
	doc := s.proj.doc
	s.mu.Unlock()
	if doc == nil {
		return SaveState{}
	}
	return doc.flush()
}

// KeepMine writes the open project over its file, which was changed from elsewhere, or
// where it was, if it was moved or deleted.
func (s *Service) KeepMine() SaveState {
	s.mu.Lock()
	doc := s.proj.doc
	s.mu.Unlock()
	if doc == nil {
		return SaveState{}
	}
	return doc.keepMine()
}

// OpenTheirs opens the open project's file again as it is now, changed from elsewhere,
// leaving what was changed here.
func (s *Service) OpenTheirs() (model.Scene, error) {
	s.mu.Lock()
	file := s.proj.key()
	s.mu.Unlock()
	if file == "" {
		return model.Scene{}, errNoProject
	}
	pf, err := readProjectFileAt(file)
	if err != nil {
		return model.Scene{}, err
	}
	s.CloseProjectDiscarding()
	return s.openFile(file, pf, false)
}

// SaveProjectAs writes the open project into a new file the user picks, with an id of its
// own, and goes on working in that file: for a file that cannot be written, or one changed
// from elsewhere. Cancelling gives an empty path.
func (s *Service) SaveProjectAs() (string, error) {
	s.mu.Lock()
	name := s.proj.name
	s.mu.Unlock()
	if name == "" {
		return "", errNoProject
	}
	to, err := chooseSaveFile("Save a copy of the project", name+" (copy)")
	if err != nil || to == "" {
		return "", err
	}
	return to, s.saveProjectAs(to)
}

func (s *Service) saveProjectAs(to string) error {
	if !strings.EqualFold(filepath.Ext(to), ProjectExtension) {
		to += ProjectExtension
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proj.doc == nil {
		return errNoProject
	}
	if err := s.proj.doc.moveTo(to); err != nil {
		return err
	}
	def, _, err := readProject(s.proj.dir())
	if err != nil {
		return err
	}
	old := s.proj.key()
	s.proj.def.ID, s.proj.name = def.ID, nameOf(s.proj.doc.path)
	softPLCs.Rename(old, s.proj.key())
	remember(s.proj.key())
	return nil
}

// chooseSaveFile asks for a project file to write in the system's dialog, "" if cancelled.
func chooseSaveFile(message, name string) (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to choose a file in")
	}
	return app.Dialog.SaveFile().SetMessage(message).SetFilename(name+ProjectExtension).
		AddFilter("Taktwin project", "*"+ProjectExtension).PromptForSingleSelection()
}

// readProject reads the project.json of a folder, or gives nothing for a folder without one.
func readProject(dir string) (ProjectDef, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, ProjectFile))
	if errors.Is(err, fs.ErrNotExist) {
		return ProjectDef{}, false, nil
	}
	if err != nil {
		return ProjectDef{}, false, err
	}
	def, err := parseProject(data)
	return def, err == nil, err
}

// parseProject reads a project.json, refusing any format but this build's.
func parseProject(data []byte) (ProjectDef, error) {
	var def ProjectDef
	if err := json.Unmarshal(data, &def); err != nil {
		return def, fmt.Errorf("%s: %w", ProjectFile, err)
	}
	switch {
	case def.Format > ProjectFormat:
		return def, fmt.Errorf("the project is of a newer version of Taktwin (format %d)", def.Format)
	case def.Format < ProjectFormat:
		return def, fmt.Errorf("the project was made by an earlier test version of Taktwin (format %d), which this version no longer opens", def.Format)
	}
	return def, nil
}

// projectJSON is a project.json as written.
func projectJSON(def ProjectDef) ([]byte, error) {
	def.Format = ProjectFormat
	data, err := json.MarshalIndent(def, "", "  ")
	return append(data, '\n'), err
}

// writeProject writes a project's file into its folder.
func writeProject(dir string, def ProjectDef) error {
	data, err := projectJSON(def)
	if err != nil {
		return err
	}
	return writeIfChanged(filepath.Join(dir, ProjectFile), data)
}

// saveProject writes the project.json of the project the simulation works in, under s.mu.
func (s *Service) saveProject() error {
	if s.proj.name == "" {
		return nil
	}
	return writeProject(s.proj.dir(), s.proj.def)
}

// projectLayers are what a project puts in front of the installed collections: its own
// station and exercise, then the collections it carries.
func projectLayers(read reader, def ProjectDef) []fs.FS {
	own := fstest.MapFS{}
	if data, err := read(StationFile); err == nil {
		own[path.Join("stations", def.Base.Station, "station.json")] = &fstest.MapFile{Data: data}
	}
	if data, err := read(ExerciseFile); err == nil && def.Base.Exercise != "" {
		own[path.Join("exercises", def.Base.Exercise, "exercise.json")] = &fstest.MapFile{Data: data}
	}
	var layers []fs.FS
	if len(own) > 0 {
		layers = append(layers, own)
	}
	return append(layers, carried(read, def)...)
}

// carried are the collections a project carries, as library layers. One that cannot be read
// is left out, and the installed one of its id stands in for it.
func carried(read reader, def ProjectDef) []fs.FS {
	var layers []fs.FS
	for _, c := range def.Collections {
		data, err := read(collectionsDir + c.Collection + model.PackageExtension)
		if err != nil {
			continue
		}
		if layer, err := model.ReadPackage(bytes.NewReader(data), int64(len(data))); err == nil {
			layers = append(layers, layer)
		}
	}
	return layers
}

// projectLibrary is the library a project's station loads from: its own station and
// exercise, the collections it carries, and then those installed here for anything else.
func projectLibrary(read reader, def ProjectDef) *model.Library {
	return libraryFrom(sources(), projectLayers(read, def))
}

// library is the library of the open project, or the collections while none is open.
func (s *Service) library() *model.Library {
	s.mu.Lock()
	p := s.proj
	s.mu.Unlock()
	return libraryOf(p)
}

// libraryOf is the library of a project, or the collections for none, with the teacher's
// exercise and station in front while a teacher checks a hand-in.
func libraryOf(p project) *model.Library {
	if p.dir() == "" {
		return Library()
	}
	layers := projectLayers(dirReader(p.dir()), p.def)
	if p.teacher != nil {
		layers = append([]fs.FS{p.teacher}, layers...)
	}
	return libraryFrom(sources(), layers)
}

// exerciseHash sums a project's exercise and its station, "" for a project on no exercise.
func exerciseHash(read reader) string {
	ex, err := read(ExerciseFile)
	if err != nil {
		return ""
	}
	station, _ := read(StationFile)
	return filesHash(map[string][]byte{ExerciseFile: ex, StationFile: station})
}

// usedBy are the collections a station is built from, by their ids, with the collections its
// own file and its exercise's come from; own lists what comes from the user's own folder.
// from is the library the station and exercise came from, before the project had copies.
func usedBy(lib, from *model.Library, bundle *model.Bundle, b Base) (ids, own []string) {
	add := func(collection, what string) {
		switch {
		case collection == "":
			if what != "" && !slices.Contains(own, what) {
				own = append(own, what)
			}
		case !slices.Contains(ids, collection):
			ids = append(ids, collection)
		}
	}
	add("base", "")
	for id := range bundle.Modules {
		add(lib.ModuleCollection(id), id)
	}
	for id := range bundle.Parts {
		add(lib.FileCollection(path.Join("parts", id+".json")), id)
	}
	if w := bundle.Station.Workpieces; w != "" {
		add(lib.FileCollection(path.Join("workpieces", w+".json")), w)
	}
	if from != nil {
		add(from.FileCollection(path.Join("stations", b.Station, "station.json")), "")
		if b.Exercise != "" {
			add(from.FileCollection(path.Join("exercises", b.Exercise, "exercise.json")), "")
		}
	}
	slices.Sort(ids)
	slices.Sort(own)
	return ids, own
}

// carry copies into a project's folder what it lacks of what it is built from: its station,
// its exercise with the instructions, and the collections they come from, each as the
// collections here have it now. What cannot be found is left out, for opening the project
// to say. It gives the project as it then is.
func carry(dir string, def ProjectDef) (ProjectDef, error) {
	srcs := sources()
	read := dirReader(dir)
	from := libraryFrom(srcs, carried(read, def))
	b := def.Base
	copyIn := func(name, from string, lib *model.Library) error {
		if exists(filepath.Join(dir, name)) {
			return nil
		}
		data, err := lib.ReadFile(from)
		if err != nil {
			return nil
		}
		return writeIfChanged(filepath.Join(dir, name), data)
	}
	if b.Exercise != "" {
		if err := copyIn(ExerciseFile, path.Join("exercises", b.Exercise, "exercise.json"), from); err != nil {
			return def, err
		}
		if err := copyIn(InstructionsFile, path.Join("exercises", b.Exercise, InstructionsFile), from); err != nil {
			return def, err
		}
	}
	if err := copyIn(StationFile, path.Join("stations", b.Station, "station.json"), from); err != nil {
		return def, err
	}
	lib := libraryFrom(srcs, projectLayers(read, def))
	var (
		bundle *model.Bundle
		err    error
	)
	if b.Exercise != "" {
		bundle, _, err = lib.LoadExercise(b.Exercise)
	} else {
		bundle, err = lib.Load(b.Station)
	}
	if err != nil {
		return def, nil
	}
	ids, _ := usedBy(lib, from, bundle, b)
	changed := false
	for _, id := range ids {
		file := filepath.Join(dir, collectionsDir, id+model.PackageExtension)
		if slices.ContainsFunc(def.Collections, func(r Requirement) bool { return r.Collection == id }) && exists(file) {
			continue
		}
		data, c, ok, err := collectionPackage(srcs, id)
		if err != nil {
			return def, err
		}
		if !ok {
			continue
		}
		if err := writeIfChanged(file, data); err != nil {
			return def, err
		}
		def.Collections = slices.DeleteFunc(def.Collections, func(r Requirement) bool { return r.Collection == id })
		def.Collections = append(def.Collections, Requirement{Collection: id, Name: c.Name, Version: c.Version})
		changed = true
	}
	if changed {
		slices.SortFunc(def.Collections, func(a, b Requirement) int { return strings.Compare(a.Collection, b.Collection) })
		return def, writeProject(dir, def)
	}
	return def, nil
}

// updatesOf are the collections a project carries of which a newer version is installed and
// shown here.
func updatesOf(srcs []source, def ProjectDef) []Requirement {
	hidden := readSettings().Hidden
	var out []Requirement
	for _, r := range def.Collections {
		for _, src := range srcs {
			if src.collection.ID != r.Collection || src.err != nil || src.kind != SourceBase && slices.Contains(hidden, r.Collection) {
				continue
			}
			if newer(src.collection.Version, r.Version) {
				out = append(out, Requirement{Collection: r.Collection, Name: cmpOr(src.collection.Name, r.Name), Version: r.Version, Installed: src.collection.Version})
			}
			break
		}
	}
	return out
}

// UpdateCollections brings the collections the open project carries up to the newer
// versions installed here, and loads its station again. Its station and exercise follow
// where the project has not changed them; what it changed stays. When the station no longer
// loads, everything is put back as it was.
func (s *Service) UpdateCollections() (model.Scene, error) {
	s.mu.Lock()
	dir, def := s.proj.dir(), s.proj.def
	s.mu.Unlock()
	if dir == "" {
		return model.Scene{}, errNoProject
	}
	srcs := sources()
	ups := updatesOf(srcs, def)
	if len(ups) == 0 {
		return s.Scene(), nil
	}
	// What is changed, to put back if the station does not load.
	names := []string{ProjectFile, StationFile, ExerciseFile}
	for _, u := range ups {
		names = append(names, collectionsDir+u.Collection+model.PackageExtension)
	}
	before := map[string][]byte{}
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))); err == nil {
			before[name] = data
		}
	}
	restore := func() {
		for _, name := range names {
			if data, ok := before[name]; ok {
				_ = os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), data, 0o644)
			} else {
				_ = os.Remove(filepath.Join(dir, filepath.FromSlash(name)))
			}
		}
	}
	follow := func(old, next fs.FS, name, from string) error {
		mine, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil
		}
		was, err := fs.ReadFile(old, from)
		if err != nil || !bytes.Equal(mine, was) {
			return nil
		}
		if now, err := fs.ReadFile(next, from); err == nil {
			return os.WriteFile(filepath.Join(dir, name), now, 0o644)
		}
		return nil
	}
	err := func() error {
		for _, u := range ups {
			file := filepath.Join(dir, collectionsDir, u.Collection+model.PackageExtension)
			data, c, ok, err := collectionPackage(srcs, u.Collection)
			if err != nil || !ok {
				return cmpErr(err, fmt.Errorf("the collection %s is no longer here", u.Collection))
			}
			if err := keepOlder(dir, u.Collection, u.Version); err != nil {
				return err
			}
			if old, err := os.ReadFile(file); err == nil {
				oldLayer, err1 := model.ReadPackage(bytes.NewReader(old), int64(len(old)))
				next, err2 := model.ReadPackage(bytes.NewReader(data), int64(len(data)))
				if err1 == nil && err2 == nil {
					if err := follow(oldLayer, next, StationFile, path.Join("stations", def.Base.Station, "station.json")); err != nil {
						return err
					}
					if def.Base.Exercise != "" {
						if err := follow(oldLayer, next, ExerciseFile, path.Join("exercises", def.Base.Exercise, "exercise.json")); err != nil {
							return err
						}
					}
				}
			}
			if err := os.WriteFile(file, data, 0o644); err != nil {
				return err
			}
			for i := range def.Collections {
				if def.Collections[i].Collection == u.Collection {
					def.Collections[i].Name, def.Collections[i].Version = c.Name, c.Version
				}
			}
		}
		if err := writeProject(dir, def); err != nil {
			return err
		}
		var err error
		def, err = carry(dir, def)
		return err
	}()
	if err != nil {
		restore()
		return model.Scene{}, err
	}
	s.mu.Lock()
	old := s.proj.def.Collections
	s.proj.def.Collections = def.Collections
	s.mu.Unlock()
	scene, err := s.ReloadStation()
	if err != nil {
		restore()
		s.mu.Lock()
		s.proj.def.Collections = old
		s.mu.Unlock()
		if _, again := s.ReloadStation(); again != nil {
			return model.Scene{}, fmt.Errorf("%w; and as it was: %v", err, again)
		}
		return model.Scene{}, fmt.Errorf("the station does not load with the newer collections, so the project stays as it was: %w", err)
	}
	return scene, nil
}

func cmpErr(err, otherwise error) error {
	if err != nil {
		return err
	}
	return otherwise
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

// openFile puts a project file into the simulation after closing the one before: its
// station, its settings, and its PLCs' connections, ready to connect. Only the PLCs the project
// says to connect as it opens are connected. With restore, it goes on with the changes a crash
// left in its working copy.
func (s *Service) openFile(file string, pf *projectFile, restore bool) (model.Scene, error) {
	saved := ""
	if !validID(pf.def.ID) {
		saved = filesHash(pf.all())
		// A file without an id gets one, which is written into it with the next save.
		pf.def.ID = newID()
		data, err := projectJSON(pf.def)
		if err != nil {
			return model.Scene{}, err
		}
		pf.files[ProjectFile] = data
	}
	s.mu.Lock()
	same := s.proj.doc != nil && s.proj.doc.id == pf.def.ID
	s.mu.Unlock()
	if same {
		// Its working copy is about to be replaced.
		s.close()
	}
	doc, err := openDocument(file, pf, restore, saved)
	if err != nil {
		return model.Scene{}, err
	}
	remember(doc.path)
	def, ok, err := readProject(doc.dir)
	if err == nil && !ok {
		err = fmt.Errorf("%s has no %s", filepath.Base(file), ProjectFile)
	}
	if err == nil && !doc.readOnly {
		// What it lacks of what it is built from is copied in, to be saved with it, and a
		// project without an original takes itself as it is now.
		def, err = carry(doc.dir, def)
		if _, has, _ := readOriginal(doc.dir); err == nil && !has && exists(filepath.Join(doc.dir, StationFile)) {
			def, err = keepOriginal(doc.dir, def)
		}
	}
	var (
		bundle   *model.Bundle
		e        *engine.Engine
		exercise string
	)
	if err == nil {
		bundle, e, exercise, err = loadBase(projectLibrary(dirReader(doc.dir), def), def.Base)
	}
	if err != nil {
		if !restore {
			dropCopy(doc.id)
		}
		return model.Scene{}, err
	}
	s.close()
	for id, v := range def.Parameters {
		// A setting the station no longer has is left out.
		_ = e.SetParameter(id, v)
	}
	s.mu.Lock()
	s.engine, s.bundle, s.plcs, s.exercise = e, bundle, map[string]PLCConfig{}, exercise
	s.proj = project{name: nameOf(doc.path), def: def, doc: doc}
	s.autoConnect()
	s.run()
	s.mu.Unlock()
	doc.start(emitSaveState)
	return e.Scene(), nil
}

// emitSaveState tells the frontend how the open project's file stands.
func emitSaveState(st SaveState) {
	if app := application.Get(); app != nil {
		app.Event.Emit(SaveStateEvent, st)
	}
}

// ReloadStation loads the open project's station again, as its files have it now, such as
// after it was built on: every PLC is disconnected, as on a restart, while the soft PLCs go
// on with their programs, ready to connect again.
func (s *Service) ReloadStation() (model.Scene, error) { return s.reload(false) }

// reload loads the open project's station again; with keep, its PLCs stay connected as they
// were, for a station whose signals are the same.
func (s *Service) reload(keep bool) (model.Scene, error) {
	s.mu.Lock()
	p := s.proj
	key, def := p.key(), p.def
	s.mu.Unlock()
	if key == "" {
		return model.Scene{}, errNoProject
	}
	bundle, e, exercise, err := loadBase(libraryOf(p), def.Base)
	if err != nil {
		return model.Scene{}, err
	}
	for id, v := range def.Parameters {
		_ = e.SetParameter(id, v)
	}
	s.StopCheck()
	s.mu.Lock()
	if s.proj.key() != key {
		s.mu.Unlock()
		e.Close()
		return model.Scene{}, errors.New("the project was closed")
	}
	old, stop, was := s.engine, s.stop, s.plcs
	s.engine, s.bundle, s.plcs, s.exercise = e, bundle, map[string]PLCConfig{}, exercise
	var problem error
	if keep {
		for k, c := range was {
			plc := k
			if k == "" && len(e.Plan().PLCs) > 0 {
				plc = e.Plan().PLCs[0]
			}
			ctl, err := s.controller(plc, c)
			if err == nil {
				err = e.SetControllerFor(plc, ctl)
			}
			if err != nil {
				problem = errors.Join(problem, err)
				continue
			}
			s.plcs[k] = c
		}
	}
	s.run()
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	if old != nil {
		old.Close()
	}
	return e.Scene(), problem
}

// editedStation gives the open project's own station, to build on, with the project's key.
func (s *Service) editedStation() (string, []byte, error) {
	s.mu.Lock()
	key, dir, exercise := s.proj.key(), s.proj.dir(), s.proj.def.Base.Exercise
	s.mu.Unlock()
	switch {
	case key == "":
		return "", nil, errNoProject
	case exercise != "":
		return "", nil, errors.New("the station of an exercise stays as the exercise has it")
	}
	data, err := os.ReadFile(filepath.Join(dir, StationFile))
	if err != nil {
		return "", nil, errors.New("the project has no station of its own to build on")
	}
	return key, data, nil
}

// runningStation is the station the simulation runs: its file, as the project has it or its
// collection, and the station as it was loaded and resolved, with what the project's exercise
// set up, such as pins swapped.
func (s *Service) runningStation() ([]byte, *model.Bundle, *model.Plan, error) {
	s.mu.Lock()
	p, bundle, e := s.proj, s.bundle, s.engine
	s.mu.Unlock()
	if p.dir() == "" || bundle == nil || e == nil {
		return nil, nil, nil, errNoProject
	}
	data, err := libraryOf(p).ReadFile(path.Join("stations", bundle.Station.ID, "station.json"))
	return data, bundle, e.Plan(), err
}

// writeStation keeps a station built in a project as the project's own, while that project,
// by its key, is open.
func (s *Service) writeStation(project string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proj.key() != project {
		return fmt.Errorf("the project %s is no longer open", nameOf(project))
	}
	return writeIfChanged(filepath.Join(s.proj.dir(), StationFile), data)
}

// close leaves the project the simulation works in: a check stops, every PLC is
// disconnected, the project's soft PLCs stop, and no station is left in the simulation. Its
// file is written; a working copy that could not be is kept, to be offered back, unless
// discard says to drop it.
func (s *Service) close() { s.closeProject(false) }

func (s *Service) closeProject(discard bool) {
	s.StopCheck()
	s.mu.Lock()
	e, stop, doc := s.engine, s.stop, s.proj.doc
	s.engine, s.bundle, s.stop, s.plcs, s.exercise, s.proj = nil, nil, nil, map[string]PLCConfig{}, "", project{}
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	if e != nil {
		e.Close()
	}
	if doc == nil {
		return
	}
	softPLCs.Drop(doc.path)
	doc.halt()
	if st := doc.flush(); discard || st.State == SaveSaved {
		dropCopy(doc.id)
	}
}

// CloseProject leaves the project the simulation works in, which keeps everything it had:
// every PLC is disconnected and its soft PLCs stop. Its file is written; changes that could
// not be written are offered back when it is opened again.
func (s *Service) CloseProject() { s.close() }

// CloseProjectDiscarding leaves the project like CloseProject, dropping the changes that
// could not be written into its file.
func (s *Service) CloseProjectDiscarding() { s.closeProject(true) }

// newProjectFile makes a project file in the user folder's projects, under a free name
// after name, with the files given besides project.json, and gives its path.
func newProjectFile(name string, def ProjectDef, files map[string][]byte) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case !projectName(name):
		return "", errName(name)
	case exists(fileFor(name)):
		return "", fmt.Errorf("there is a project %s already", name)
	}
	if err := os.MkdirAll(projectsDir(), 0o755); err != nil {
		return "", err
	}
	def.ID = newID()
	data, err := projectJSON(def)
	if err != nil {
		return "", err
	}
	all := map[string][]byte{ProjectFile: data}
	maps.Copy(all, files)
	file := fileFor(name)
	return file, writeZipOver(file, all)
}

// fileFor is the file of a project in the user folder's projects.
func fileFor(name string) string { return filepath.Join(projectsDir(), name+ProjectExtension) }

// openNew opens a project file just made, which is removed again if it does not open.
func (s *Service) openNew(file string) (model.Scene, error) {
	pf, err := readProjectFileAt(file)
	if err == nil {
		var scene model.Scene
		if scene, err = s.openFile(file, pf, false); err == nil {
			return scene, nil
		}
	}
	_ = os.Remove(file)
	forget(file)
	return model.Scene{}, err
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
	if strings.TrimSpace(name) == "" {
		name = freeName(cmpOr(title, base.Station))
	}
	file, err := newProjectFile(name, ProjectDef{Base: base}, nil)
	if err != nil {
		return model.Scene{}, err
	}
	return s.openNew(file)
}

// NewEmptyProject makes a project with an empty station of its own, standing on what a
// collection has to stand on, and opens it to be built on. Without a name it is called New
// station, with a number where that is taken.
func (s *Service) NewEmptyProject(name, collection string) (model.Scene, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = freeName("New station")
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
	file, err := newProjectFile(name, ProjectDef{Base: Base{Station: d.ID}}, map[string][]byte{StationFile: data})
	if err != nil {
		return model.Scene{}, err
	}
	return s.openNew(file)
}

var forbiddenName = regexp.MustCompile(`[/\\:*?"<>|]`)

// projectName says whether a name can be that of a project's file, without the extension.
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

// freeName makes a name a project in the user folder's projects can have and none has yet:
// the name, made fit for a file, with a number where it is taken.
func freeName(name string) string {
	name = strings.TrimLeft(strings.TrimSpace(forbiddenName.ReplaceAllString(name, "-")), ".")
	if len(name) > 70 {
		name = strings.TrimSpace(name[:70])
	}
	if !projectName(name) {
		name = "Project"
	}
	free := name
	for n := 2; exists(fileFor(free)); n++ {
		free = fmt.Sprintf("%s (%d)", name, n)
	}
	return free
}

// Projects lists the project files in the user folder's projects and those opened from
// elsewhere, the latest first, and last those no longer where they were.
func (s *Service) Projects() []ProjectInfo {
	var files []string
	if items, err := os.ReadDir(projectsDir()); err == nil {
		for _, it := range items {
			if !it.IsDir() && strings.EqualFold(filepath.Ext(it.Name()), ProjectExtension) && !strings.HasPrefix(it.Name(), ".") {
				files = append(files, filepath.Join(projectsDir(), it.Name()))
			}
		}
	}
	for _, r := range readSettings().Recent {
		files = append(files, r.Path)
	}
	out := []ProjectInfo{}
	var seen []os.FileInfo
	seenPath := map[string]bool{}
	srcs := sources()
	for _, file := range files {
		if seenPath[file] {
			continue
		}
		seenPath[file] = true
		fi, err := os.Stat(file)
		if err != nil {
			out = append(out, ProjectInfo{Name: nameOf(file), Path: file, Missing: true, Problem: "The file is no longer where it was opened from."})
			continue
		}
		// The same file by another path, such as another case of its name, is listed once.
		if slices.ContainsFunc(seen, func(o os.FileInfo) bool { return os.SameFile(o, fi) }) {
			continue
		}
		seen = append(seen, fi)
		pf, err := readProjectFileAt(file)
		if err != nil {
			out = append(out, ProjectInfo{Name: nameOf(file), Path: file, InFolder: inProjectsDir(file), Modified: fi.ModTime().UTC().Format(time.RFC3339), Problem: err.Error()})
			continue
		}
		out = append(out, projectInfo(srcs, file, mapReader(pf.all()), pf.def))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Missing != out[j].Missing {
			return !out[i].Missing
		}
		return out[i].Modified > out[j].Modified
	})
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

// closed checks that a project file exists and is not the one open.
func (s *Service) closed(file string) error {
	fi, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("%s is no longer there", file)
	}
	s.mu.Lock()
	open := s.proj.key()
	s.mu.Unlock()
	if o, err := os.Stat(open); open != "" && err == nil && os.SameFile(o, fi) {
		return fmt.Errorf("the project %s is open: close it first", nameOf(file))
	}
	return nil
}

// RenameProject gives a project file that is not open another name, in the same folder.
func (s *Service) RenameProject(file, to string) (ProjectInfo, error) {
	to = strings.TrimSpace(to)
	if err := s.closed(file); err != nil {
		return ProjectInfo{}, err
	}
	if !projectName(to) {
		return ProjectInfo{}, errName(to)
	}
	target := filepath.Join(filepath.Dir(file), to+ProjectExtension)
	// A name that differs only in case is the same file on macOS and Windows.
	if to != nameOf(file) {
		if exists(target) && !strings.EqualFold(to, nameOf(file)) {
			return ProjectInfo{}, fmt.Errorf("there is a project %s already", to)
		}
		if err := os.Rename(file, target); err != nil {
			return ProjectInfo{}, err
		}
		moved(file, target)
	}
	pf, err := readProjectFileAt(target)
	if err != nil {
		return ProjectInfo{Name: to, Path: target}, err
	}
	return projectInfo(sources(), target, mapReader(pf.all()), pf.def), nil
}

// SaveProjectCopy writes a copy of a project into a file the user picks, with an id of its
// own; for the open project, as it is now. Cancelling gives an empty path.
func (s *Service) SaveProjectCopy(file string) (string, error) {
	to, err := chooseSaveFile("Save a copy of the project", nameOf(file)+" (copy)")
	if err != nil || to == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(to), ProjectExtension) {
		to += ProjectExtension
	}
	return to, s.copyProject(file, to)
}

// copyProject writes a copy of a project file into another, with an id of its own.
func (s *Service) copyProject(file, to string) error {
	files, err := s.projectFiles(file)
	if err != nil {
		return err
	}
	def, err := parseProject(files[ProjectFile])
	if err != nil {
		return err
	}
	def.ID = newID()
	if files[ProjectFile], err = projectJSON(def); err != nil {
		return err
	}
	if err := writeZipOver(to, files); err != nil {
		return err
	}
	remember(to)
	return nil
}

// projectFiles reads a project's files: the open project's as its working copy has them,
// any other's from its file.
func (s *Service) projectFiles(file string) (map[string][]byte, error) {
	s.mu.Lock()
	open, dir := s.proj.key(), s.proj.dir()
	s.mu.Unlock()
	if open != "" && sameFile(open, file) {
		return readTree(dir)
	}
	pf, err := readProjectFileAt(file)
	if err != nil {
		return nil, err
	}
	return pf.all(), nil
}

// sameFile says whether two paths are of the same file.
func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

// DeleteProject removes a project file that is not open, and a working copy left of it.
func (s *Service) DeleteProject(file string) error {
	if err := s.closed(file); err != nil {
		return err
	}
	if pf, err := readProjectFileAt(file); err == nil {
		dropCopy(pf.def.ID)
	}
	if err := os.Remove(file); err != nil {
		return err
	}
	forget(file)
	return nil
}

// ForgetProject takes a project file off the list of those opened from elsewhere, leaving
// the file where it is.
func (s *Service) ForgetProject(file string) { forget(file) }

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

// zipped makes a zip of files, in the order of their names.
func zipped(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		w, err := z.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, err
		}
	}
	err := z.Close()
	return buf.Bytes(), err
}

// FileOpenedEvent says that the system asked the app to open a file, such as a project file
// that was double-clicked; TakeOpenedFile gives it.
const FileOpenedEvent = "files:opened"

// FilesDroppedEvent carries the paths of files dropped onto the window to the frontend.
const FilesDroppedEvent = "files:dropped"

// MenuCommandEvent carries a command of the app's menu on a Mac to the frontend: import or
// close a project, or show the settings.
const MenuCommandEvent = "menu:command"
