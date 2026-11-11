package simapp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/sim"
	"github.com/toblu96/taktwin/internal/softplc"
)

// BuilderEvent is emitted once per frame with the snapshot of the station being built.
const BuilderEvent = "builder:snapshot"

// BuilderService edits one station at a time. It holds the station as it is being built,
// checks every change by loading it, and runs it without a PLC so that it can be looked at
// and tried. A change that does not load is refused, and the station stays as it was.
type BuilderService struct {
	mu     sync.Mutex
	ctx    context.Context
	emit   func(engine.Snapshot)
	lib    *model.Library
	draft  *draft
	bundle *model.Bundle
	plan   *model.Plan
	engine *engine.Engine
	stop   context.CancelFunc
	saved  string
}

// NewBuilderService builds stations from the catalog and the user's own files.
func NewBuilderService() *BuilderService {
	return &BuilderService{
		ctx: context.Background(),
		emit: func(snap engine.Snapshot) {
			application.Get().Event.Emit(BuilderEvent, snap)
		},
	}
}

// ServiceStartup keeps the context that ends the builder's engine with the app.
func (s *BuilderService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.ctx = ctx
	return nil
}

// ServiceShutdown stops the builder's engine.
func (s *BuilderService) ServiceShutdown() error {
	s.Close()
	return nil
}

// BuilderState is what the Builder page shows: the station, its modules with where they
// are screwed on, and every place a module could be screwed to.
type BuilderState struct {
	Scene   model.Scene      `json:"scene"`
	Station BuilderStation   `json:"station"`
	Modules []BuilderModule  `json:"modules"`
	Sites   []model.SitePlan `json:"sites"`
	// Saved is the file the station was last saved to, or empty.
	Saved string `json:"saved"`
	// Paths are where workpieces travel, and Marks the special positions of the modules.
	Paths []BuilderPath `json:"paths"`
	Marks []sim.Mark    `json:"marks"`
}

// BuilderPath is a transport of a module as a line through points, in station coordinates.
type BuilderPath struct {
	Owner  string        `json:"owner"`
	Points []model.Point `json:"points"`
}

// BuilderStation names the station being built.
type BuilderStation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// BuilderModule is a module of the station. Place is nil for one that stands at a fixed
// position, such as the stand. A part of another module, such as a conveyor's sensor, has
// that module as Parent; Optional says whether the station may leave it out. Line is the
// site it is screwed onto. Removed lists the optional parts the module is without.
type BuilderModule struct {
	ID       string          `json:"id"`
	Module   string          `json:"module"`
	Name     string          `json:"name"`
	Place    *BuilderPlace   `json:"place"`
	Mounts   []string        `json:"mounts"`
	Params   []BuilderParam  `json:"params"`
	Parent   string          `json:"parent"`
	Optional bool            `json:"optional"`
	Line     *model.SitePlan `json:"line"`
	Removed  []BuilderPart   `json:"removed"`
	// Floor is the size of one that stands on the floor, such as a base plate.
	Floor *BuilderFloor `json:"floor"`
}

// BuilderFloor is how much room a module takes on the floor, in mm along x and y.
type BuilderFloor struct {
	Width float64 `json:"width"`
	Depth float64 `json:"depth"`
}

// BuilderPart is a part that a module comes with, by its full id and name.
type BuilderPart struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BuilderPlace is where a module is screwed on: a site of another module, which repeat of
// it, how far along in mm and how far turned in degrees.
type BuilderPlace struct {
	On   string  `json:"on"`
	Slot int     `json:"slot"`
	At   float64 `json:"at"`
	Turn float64 `json:"turn"`
	// ClearRail removes what stands where a rail has to be cut for a slide.
	ClearRail bool `json:"clearRail"`
}

// BuilderParam is a setting of a module: what the module has by default, and what this
// station gives it, empty for the default.
type BuilderParam struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Unit    string `json:"unit"`
	Default string `json:"default"`
	Value   string `json:"value"`
	// Kind is build or physics, for a setting people see.
	Kind string `json:"kind"`
	// Flag is a setting that is on or off.
	Flag bool `json:"flag"`
	// Description says what it does. A number keeps between Min and Max in steps of Step, or
	// takes one of Options, which Names say what they are.
	Description string    `json:"description"`
	Min         *float64  `json:"min"`
	Max         *float64  `json:"max"`
	Step        float64   `json:"step"`
	Options     []float64 `json:"options"`
	Names       []string  `json:"names"`
}

// Start begins a new station: a copy of the station with that id, or an empty stand.
func (s *BuilderService) Start(from string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lib = Library()
	if collection, ok := strings.CutPrefix(from, EmptyFrom); ok || from == "" {
		if !ok {
			// That of the station the app opens first.
			collection = s.lib.StationCollection(defaultStation(s.lib))
		}
		return s.startEmpty(collection)
	}
	data, err := s.lib.ReadFile(filepath.ToSlash(filepath.Join("stations", from, "station.json")))
	if err != nil {
		return BuilderState{}, err
	}
	d := &draft{}
	if err := json.Unmarshal(data, d); err != nil {
		return BuilderState{}, fmt.Errorf("station %s: %w", from, err)
	}
	d.ID, d.Name = from+"-copy", "Copy of "+d.Name
	// A program belongs to the original, and would not fit what is built.
	d.PLC.Program = ""
	s.saved = ""
	return s.commit(d)
}

// State returns the station as it is now.
func (s *BuilderService) State() (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	return s.state(), nil
}

// Add puts a module into the station, screwed onto the first place where it fits, and
// wires it to a free socket or terminal pin.
func (s *BuilderService) Add(module string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	d, err := s.added(s.draft, module, "")
	if err != nil {
		return BuilderState{}, err
	}
	return s.commit(d)
}

// AddAt puts a module into the station at a place picked for it, and wires it up.
func (s *BuilderService) AddAt(module string, place BuilderPlace) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	def, err := s.module(module)
	if err != nil {
		return BuilderState{}, err
	}
	d, id, _, err := s.placed(module, "", place)
	if err != nil {
		return BuilderState{}, plain(err)
	}
	if err := s.wire(d, def, id); err != nil {
		return BuilderState{}, err
	}
	return s.commit(d)
}

// UserStations lists the stations saved in the user folder, which can be deleted.
func (s *BuilderService) UserStations() []string {
	entries, _ := os.ReadDir(filepath.Join(UserFolder(), "stations"))
	out := []string{}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(UserFolder(), "stations", e.Name(), "station.json")); e.IsDir() && err == nil {
			out = append(out, e.Name())
		}
	}
	return out
}

// DeleteStation removes a station from the user folder, with its whole folder. Stations of
// the app itself cannot be deleted.
func (s *BuilderService) DeleteStation(id string) error {
	if !safeName.MatchString(id) {
		return fmt.Errorf("invalid station id %q", id)
	}
	dir := filepath.Join(UserFolder(), "stations", id)
	if _, err := os.Stat(filepath.Join(dir, "station.json")); err != nil {
		return fmt.Errorf("%s is not one of your stations", id)
	}
	return os.RemoveAll(dir)
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Move screws a module onto another place: a site of another module, which repeat of it,
// how far along and how far turned. A part of a module moves on the sites of that module.
func (s *BuilderService) Move(id string, place BuilderPlace) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	if place.On == floorSite {
		d := s.draft.clone()
		if err := s.moveStand(d, id, place); err != nil {
			return BuilderState{}, err
		}
		st, err := s.commit(d)
		return st, plain(err)
	}
	d, _, _, err := s.placed("", id, place)
	if err != nil {
		return BuilderState{}, plain(err)
	}
	return s.commit(d)
}

