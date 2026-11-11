package simapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SettingsService holds the app-wide settings, such as where the user's own stations are.
type SettingsService struct{}

// ServiceStartup creates the user folder, so that it is there to put stations into.
func (SettingsService) ServiceStartup(context.Context, application.ServiceOptions) error {
	if _, err := MakeUserFolder(); err != nil {
		log.Printf("cannot create the user folder: %v", err)
	}
	return nil
}

// UserFolder is where the app reads the user's own stations and packages from.
func (SettingsService) UserFolder() string { return UserFolder() }

// FolderInfo says where the user folder is, whether it is the default one, and whether the
// environment fixes it, so that it cannot be changed in the app.
type FolderInfo struct {
	Path    string `json:"path"`
	Default bool   `json:"default"`
	Fixed   bool   `json:"fixed"`
}

// Folder describes the user folder.
func (SettingsService) Folder() FolderInfo {
	return FolderInfo{Path: UserFolder(), Default: readSettings().UserFolder == "", Fixed: os.Getenv("TAKTWIN_HOME") != ""}
}

// SetUserFolder makes a folder the user folder, creating its stations folder in it, or goes
// back to the default one for an empty path. The stations already in the old one stay there.
func (s SettingsService) SetUserFolder(path string) (FolderInfo, error) {
	if os.Getenv("TAKTWIN_HOME") != "" {
		return s.Folder(), errors.New("TAKTWIN_HOME sets the folder, so it cannot be changed here")
	}
	if path != "" {
		if !filepath.IsAbs(path) {
			return s.Folder(), fmt.Errorf("%q is not a full path", path)
		}
		if err := os.MkdirAll(filepath.Join(path, "stations"), 0o755); err != nil {
			return s.Folder(), fmt.Errorf("cannot use %s: %w", path, err)
		}
	}
	set := readSettings()
	set.UserFolder = path
	if err := writeSettings(set); err != nil {
		return s.Folder(), err
	}
	return s.Folder(), nil
}

// ChooseUserFolder lets the user pick another user folder in the system's dialog. Cancelling
// it changes nothing.
func (s SettingsService) ChooseUserFolder() (FolderInfo, error) {
	app := application.Get()
	if app == nil {
		return s.Folder(), errors.New("no window to choose a folder in")
	}
	path, err := app.Dialog.OpenFile().SetTitle("Folder for your stations").CanChooseDirectories(true).CanChooseFiles(false).
		CanCreateDirectories(true).SetDirectory(UserFolder()).PromptForSingleSelection()
	if err != nil || path == "" {
		return s.Folder(), err
	}
	return s.SetUserFolder(path)
}

// OpenUserFolder shows the user folder in the system's file manager, creating it first.
func (SettingsService) OpenUserFolder() error {
	dir, err := MakeUserFolder()
	if err != nil {
		return err
	}
	return application.Get().Env.OpenFileManager(dir, false)
}
