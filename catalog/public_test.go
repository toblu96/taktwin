package catalog

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// forbidden are words of a particular vendor's training systems and of the course that used
// them, which the app's own files do not use: those systems come as a collection of their
// own, with their tests and course material.
var forbidden = regexp.MustCompile(`(?i)festo|\bmps\b|syslink|trolley|aufgabe|lösung|loesung`)

// private are the paths that hold such a collection and what goes with it, until they move
// to the repository of that collection.
var private = []string{
	"collections/",
	"context/",
	"tools/",
	"internal/acceptance/",
	"internal/collectiontest/",
	"internal/sim/testdata/stations/four-chutes/",
	"catalog/public_test.go",
}

// The app's own files, from the Go code and the frontend to the built-in catalog and the
// schemas, name nothing of another collection.
func TestTheAppsFilesAreNeutral(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard").Output()
	if err != nil {
		t.Skip("git lists the files:", err)
	}
	checked := 0
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if isPrivate(name) || !text(name) {
			continue
		}
		f, err := os.Open(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue // deleted, not yet committed
		}
		if err != nil {
			t.Fatal(err)
		}
		lines := bufio.NewScanner(f)
		lines.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for n := 1; lines.Scan(); n++ {
			if m := forbidden.Find(lines.Bytes()); m != nil {
				t.Errorf("%s:%d: %q", name, n, m)
			}
		}
		f.Close()
		checked++
	}
	if checked < 100 {
		t.Errorf("only %d files checked", checked)
	}
}

func isPrivate(name string) bool {
	if strings.HasSuffix(name, "_mps_test.go") {
		return true
	}
	for _, p := range private {
		if name == p || strings.HasSuffix(p, "/") && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// text says whether a file is one people write, which the check reads.
func text(name string) bool {
	switch filepath.Ext(name) {
	case ".go", ".ts", ".vue", ".js", ".json", ".md", ".st", ".css", ".html", ".yml", ".yaml", ".txt", ".toml", ".sh", ".py":
		return !bytes.Contains([]byte(name), []byte("node_modules/"))
	}
	return false
}