// Fit moves a module along the site it is on, to the nearest place where the station has
// the fewest layout problems, such as a slide right under its gate.
func (s *BuilderService) Fit(id string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	d := s.draft.clone()
	place, err := s.placeRef(d, id)
	if err != nil {
		return BuilderState{}, err
	}
	now := s.plan.Placed[id]
	site := s.site(now.On, now.Slot)
	if site == nil {
		return BuilderState{}, fmt.Errorf("%s is on no site", id)
	}
	from := place.At
	// Docks and signal names do not change where things fit, so the tries go without.
	bare := d.clone()
	bare.Actions, bare.Docks, bare.PLC.Signals = nil, nil, nil
	moving, _ := s.placeRef(bare, id)
	try := func(at float64) (int, bool) {
		moving.At = at
		_, plan, err := s.load(bare)
		if err != nil {
			return 0, false
		}
		return len(sim.CheckLayout(plan)), true
	}
	best, fewest := from, math.MaxInt
	search := func(lo, hi, step float64) {
		for at := lo; at <= hi+1e-9; at += step {
			at := round(at)
			if n, ok := try(at); ok && (n < fewest || n == fewest && math.Abs(at-from) < math.Abs(best-from)) {
				best, fewest = at, n
			}
		}
	}
	// Coarse, then in whole mm around the best. A gripper may miss by 3 mm, so 5 mm steps find it.
	search(math.Ceil(site.Min), site.Max, 5)
	search(math.Max(site.Min, best-5), math.Min(site.Max, best+5), 1)
	if fewest == math.MaxInt {
		return BuilderState{}, fmt.Errorf("%s fits nowhere on %s", id, now.On)
	}
	place.At = best
	return s.commit(d)
}

// Remove takes a module out of the station, with everything screwed onto it and every
// cable and wire to it. A part that a module comes with is left out of it, if it may be.
func (s *BuilderService) Remove(id string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	d := s.draft.clone()
	if _, err := s.drop(d, id); err != nil {
		return BuilderState{}, err
	}
	return s.commit(d)
}

// PutBack gives a module back a part that it comes with and was left out, where it was.
func (s *BuilderService) PutBack(id string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	d := s.draft.clone()
	parent, child, ok := strings.Cut(id, "/")
	i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == parent })
	if !ok || i < 0 || d.Modules[i].Parts[child] == nil || !d.Modules[i].Parts[child].Removed {
		return BuilderState{}, fmt.Errorf("%s is not a part that was left out", id)
	}
	part := d.Modules[i].Parts[child]
	part.Removed = false
	if part.Place == nil && len(part.Params) == 0 {
		delete(d.Modules[i].Parts, child)
	}
	st, err := s.commit(d)
	return st, plain(err)
}

// drop takes a module out of a station, or leaves a part out of the module it belongs to,
// with what is screwed onto it and the cables and wires to it. It returns its name.
func (s *BuilderService) drop(d *draft, id string) (string, error) {
	name := id
	for _, in := range s.plan.Scene.Instances {
		if in.ID == id {
			name = in.Name
		}
	}
	gone := map[string]bool{id: true}
	parent, child, isPart := strings.Cut(id, "/")
	i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == parent })
	switch {
	case i < 0:
		return "", fmt.Errorf("the station has no module %s", id)
	case isPart:
		def := s.bundle.Modules[d.Modules[i].Module]
		j := slices.IndexFunc(def.Modules, func(c model.InstanceDef) bool { return c.ID == child })
		if j < 0 || strings.Contains(child, "/") {
			return "", fmt.Errorf("%s is part of %s and cannot be taken out on its own", name, parent)
		}
		if !def.Modules[j].Optional {
			return "", fmt.Errorf("%s needs its %s. Move it instead", def.Name, name)
		}
		m := &d.Modules[i]
		if m.Parts == nil {
			m.Parts = map[string]*draftPart{}
		}
		if m.Parts[child] == nil {
			m.Parts[child] = &draftPart{}
		}
		m.Parts[child].Removed = true
	case s.standing(d.Modules[i]) && s.carriesPLC(d, parent):
		return "", fmt.Errorf("%s carries the PLC, and stays", name)
	case d.Modules[i].Place == nil && !s.standing(d.Modules[i]):
		return "", fmt.Errorf("%s is what the station stands on, and stays", name)
	}
	// What is screwed onto it goes with it.
	under := func(on string) bool {
		o, _, _ := strings.Cut(on, ".")
		return gone[o] || !isPart && gone[owner(on)]
	}
	for changed := true; changed; {
		changed = false
		for _, o := range d.Modules {
			if o.Place != nil && !gone[o.ID] && under(o.Place.On) {
				gone[o.ID], changed = true, true
			}
		}
	}
	d.Modules = slices.DeleteFunc(d.Modules, func(o draftModule) bool { return gone[o.ID] && o.ID != parent })
	if !isPart {
		d.Modules = slices.DeleteFunc(d.Modules, func(o draftModule) bool { return o.ID == parent })
	}
	touches := func(j draftJoin) bool {
		for _, end := range []string{j.From, j.To} {
			inst, _, _ := strings.Cut(end, ".")
			if gone[inst] || !isPart && gone[owner(end)] {
				return true
			}
		}
		return false
	}
	d.Cables = slices.DeleteFunc(d.Cables, touches)
	d.Wires = slices.DeleteFunc(d.Wires, touches)
	if gone[d.PLC.Module] {
		// Another PLC takes over the station's first place, with its names.
		d.PLC = draftPLC{}
		if len(d.PLCs) > 0 {
			d.PLC, d.PLCs = d.PLCs[0], d.PLCs[1:]
		}
	}
	d.PLCs = slices.DeleteFunc(d.PLCs, func(p draftPLC) bool { return gone[p.Module] })
	return name, nil
}

// cablePLC connects a PLC that was just put in: every console, interface and terminal that
// has a free socket of a kind the PLC has, each to a free connector of the PLC. A connector
// that prefers one of the PLC's gets it, and the others are taken last.
func (s *BuilderService) cablePLC(d *draft, plc *model.ModuleDef, id string, taken func(string) bool) {
	stand := s.standOf(d, id)
	for _, m := range d.Modules {
		def := s.bundle.Modules[m.Module]
		// Only what stands on its own stand: another stand has a PLC of its own.
		if def == nil || !cabledToPLC(def.Role) || s.standOf(d, m.ID) != stand {
			continue
		}
		for _, c := range def.Terminal.Connectors {
			if s.shown(m.ID, c) && !taken(m.ID+"."+c.ID) {
				if port := s.plcPort(d, plc, id, c, taken); port != "" {
					d.Cables = append(d.Cables, draftJoin{From: m.ID + "." + c.ID, To: port})
				}
			}
		}
	}
}

// cabledToPLC tells the roles whose sockets of the PLC's kind are cabled to it.
func cabledToPLC(role string) bool {
	return role == model.RoleConsole || role == model.RoleInterface || role == model.RoleTerminal
}

