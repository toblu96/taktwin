package simapp

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
)

// CollectionsFolder is the folder of the user folder that collections are installed into,
// each a .taktwin file or a folder with the same folders as the catalog.
const CollectionsFolder = "collections"

// Where a library layer comes from.
const (
	SourceOwn         = "own"         // the user's own stations, in the user folder
	SourceInstalled   = "installed"   // a collection in the user folder, which can be removed
	SourceDevelopment = "development" // a collection TAKTWIN_COLLECTIONS names
	SourceBuiltIn     = "built-in"    // the app's own reference models
	SourceBase        = "base"        // what every collection can use, always there
)

// source is a library layer and where it comes from.
type source struct {
	layer      fs.FS
	kind       string
	path       string
	collection model.CollectionDef
	err        error
}

// sources lists the library's layers, front first: the user's own stations, the collections
// installed in the user folder (and station packages put beside the stations), those
// TAKTWIN_COLLECTIONS names (a path list, for development and tests), the app's reference
// models, and last the base every collection can use. A file that cannot be opened is
// listed with its error and no layer.
func sources() []source {
	var out []source
	dir := UserFolder()
	if info, err := os.Stat(dir); dir != "" && err == nil && info.IsDir() {
		out = append(out, source{layer: os.DirFS(dir), kind: SourceOwn, path: dir})
		packages, _ := filepath.Glob(filepath.Join(dir, "*"+model.PackageExtension))
		installed, _ := filepath.Glob(filepath.Join(dir, CollectionsFolder, "*"))
		out = append(out, opened(append(packages, installed...), SourceInstalled)...)
	}
	out = append(out, opened(filepath.SplitList(os.Getenv("TAKTWIN_COLLECTIONS")), SourceDevelopment)...)
	out = append(out, source{layer: catalog.Reference, kind: SourceBuiltIn}, source{layer: catalog.Base, kind: SourceBase})
	for i := range out {
		if out[i].layer != nil && out[i].err == nil {
			out[i].collection, _, out[i].err = model.ReadCollection(out[i].layer)
		}
	}
	return out
}

// opened opens collections as library layers: folders as they are and .taktwin files as
// zips. Anything else is skipped.
func opened(paths []string, kind string) []source {
	var out []source
	for _, path := range paths {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		switch {
		case err != nil:
			continue
		case info.IsDir():
			out = append(out, source{layer: os.DirFS(path), kind: kind, path: path})
		case strings.HasSuffix(path, model.PackageExtension):
			s := source{kind: kind, path: path}
			s.layer, s.err = readPackage(path)
			out = append(out, s)
		}
	}
	return out
}

func readPackage(path string) (fs.FS, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return model.ReadPackage(bytes.NewReader(data), int64(len(data)))
}

// Library is the library with the user's own stations in front, then the collections, read
// afresh each time so that a new or edited file shows without restarting. Hidden
// collections are left out, as is a collection that cannot be read, or one whose id an
// earlier layer already brings, so that an installed collection stands in for the same one
// built in or given for development.
func Library() *model.Library {
	hidden := readSettings().Hidden
	var layers []fs.FS
	seen := map[string]bool{}
	for _, s := range sources() {
		id := s.collection.ID
		if s.layer == nil || s.err != nil || id != "" && (seen[id] || s.kind != SourceBase && slices.Contains(hidden, id)) {
			continue
		}
		if id != "" {
			seen[id] = true
		}
		layers = append(layers, s.layer)
	}
	return model.NewLibrary(layers...)
}

// defaultStation is the station the app opens first: that of its own reference collection,
// unless it is hidden, or else the one the library's first collection names.
func defaultStation(lib *model.Library) string {
	if c, ok, _ := model.ReadCollection(catalog.Reference); ok && c.Station != "" {
		if _, shown := lib.Collection(c.ID); shown && slices.Contains(lib.Stations(), c.Station) {
			return c.Station
		}
	}
	return lib.DefaultStation()
}

// CollectionInfo is a collection as the settings show it.
type CollectionInfo struct {
	model.CollectionDef
	// Source is where it comes from: installed, development or built-in.
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
	Hidden bool   `json:"hidden"`
	// Shadowed is a collection that another one with the same id stands in for.
	Shadowed bool   `json:"shadowed,omitempty"`
	Error    string `json:"error,omitempty"`
	Stations int    `json:"stations"`
	Modules  int    `json:"modules"`
	Parts    int    `json:"parts"`
}

