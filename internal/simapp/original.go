package simapp

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/toblu96/taktwin/internal/model"
)

// Every project keeps its original under original/: the state it started from, which it can
// go back to a part at a time, or as a whole. It is set when the project is made, by Hand
// out… for the copy handed out, and on request. See ADR 0002.

// originalDir is the folder a project file keeps its original in.
const originalDir = "original/"

// The parts of a project that can be put back as the original has them.
const (
	// PartConnection is how a PLC is connected.
	PartConnection = "connection"
	// PartPrograms are a PLC's soft PLC programs.
	PartPrograms = "programs"
	// PartStation is the station as built and wired; an exercise's cannot be changed.
	PartStation = "station"
	// PartSettings are the station's settings as set in the simulation.
	PartSettings = "settings"
	// PartEverything is all of them, the exercise and the collections' versions too.
	PartEverything = "everything"
)

// inOriginal says whether a file belongs in the original: what the project is, not how it was
// last seen or checked.
func inOriginal(name string) bool {
	switch name {
	case ProjectFile, StationFile, ExerciseFile, InstructionsFile:
		return true
	}
	return strings.HasPrefix(name, "programs/")
}

// originalFiles are the files of the original, by their paths in it, from a project's files.
func originalFiles(files map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for name, data := range files {
		if rest, ok := strings.CutPrefix(name, originalDir); ok {
			out[rest] = data
		}
	}
	return out
}

// keepOriginal makes the project in a folder its own original, as it is now, and records when.
// The collections' older versions kept for an earlier original are dropped.
func keepOriginal(dir string, def ProjectDef) (ProjectDef, error) {
	files, err := readTree(dir)
	if err != nil {
		return def, err
	}
	if err := os.RemoveAll(filepath.Join(dir, filepath.FromSlash(originalDir))); err != nil {
		return def, err
	}
	def.Original = time.Now().UTC().Format(time.RFC3339)
	if err := writeProject(dir, def); err != nil {
		return def, err
	}
	data, err := projectJSON(originalDef(def))
	if err != nil {
		return def, err
	}
	files[ProjectFile] = data
	for name, data := range files {
		if !inOriginal(name) {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(originalDir+name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return def, err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return def, err
		}
	}
	return def, nil
}

// originalDef is a project.json as the original keeps it: without how the project was last
// checked.
func originalDef(def ProjectDef) ProjectDef {
	def.LastCheck = nil
	return def
}

// readOriginal reads the original of the project in a folder, if it has one.
func readOriginal(dir string) (ProjectDef, bool, error) {
	return readProject(filepath.Join(dir, filepath.FromSlash(originalDir)))
}

// keepOlder keeps a collection's package in the original before the project takes another
// version of it, while the original uses the version it has now.
func keepOlder(dir, id, version string) error {
	orig, ok, err := readOriginal(dir)
	if err != nil || !ok {
		return err
	}
	if !slices.ContainsFunc(orig.Collections, func(r Requirement) bool { return r.Collection == id && r.Version == version }) {
		return nil
	}
	target := filepath.Join(dir, filepath.FromSlash(originalDir+collectionsDir), id+model.PackageExtension)
	if exists(target) {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(collectionsDir), id+model.PackageExtension))
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

// OriginalInfo says what the open project's original is, and which of its parts differ from it.
type OriginalInfo struct {
	// At is when the original was set, as RFC 3339; "" for a project without one.
	At string `json:"at,omitempty"`
	// Station and Settings say that they differ from the original.
	Station  bool `json:"station"`
	Settings bool `json:"settings"`
	// Connections and Programs list the PLCs, by module, "" for the first, whose connection
	// or programs differ from the original.
	Connections []string `json:"connections"`
	Programs    []string `json:"programs"`
	// Changed says that anything differs, the exercise and collections' versions included.
	Changed bool `json:"changed"`
}

// Original says what the open project's original is, and what differs from it.
func (s *Service) Original() OriginalInfo {
	s.mu.Lock()
	dir, def := s.proj.dir(), s.proj.def
	s.mu.Unlock()
	info := OriginalInfo{Connections: []string{}, Programs: []string{}}
	if dir == "" {
		return info
	}
	orig, ok, err := readOriginal(dir)
	if err != nil || !ok {
		return info
	}
	info.At = def.Original
	files, err := readTree(dir)
	if err != nil {
		return info
	}
	was := originalFiles(files)
	info.Station = !bytes.Equal(files[StationFile], was[StationFile])
	info.Settings = !maps.Equal(def.Parameters, orig.Parameters)
	for _, key := range slices.Sorted(maps.Keys(union(def.PLCs, orig.PLCs))) {
		if !samePLC(def.PLCs[key], orig.PLCs[key]) {
			info.Connections = append(info.Connections, key)
		}
	}
	plcs := map[string]bool{}
	for _, m := range []map[string][]byte{files, was} {
		for name := range m {
			if rest, ok := strings.CutPrefix(name, "programs/"); ok {
				plcs[strings.SplitN(rest, "/", 2)[0]] = true
			}
		}
	}
	for _, plc := range slices.Sorted(maps.Keys(plcs)) {
		if !maps.EqualFunc(under(files, "programs/"+plc+"/"), under(was, "programs/"+plc+"/"), bytes.Equal) {
			info.Programs = append(info.Programs, plc)
		}
	}
	collections := func(rs []Requirement) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Collection+"@"+r.Version)
		}
		slices.Sort(out)
		return out
	}
	info.Changed = info.Station || info.Settings || len(info.Connections) > 0 || len(info.Programs) > 0 ||
		!bytes.Equal(files[ExerciseFile], was[ExerciseFile]) || !bytes.Equal(files[InstructionsFile], was[InstructionsFile]) ||
		!slices.Equal(collections(def.Collections), collections(orig.Collections))
	return info
}