// plcPort is the free connector of the PLC with that id that a connector is cabled to: the
// one it prefers, else the first of its type that no connector of the station prefers, else
// any of its type. It is empty when none is free.
func (s *BuilderService) plcPort(d *draft, plc *model.ModuleDef, id string, c model.ConnectorDef, taken func(string) bool) string {
	preferred := map[string]bool{}
	for _, m := range d.Modules {
		if def := s.bundle.Modules[m.Module]; def != nil {
			for _, o := range def.Terminal.Connectors {
				preferred[o.Prefer] = o.Prefer != ""
			}
		}
	}
	var fits []string
	for _, p := range plc.Terminal.Connectors {
		if p.Type == c.Type && !taken(id+"."+p.ID) {
			fits = append(fits, p.ID)
		}
	}
	if slices.Contains(fits, c.Prefer) {
		return id + "." + c.Prefer
	}
	for _, last := range []bool{false, true} {
		for _, p := range fits {
			if preferred[p] == last {
				return id + "." + p
			}
		}
	}
	return ""
}

// shown reports whether a connector of a module in the station is there, as the console of a
// stand is only with its console switched on.
func (s *BuilderService) shown(instance string, c model.ConnectorDef) bool {
	if !c.When.Set() {
		return true
	}
	for _, in := range s.plan.Scene.Instances {
		if in.ID == instance {
			return slices.ContainsFunc(in.Connectors, func(sc model.SceneConnector) bool { return sc.ID == c.ID })
		}
	}
	return false
}

// thePLC is the PLC of the station and its module, or nil.
// standOf is the module a module stands on in the end, such as the stand under a sensor
// on a belt on its plate.
func (s *BuilderService) standOf(d *draft, id string) string {
	for range len(d.Modules) {
		i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == id })
		if i < 0 || d.Modules[i].Place == nil || d.Modules[i].Place.On == "" || s.role(d.Modules[i].Module) == model.RoleStation {
			return id
		}
		id = owner(d.Modules[i].Place.On)
	}
	return id
}

// plcOn is the PLC in a stand, or "".
func (s *BuilderService) plcOn(d *draft, stand string) string {
	for _, m := range d.Modules {
		if def := s.bundle.Modules[m.Module]; def != nil && def.Role == model.RolePLC && m.Place != nil && s.standOf(d, owner(m.Place.On)) == stand {
			return m.ID
		}
	}
	return ""
}

// plcFor is the PLC that runs a module: the one on the stand it stands on, else the
// station's first.
func (s *BuilderService) plcFor(d *draft, id string) (string, *model.ModuleDef) {
	if plc := s.plcOn(d, s.standOf(d, id)); plc != "" {
		i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == plc })
		return plc, s.bundle.Modules[d.Modules[i].Module]
	}
	return s.thePLC(d)
}

func (s *BuilderService) thePLC(d *draft) (string, *model.ModuleDef) {
	for _, m := range d.Modules {
		if def := s.bundle.Modules[m.Module]; def != nil && def.Role == model.RolePLC {
			return m.ID, def
		}
	}
	return "", nil
}

// placeRef returns where in the station a module's place is written: its own, or for a part
// of another module, that module's change of it, made from where the part is now.
func (s *BuilderService) placeRef(d *draft, id string) (*draftPlace, error) {
	parent, child, isPart := strings.Cut(id, "/")
	i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == parent })
	if i < 0 {
		return nil, fmt.Errorf("the station has no module %s", id)
	}
	m := &d.Modules[i]
	if !isPart {
		if m.Place == nil {
			return nil, fmt.Errorf("%s stands at a fixed position", id)
		}
		return m.Place, nil
	}
	now, ok := s.plan.Placed[id]
	if !ok || strings.Contains(child, "/") {
		return nil, fmt.Errorf("%s is not screwed onto a site of %s, so it stays where it is", id, parent)
	}
	if m.Parts == nil {
		m.Parts = map[string]*draftPart{}
	}
	if m.Parts[child] == nil {
		m.Parts[child] = &draftPart{}
	}
	part := m.Parts[child]
	if part.Place == nil {
		_, site, _ := strings.Cut(now.On, ".")
		part.Place = &draftPlace{On: "." + site, Slot: now.Slot, At: round(now.At), Turn: round(now.Turn)}
	}
	return part.Place, nil
}

// plain is the reason a module does not fit, without where in the files it was found.
func plain(err error) error {
	var pe *model.PlaceError
	if errors.As(err, &pe) {
		return errors.New(pe.Text)
	}
	return err
}

// SetParam gives a module's setting a value for this station: a number, true or false, or
// an expression. An empty value goes back to the module's default.
func (s *BuilderService) SetParam(id, param, value string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parent, child, isPart := strings.Cut(id, "/")
	d, m, err := s.edit(parent)
	if err != nil {
		return BuilderState{}, err
	}
	// A part's settings are the module's change of that part.
	given := &m.Params
	if isPart {
		if m.Parts == nil {
			m.Parts = map[string]*draftPart{}
		}
		if m.Parts[child] == nil {
			m.Parts[child] = &draftPart{}
		}
		given = &m.Parts[child].Params
	}
	value = strings.TrimSpace(value)
	if value == "" {
		delete(*given, param)
	} else {
		if *given == nil {
			*given = map[string]json.RawMessage{}
		}
		raw := json.RawMessage(value)
		if !json.Valid(raw) || strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
			raw, _ = json.Marshal(value)
		}
		(*given)[param] = raw
	}
	if isPart {
		if part := m.Parts[child]; part.Place == nil && len(part.Params) == 0 && !part.Removed {
			delete(m.Parts, child)
		}
	}
	st, err := s.commit(d)
	return st, plain(err)
}

// SetStation names the station. Its id, and so its folder, follows the name.
func (s *BuilderService) SetStation(name, description string) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	id := slug(name)
	if id == "" {
		return BuilderState{}, errors.New("the name needs a letter or a digit")
	}
	d := s.draft.clone()
	d.ID, d.Name, d.Description = id, strings.TrimSpace(name), strings.TrimSpace(description)
	return s.commit(d)
}

// Save writes the station into the user folder, as stations/<id>/station.json, and
// returns where. Without overwrite, it refuses to replace a station that is there.
func (s *BuilderService) Save(overwrite bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return "", errors.New("no station is being built")
	}
	dir := filepath.Join(UserFolder(), "stations", s.draft.ID)
	file := filepath.Join(dir, "station.json")
	if _, err := os.Stat(file); err == nil && !overwrite {
		return file, fmt.Errorf("%s: %w", file, fs.ErrExist)
	}
	data, err := s.draft.bytes()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return "", err
	}
	s.saved = file
	return file, nil
}

// Close stops the builder's engine.
func (s *BuilderService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.halt()
}

// RunAction triggers a function of the station being built, such as filling a magazine.
func (s *BuilderService) RunAction(name string, args map[string]string) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.RunAction(name, args)
}

// SetControl presses or releases a button, or turns a key switch.
func (s *BuilderService) SetControl(name string, on bool) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.SetControl(name, on)
}

// SetFault enables or clears a fault.
func (s *BuilderService) SetFault(name string, on bool) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.SetFault(name, on)
}

// SetPin holds a signal on or off, to try what was built.
func (s *BuilderService) SetPin(state int, on bool) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.SetPin(state, on)
}

