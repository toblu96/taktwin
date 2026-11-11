package simapp

import (
	"fmt"
	"maps"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/plcprog"
	"github.com/toblu96/taktwin/st"
)

// softPLCs are the app's soft PLCs, one for each PLC of each station that has been
// programmed or connected.
var softPLCs plcprog.Manager

// ProgramService is the frontend's programming tool for the PLCs of the station in the
// simulation: their Structured Text projects, building and downloading them into the soft
// PLC, and watching, writing and forcing its variables online. Each method takes the PLC
// by its module, the station's first for "".
type ProgramService struct {
	sim *Service
}

// NewProgramService returns the programming tool for the simulation's station.
func NewProgramService(sim *Service) *ProgramService { return &ProgramService{sim: sim} }

// ProgramProject is a PLC's project as the editor opens it.
type ProgramProject struct {
	Station string         `json:"station"`
	PLC     string         `json:"plc"`
	Title   string         `json:"title"`
	Files   []plcprog.File `json:"files"`
	Meta    plcprog.Meta   `json:"meta"`
	Status  ProgramStatus  `json:"status"`
	Folder  string         `json:"folder"`
	// Example lists the files of the example program the station's collection has for this
	// PLC, if it has one.
	Example []string `json:"example"`
}

// ProgramStatus is the soft PLC's state, whether the simulation is connected to it, and the
// station the simulation shows: an editor open on another station's project reloads.
type ProgramStatus struct {
	plcprog.Status
	Connected bool   `json:"connected"`
	Station   string `json:"station"`
}

// BuildResult is what building a project says.
type BuildResult struct {
	OK          bool            `json:"ok"`
	Diagnostics []st.Diagnostic `json:"diagnostics"`
}

// project opens the project of a PLC of the simulation's station, writing its Bits list
// from the station's I/O table.
func (p *ProgramService) project(plc string) (*plcprog.Project, string, string, error) {
	p.sim.mu.Lock()
	station, id, err := p.sim.plcOf(plc)
	scene := p.sim.engine.Plan().Scene
	p.sim.mu.Unlock()
	if err != nil {
		return nil, "", "", err
	}
	table := scene.PLC
	for _, sp := range scene.PLCs {
		if sp.Module == id {
			table = sp
		}
	}
	proj, err := plcprog.Open(filepath.Join(UserFolder(), "programs", station, id), plcprog.Signals(table))
	return proj, station, id, err
}

// Open opens the project of a PLC, making it from the template the first time.
func (p *ProgramService) Open(plc string) (ProgramProject, error) {
	proj, station, id, err := p.project(plc)
	if err != nil {
		return ProgramProject{}, err
	}
	files, err := proj.Files()
	if err != nil {
		return ProgramProject{}, err
	}
	p.sim.mu.Lock()
	name := p.sim.bundle.Station.Name
	p.sim.mu.Unlock()
	example, _ := Library().Folder(exampleDir(station, id))
	names := slices.Sorted(maps.Keys(example))
	if names == nil {
		names = []string{}
	}
	return ProgramProject{Station: station, PLC: id, Title: fmt.Sprintf("%s – %s", name, id), Files: files, Meta: proj.Meta,
		Status: p.status(station, id, plc), Folder: proj.Dir, Example: names}, nil
}

// exampleDir is where a collection keeps the example program for a PLC of a station.
func exampleDir(station, plc string) string { return path.Join("programs", station, plc) }

