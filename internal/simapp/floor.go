package simapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/toblu96/taktwin/internal/model"
)

// Modules of the station role, such as stands, stand on the floor, which the builder offers
// as lines along x, one every floorStep mm across, so that they move like modules on a plate:
// along x in mm, and across a line at a time. Stations line up side by side along x, as on a
// real production line, where the end of one belt meets the start of the next.
const (
	floorSite  = "floor.area"
	floorStep  = 50.0
	floorSlots = 41
	floorReach = 5000.0
)

// role is what a module of the station is to the builder, such as model.RoleStation.
func (s *BuilderService) role(module string) string {
	if def := s.bundle.Modules[module]; def != nil {
		return def.Role
	}
	return ""
}

// standing reports whether a module of the station stands on the floor.
func (s *BuilderService) standing(m draftModule) bool {
	return m.Place == nil && s.role(m.Module) == model.RoleStation
}

// floorSites are the lines stands move on.
func floorSites() []model.SitePlan {
	out := make([]model.SitePlan, 0, floorSlots)
	for k := range floorSlots {
		out = append(out, model.SitePlan{
			Owner: "floor", ID: "area", Type: "floor", Slot: k,
			From: model.Point{Y: float64(k-floorSlots/2) * floorStep}, Dir: model.Point{X: 1},
			Min: -floorReach, Max: floorReach, Gaps: []model.SiteGap{},
		})
	}
	return out
}

// footprint is where a stand is on the floor: from its corner, x along its width.
type footprint struct {
	id             string
	x, y, w, depth float64
}

func (f footprint) overlaps(o footprint) bool {
	const touch = 0.5
	return f.x < o.x+o.w-touch && o.x < f.x+f.w-touch && f.y < o.y+o.depth-touch && o.y < f.y+f.depth-touch
}

// stands lists what stands on the floor and where.
func (s *BuilderService) stands(d *draft) []footprint {
	var out []footprint
	for _, m := range d.Modules {
		if !s.standing(m) {
			continue
		}
		at := position(m.At)
		f := footprint{id: m.ID, x: at[0], y: at[1]}
		f.w, f.depth = s.size(m.Module, m.Params)
		out = append(out, f)
	}
	return out
}

// size is how wide and deep a module of the station role is on the floor, with the given settings.
func (s *BuilderService) size(module string, params map[string]json.RawMessage) (float64, float64) {
	def := s.bundle.Modules[module]
	if def == nil || def.Floor == nil {
		return 0, 0
	}
	w, _ := def.Value(def.Floor.Width, params)
	depth, _ := def.Value(def.Floor.Depth, params)
	return w, depth
}

// position reads the at of a module, where it is a plain point.
func position(raw json.RawMessage) [3]float64 {
	var at [3]float64
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &at)
	}
	return at
}

// floorPlace is where a stand is, as a place on the floor's lines.
func floorPlace(m draftModule) *BuilderPlace {
	at := position(m.At)
	return &BuilderPlace{On: floorSite, Slot: int(math.Round(at[1]/floorStep)) + floorSlots/2, At: round(at[0])}
}

// moveStand puts a stand elsewhere on the floor, with everything on it, unless it would
// stand in another one.
func (s *BuilderService) moveStand(d *draft, id string, place BuilderPlace) error {
	i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == id })
	if i < 0 || !s.standing(d.Modules[i]) {
		return fmt.Errorf("only a stand goes on the floor")
	}
	if place.Slot < 0 || place.Slot >= floorSlots || math.Abs(place.At) > floorReach {
		return errors.New("that is too far away from the rest")
	}
	at := position(d.Modules[i].At)
	at[0], at[1] = round(place.At), float64(place.Slot-floorSlots/2)*floorStep
	raw, _ := json.Marshal(at)
	d.Modules[i].At = raw
	all := s.stands(d)
	me := all[slices.IndexFunc(all, func(f footprint) bool { return f.id == id })]
	for _, o := range all {
		if o.id != id && me.overlaps(o) {
			return fmt.Errorf("%s would stand in %s", s.nameIn(d, id), s.nameIn(d, o.id))
		}
	}
	return nil
}

// nameIn is how people know a module of a station.
func (s *BuilderService) nameIn(d *draft, id string) string {
	for _, m := range d.Modules {
		if m.ID != id {
			continue
		}
		if m.Name != "" {
			return m.Name
		}
		if def := s.bundle.Modules[m.Module]; def != nil {
			return def.Name
		}
	}
	return id
}

// AddStand puts an empty stand right beside another one, on its left (side -1, towards
// -x) or its right (side 1), so that a station can grow into a line of stations that hand
// workpieces on to each other. It is one of the same kind, with the settings its floor gives
// one beside it, such as no console, and no PLC: the station's PLC runs it, through a C
// interface on its plate.
func (s *BuilderService) AddStand(beside string, side int) (BuilderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draft == nil {
		return BuilderState{}, errors.New("no station is being built")
	}
	d := s.draft.clone()
	all := s.stands(d)
	i := slices.IndexFunc(all, func(f footprint) bool { return f.id == beside })
	if i < 0 {
		return BuilderState{}, fmt.Errorf("the station has no stand %s", beside)
	}
	j := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == beside })
	module := d.Modules[j].Module
	params := map[string]json.RawMessage{}
	if def := s.bundle.Modules[module]; def != nil && def.Floor != nil {
		maps.Copy(params, def.Floor.Beside)
	}
	next := footprint{y: all[i].y}
	next.w, next.depth = s.size(module, params)
	next.x = all[i].x + all[i].w
	if side < 0 {
		next.x = all[i].x - next.w
	}
	for _, o := range all {
		if next.overlaps(o) {
			return BuilderState{}, fmt.Errorf("%s already stands there", s.nameIn(d, o.id))
		}
	}
	id := s.freeID(d, module)
	at, _ := json.Marshal([3]float64{round(next.x), next.y, 0})
	name := s.nameIn(d, beside)
	if def := s.bundle.Modules[module]; def != nil {
		name = def.Name
	}
	if len(params) == 0 {
		params = nil
	}
	d.Modules = append(d.Modules, draftModule{ID: id, Module: module, Name: fmt.Sprintf("%s %d", name, len(all)+1), At: at, Params: params})
	st, err := s.commit(d)
	if err != nil {
		return st, plain(err)
	}
	// A station beside another one runs on a PLC of its own, with a console of its own.
	st, err = s.equip(id, st)
	return st, plain(err)
}

// carriesPLC is a stand with the station's PLC in it, which stays.
func (s *BuilderService) carriesPLC(d *draft, id string) bool {
	plc, _ := s.thePLC(d)
	i := slices.IndexFunc(d.Modules, func(m draftModule) bool { return m.ID == plc })
	return i >= 0 && d.Modules[i].Place != nil && owner(d.Modules[i].Place.On) == id
}
