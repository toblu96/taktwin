package simapp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// EditorState is where the programming tool of a PLC was left: its open files, the one in
// front, and the place in each. It is kept on this computer, by project, not in the project
// file, so that switching files changes no project and a project handed on opens as its
// receiver last left it.
type EditorState struct {
	Open   []string `json:"open"`
	Active string   `json:"active"`
	// Places are where the cursor and the view were in each file, by name.
	Places map[string]EditorPlace `json:"places,omitempty"`
}

// EditorPlace is where the cursor was in a file, as an offset, and how far it was scrolled.
type EditorPlace struct {
	Cursor int     `json:"cursor"`
	Top    float64 `json:"top"`
}

// editorFile is where the editors of a project are remembered, by its id.
func editorFile(id string) string { return filepath.Join(configDir(), "editors", id+".json") }

// EditorState says where the programming tool of one of the station's PLCs was left in the
// open project, empty for none.
func (p *ProgramService) EditorState(plc string) EditorState {
	all := p.editorStates()
	st := all[p.editorKey(plc)]
	if st.Open == nil {
		st.Open = []string{}
	}
	return st
}

// SetEditorState remembers where the programming tool of one of the station's PLCs is left.
func (p *ProgramService) SetEditorState(plc string, st EditorState) error {
	p.sim.mu.Lock()
	id := p.sim.proj.def.ID
	p.sim.mu.Unlock()
	if !validID(id) {
		return errNoProject
	}
	all := p.editorStates()
	all[p.editorKey(plc)] = st
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(editorFile(id)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(editorFile(id), data, 0o644)
}

// editorStates are the remembered editors of the open project, by PLC.
func (p *ProgramService) editorStates() map[string]EditorState {
	p.sim.mu.Lock()
	id := p.sim.proj.def.ID
	p.sim.mu.Unlock()
	all := map[string]EditorState{}
	if !validID(id) {
		return all
	}
	if data, err := os.ReadFile(editorFile(id)); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	return all
}

// editorKey is the PLC's module, "" being the station's first.
func (p *ProgramService) editorKey(plc string) string {
	p.sim.mu.Lock()
	defer p.sim.mu.Unlock()
	if _, id, err := p.sim.plcOf(plc); err == nil {
		return id
	}
	return plc
}