// SetParameter changes a setting of the station while it runs.
func (s *BuilderService) SetParameter(id string, value float64) error {
	e, err := s.running()
	if err != nil {
		return err
	}
	return e.SetParameter(id, value)
}

// Reset puts the station being built back to how it started.
func (s *BuilderService) Reset() error {
	e, err := s.running()
	if err != nil {
		return err
	}
	e.Reset()
	return nil
}

func (s *BuilderService) running() (*engine.Engine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		return nil, errors.New("no station is being built")
	}
	return s.engine, nil
}

func (s *BuilderService) halt() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	s.engine = nil
}

// edit returns a copy of the station to change, and the module with that id in it.
func (s *BuilderService) edit(id string) (*draft, *draftModule, error) {
	if s.draft == nil {
		return nil, nil, errors.New("no station is being built")
	}
	d := s.draft.clone()
	for i := range d.Modules {
		if d.Modules[i].ID == id {
			return d, &d.Modules[i], nil
		}
	}
	return nil, nil, fmt.Errorf("the station has no module %s", id)
}

// commit makes a changed station the one being built, if it loads, and runs it.
func (s *BuilderService) commit(d *draft) (BuilderState, error) {
	bundle, _, err := s.resolve(d)
	if err != nil {
		return BuilderState{}, err
	}
	e, err := engine.New(bundle, softplc.Idle{})
	if err != nil {
		return BuilderState{}, err
	}
	s.halt()
	ctx, stop := context.WithCancel(s.ctx)
	s.draft, s.bundle, s.plan, s.engine, s.stop = d, bundle, e.Plan(), e, stop
	go e.Run(ctx, frame, s.emit)
	return s.state(), nil
}

// resolve loads a station as the builder writes it: with docks for what can be filled or
// placed, and only the signal names its PLC bits still have.
func (s *BuilderService) resolve(d *draft) (*model.Bundle, *model.Plan, error) {
	bare := d.clone()
	bare.Actions, bare.Docks, bare.PLC.Signals = nil, nil, nil
	for i := range bare.PLCs {
		bare.PLCs[i].Signals = nil
	}
	bundle, plan, err := s.load(bare)
	if err != nil {
		return nil, nil, err
	}
	d.Actions, d.Docks = docks(d, bundle, plan)
	// Each PLC keeps the names of the bits that still reach something.
	connected := map[string]bool{}
	key := func(k int, address string) string { return fmt.Sprint(k, address) }
	for _, b := range plan.PLCIn {
		connected[key(b.PLC, plan.Naming[b.PLC].Input(b.Addr))] = true
	}
	for _, b := range plan.PLCOut {
		connected[key(b.PLC, plan.Naming[b.PLC].Output(b.Addr))] = true
	}
	for _, a := range plan.PLCAnalog {
		connected[key(a.PLC, plan.Naming[a.PLC].AnalogInput(a.Channel))] = true
	}
	for k, id := range plan.PLCs {
		if def := d.plcDef(id); def != nil && (k > 0 || def == &d.PLC) {
			def.Signals = slices.DeleteFunc(def.Signals, func(sig draftSignal) bool { return !connected[key(k, sig.Address)] })
		}
	}
	return s.load(d)
}

// placed returns the station with a module put on a place, a new one when id is empty. With
// clearRail, what stands where a rail has to be cut for a slide is removed, and named in
// what it returns with the module's id.
func (s *BuilderService) placed(module, id string, place BuilderPlace) (*draft, string, []string, error) {
	d := s.draft.clone()
	site := s.site(place.On, place.Slot)
	if id == "" {
		def, err := s.module(module)
		if err != nil {
			return nil, "", nil, err
		}
		id = s.freeID(d, module)
		if def.Role == model.RolePLC {
			// Each stand has a PLC of its own, which runs the modules on it.
			stand := s.standOf(d, owner(place.On))
			if other := s.plcOn(d, stand); other != "" {
				return nil, "", nil, fmt.Errorf("%s has its PLC already", s.nameIn(d, stand))
			}
			if first, _ := s.thePLC(d); first == "" {
				if !slices.ContainsFunc(d.Modules, func(m draftModule) bool { return m.ID == "plc" }) {
					id = "plc"
				}
				d.PLC.Module = id
			} else {
				d.PLCs = append(d.PLCs, draftPLC{Module: id})
			}
		}
		m := draftModule{ID: id, Module: module, Name: def.Name, Place: &draftPlace{}}
		if site != nil {
			switchOnMount(def, *site, &m)
		}
		d.Modules = append(d.Modules, m)
	}
	ref, err := s.placeRef(d, id)
	if err != nil {
		return nil, "", nil, err
	}
	on := place.On
	if parent, _, isPart := strings.Cut(id, "/"); isPart {
		o, site, _ := strings.Cut(on, ".")
		if o != parent {
			return nil, "", nil, fmt.Errorf("%s belongs to %s and stays on it", id, parent)
		}
		on = "." + site
	}
	*ref = draftPlace{On: on, Slot: place.Slot, At: round(place.At), Turn: round(place.Turn), Mount: ref.Mount}
	_, _, err = s.resolve(d)
	var removed []string
	for tries := 0; err != nil && place.ClearRail && tries < 8; tries++ {
		var pe *model.PlaceError
		if !errors.As(err, &pe) || pe.By == "" || pe.Instance == id {
			break
		}
		name, dropErr := s.drop(d, pe.Instance)
		if dropErr != nil {
			break
		}
		removed = append(removed, name)
		_, _, err = s.resolve(d)
	}
	return d, id, removed, err
}

// BuilderCheck says whether a module fits on a place, without putting it there: why not,
// and what would make it fit.
type BuilderCheck struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
	// Fix names what clearRail would remove, for a slide that needs a rail cut where it stands.
	Fix string `json:"fix"`
}

// Check tries a module on a place: a new one of that module when id is empty, or the module
// with that id moved there.
func (s *BuilderService) Check(module, id string, place BuilderPlace) BuilderCheck {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderCheck{Reason: "no station is being built"}
	}
	clear := place
	clear.ClearRail = false
	_, _, _, err := s.placed(module, id, clear)
	if err == nil {
		return BuilderCheck{OK: true}
	}
	check := BuilderCheck{Reason: plain(err).Error()}
	var pe *model.PlaceError
	if errors.As(err, &pe) && pe.By != "" {
		clear.ClearRail = true
		if _, _, removed, err := s.placed(module, id, clear); err == nil && len(removed) > 0 {
			check.Fix = "Remove " + strings.Join(removed, " and ")
		}
	}
	return check
}

// BuilderGhost is what a module looks like before it is placed, at the origin, and how it can
// be screwed on, so that it can follow the pointer.
type BuilderGhost struct {
	Instances []model.SceneInstance `json:"instances"`
	Mounts    []model.MountPlan     `json:"mounts"`
}

