package simapp

import (
	"path/filepath"
	"slices"
	"time"
)

// recentProject is a project file the app opened, for the projects page to list wherever it
// is.
type recentProject struct {
	Path string `json:"path"`
	// Opened is when it was last opened, as RFC 3339.
	Opened string `json:"opened"`
}

// remember puts a project file at the top of the recent list.
func remember(file string) {
	if abs, err := filepath.Abs(file); err == nil {
		file = abs
	}
	set := readSettings()
	set.Recent = slices.DeleteFunc(set.Recent, func(r recentProject) bool { return r.Path == file })
	set.Recent = append([]recentProject{{Path: file, Opened: time.Now().UTC().Format(time.RFC3339)}}, set.Recent...)
	if len(set.Recent) > maxRecent {
		set.Recent = set.Recent[:maxRecent]
	}
	_ = writeSettings(set)
}

// forget takes a project file off the recent list.
func forget(file string) {
	set := readSettings()
	n := len(set.Recent)
	set.Recent = slices.DeleteFunc(set.Recent, func(r recentProject) bool { return r.Path == file })
	if len(set.Recent) != n {
		_ = writeSettings(set)
	}
}

// moved follows a project file renamed or moved by the app in the recent list.
func moved(file, to string) {
	set := readSettings()
	for i := range set.Recent {
		if set.Recent[i].Path == file {
			set.Recent[i].Path = to
			_ = writeSettings(set)
			return
		}
	}
}
