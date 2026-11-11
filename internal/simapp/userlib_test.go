package simapp

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestUserFolderIsInTheSystemsDocuments(t *testing.T) {
	t.Setenv("TAKTWIN_HOME", "")
	if got := UserFolder(); filepath.Base(got) != "Taktwin" || !filepath.IsAbs(got) {
		t.Errorf("user folder %q", got)
	}
}

// The app always has a station to open: the last collection with one cannot be hidden.
func TestTheLastCollectionWithAStationStays(t *testing.T) {
	t.Setenv("TAKTWIN_HOME", t.TempDir())
	t.Setenv("TAKTWIN_CONFIG", t.TempDir())
	t.Setenv("TAKTWIN_COLLECTIONS", "")
	lib := &LibraryService{}
	if err := lib.SetCollectionHidden("taktwin", true); err == nil {
		t.Fatal("the reference collection is the only one, and stays")
	}
	if slices.Contains(readSettings().Hidden, "taktwin") || defaultStation(Library()) != "pallet-loading" {
		t.Errorf("nothing changed: %v", readSettings().Hidden)
	}
}