// Collections lists the collections, front first, with what they hold.
func (s *LibraryService) Collections() []CollectionInfo {
	hidden := readSettings().Hidden
	seen := map[string]bool{}
	out := []CollectionInfo{}
	for _, src := range sources() {
		if src.kind == SourceOwn || src.kind == SourceBase {
			continue
		}
		info := CollectionInfo{CollectionDef: src.collection, Source: src.kind, Path: src.path}
		if info.Name == "" {
			info.Name = cmpOr(info.ID, filepath.Base(src.path))
		}
		switch {
		case src.err != nil:
			info.Error = src.err.Error()
		case src.collection.ID == "":
			info.Error = "it has no " + model.CollectionFile + ", so it is read as a station package"
		}
		info.Hidden = info.ID != "" && slices.Contains(hidden, info.ID)
		info.Shadowed = info.ID != "" && seen[info.ID]
		if info.ID != "" {
			seen[info.ID] = true
		}
		if src.layer != nil {
			info.Stations = count(src.layer, "stations", true)
			info.Modules = count(src.layer, "modules", true) + count(src.layer, "accessories", true)
			info.Parts = count(src.layer, "parts", false)
		}
		out = append(out, info)
	}
	return out
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func count(layer fs.FS, dir string, folders bool) int {
	items, _ := fs.ReadDir(layer, dir)
	n := 0
	for _, it := range items {
		if it.IsDir() == folders && (folders || strings.HasSuffix(it.Name(), ".json")) {
			n++
		}
	}
	return n
}

// SetCollectionHidden hides a collection from the library, or shows it again.
func (s *LibraryService) SetCollectionHidden(id string, hide bool) error {
	set := readSettings()
	set.Hidden = slices.DeleteFunc(set.Hidden, func(h string) bool { return h == id })
	if hide {
		set.Hidden = append(set.Hidden, id)
	}
	before := readSettings()
	if err := writeSettings(set); err != nil {
		return err
	}
	// The app always has a station to open: the last collection with one stays.
	if hide && defaultStation(Library()) == "" {
		if err := writeSettings(before); err != nil {
			return err
		}
		return errors.New("this is the last collection with a station: import or show another one first")
	}
	return nil
}

// ImportCollection lets the user pick a .taktwin file in the system's dialog and installs it.
// Cancelling it changes nothing and gives an empty id.
func (s *LibraryService) ImportCollection() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("no window to choose a file in")
	}
	path, err := app.Dialog.OpenFile().SetTitle("Import a collection").AddFilter("Taktwin collection", "*"+model.PackageExtension).
		CanChooseFiles(true).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	c, err := InstallCollection(path)
	return c.ID, err
}

// InstallCollection copies a collection file into the user folder's collections folder, as
// <id>.taktwin. One with the same id that is installed already is replaced, which is how a
// collection is updated.
func InstallCollection(path string) (model.CollectionDef, error) {
	layer, err := readPackage(path)
	if err != nil {
		return model.CollectionDef{}, err
	}
	c, ok, err := model.ReadCollection(layer)
	if err != nil {
		return c, err
	}
	if !ok {
		return c, fmt.Errorf("%s is no collection: it has no %s", filepath.Base(path), model.CollectionFile)
	}
	if stations := model.NewLibrary(layer).Stations(); len(stations) == 0 && count(layer, "modules", true)+count(layer, "accessories", true)+count(layer, "parts", false) == 0 {
		return c, fmt.Errorf("the collection %s holds nothing", c.ID)
	}
	dir, err := MakeUserFolder()
	if err != nil {
		return c, err
	}
	for _, src := range sources() {
		if src.kind == SourceInstalled && src.collection.ID == c.ID && src.path != "" {
			if err := os.RemoveAll(src.path); err != nil {
				return c, err
			}
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	return c, os.WriteFile(filepath.Join(dir, CollectionsFolder, c.ID+model.PackageExtension), data, 0o644)
}

// RemoveCollection deletes an installed collection from the user folder. The built-in one
// can only be hidden.
func (s *LibraryService) RemoveCollection(id string) error {
	other := ""
	removed := false
	for _, src := range sources() {
		switch {
		case src.collection.ID != id || id == "":
		case src.kind != SourceInstalled:
			other = src.kind
		default:
			if err := os.RemoveAll(src.path); err != nil {
				return err
			}
			removed = true
		}
	}
	switch {
	case removed:
		return nil
	case other != "":
		return fmt.Errorf("the collection %s is %s and can only be hidden", id, other)
	}
	return fmt.Errorf("there is no installed collection %s", id)
}
