package simapp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// A project is its .taktwin-project file, wherever it is. The app works in a copy of it, the
// working copy, in its settings folder, and writes the file anew shortly after every change:
// into a temporary file beside it, renamed over it, so the file is never half written. A file
// changed from elsewhere is not overwritten, one that cannot be written opens read only, and a
// working copy left by a crash is offered back. See ADR 0002.

// The states of saving a project file.
const (
	// SaveSaved is a file that has everything the project has.
	SaveSaved = "saved"
	// SavePending is a project changed since, which is written shortly.
	SavePending = "pending"
	// SaveReadOnly is a file that cannot be written, such as on a read-only share: the project
	// keeps its changes in the working copy until it is saved as a copy.
	SaveReadOnly = "readOnly"
	// SaveConflict is a file changed, moved or deleted from elsewhere since the app read or
	// wrote it, which it does not overwrite without asking.
	SaveConflict = "conflict"
	// SaveFailed is a file that could not be written, such as one locked by another program;
	// it is tried again.
	SaveFailed = "failed"
)

// SaveState says how the open project's file stands.
type SaveState struct {
	State string `json:"state"`
	// Problem says why the file is not saved, for a conflict or a failure.
	Problem string `json:"problem,omitempty"`
	// Saved is when the file was last written or read, as RFC 3339.
	Saved string `json:"saved,omitempty"`
	// Unsaved says that the project has changes its file lacks: always while pending, and
	// for a file that is read only, in conflict or failed, once something was changed.
	Unsaved bool `json:"unsaved,omitempty"`
}

// SaveStateEvent carries the open project's SaveState to the frontend when it changes.
const SaveStateEvent = "project:save"

// saveEvery is how often the working copy is looked at for changes; a change is written once
// it has stayed for one more look.
var saveEvery = time.Second

// stamp is how a file stood when it was read or written: enough to see that it was changed.
type stamp struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

func stampOf(p string) (stamp, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return stamp{}, err
	}
	return stamp{fi.Size(), fi.ModTime()}, nil
}

// copyMeta is kept beside a working copy, so that a working copy a crash left behind is
// known: which file it is of, and its content as last written to or read from that file.
type copyMeta struct {
	Path  string `json:"path"`
	Saved string `json:"saved"`
	Stamp stamp  `json:"stamp"`
	// Changed is when the working copy was last seen changed, as RFC 3339.
	Changed string `json:"changed,omitempty"`
}

// configDir is the app's settings folder. TAKTWIN_CONFIG overrides it.
func configDir() string {
	if dir := os.Getenv("TAKTWIN_CONFIG"); dir != "" {
		return dir
	}
	return filepath.Join(application.Path(application.PathConfigHome), "Taktwin")
}

// workingDir is where the working copies are kept, by the projects' ids.
func workingDir() string { return filepath.Join(configDir(), ProjectsFolder) }

// validID says whether an id can name a working copy: letters, digits and dashes only.
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// newID makes a project's id, a random UUID.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// document is the open project's file and its working copy.
type document struct {
	path string
	id   string
	// dir is the working copy.
	dir string

	mu       sync.Mutex
	readOnly bool
	// stamp is the file as the app last read or wrote it.
	stamp stamp
	// saved is the working copy's content as last written to or read from the file.
	saved string
	// seen is the working copy's content at the last look, and when it changed.
	seen    string
	changed time.Time
	state   SaveState
	notify  func(SaveState)

	stop chan struct{}
	done chan struct{}
}

// treeHash sums a folder's files, their paths and content, so that any change shows.
func treeHash(dir string) (string, error) {
	files, err := readTree(dir)
	if err != nil {
		return "", err
	}
	return filesHash(files), nil
}

// filesHash sums files by their paths and content.
func filesHash(files map[string][]byte) string {
	h := sha256.New()
	for _, name := range slices.Sorted(maps.Keys(files)) {
		fmt.Fprintf(h, "%s\x00%d\x00", name, len(files[name]))
		h.Write(files[name])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// readTree reads a folder's files by their slashed paths in it.
func readTree(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = data
		return err
	})
	return files, err
}

// writeTree writes files into a folder that is emptied first.
func writeTree(dir string, files map[string][]byte) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, data := range files {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func metaFile(id string) string { return filepath.Join(workingDir(), id+".json") }

func readMeta(id string) (copyMeta, bool) {
	var m copyMeta
	data, err := os.ReadFile(metaFile(id))
	if err != nil || json.Unmarshal(data, &m) != nil {
		return m, false
	}
	return m, true
}

func writeMeta(id string, m copyMeta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaFile(id), data, 0o644)
}