// Ghost returns a module as it looks before it is placed.
func (s *BuilderService) Ghost(module string) (BuilderGhost, error) {
	s.mu.Lock()
	lib := s.lib
	s.mu.Unlock()
	if lib == nil {
		lib = Library()
	}
	for _, level := range []string{model.LevelModule, model.LevelAccessory} {
		b, err := lib.Preview(level, module)
		if err != nil || b.Modules[module] == nil {
			continue
		}
		// Its mounts that settings switch on, such as a sensor's holder, are on.
		for i := range b.Station.Modules {
			if in := &b.Station.Modules[i]; in.Module == module {
				for _, mount := range b.Modules[module].Mounts {
					if identifier.MatchString(mount.When.Source()) {
						if in.Params == nil {
							in.Params = map[string]model.Num{}
						}
						in.Params[mount.When.Source()] = model.NumOf(1)
					}
				}
			}
		}
		plan, err := b.Resolve(nil)
		if err != nil {
			return BuilderGhost{}, err
		}
		return BuilderGhost{Instances: plan.Scene.Instances, Mounts: plan.Mounts[module]}, nil
	}
	return BuilderGhost{}, fmt.Errorf("the library has no module %s", module)
}

func (s *BuilderService) load(d *draft) (*model.Bundle, *model.Plan, error) {
	data, err := d.bytes()
	if err != nil {
		return nil, nil, err
	}
	lib := s.lib.With(fstest.MapFS{"stations/" + d.ID + "/station.json": {Data: data}})
	bundle, err := lib.Load(d.ID)
	if err != nil {
		return nil, nil, err
	}
	plan, err := bundle.Resolve(nil)
	if err != nil {
		return nil, nil, err
	}
	return bundle, plan, nil
}

// site finds a site of the station by owner and id, as a place names it.
func (s *BuilderService) site(on string, slot int) *model.SitePlan {
	if on == floorSite {
		if slot < 0 || slot >= floorSlots {
			return nil
		}
		return &floorSites()[slot]
	}
	o, id, _ := strings.Cut(on, ".")
	for i := range s.plan.Sites {
		if t := &s.plan.Sites[i]; t.Owner == o && t.ID == id && t.Slot == slot {
			return t
		}
	}
	return nil
}

func (s *BuilderService) state() BuilderState {
	st := BuilderState{
		Scene:   s.engine.Scene(),
		Station: BuilderStation{ID: s.draft.ID, Name: s.draft.Name, Description: s.draft.Description},
		Modules: []BuilderModule{},
		Sites:   append(append([]model.SitePlan{}, s.plan.Sites...), floorSites()...),
		Saved:   s.saved,
		Paths:   []BuilderPath{},
		Marks:   sim.Marks(s.plan),
	}
	if plc, _ := s.thePLC(s.draft); plc == "" {
		st.Scene.Checks = append([]model.SceneCheck{{Text: "The station has no PLC: put one onto its stand", Instances: []string{}}}, st.Scene.Checks...)
	}
	for i := range s.plan.Segments {
		seg := &s.plan.Segments[i]
		if seg.Length == 0 {
			continue
		}
		path := BuilderPath{Owner: owner(seg.Name)}
		steps := 1
		if seg.Arc {
			steps = 12
		}
		for k := range steps + 1 {
			path.Points = append(path.Points, model.PointOn(seg, seg.Length*float64(k)/float64(steps)))
		}
		st.Paths = append(st.Paths, path)
	}
	for _, m := range s.draft.Modules {
		b := BuilderModule{ID: m.ID, Module: m.Module, Name: m.Name, Mounts: []string{}, Params: []BuilderParam{}, Removed: []BuilderPart{}, Optional: true}
		def := s.bundle.Modules[m.Module]
		if b.Name == "" && def != nil {
			b.Name = def.Name
		}
		switch {
		case m.Place != nil:
			b.Place = &BuilderPlace{On: m.Place.On, Slot: m.Place.Slot, At: m.Place.At, Turn: m.Place.Turn}
			b.Line = s.lineOf(m.Place.On, m.Place.Slot)
		case s.standing(m):
			// A stand is on the floor, and one without the PLC can go.
			b.Place = floorPlace(m)
			b.Line = s.lineOf(b.Place.On, b.Place.Slot)
			b.Optional = !s.carriesPLC(s.draft, m.ID)
			w, depth := s.size(m.Module, m.Params)
			b.Floor = &BuilderFloor{Width: w, Depth: depth}
		default:
			b.Optional = false
		}
		if def != nil {
			b.Mounts = mountTypes(def)
			if s.standing(m) {
				b.Mounts = []string{"floor"}
			}
			b.Params = params(def, m.Params, nil)
			st.Modules = append(st.Modules, b)
			st.Modules = append(st.Modules, s.parts(m, def, &st.Modules[len(st.Modules)-1])...)
			continue
		}
		st.Modules = append(st.Modules, b)
	}
	return st
}

// lineOf is a copy of the site a module is on.
func (s *BuilderService) lineOf(on string, slot int) *model.SitePlan {
	if line := s.site(on, slot); line != nil {
		copied := *line
		return &copied
	}
	return nil
}

func mountTypes(def *model.ModuleDef) []string {
	out := []string{}
	for _, mount := range def.Mounts {
		if !slices.Contains(out, mount.Type) {
			out = append(out, mount.Type)
		}
	}
	return out
}