func union[V any](a, b map[string]V) map[string]V {
	out := maps.Clone(a)
	if out == nil {
		out = map[string]V{}
	}
	maps.Copy(out, b)
	return out
}

// samePLC says whether two ways of connecting a PLC are the same; none and unset are.
func samePLC(a, b PLCConfig) bool {
	if a.Mode == "" {
		a.Mode = PLCNone
	}
	if b.Mode == "" {
		b.Mode = PLCNone
	}
	if a.Mode != b.Mode || a.Connection != b.Connection || len(a.Values) != len(b.Values) {
		return false
	}
	for k, v := range a.Values {
		if fmt.Sprint(b.Values[k]) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

// under are the files below a folder, by their paths in it.
func under(files map[string][]byte, dir string) map[string][]byte {
	out := map[string][]byte{}
	for name, data := range files {
		if rest, ok := strings.CutPrefix(name, dir); ok {
			out[rest] = data
		}
	}
	return out
}

// KeepAsOriginal makes the open project, as it is now, its original, which putting back goes
// back to.
func (s *Service) KeepAsOriginal() (OriginalInfo, error) {
	err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.proj.dir() == "" {
			return errNoProject
		}
		def, err := keepOriginal(s.proj.dir(), s.proj.def)
		s.proj.def.Original = def.Original
		return err
	}()
	if err != nil {
		return OriginalInfo{}, err
	}
	return s.Original(), nil
}

// PutBack puts a part of the open project back as its original has it: a PLC's connection or
// programs, by its module, "" for the first; the station; its settings; or everything. A
// changed station loads again, every PLC disconnected as on a restart.
func (s *Service) PutBack(part, plc string) (OriginalInfo, error) {
	s.mu.Lock()
	dir, exercise := s.proj.dir(), s.proj.def.Base.Exercise
	key := s.plcKey(plc)
	s.mu.Unlock()
	if dir == "" {
		return OriginalInfo{}, errNoProject
	}
	orig, ok, err := readOriginal(dir)
	if err != nil {
		return OriginalInfo{}, err
	}
	if !ok {
		return OriginalInfo{}, errors.New("the project has no original to put back")
	}
	files, err := readTree(dir)
	if err != nil {
		return OriginalInfo{}, err
	}
	was := originalFiles(files)
	reload := false
	switch part {
	case PartConnection:
		s.mu.Lock()
		c, had := orig.PLCs[key]
		if !had {
			c = PLCConfig{Mode: PLCNone}
		}
		if s.proj.def.PLCs == nil {
			s.proj.def.PLCs = map[string]PLCConfig{}
		}
		s.proj.def.PLCs[key] = c
		err = s.saveProject()
		s.mu.Unlock()
		if err == nil {
			// It is connected again as it was, by hand.
			err = s.DisconnectPLCFor(plc)
		}
	case PartPrograms:
		if key == "" {
			key = s.firstPLC()
		}
		err = replaceUnder(dir, "programs/"+key+"/", under(was, "programs/"+key+"/"))
	case PartStation:
		if exercise != "" {
			return OriginalInfo{}, errors.New("the station of an exercise stays as the exercise has it")
		}
		err = writeIfChanged(filepath.Join(dir, StationFile), was[StationFile])
		reload = true
	case PartSettings:
		s.mu.Lock()
		s.proj.def.Parameters = maps.Clone(orig.Parameters)
		err = s.saveProject()
		s.mu.Unlock()
		reload = true
	case PartEverything:
		err = s.putBackEverything(dir, orig, files, was)
		reload = true
	default:
		return OriginalInfo{}, fmt.Errorf("%q is no part of a project", part)
	}
	if err != nil {
		return OriginalInfo{}, err
	}
	if reload {
		if _, err := s.ReloadStation(); err != nil {
			return OriginalInfo{}, err
		}
	}
	return s.Original(), nil
}

// firstPLC is the module of the station's first PLC, whose programs are kept under its id.
func (s *Service) firstPLC() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return ""
	}
	return s.engine.Plan().Scene.PLC.Module
}