// leftOver says whether a working copy of the project was left with changes its file does
// not have, such as by a crash, and when it was last changed.
func leftOver(id string) (string, bool) {
	if !validID(id) {
		return "", false
	}
	m, ok := readMeta(id)
	if !ok {
		return "", false
	}
	sum, err := treeHash(filepath.Join(workingDir(), id))
	if err != nil || sum == m.Saved {
		return "", false
	}
	return m.Changed, true
}

// dropCopy removes a project's working copy.
func dropCopy(id string) {
	if !validID(id) {
		return
	}
	_ = os.RemoveAll(filepath.Join(workingDir(), id))
	_ = os.Remove(metaFile(id))
}

// writable says whether a file can be written where it is: the file itself, and a new file
// beside it, as saving writes one and renames it over the file.
func writable(p string) bool {
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	tmp, err := os.CreateTemp(filepath.Dir(p), ".taktwin-*.tmp")
	if err != nil {
		return false
	}
	tmp.Close()
	_ = os.Remove(tmp.Name())
	return true
}

// openDocument makes the working copy of a project file read as pf: from the file, or, with
// restore, the one a crash left behind. saved is what the file has, if pf was changed since
// it was read; "" for pf as read.
func openDocument(path string, pf *projectFile, restore bool, saved string) (*document, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := stampOf(path)
	if err != nil {
		return nil, err
	}
	d := &document{path: path, id: pf.def.ID, readOnly: !writable(path), stamp: st}
	d.dir = filepath.Join(workingDir(), d.id)
	if err := os.MkdirAll(workingDir(), 0o755); err != nil {
		return nil, err
	}
	files := pf.all()
	// What the file has is what is saved, also when a working copy is restored.
	d.saved = cmpOr(saved, filesHash(files))
	if _, left := leftOver(d.id); !restore || !left {
		if err := writeTree(d.dir, files); err != nil {
			return nil, err
		}
	}
	d.seen, err = treeHash(d.dir)
	if err != nil {
		return nil, err
	}
	d.state = SaveState{State: SaveSaved, Saved: st.ModTime.UTC().Format(time.RFC3339)}
	switch {
	case d.readOnly:
		d.state.State = SaveReadOnly
	case d.seen != d.saved:
		d.state.State, d.state.Unsaved, d.changed = SavePending, true, time.Now()
	}
	if d.readOnly {
		d.state.Unsaved = d.seen != d.saved
	}
	return d, d.keepMeta()
}

// keepMeta writes what is known of the working copy beside it, under d.mu or before it is
// shared.
func (d *document) keepMeta() error {
	m := copyMeta{Path: d.path, Saved: d.saved, Stamp: d.stamp}
	if !d.changed.IsZero() {
		m.Changed = d.changed.UTC().Format(time.RFC3339)
	}
	return writeMeta(d.id, m)
}

// start looks at the working copy every saveEvery, and writes the file once a change has
// stayed for one look. notify hears of every new state.
func (d *document) start(notify func(SaveState)) {
	stop, done := make(chan struct{}), make(chan struct{})
	d.mu.Lock()
	d.notify = notify
	d.stop, d.done = stop, done
	d.mu.Unlock()
	go func() {
		defer close(done)
		t := time.NewTicker(saveEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				d.look()
			}
		}
	}()
}

// look notes a change of the working copy, and writes the file once it has stayed.
func (d *document) look() {
	sum, err := treeHash(d.dir)
	if err != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if sum != d.seen {
		d.seen, d.changed = sum, time.Now()
		_ = d.keepMeta()
		st := d.state
		st.Unsaved = sum != d.saved
		if st.State == SaveSaved && st.Unsaved {
			st.State = SavePending
		}
		d.set(st)
		return
	}
	if sum != d.saved && (d.state.State == SavePending || d.state.State == SaveFailed) {
		d.write(false)
	}
}

// set changes the state, telling the frontend, under d.mu.
func (d *document) set(st SaveState) {
	if st == d.state {
		return
	}
	d.state = st
	if d.notify != nil {
		go d.notify(st)
	}
}