// params lists the settings of a module, with the values a station gives them. A part's
// defaults are what the module it belongs to gives it.
func params(def *model.ModuleDef, given map[string]json.RawMessage, defaults map[string]model.Num) []BuilderParam {
	out := []BuilderParam{}
	for _, p := range def.Parameters {
		fallback := p.Default
		if v, ok := defaults[p.ID]; ok {
			fallback = v
		}
		value := ""
		if raw, ok := given[p.ID]; ok {
			var text string
			if json.Unmarshal(raw, &text) == nil {
				value = text
			} else {
				value = string(raw)
			}
		}
		shown := fallback.Source()
		if fallback.Flag() {
			shown = map[string]string{"0": "false", "1": "true"}[shown]
		}
		kind := p.Kind
		if p.Label != "" && kind == "" {
			kind = model.KindPhysics
		}
		out = append(out, BuilderParam{
			ID: p.ID, Label: p.Label, Unit: p.Unit, Default: shown, Value: value, Kind: kind, Flag: p.Default.Flag(),
			Description: p.Description, Min: p.Min, Max: p.Max, Step: p.Step, Options: p.Options, Names: p.Names,
		})
	}
	return out
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parts lists the modules that a module comes with and that can be picked on their own, such
// as a conveyor's sensors, I/O block and motor controller, and notes on the module those that
// the station leaves out, so that they can be put back.
func (s *BuilderService) parts(m draftModule, def *model.ModuleDef, owner *BuilderModule) []BuilderModule {
	instances := map[string]model.SceneInstance{}
	for _, in := range s.plan.Scene.Instances {
		instances[in.ID] = in
	}
	var out []BuilderModule
	for _, c := range def.Modules {
		id := m.ID + "/" + c.ID
		if c.SelectAs != "" {
			continue
		}
		in, shown := instances[id]
		if !shown {
			if change := m.Parts[c.ID]; change != nil && change.Removed {
				name := c.Name
				if name == "" {
					if sub := s.bundle.Modules[c.Module]; sub != nil {
						name = sub.Name
					}
				}
				owner.Removed = append(owner.Removed, BuilderPart{ID: id, Name: name})
			}
			continue
		}
		// Only what can be moved or taken off is a module of its own; the rest is just part of it.
		if _, placed := s.plan.Placed[id]; !placed && !c.Optional {
			continue
		}
		part := BuilderModule{ID: id, Module: c.Module, Name: in.Name, Parent: m.ID, Optional: c.Optional,
			Mounts: []string{}, Params: []BuilderParam{}, Removed: []BuilderPart{}}
		if sub := s.bundle.Modules[c.Module]; sub != nil {
			part.Mounts = mountTypes(sub)
			var given map[string]json.RawMessage
			if change := m.Parts[c.ID]; change != nil {
				given = change.Params
			}
			part.Params = params(sub, given, c.Params)
		}
		if now, ok := s.plan.Placed[id]; ok {
			part.Place = &BuilderPlace{On: now.On, Slot: now.Slot, At: round(now.At), Turn: round(now.Turn)}
			part.Line = s.lineOf(now.On, now.Slot)
		}
		out = append(out, part)
	}
	return out
}

// maxTries limits how many places Add tries before it gives up.
const maxTries = 400

// added returns the station with a module added where it fits first, wired up. With an id,
// the module gets that id.
func (s *BuilderService) added(base *draft, module, id string) (*draft, error) {
	def, err := s.module(module)
	if err != nil {
		return nil, err
	}
	types := map[string]bool{}
	for _, m := range def.Mounts {
		types[m.Type] = true
	}
	if len(types) == 0 {
		return nil, fmt.Errorf("%s has no mounts, so it cannot be screwed on anywhere", def.Name)
	}
	if id == "" {
		id = s.freeID(base, module)
	}
	// Only a place where it stands in nothing is taken. That a gate still needs a slide
	// is for the station checks to say.
	before := len(sim.Overlaps(s.plan))
	tries := 0
	// The middle slots of the plate first, where a module is likely to work with the rest.
	sites := slices.Clone(s.plan.Sites)
	repeats := map[string]int{}
	for _, site := range sites {
		repeats[site.Owner+"."+site.ID]++
	}
	slices.SortStableFunc(sites, func(a, b model.SitePlan) int {
		middle := func(t model.SitePlan) float64 {
			return math.Abs(float64(t.Slot) - float64(repeats[t.Owner+"."+t.ID]/2))
		}
		return cmp.Compare(middle(a), middle(b))
	})
	for _, site := range sites {
		if !types[site.Type] {
			continue
		}
		for at := site.Min + 10; at <= site.Max-10 && tries < maxTries; at += 10 {
			tries++
			d := base.clone()
			m := draftModule{ID: id, Module: module, Name: def.Name, Place: &draftPlace{On: site.Owner + "." + site.ID, Slot: site.Slot, At: round(at)}}
			switchOnMount(def, site, &m)
			d.Modules = append(d.Modules, m)
			if _, plan, err := s.resolve(d); err != nil || len(sim.Overlaps(plan)) > before {
				continue
			}
			if err := s.wire(d, def, id); err != nil {
				return nil, err
			}
			return d, nil
		}
	}
	return nil, fmt.Errorf("there is no free place for %s", def.Name)
}

// switchOnMount switches on a mount that a setting switches on, such as a sensor's holder,
// for a module put onto a site of that type.
func switchOnMount(def *model.ModuleDef, site model.SitePlan, m *draftModule) {
	for _, mount := range def.Mounts {
		if mount.Type == site.Type && mount.Fits(site.Dir) && identifier.MatchString(mount.When.Source()) {
			if m.Params == nil {
				m.Params = map[string]json.RawMessage{}
			}
			m.Params[mount.When.Source()] = json.RawMessage("true")
		}
	}
}

// wire connects a module that was just added. An interface module gets a cable to a free connector
// of the PLC and to the D-Sub of up to two modules that have none yet, nearest first, and an
// accessory's wires go to free pins of the I/O block of the module it is screwed onto. Other
// modules are not cabled: which interface module they join is up to whoever builds the station.
func (s *BuilderService) wire(d *draft, def *model.ModuleDef, id string) error {
	taken := func(end string) bool {
		return slices.ContainsFunc(d.Cables, func(c draftJoin) bool { return c.From == end || c.To == end }) ||
			slices.ContainsFunc(d.Wires, func(w draftJoin) bool { return w.From == end || w.To == end })
	}
	if def.Role == model.RolePLC {
		s.cablePLC(d, def, id, taken)
		return nil
	}
	plc, plcDef := s.plcFor(d, id)
	for _, c := range def.Terminal.Connectors {
		fitsPLC := plcDef != nil && slices.ContainsFunc(plcDef.Terminal.Connectors, func(p model.ConnectorDef) bool { return p.Type == c.Type })
		switch {
		case cabledToPLC(def.Role) && fitsPLC:
			if port := s.plcPort(d, plcDef, plc, c, taken); port != "" {
				d.Cables = append(d.Cables, draftJoin{From: id + "." + c.ID, To: port})
			}
		case def.Role == model.RoleInterface && c.Type != "wires":
			if end := s.nearestFree(d, id, c.Type, taken); end != "" && !taken(id+"."+c.ID) {
				d.Cables = append(d.Cables, draftJoin{From: end, To: id + "." + c.ID})
			}
		case c.Type == "wires":
			m := d.Modules[len(d.Modules)-1]
			if m.Place == nil {
				continue
			}
			// The terminals of the module it is screwed onto first, then any other in the station.
			var terminals []string
			for _, own := range []bool{true, false} {
				for _, in := range s.plan.Scene.Instances {
					term := s.bundle.Modules[in.Module]
					if term == nil || term.Role != model.RoleTerminal || strings.HasPrefix(in.ID, owner(m.Place.On)+"/") != own {
						continue
					}
					for _, tc := range term.Terminal.Connectors {
						if !tc.Internal && !slices.Contains(terminals, in.ID+"."+tc.ID) {
							terminals = append(terminals, in.ID+"."+tc.ID)
						}
					}
				}
			}
			// A pin that a module wires to a part it comes with stays that part's, also while
			// the part is left out, so that it can be put back. It is taken only when no other is free.
			reserved := s.reservedPins(d, owner(m.Place.On))
			pin := func(prefix string) string {
				for _, spare := range []bool{false, true} {
					for _, terminal := range terminals {
						for n := range 8 {
							end := fmt.Sprintf("%s.%s%d", terminal, prefix, n)
							if _, ok := s.pinExists(end); ok && !s.plan.InUse[end] && !taken(end) && reserved[end] == spare {
								return end
							}
						}
					}
				}
				return ""
			}
			// Pins the module wires inside itself, as a magazine to its own I/O block, stay as
			// they are. A wire to a part that is off by default, such as an amplifier, does not count.
			inside := map[string]bool{}
			for _, w := range def.Wires {
				if when := w.When.Source(); identifier.MatchString(when) && offByDefault(def, when) {
					continue
				}
				inside[w.From], inside[w.To] = true, true
			}
			reads := false
			for _, p := range c.Pins {
				if inside[c.ID+"."+p.Pin] {
					continue
				}
				var to string
				switch {
				case p.Drives != "":
					to = pin("Q")
				case p.Reads != "" && !reads:
					to, reads = pin("I"), true
				}
				if to != "" {
					d.Wires = append(d.Wires, draftJoin{From: id + "." + c.ID + "." + p.Pin, To: to})
				}
			}
		}
	}
	return nil
}

// reservedPins are the pins of a module's own wires, by full name, as "conveyor/io.T.I1".
func (s *BuilderService) reservedPins(d *draft, id string) map[string]bool {
	out := map[string]bool{}
	i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == id })
	if i < 0 {
		return out
	}
	def := s.bundle.Modules[d.Modules[i].Module]
	if def == nil {
		return out
	}
	for _, w := range def.Wires {
		for _, end := range []string{w.From, w.To} {
			if part, rest, ok := strings.Cut(end, "."); ok && slices.ContainsFunc(def.Modules, func(c model.InstanceDef) bool { return c.ID == part }) {
				out[id+"/"+part+"."+rest] = true
			}
		}
	}
	return out
}

