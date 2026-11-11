package simapp

import (
	"os"
	"path/filepath"
	"testing"
)

// The user folder can be moved to another folder, which then holds a stations folder, and
// back to the default one; the settings keep it between runs.
func TestChangingTheUserFolder(t *testing.T) {
	t.Setenv("TAKTWIN_HOME", "")
	os.Unsetenv("TAKTWIN_HOME")
	t.Setenv("TAKTWIN_CONFIG", t.TempDir())
	var s SettingsService
	if info := s.Folder(); !info.Default || info.Fixed {
		t.Fatalf("it starts at the default folder: %+v", info)
	}
	if _, err := s.SetUserFolder("relative/path"); err == nil {
		t.Error("a folder must be given in full")
	}
	dir := filepath.Join(t.TempDir(), "My stations")
	info, err := s.SetUserFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != dir || info.Default || UserFolder() != dir {
		t.Errorf("the folder is the chosen one: %+v", info)
	}
	if st, err := os.Stat(filepath.Join(dir, "stations")); err != nil || !st.IsDir() {
		t.Errorf("it has a stations folder: %v", err)
	}
	if readSettings().UserFolder != dir {
		t.Error("the settings keep it")
	}
	if info, err := s.SetUserFolder(""); err != nil || !info.Default || info.Path != DefaultUserFolder() {
		t.Errorf("an empty path goes back to the default: %+v %v", info, err)
	}
	t.Setenv("TAKTWIN_HOME", t.TempDir())
	if _, err := s.SetUserFolder(dir); err == nil || !s.Folder().Fixed {
		t.Error("TAKTWIN_HOME fixes the folder")
	}
}