// putBackEverything makes the project in a folder its original again, keeping its id, the
// last check, and the collections' versions the original no longer needs.
func (s *Service) putBackEverything(dir string, orig ProjectDef, files, was map[string][]byte) error {
	for name := range files {
		if inOriginal(name) && name != ProjectFile {
			if _, ok := was[name]; !ok {
				if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
					return err
				}
			}
		}
	}
	for name, data := range was {
		if !inOriginal(name) || name == ProjectFile {
			continue
		}
		if err := writeIfChanged(filepath.Join(dir, filepath.FromSlash(name)), data); err != nil {
			return err
		}
	}
	// The older versions of collections the original kept come back.
	for name, data := range was {
		if rest, ok := strings.CutPrefix(name, collectionsDir); ok && !strings.Contains(rest, "/") {
			if err := writeIfChanged(filepath.Join(dir, filepath.FromSlash(name)), data); err != nil {
				return err
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	def := s.proj.def
	def.Base, def.PLCs, def.Parameters = orig.Base, maps.Clone(orig.PLCs), maps.Clone(orig.Parameters)
	for _, r := range orig.Collections {
		i := slices.IndexFunc(def.Collections, func(c Requirement) bool { return c.Collection == r.Collection })
		if i < 0 {
			def.Collections = append(def.Collections, r)
		} else {
			def.Collections[i] = r
		}
	}
	s.proj.def = def
	return s.saveProject()
}

// replaceUnder makes a folder of a project hold exactly the files given, by their paths in it.
func replaceUnder(dir, folder string, files map[string][]byte) error {
	target := filepath.Join(dir, filepath.FromSlash(folder))
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	for name, data := range files {
		p := filepath.Join(target, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// HandOut writes a copy of a project for others into a file the user picks: as it is now,
// with an id of its own, the programs only when asked, and without the last check. The
// copy's original is the state it is handed out in. Cancelling gives an empty path.
func (s *Service) HandOut(file string, programs bool) (string, error) {
	files, err := s.projectFiles(file)
	if err != nil {
		return "", err
	}
	def, err := parseProject(files[ProjectFile])
	if err != nil {
		return "", err
	}
	name := nameOf(file)
	if def.Base.Exercise != "" {
		if ex, err := projectLibrary(mapReader(files), def).Exercise(def.Base.Exercise); err == nil {
			name = cmpOr(freeFileName(ex.Name), name)
		}
	}
	to, err := chooseSaveFile("Hand out the project", name)
	if err != nil || to == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(to), ProjectExtension) {
		to += ProjectExtension
	}
	return to, handOut(files, to, programs)
}

// handOut writes a copy of a project's files for others; see HandOut.
func handOut(files map[string][]byte, to string, programs bool) error {
	def, err := parseProject(files[ProjectFile])
	if err != nil {
		return err
	}
	lib := projectLibrary(mapReader(files), def)
	var bundle *model.Bundle
	if def.Base.Exercise != "" {
		bundle, _, err = lib.LoadExercise(def.Base.Exercise)
	} else {
		bundle, err = lib.Load(def.Base.Station)
	}
	if err != nil {
		return err
	}
	if _, own := usedBy(lib, nil, bundle, def.Base); len(own) > 0 {
		return fmt.Errorf("the station uses %s from your own folder, which others do not have: put them into a collection first", strings.Join(own, ", "))
	}
	out := map[string][]byte{}
	for name, data := range files {
		switch {
		case strings.HasPrefix(name, originalDir):
		case !programs && strings.HasPrefix(name, "programs/"):
		default:
			out[name] = data
		}
	}
	def.ID, def.LastCheck = newID(), nil
	def.Original = time.Now().UTC().Format(time.RFC3339)
	if out[ProjectFile], err = projectJSON(def); err != nil {
		return err
	}
	orig, err := projectJSON(originalDef(def))
	if err != nil {
		return err
	}
	for name, data := range out {
		if inOriginal(name) {
			out[originalDir+name] = data
		}
	}
	out[originalDir+ProjectFile] = orig
	return writeZipOver(to, out)
}

// originalPath says whether a project file may hold a file of that path in its original.
func originalPath(name string) bool {
	rest, ok := strings.CutPrefix(name, originalDir)
	if !ok {
		return false
	}
	if c, ok := strings.CutPrefix(rest, collectionsDir); ok {
		return !strings.Contains(c, "/") && strings.HasSuffix(c, model.PackageExtension)
	}
	return inOriginal(rest) && fs.ValidPath(rest) && path.Clean(rest) == rest
}