// halt stops looking at the working copy.
func (d *document) halt() {
	d.mu.Lock()
	stop, done := d.stop, d.done
	d.stop = nil
	d.mu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

// flush writes the file if the working copy has changes it lacks and it can, and gives the
// state then.
func (d *document) flush() SaveState {
	sum, err := treeHash(d.dir)
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		d.set(SaveState{State: SaveFailed, Problem: err.Error(), Saved: d.state.Saved})
		return d.state
	}
	d.seen = sum
	if sum != d.saved && d.state.State != SaveReadOnly && d.state.State != SaveConflict {
		d.write(false)
	}
	st := d.state
	st.Unsaved = d.seen != d.saved
	d.set(st)
	return d.state
}

// write writes the working copy into the file, under d.mu. Unless forced, it does not write
// over a file changed since the app last read or wrote it.
func (d *document) write(force bool) {
	if d.readOnly {
		d.set(SaveState{State: SaveReadOnly, Saved: d.state.Saved, Unsaved: true})
		return
	}
	if !force {
		now, err := stampOf(d.path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			d.set(SaveState{State: SaveConflict, Problem: "The file was moved or deleted.", Saved: d.state.Saved, Unsaved: true})
			return
		case err == nil && (now.Size != d.stamp.Size || !now.ModTime.Equal(d.stamp.ModTime)):
			d.set(SaveState{State: SaveConflict, Problem: "The file was changed from elsewhere.", Saved: d.state.Saved, Unsaved: true})
			return
		}
	}
	files, err := readTree(d.dir)
	if err == nil {
		err = writeProjectOver(d.path, files)
	}
	if err != nil {
		d.set(SaveState{State: SaveFailed, Problem: err.Error(), Saved: d.state.Saved, Unsaved: true})
		return
	}
	d.stamp, _ = stampOf(d.path)
	d.saved, _ = treeHash(d.dir)
	d.seen = d.saved
	d.changed = time.Time{}
	_ = d.keepMeta()
	d.set(SaveState{State: SaveSaved, Saved: time.Now().UTC().Format(time.RFC3339)})
}

// saveState gives the state of the file.
func (d *document) saveState() SaveState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// keepMine writes the working copy over a file changed from elsewhere, or where it was if it
// was moved or deleted.
func (d *document) keepMine() SaveState {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.write(true)
	return d.state
}

// moveTo makes the project that of another file, written there now with a new id, such as
// after Save a copy: the working copy moves with it.
func (d *document) moveTo(to string) error {
	to, err := filepath.Abs(to)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	id := newID()
	if err := setID(d.dir, id); err != nil {
		return err
	}
	files, err := readTree(d.dir)
	if err != nil {
		return err
	}
	if err := writeProjectOver(to, files); err != nil {
		return err
	}
	dir := filepath.Join(workingDir(), id)
	if err := os.Rename(d.dir, dir); err != nil {
		return err
	}
	_ = os.Remove(metaFile(d.id))
	d.path, d.id, d.dir, d.readOnly = to, id, dir, !writable(to)
	d.stamp, _ = stampOf(to)
	d.saved, _ = treeHash(d.dir)
	d.seen, d.changed = d.saved, time.Time{}
	_ = d.keepMeta()
	st := SaveState{State: SaveSaved, Saved: time.Now().UTC().Format(time.RFC3339)}
	if d.readOnly {
		st.State = SaveReadOnly
	}
	d.set(st)
	return nil
}

// setID gives the project.json of a folder another id.
func setID(dir, id string) error {
	def, ok, err := readProject(dir)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s has no %s", dir, ProjectFile)
	}
	def.ID = id
	return writeProject(dir, def)
}

// writeProjectOver writes a project file: files as a zip over to, as writeZipOver does. It
// writes nothing without a project.json that reads, so that a working copy that lost its
// files, whatever the reason, never empties the file it is saved into.
func writeProjectOver(to string, files map[string][]byte) error {
	if _, err := parseProject(files[ProjectFile]); err != nil || len(files[ProjectFile]) == 0 {
		return fmt.Errorf("%s was not written: what it would hold has no project.json that reads, so the file is left as it was", filepath.Base(to))
	}
	return writeZipOver(to, files)
}

// writeZipOver writes files as a zip into a new file beside to, then renames it over to,
// so that to is never left half written. The new file keeps the old one's permissions.
func writeZipOver(to string, files map[string][]byte) error {
	data, err := zipped(files)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(to); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(to), "."+filepath.Base(to)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), to)
}
