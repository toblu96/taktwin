package simapp

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// UserFolder is where people keep their own stations, outside the app: a stations folder in
// the same layout as the catalog, and station packages next to it. It is the folder chosen
// in the settings, or else a Taktwin folder in the documents folder the system names, which
// on Windows may be in OneDrive and on Linux has the language of the desktop. TAKTWIN_HOME
// overrides both.
func UserFolder() string {
	if dir := os.Getenv("TAKTWIN_HOME"); dir != "" {
		return dir
	}
	if dir := readSettings().UserFolder; dir != "" {
		return dir
	}
	return DefaultUserFolder()
}

// DefaultUserFolder is the user folder when none is chosen.
func DefaultUserFolder() string {
	return filepath.Join(application.Path(application.PathDocuments), "Taktwin")
}

// settings are what the app remembers between runs, in its folder of the system's config
// folder.
type settings struct {
	// UserFolder is the user folder chosen in the settings, or empty for the default one.
	UserFolder string `json:"userFolder,omitempty"`
	// Hidden are the collections, by id, that the library leaves out.
	Hidden []string `json:"hiddenCollections,omitempty"`
}

// settingsFile is where the settings are kept. TAKTWIN_CONFIG overrides its folder.
func settingsFile() string {
	dir := os.Getenv("TAKTWIN_CONFIG")
	if dir == "" {
		dir = filepath.Join(application.Path(application.PathConfigHome), "Taktwin")
	}
	return filepath.Join(dir, "settings.json")
}

func readSettings() settings {
	var s settings
	if data, err := os.ReadFile(settingsFile()); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func writeSettings(s settings) error {
	path := settingsFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// MakeUserFolder creates the user folder with its stations and collections folders, so that
// there is a place to put files into.
func MakeUserFolder() (string, error) {
	dir := UserFolder()
	for _, sub := range []string{"stations", CollectionsFolder} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return dir, err
		}
	}
	return dir, nil
}