// LoadExample copies the example program of the station's collection into the project of a
// PLC, over the files of the same names. It gives the files it wrote.
func (p *ProgramService) LoadExample(plc string) ([]string, error) {
	proj, station, id, err := p.project(plc)
	if err != nil {
		return nil, err
	}
	files, err := Library().Folder(exampleDir(station, id))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("the station has no example program for %s", id)
	}
	names := slices.Sorted(maps.Keys(files))
	for _, name := range names {
		if err := proj.Write(name, string(files[name])); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// ReadFile gives a source file.
func (p *ProgramService) ReadFile(plc, name string) (string, error) {
	proj, _, _, err := p.project(plc)
	if err != nil {
		return "", err
	}
	return proj.Read(name)
}

// SaveFile saves a source file of the project of a station's PLC, both as the project names
// them: the file goes back to where it was opened from, also when the simulation has
// changed to another station since.
func (p *ProgramService) SaveFile(station, plc, name, src string) error {
	if !folderName(station) || !folderName(plc) {
		return fmt.Errorf("there is no project for %q of %q", plc, station)
	}
	proj := &plcprog.Project{Dir: filepath.Join(UserFolder(), "programs", station, plc)}
	return proj.Write(name, src)
}

// folderName tells whether a station or module id is one folder's name, and no way out of
// the programs folder.
func folderName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`) && filepath.Base(s) == s
}

// AddFile makes a new file of a kind: PROGRAM, FUNCTION_BLOCK, FUNCTION, GVL or TYPE. It
// gives the file's name.
func (p *ProgramService) AddFile(plc, name, kind string) (string, error) {
	proj, _, _, err := p.project(plc)
	if err != nil {
		return "", err
	}
	return proj.Add(name, kind)
}

// RemoveFile deletes a file.
func (p *ProgramService) RemoveFile(plc, name string) error {
	proj, _, _, err := p.project(plc)
	if err != nil {
		return err
	}
	return proj.Remove(name)
}

// RenameFile renames a file.
func (p *ProgramService) RenameFile(plc, name, to string) error {
	proj, _, _, err := p.project(plc)
	if err != nil {
		return err
	}
	return proj.Rename(name, to)
}

// SetTask changes the task: its programs, interval and step variable.
func (p *ProgramService) SetTask(plc string, meta plcprog.Meta) error {
	proj, _, _, err := p.project(plc)
	if err != nil {
		return err
	}
	return proj.SetMeta(meta)
}

// Build compiles the project as saved, and says what is wrong with it.
func (p *ProgramService) Build(plc string) (BuildResult, error) {
	proj, _, _, err := p.project(plc)
	if err != nil {
		return BuildResult{}, err
	}
	prog, diags, err := proj.Build()
	if err != nil {
		return BuildResult{}, err
	}
	if diags == nil {
		diags = []st.Diagnostic{}
	}
	return BuildResult{OK: prog != nil, Diagnostics: diags}, nil
}

// Download builds the project and loads it into the soft PLC, stopped with every variable
// at its initial value, as a login with download does.
func (p *ProgramService) Download(plc string) (BuildResult, error) {
	proj, station, id, err := p.project(plc)
	if err != nil {
		return BuildResult{}, err
	}
	prog, diags, err := proj.Build()
	if err != nil {
		return BuildResult{}, err
	}
	if diags == nil {
		diags = []st.Diagnostic{}
	}
	if prog == nil {
		return BuildResult{Diagnostics: diags}, nil
	}
	soft := softPLCs.Get(station, id)
	p.sim.mu.Lock()
	naming := p.sim.naming(plc)
	p.sim.mu.Unlock()
	soft.SetNaming(naming)
	if err := soft.Serve(p.sim.ctx); err != nil {
		return BuildResult{}, err
	}
	if err := soft.Download(prog, proj.Meta.Step); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{OK: true, Diagnostics: diags}, nil
}

func (p *ProgramService) soft(plc string) (*plcprog.Soft, error) {
	p.sim.mu.Lock()
	station, id, err := p.sim.plcOf(plc)
	p.sim.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return softPLCs.Get(station, id), nil
}

// Start runs the program.
func (p *ProgramService) Start(plc string) error {
	s, err := p.soft(plc)
	if err != nil {
		return err
	}
	return s.Start()
}

// Stop holds the program.
func (p *ProgramService) Stop(plc string) error {
	s, err := p.soft(plc)
	if err != nil {
		return err
	}
	s.Stop()
	return nil
}

// Reset resets the program's variables, cold or keeping RETAIN ones, and stops it.
func (p *ProgramService) Reset(plc string, cold bool) error {
	s, err := p.soft(plc)
	if err != nil {
		return err
	}
	s.ResetProgram(cold)
	return nil
}

// Status reports the soft PLC's state.
func (p *ProgramService) Status(plc string) (ProgramStatus, error) {
	p.sim.mu.Lock()
	station, id, err := p.sim.plcOf(plc)
	p.sim.mu.Unlock()
	if err != nil {
		return ProgramStatus{}, err
	}
	return p.status(station, id, plc), nil
}

func (p *ProgramService) status(station, id, plc string) ProgramStatus {
	return ProgramStatus{Status: softPLCs.Get(station, id).Status(), Connected: p.sim.PLCFor(plc).Mode == PLCSoft, Station: station}
}

// Variables lists the program's variables with their values, online.
func (p *ProgramService) Variables(plc string) ([]st.WatchVar, error) {
	s, err := p.soft(plc)
	if err != nil {
		return nil, err
	}
	return s.Variables(), nil
}

// Write sets a variable once, to a value written as a literal.
func (p *ProgramService) Write(plc, path, value string) error {
	s, err := p.soft(plc)
	if err != nil {
		return err
	}
	return s.Write(path, value)
}

// Force holds a variable at a value until it is released.
func (p *ProgramService) Force(plc, path, value string) error {
	s, err := p.soft(plc)
	if err != nil {
		return err
	}
	return s.Force(path, value)
}

// Release lets a forced variable go; "" lets all go.
func (p *ProgramService) Release(plc, path string) error {
	s, err := p.soft(plc)
	if err != nil {
		return err
	}
	s.Release(path)
	return nil
}

// Connect connects the simulation to the soft PLC of a PLC, over OPC UA.
func (p *ProgramService) Connect(plc string) error {
	return p.sim.SetPLCFor(plc, PLCConfig{Mode: PLCSoft})
}

// OpenWindow opens the programming tool of a PLC in a window of its own, to have it beside
// the simulation.
func (p *ProgramService) OpenWindow(plc string) error {
	app := application.Get()
	if app == nil {
		return fmt.Errorf("there is no app to open a window in")
	}
	proj, err := p.Open(plc)
	if err != nil {
		return err
	}
	q := url.Values{"ide": {proj.PLC}, "station": {proj.Station}}
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "PLC " + proj.Title,
		Width:            1280,
		Height:           820,
		MinWidth:         900,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(11, 31, 34),
		URL:              "/?" + q.Encode(),
	})
	return nil
}