// offByDefault reports whether a setting of a module is false unless a station sets it.
func offByDefault(def *model.ModuleDef, param string) bool {
	for _, p := range def.Parameters {
		if p.ID == param {
			v := p.Default.Source()
			return v == "0" || v == "false"
		}
	}
	return false
}

// nearestFree is the connector of a type, such as a D-Sub socket, of a module that is no
// interface and has no cable there yet, the one nearest to the module with that id.
func (s *BuilderService) nearestFree(d *draft, id, kind string, taken func(string) bool) string {
	bare := d.clone()
	bare.Actions, bare.Docks, bare.PLC.Signals = nil, nil, nil
	_, plan, err := s.load(bare)
	if err != nil {
		return ""
	}
	at := map[string]model.Point{}
	for _, in := range plan.Scene.Instances {
		at[in.ID] = in.At
	}
	here, ok := at[id]
	if !ok {
		return ""
	}
	best, bestDist := "", math.Inf(1)
	for _, m := range d.Modules {
		def := s.bundle.Modules[m.Module]
		if m.ID == id || def == nil || def.Role == model.RoleInterface {
			continue
		}
		for _, c := range def.Terminal.Connectors {
			end := m.ID + "." + c.ID
			if c.Type != kind || c.Internal || taken(end) {
				continue
			}
			there := at[m.ID]
			if dist := math.Hypot(there.X-here.X, there.Y-here.Y); dist < bestDist {
				best, bestDist = end, dist
			}
		}
	}
	return best
}

// pinExists reports whether a terminal pin is in the station as it is now.
func (s *BuilderService) pinExists(end string) (string, bool) {
	inst, rest, _ := strings.Cut(end, ".")
	for _, in := range s.plan.Scene.Instances {
		if in.ID != inst {
			continue
		}
		conn, pin, _ := strings.Cut(rest, ".")
		for _, c := range in.Connectors {
			if c.ID != conn {
				continue
			}
			for _, p := range c.Pins {
				if p.Pin == pin {
					return in.ID, true
				}
			}
		}
	}
	return "", false
}

// module reads a module or accessory of the library.
func (s *BuilderService) module(id string) (*model.ModuleDef, error) {
	for _, level := range []string{model.LevelModule, model.LevelAccessory} {
		b, err := s.lib.Preview(level, id)
		if err == nil && b.Modules[id] != nil {
			return b.Modules[id], nil
		}
	}
	return nil, fmt.Errorf("the library has no module %s", id)
}

// freeID returns an id for another instance of a module, such as stack-magazine2.
func (s *BuilderService) freeID(d *draft, module string) string {
	for n := 1; ; n++ {
		id := module
		if n > 1 {
			id = fmt.Sprintf("%s%d", module, n)
		}
		if !slices.ContainsFunc(d.Modules, func(m draftModule) bool { return m.ID == id }) {
			return id
		}
	}
}

// docks lists what can be filled, placed or taken away in the station, as the buttons at the
// bottom: a group for each module whose actions are meant for that, such as a magazine's fill,
// with a button for each kind of workpiece a placeWorkpiece takes. The station itself needs
// no actions of its own for them, as they are the modules'.
func docks(d *draft, bundle *model.Bundle, plan *model.Plan) ([]json.RawMessage, []json.RawMessage) {
	type widget = map[string]any
	var actions, groups []widget
	kinds := func(family string) []widget {
		var out []widget
		for _, k := range bundle.Workpieces.Kinds {
			if family == "" || k.Family == family {
				out = append(out, widget{"type": "button", "label": k.Label, "args": map[string]string{"kind": k.ID}, "swatch": k.Material.Color})
			}
		}
		return out
	}
	names := map[string]string{}
	for _, in := range plan.Scene.Instances {
		names[in.ID] = in.Name
	}
	// The actions of a module and of what it contains, in the order the files declare them.
	var walk func(def *model.ModuleDef, prefix string, seen map[string]bool) []widget
	walk = func(def *model.ModuleDef, prefix string, seen map[string]bool) []widget {
		var buttons []widget
		for _, a := range def.Actions {
			name := prefix + "." + a.ID
			plan, ok := plan.Actions[name]
			if !ok || !plan.Dock || seen[name] {
				continue
			}
			seen[name] = true
			if plan.Type == "placeWorkpiece" {
				for _, b := range kinds(plan.Family) {
					b["action"] = name
					buttons = append(buttons, b)
				}
				continue
			}
			buttons = append(buttons, widget{"type": "button", "label": cmp.Or(plan.Label, a.ID), "action": name})
		}
		for _, c := range def.Modules {
			if inner := bundle.Modules[c.Module]; inner != nil {
				buttons = append(buttons, walk(inner, prefix+"/"+c.ID, seen)...)
			}
		}
		return buttons
	}
	for _, m := range d.Modules {
		def := bundle.Modules[m.Module]
		if def == nil {
			continue
		}
		if buttons := walk(def, m.ID, map[string]bool{}); len(buttons) > 0 {
			groups = append(groups, widget{"type": "group", "label": names[m.ID], "widgets": buttons})
		}
	}
	raw := func(list []widget) []json.RawMessage {
		var out []json.RawMessage
		for _, w := range list {
			data, _ := json.Marshal(w)
			out = append(out, data)
		}
		return out
	}
	if len(groups) == 0 {
		return raw(actions), nil
	}
	dock, _ := json.Marshal(widget{"place": "bottom", "widgets": groups})
	return raw(actions), []json.RawMessage{dock}
}

// owner is the top-level module a site, connector or pin belongs to.
func owner(ref string) string {
	ref, _, _ = strings.Cut(ref, ".")
	ref, _, _ = strings.Cut(ref, "/")
	return ref
}

func round(v float64) float64 { return math.Round(v*10) / 10 }

var notID = regexp.MustCompile(`[^a-z0-9]+`)

// slug turns a name into a station id, such as two-magazines for "Two magazines".
func slug(name string) string {
	return strings.Trim(notID.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// draft is a station.json as the builder writes it. What the builder does not change is
// kept as it was written.
type draft struct {
	Format      int               `json:"format"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Workpieces  string            `json:"workpieces"`
	Parameters  []json.RawMessage `json:"parameters,omitempty"`
	Modules     []draftModule     `json:"modules"`
	Links       []json.RawMessage `json:"links,omitempty"`
	Cables      []draftJoin       `json:"cables,omitempty"`
	Wires       []draftJoin       `json:"wires,omitempty"`
	Actions     []json.RawMessage `json:"actions,omitempty"`
	Docks       []json.RawMessage `json:"docks,omitempty"`
	PLC         draftPLC          `json:"plc"`
	// PLCs are the PLCs besides the first one, on other stands of the station.
	PLCs []draftPLC `json:"plcs,omitempty"`
}

type draftPLC struct {
	Program string            `json:"program,omitempty"`
	Module  string            `json:"module,omitempty"`
	Signals []draftSignal     `json:"signals,omitempty"`
	Maps    []json.RawMessage `json:"maps,omitempty"`
}

// plcDef is where the station names the signals of a PLC, by its module, the first one for "".
func (d *draft) plcDef(plc string) *draftPLC {
	if plc == "" || plc == d.PLC.Module {
		return &d.PLC
	}
	for i := range d.PLCs {
		if d.PLCs[i].Module == plc {
			return &d.PLCs[i]
		}
	}
	return nil
}

type draftModule struct {
	ID        string                     `json:"id"`
	Module    string                     `json:"module"`
	Name      string                     `json:"name,omitempty"`
	When      json.RawMessage            `json:"when,omitempty"`
	At        json.RawMessage            `json:"at,omitempty"`
	RotateX   json.RawMessage            `json:"rotateX,omitempty"`
	RotateY   json.RawMessage            `json:"rotateY,omitempty"`
	RotateZ   json.RawMessage            `json:"rotateZ,omitempty"`
	Place     *draftPlace                `json:"place,omitempty"`
	Params    map[string]json.RawMessage `json:"params,omitempty"`
	Texts     map[string]string          `json:"texts,omitempty"`
	SelectAs  string                     `json:"selectAs,omitempty"`
	MountedOn string                     `json:"mountedOn,omitempty"`
	Parts     map[string]*draftPart      `json:"parts,omitempty"`
	Rewire    map[string]string          `json:"rewire,omitempty"`
}

// draftPart changes a part that a module comes with.
type draftPart struct {
	Place   *draftPlace                `json:"place,omitempty"`
	Params  map[string]json.RawMessage `json:"params,omitempty"`
	Removed bool                       `json:"removed,omitempty"`
}

type draftPlace struct {
	On    string  `json:"on"`
	Slot  int     `json:"slot,omitempty"`
	At    float64 `json:"at"`
	Turn  float64 `json:"turn,omitempty"`
	Mount string  `json:"mount,omitempty"`
}

type draftJoin struct {
	From string          `json:"from"`
	To   string          `json:"to"`
	When json.RawMessage `json:"when,omitempty"`
}

type draftSignal struct {
	Address     string `json:"address"`
	Name        string `json:"name"`
	Variable    string `json:"variable,omitempty"`
	Description string `json:"description,omitempty"`
}

func (d *draft) bytes() ([]byte, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	return append(data, '\n'), err
}

func (d *draft) clone() *draft {
	data, _ := json.Marshal(d)
	c := &draft{}
	_ = json.Unmarshal(data, c)
	return c
}

// EmptyFrom starts an empty station with what a collection has to stand on, when Start is
// given it followed by the collection's id.
const EmptyFrom = "empty:"

// prefer orders module ids with those of a collection first, keeping the order otherwise.
func (s *BuilderService) prefer(ids []string, collection string) []string {
	out := slices.Clone(ids)
	slices.SortStableFunc(out, func(a, b string) int {
		return b2i(s.lib.ModuleCollection(b) == collection) - b2i(s.lib.ModuleCollection(a) == collection)
	})
	return out
}

// moduleOf is the module an instance of the station being built is of.
func (s *BuilderService) moduleOf(instance string) string {
	if s.draft != nil {
		for _, m := range s.draft.Modules {
			if m.ID == instance {
				return m.Module
			}
		}
	}
	return instance
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// startEmpty begins a station with what the library has to stand on, preferring one of the
// given collection, with that collection's workpieces, and a PLC on it, cabled as the builder
// cables one that is put in.
func (s *BuilderService) startEmpty(collection string) (BuilderState, error) {
	stands := s.prefer(s.lib.WithRole(model.RoleStation), collection)
	if len(stands) == 0 {
		return BuilderState{}, errors.New("the library has nothing for a station to stand on")
	}
	workpieces := s.lib.DefaultWorkpieces()
	if c, ok := s.lib.Collection(s.lib.ModuleCollection(stands[0])); ok && c.Workpieces != "" {
		workpieces = c.Workpieces
	}
	d := &draft{Format: model.StationFormat, ID: "new-station", Name: "New station", Workpieces: workpieces}
	d.Modules = []draftModule{{ID: stands[0], Module: stands[0]}}
	s.saved = ""
	st, err := s.commit(d)
	if err != nil {
		return st, err
	}
	return s.equip(stands[0], st)
}

// screwSpan is how far a module's screws reach along a site of that type, from where it is
// placed: its extent on a rail.
func screwSpan(def *model.ModuleDef, siteType string) (lo, hi float64) {
	for _, m := range def.Mounts {
		if m.Type != siteType {
			continue
		}
		for i, sc := range m.Screws {
			x, _ := def.Value(sc[0], nil)
			if i == 0 || x < lo {
				lo = x
			}
			if i == 0 || x > hi {
				hi = x
			}
		}
		return lo, hi
	}
	return 0, 0
}

// taken says whether a module stands on a site, on any of its slots, within from to to along
// it, or less than a hand's breadth from there. A spot is taken by whatever stands on it.
func (s *BuilderService) taken(site model.SitePlan, from, to float64) bool {
	const gap = 100
	for _, m := range s.draft.Modules {
		if m.Place == nil || m.Place.On != site.Owner+"."+site.ID {
			continue
		}
		if site.Min == site.Max && m.Place.Slot == site.Slot {
			return true
		}
		def, err := s.module(m.Module)
		if err != nil {
			continue
		}
		lo, hi := screwSpan(def, site.Type)
		if m.Place.At+lo < to+gap && m.Place.At+hi > from-gap {
			return true
		}
	}
	return false
}

// equip puts a PLC and a console onto a stand that was just put down, each on the first
// site of the stand it fits, such as its rack, cabled as the builder cables one put in.
func (s *BuilderService) equip(stand string, st BuilderState) (BuilderState, error) {
	for _, role := range []string{model.RolePLC, model.RoleConsole} {
		// The PLC and console of the same collection as what they stand on.
		ids := s.prefer(s.lib.WithRole(role), s.lib.ModuleCollection(s.moduleOf(stand)))
		if len(ids) == 0 {
			continue
		}
		def, err := s.module(ids[0])
		if err != nil {
			continue
		}
	sites:
		for _, site := range s.plan.Sites {
			if site.Owner != stand || !slices.ContainsFunc(def.Mounts, func(m model.MountDef) bool { return m.Type == site.Type }) {
				continue
			}
			// A spot such as a rack, or along a rail: its start, its middle or its end, where
			// the module fits and nothing stands yet.
			lo, hi := screwSpan(def, site.Type)
			candidates := []float64{site.Min}
			if site.Max > site.Min {
				candidates = []float64{site.Min - lo, (site.Min + site.Max) / 2, site.Max - hi}
			}
			for _, at := range candidates {
				if s.taken(site, at+lo, at+hi) {
					continue
				}
				d, id, _, err := s.placed(ids[0], "", BuilderPlace{On: site.Owner + "." + site.ID, Slot: site.Slot, At: at})
				if err != nil || s.wire(d, def, id) != nil {
					continue
				}
				if st, err = s.commit(d); err != nil {
					return st, err
				}
				break sites
			}
		}
	}
	return st, nil
}
