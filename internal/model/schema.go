// Package model loads stations and modules from JSON files and resolves them
// into a Plan, the fully numeric description used by the simulation and the 3D
// view. See catalog/README.md for the file format.
package model

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/toblu96/taktwin/internal/plcio"
	"github.com/toblu96/taktwin/st"
)

// FormatVersion is the format of module and part files this build reads. Stations have
// their own, StationFormat.
const FormatVersion = 2

// StationFormat is the format of station files this build reads and writes.
const StationFormat = 5

// Vec3 is a point or direction in a model file.
type Vec3 [3]Num

// ParameterDef declares a module parameter. One with a Label is a setting that people can
// change, of a Kind: build for how the module is put together, such as the number on a slide
// or the angle of a sensor, or physics for how it behaves, such as the speed of a belt.
// Description says in a sentence or two what it does. A setting that is a number keeps
// between Min and Max, in steps of Step, or takes one of Options. One without a Label is the
// module's own business, and not offered to people.
type ParameterDef struct {
	ID          string    `json:"id"`
	Default     Num       `json:"default"`
	Label       string    `json:"label,omitempty"`
	Unit        string    `json:"unit,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Description string    `json:"description,omitempty"`
	Min         *float64  `json:"min,omitempty"`
	Max         *float64  `json:"max,omitempty"`
	Step        float64   `json:"step,omitempty"`
	Options     []float64 `json:"options,omitempty"`
	// Names says what each of Options is, such as Round and Square for a body's shape.
	Names []string `json:"names,omitempty"`
}

// Allows reports whether a value is within the parameter's limits, and if not, why.
func (p *ParameterDef) Allows(v float64) error {
	switch {
	case len(p.Options) > 0 && !slices.Contains(p.Options, v):
		return fmt.Errorf("%s is one of %v, not %g", cmp.Or(p.Label, p.ID), p.Options, v)
	case p.Min != nil && v < *p.Min-1e-9, p.Max != nil && v > *p.Max+1e-9:
		return fmt.Errorf("%s is between %g and %g %s, not %g", cmp.Or(p.Label, p.ID), deref(p.Min), deref(p.Max), p.Unit, v)
	}
	return nil
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// Kinds of settings.
const (
	KindBuild   = "build"
	KindPhysics = "physics"
)

// NeedDef is how many modules of a kind a module needs on it, or among its parts, to work,
// such as the motor controller of a conveyor. A station without them still loads, and the
// station checks say what is missing, and Why it matters.
type NeedDef struct {
	Module string `json:"module"`
	Min    int    `json:"min,omitempty"`
	Why    string `json:"why,omitempty"`
}

// GeometryDef uses the names and parameters of three.js geometries where one
// exists. From and To on a CylinderGeometry, and SectorGeometry, are extensions.
// ExtrudeGeometry takes its Shape as Points and Holes, and extrudes along +z.
type GeometryDef struct {
	Type           string `json:"type"`
	Width          Num    `json:"width"`
	Height         Num    `json:"height"`
	Depth          Num    `json:"depth"`
	Radius         Num    `json:"radius"`
	RadialSegments Num    `json:"radialSegments"`
	From           Vec3   `json:"from"`
	To             Vec3   `json:"to"`
	Center         [2]Num `json:"center"`

	Points [][2]Num   `json:"points,omitempty"`
	Holes  [][][2]Num `json:"holes,omitempty"`

	Inner      Num `json:"inner"`
	Outer      Num `json:"outer"`
	Start      Num `json:"start"`
	End        Num `json:"end"`
	InnerStart Num `json:"innerStart"`
}

// LabelDef draws text on a node, on a plate if it has a Background.
type LabelDef struct {
	Text       string `json:"text"`
	Size       Num    `json:"size"`
	Color      string `json:"color,omitempty"`
	Background string `json:"background,omitempty"`
}

// RepeatDef places copies of a node along a step.
type RepeatDef struct {
	Count Num  `json:"count"`
	Step  Vec3 `json:"step"`
}

// NodeDef is one element of a module's 3D model, shaped like a three.js Object3D.
// Rotation is in degrees. Logic refers to nodes by Name and never to their shape,
// so a node can be refined without touching the module's behaviour.
type NodeDef struct {
	Name     string       `json:"name,omitempty"`
	When     Num          `json:"when"`
	Position Vec3         `json:"position"`
	Rotation Vec3         `json:"rotation"`
	Geometry *GeometryDef `json:"geometry,omitempty"`
	Material string       `json:"material,omitempty"`
	Label    *LabelDef    `json:"label,omitempty"`
	Belt     string       `json:"belt,omitempty"` // transport a belt surface follows
	Part     string       `json:"part,omitempty"` // shared part drawn at this node
	NoPick   bool         `json:"noPick,omitempty"`
	Repeat   *RepeatDef   `json:"repeat,omitempty"`
	Children []NodeDef    `json:"children,omitempty"`
}

// PartDef is a shared 3D part, such as a connector, that modules draw with the
// Part field of a node. Improving a part improves every module that uses it. A
// node in a part with the material "inherit" takes the material of that node.
type PartDef struct {
	Format int       `json:"format"`
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Model  []NodeDef `json:"model"`
}

// JointDef moves a node with a state. The state's range 0..1 maps to Range, in
// degrees for a revolute joint and mm for a prismatic one.
type JointDef struct {
	Node   string `json:"node"`
	Type   string `json:"type"` // revolute or prismatic
	Axis   Vec3   `json:"axis"`
	Origin Vec3   `json:"origin"`
	Range  [2]Num `json:"range"`
	Driver string `json:"driver"`
	When   Num    `json:"when"`
}

// DeviceDef is one working part of a module. Type selects which fields apply: cylinder,
// reedSwitch, axis, motorController, sensor, distanceSensor, gripper, pusher, solid, process,
// button, key, lamp, logic, safetyRelay, program or plc.
type DeviceDef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	When Num    `json:"when"`

	// cylinder
	Valve  string `json:"valve,omitempty"` // monostable or bistable
	Rest   string `json:"rest,omitempty"`  // retracted or extended
	Stroke Num    `json:"stroke"`
	Speed  Num    `json:"speed"`
	// Limited is a cylinder whose force a pressure regulator turns down: it stops on what it
	// presses on, and that is no collision.
	Limited Num `json:"limited"`
	// Stop names a state that, while on, holds the cylinder at StopAt mm on its way out, as a
	// stop cylinder that comes out into its path gives a lifting axis a middle position.
	Stop   string `json:"stop,omitempty"`
	StopAt Num    `json:"stopAt"`

	// axis: a motor that moves Stroke mm or degrees at Speed per second, forward while its
	// forward input is on and backward while backward is, and stops wherever both are off or
	// on. Forward and Backward instead make it follow states, such as a motor controller's
	// forward. Wrap makes it turn on endlessly, as a turntable, instead of stopping at its ends.
	Forward  string `json:"forward,omitempty"`
	Backward string `json:"backward,omitempty"`
	Wrap     bool   `json:"wrap,omitempty"`
	// Targets make it a positioning axis, as a motor controller with taught records: while
	// its input go comes on, it drives to the target that its inputs record0, record1 and
	// record2 select, counted in binary, in mm (or degrees) from its start. Its state done is
	// on once it is there, and off while it moves.
	Targets []Num `json:"targets,omitempty"`
	// Enable gives a positioning axis's controller an input enable, without which it does
	// not move, and the states ready (enabled), homed and error, as a stepper motor's
	// controller reports them.
	Enable bool `json:"enable,omitempty"`
	// Setpoint makes it a positioning axis that drives to the value of its input setpoint,
	// in mm (or degrees) from its start, as its input go comes on, as a drive with a numeric
	// setpoint. Its state done is on once it is there.
	Setpoint bool `json:"setpoint,omitempty"`

	// program: a controller of the module's own, as a machine's built-in control, written in
	// Structured Text: the file Source in the module's folder, running its PROGRAM named
	// Program (the first one without) every Cycle seconds (0.01 without) on the simulation's
	// clock. Its VAR_INPUT variables are inputs of the device that pins can drive, and its
	// VAR_OUTPUT variables states that pins can read, by their names, as controller.busy.
	// Reads gives variables the values of states of the module, such as "x.value", or of the
	// station, by a name starting with / that the station passes in its texts, such as
	// "/{belt}.surface.count", which keeps its initial value where the station names none; a
	// transport's count is how many workpieces are on it, and its
	// before is on while another transport takes them over at its start. Writes
	// sets states of the module to variables, such as the input "x.setpoint", each by the
	// variable's name; a name ending in [a..b] binds the elements of an array, with {} in the
	// state standing for the index. A BOOL is 0 or 1 and a TIME counts seconds.
	Source  string            `json:"source,omitempty"`
	Program string            `json:"program,omitempty"`
	Cycle   Num               `json:"cycle"`
	Reads   map[string]string `json:"reads,omitempty"`
	Writes  map[string]string `json:"writes,omitempty"`
	// compiled is the program, read with the module, and inputs and outputs the names of its
	// VAR_INPUT and VAR_OUTPUT variables.
	compiled        *st.Program
	pou             string
	inputs, outputs []string

	// reedSwitch
	Cylinder string `json:"cylinder,omitempty"`
	At       string `json:"at,omitempty"` // retracted or extended

	// sensor
	Origin    Vec3 `json:"origin"`
	Direction Vec3 `json:"direction"`
	Reach     Num  `json:"reach"`
	// Detect is what the sensor reacts to: metal, reflection (a workpiece reflects
	// at least Threshold percent of the light) or beam (less than Threshold percent
	// of the light gets through a workpiece in the beam).
	Detect    string `json:"detect,omitempty"`
	Threshold Num    `json:"threshold"`
	OffDelay  Num    `json:"offDelay"`
	// Mirror folds the beam at the end of Reach, so one sensor watches two places.
	Mirror *MirrorDef `json:"mirror,omitempty"`
	// distanceSensor: looks straight down from its origin, over Min to Reach mm, and
	// sees Surface mm below it when nothing is there. Threshold is the distance it
	// switches at, and its analogue value is 0 to 10 V across that range.
	Min     Num `json:"min"`
	Surface Num `json:"surface"`

	// gripper: Cylinder opens its jaws, and what they close on at the node Tip is held. Without
	// one it is a suction cup or a magnet, which holds while its input on is, or State instead:
	// what its tip touches from above, within Reach mm, and with Detect "metal" only metal.
	Tip   string `json:"tip,omitempty"`
	Fault string `json:"fault,omitempty"` // label of a fault that keeps it low

	// button, key and lamp: the node that is pressed or lit. solid: the node it moves with.
	Node string `json:"node,omitempty"`

	// solid: a box that workpieces cannot enter, and that cannot move into them. Its node
	// may only turn about the vertical. Moving into a workpiece is a collision, named by
	// Label. A workpiece running into it just stops.
	Box   *BoxDef `json:"box,omitempty"`
	Label string  `json:"label,omitempty"`
	// Shoves makes it push a workpiece along its transport instead of stopping, as the belt
	// just slips under it, so it can return between two workpieces in a queue.
	Shoves bool `json:"shoves,omitempty"`
	// Outline is its shape seen from above, for a part that a box fits badly. Workpieces then
	// meet that shape, between the heights of Box, which still counts for everything else.
	Outline [][2]Num `json:"outline,omitempty"`

	// process: works on the workpiece under Origin, within Reach mm below it, while its input
	// on is: after Time seconds, it sets the workpiece's Mark to Amount (1 without one), or
	// with Adds adds Amount to it, as each cycle of a filler adds to what is in it. It works on
	// each workpiece once until it leaves, and its states are busy and done.
	Mark   string `json:"mark,omitempty"`
	Amount Num    `json:"amount"`
	Adds   bool   `json:"adds,omitempty"`
	Time   Num    `json:"time"`
	// sensor: Has makes it see only workpieces with that mark, or with a joined part of that
	// family or kind, as a probe that checks for a hole or a cap.
	Has string `json:"has,omitempty"`

	// safetyRelay: the station's safety circuit. It opens while Reset holds, such as an
	// emergency stop being pressed, and only closes again while Set holds and Reset does not,
	// such as a reset button pressed after the emergency stop is released. Its state ok is on
	// while it is closed. While any safety relay is open, the PLC's outputs that pass through
	// a switched connector are off, and positioning axes stop and forget their target.
	//
	// logic: a state worked out from others, as a relay circuit or a module's own small
	// controller would. Expr is an expression over states of the module and its parts, written
	// with dots, and its parameters, such as "gate.extended && !stop.detected": true is 1, and a
	// state counts as true from 0.5. With Set and Reset instead, it turns on when Set holds and
	// off when Reset does, Reset winning. OnDelay and OffDelay hold off a change for that many
	// seconds. Its states are on and value, and Drives sets an input of a device of the module,
	// such as a valve's, to it, unless a pin of the PLC drives that.
	Expr    string `json:"expr,omitempty"`
	Set     string `json:"set,omitempty"`
	Reset   string `json:"reset,omitempty"`
	OnDelay Num    `json:"onDelay"`
	Drives  string `json:"drives,omitempty"`

	// pusher: Cylinder pushes the lowest workpiece of the From transport along the To
	// transport, and leaves it on what lies below at the end of its stroke. Without From, it
	// takes along whatever workpiece stands at the start of To as it starts moving, as an
	// ejector does from the nest of a turntable.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`

	// lamp: lit by State, by a pin that drives its "on" input, or always when it
	// shows that the station has power. Glow is the diameter in mm of the halo
	// around it while lit.
	State    string  `json:"state,omitempty"`
	AlwaysOn bool    `json:"alwaysOn,omitempty"`
	On       string  `json:"on,omitempty"`
	Off      string  `json:"off,omitempty"`
	Opacity  float64 `json:"opacity,omitempty"`
	Glow     Num     `json:"glow"`
}

// PathDef is the centre line of a transport.
type PathDef struct {
	Line *struct {
		From Vec3 `json:"from"`
		To   Vec3 `json:"to"`
	} `json:"line,omitempty"`
	Arc *struct {
		Center [2]Num `json:"center"`
		Radius Num    `json:"radius"`
		Z      Num    `json:"z"`
		Start  Num    `json:"start"`
		Sweep  Num    `json:"sweep"`
	} `json:"arc,omitempty"`
}

// TransportDef is a path that carries workpieces. Drive names the state that runs
// it; without one it is gravity driven, unless Shared says a station link gives
// it the drive and speed of another transport. Reverse names the state that runs
// it backwards, and Factor a state that scales its speed.
type TransportDef struct {
	ID string `json:"id"`
	// Label is how the library names it where it offers to put a workpiece on it.
	Label   string  `json:"label,omitempty"`
	When    Num     `json:"when"`
	Path    PathDef `json:"path"`
	Drive   string  `json:"drive,omitempty"`
	Reverse string  `json:"reverse,omitempty"`
	Factor  string  `json:"factor,omitempty"`
	Shared  bool    `json:"shared,omitempty"`
	// Static is a surface that moves nothing, such as a table to put a workpiece on.
	Static bool `json:"static,omitempty"`
	// Open ends let workpieces fall off the station, such as a belt running over its roller.
	Open     bool `json:"open,omitempty"`
	Speed    Num  `json:"speed"`
	FreeFrom Num  `json:"freeFrom"`
	// HandsOver means its end has to lead onto another module's transport, as a gate's
	// exit onto a slide. Without one there, the station checks say so.
	HandsOver bool `json:"handsOver,omitempty"`
	// Node makes it ride on a node that moves, such as a lift's platform or a nest of a
	// turntable. Its path, a line, is then in the frame of that node. Where its ends meet
	// another transport's is worked out as it moves, so it takes workpieces on and hands them
	// over wherever it lines up.
	Node string `json:"node,omitempty"`
	// Branch makes it start on a transport of another module, wherever it is mounted, and
	// take workpieces off that one while When holds, or always without it. A shared
	// transport also runs with that one's drive.
	Branch *BranchDef `json:"branch,omitempty"`
}

// BranchDef is when a transport takes workpieces off the one it starts on.
type BranchDef struct {
	When string `json:"when,omitempty"`
}

// PinDef describes one pin of a connector. Pins named I<n> carry signals to the
// PLC and pins named Q<n> carry signals from it.
type PinDef struct {
	Analog   bool   `json:"analog,omitempty"` // carries a value instead of on or off
	Pin      string `json:"pin"`
	Function string `json:"function,omitempty"`
	Drives   string `json:"drives,omitempty"` // device input this pin controls
	Reads    string `json:"reads,omitempty"`  // state this pin reports
	Invert   bool   `json:"invert,omitempty"`
	Constant *bool  `json:"constant,omitempty"`
	Spare    bool   `json:"spare,omitempty"`
	// When leaves the pin out where it is false, as of a part a module may come without.
	When Num `json:"when"`
}

// ConnectorDef is a physical connector. Type names an entry of connectors.json,
// which gives its standard pins. Byte marks a PLC connector: its pins are the
// bits of that image byte. Internal hides a connector that only repeats the
// signals of another one from the module's pin tables.
type ConnectorDef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	When Num    `json:"when"`
	Byte *int   `json:"byte,omitempty"`
	// AnalogIn makes the connector's UE pins the PLC's analogue inputs: UE1 is channel 0.
	AnalogIn bool     `json:"analogIn,omitempty"`
	Internal bool     `json:"internal,omitempty"`
	Pins     []PinDef `json:"pins,omitempty"`
	// Prefer is the connector that the builder cables it to first, on a module that has one
	// of that name and type, as a stand's console goes to the PLC's socket for it.
	Prefer string `json:"prefer,omitempty"`
	// Switched feeds the outputs that pass through it from the station's safety circuit, as
	// the actuator supply of an I/O block: while a safety relay is open, they are off.
	Switched bool `json:"switched,omitempty"`
}

// BridgeDef connects two connectors of a module inside it. Digital pin n of A
// joins pin n+Offset of B, for each direction. With Pins it joins only those
// pins instead, from A's names to B's, which is how analogue signals are routed.
type BridgeDef struct {
	A      string            `json:"a"`
	B      string            `json:"b"`
	Offset int               `json:"offset,omitempty"`
	Pins   map[string]string `json:"pins,omitempty"`
}

// TerminalDef is the fixed electrical interface of a module.
type TerminalDef struct {
	Connectors []ConnectorDef `json:"connectors,omitempty"`
	Bridges    []BridgeDef    `json:"bridges,omitempty"`
}

// ActionDef is a function of the model that people can trigger. Type is
// clearTransport or placeWorkpiece.
type ActionDef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// Label is how a button for it reads. A placeWorkpiece gets a button for each kind of
	// workpiece it takes instead.
	Label string `json:"label,omitempty"`
	// Dock offers it at the bottom of every station the builder makes, as a magazine's fill
	// or putting a workpiece onto a belt by hand.
	Dock      bool   `json:"dock,omitempty"`
	Transport string `json:"transport"`
	At        Num    `json:"at"`
	BlockedBy string `json:"blockedBy,omitempty"`
	When      Num    `json:"when"`
	// Family limits a placeWorkpiece to the kinds of that family, as a magazine set up for
	// caps takes nothing else.
	Family string `json:"family,omitempty"`
	// Over, instead of a transport, puts the workpiece onto whatever transport passes under
	// that point of the module, or takes away what stands there, as the nest of a turntable
	// that is at its loading place.
	Over Vec3 `json:"over"`
	// Heading is the way a placed workpiece with a size points its length, in the module's
	// coordinates, as a pallet put into a magazine. Without it, it points along the transport.
	Heading Vec3 `json:"heading"`
}

// WidgetDef is a control shown when a module is clicked, or in a station dock.
// Type is push, switch, lamp, fill, button or group.
type WidgetDef struct {
	Type string `json:"type"`
	When Num    `json:"when"`
	Show string `json:"show,omitempty"` // popup (default) or panel, on modules

	Label  string   `json:"label,omitempty"`
	Tag    string   `json:"tag,omitempty"`
	Tone   string   `json:"tone,omitempty"`
	Labels []string `json:"labels,omitempty"`

	Device string `json:"device,omitempty"` // push and switch
	State  string `json:"state,omitempty"`  // lamp, and the lamp of a push

	Action       string            `json:"action,omitempty"` // button
	Args         map[string]string `json:"args,omitempty"`
	Swatch       string            `json:"swatch,omitempty"`
	DisabledWhen string            `json:"disabledWhen,omitempty"`

	Transport string `json:"transport,omitempty"` // fill
	Capacity  Num    `json:"capacity"`
	Expects   string `json:"expects,omitempty"`

	Widgets []WidgetDef `json:"widgets,omitempty"` // group
}

// ModuleDef is the content of module.json.
// MirrorDef is what a beam does at a mirror: it turns by Angle degrees about the
// module's vertical and runs on for Reach mm.
type MirrorDef struct {
	When  Num `json:"when"`
	Angle Num `json:"angle"`
	Reach Num `json:"reach"`
}

// BoxDef is a box between two corners.
type BoxDef struct {
	From Vec3 `json:"from"`
	To   Vec3 `json:"to"`
}

type ModuleDef struct {
	Level string `json:"-"`               // module or accessory, from the folder it was found in
	Group string `json:"group,omitempty"` // where the library lists an accessory, such as Sensors

	// Role is what a module is to the builder beyond something to screw on, one of the Role
	// constants. Without one, it is just that.
	Role string `json:"role,omitempty"`
	// Addresses is how the program of a module of the plc role names the variables of its
	// image, which it needs: inputs, outputs and analogue inputs, with {byte}, {bit} (or
	// {bit:2} for two digits) and {channel}, and the variable list they are in, as a PLC
	// offers them over OPC UA. Station files name the PLC's signals by these names.
	Addresses *plcio.Naming `json:"addresses,omitempty"`
	// Floor is the size of a module of the station role on the floor.
	Floor *FloorDef `json:"floor,omitempty"`

	Format      int            `json:"format"`
	ID          string         `json:"id"`
	Version     int            `json:"version"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  []ParameterDef `json:"parameters,omitempty"`
	Model       []NodeDef      `json:"model,omitempty"`
	Joints      []JointDef     `json:"joints,omitempty"`
	Devices     []DeviceDef    `json:"devices,omitempty"`
	Transport   []TransportDef `json:"transport,omitempty"`
	Terminal    TerminalDef    `json:"terminal"`
	Actions     []ActionDef    `json:"actions,omitempty"`
	Controls    []WidgetDef    `json:"controls,omitempty"`

	// A module can contain other modules, which stay separately clickable. Its
	// wires and cables join them; references name a contained module or the
	// module's own connector, such as "io.X1" or "X1".
	Modules []InstanceDef `json:"modules,omitempty"`
	Wires   []WireDef     `json:"wires,omitempty"`
	Cables  []CableDef    `json:"cables,omitempty"`
	// Links join the module's own transports and those of what it contains.
	Links []LinkDef `json:"links,omitempty"`

	// Sites are where other modules can be screwed on, and Mounts where this one can be
	// screwed down, the way the real hardware is put together.
	Sites  []SiteDef  `json:"sites,omitempty"`
	Mounts []MountDef `json:"mounts,omitempty"`

	// CutsRails makes the module cut a gap into every rail it stands in, as a slide does into
	// the rail along a belt so that workpieces can roll onto it.
	CutsRails bool `json:"cutsRails,omitempty"`
	// CutBox is where a rail is in the way of a module that cuts rails, as a box in its own
	// coordinates. A gap runs across its x axis, and leaves pieces with length, capStart
	// and capEnd set, so that every piece has both its end caps.
	CutBox *BoxDef `json:"cutBox,omitempty"`

	// Needs are the parts it does not work without, even where they can be taken off.
	Needs []NeedDef `json:"needs,omitempty"`
}

// Roles of modules.
const (
	// RoleStation is what a station stands on, such as a base plate or a frame on wheels. It
	// stands on the floor, beside others, and what is screwed onto it belongs to that station.
	RoleStation = "station"
	// RolePLC is a station's PLC. A station has one, which the builder cables to every console,
	// interface and terminal with a socket of its kind.
	RolePLC = "plc"
	// RoleConsole is the operator panel of a station, with its buttons and lamps, such as the
	// control console at the front of a stand. The builder cables it to the PLC.
	RoleConsole = "console"
	// RoleInterface passes a PLC's signals on to terminals, such as a fieldbus coupler: the builder
	// cables what fits the PLC to it, and its other sockets to the nearest free one of the same
	// type.
	RoleInterface = "interface"
	// RoleTerminal is where devices are wired to, such as an I/O block: its connectors that are
	// not internal take the wires of an accessory put onto a module.
	RoleTerminal = "terminal"
)

// Roles lists the roles a module can have.
var Roles = []string{RoleStation, RolePLC, RoleConsole, RoleInterface, RoleTerminal}

// FloorDef is how a module of the station role stands on the floor: Width along x and Depth
// along y from its corner. Beside are the settings of one that is put beside it.
type FloorDef struct {
	Width  Num                        `json:"width"`
	Depth  Num                        `json:"depth"`
	Beside map[string]json.RawMessage `json:"beside,omitempty"`
}

// SiteDef is a straight place on a module that others are screwed to, such as a T-slot of
// a plate or the groove of a guide rail. A position on it counts in mm from From along
// Dir, within Range. Repeat makes parallel copies, one per slot. A module on it faces
// along Dir, with its y axis to the left of Dir, turned further by Turn.
type SiteDef struct {
	ID     string     `json:"id"`
	Type   string     `json:"type"`
	From   Vec3       `json:"from"`
	Dir    Vec3       `json:"dir"`
	Range  [2]Num     `json:"range"`
	Repeat *RepeatDef `json:"repeat,omitempty"`
	Turn   Num        `json:"turn"`
	When   Num        `json:"when"`
	// Rail is the contained rail whose groove this is: where that rail is cut, so is the site,
	// and the rail's own sites are left to this one.
	Rail string `json:"rail,omitempty"`
	// Up is where a module's z axis points on the site, by default up. With its x axis along
	// the site, its y axis runs into the face it is screwed to, as on the T-slot at the side
	// of a profile. A site that runs up, such as a T-slot of an upright profile, needs one.
	Up Vec3 `json:"up"`
}

// MountDef is how a module is screwed onto a site of Type: At is the point of the module
// that sits on the site, and every one of Screws has to find a site of the same type
// on the same module. Turn is how the module faces on the site. With Head, each screw
// is drawn as the shared plate-screw part, its head that high above the site.
type MountDef struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	At     Vec3   `json:"at"`
	Screws []Vec3 `json:"screws,omitempty"`
	Head   Num    `json:"head"`
	Turn   Num    `json:"turn"`
	// RotateX stands the module up first, as a I/O block on its bracket at the side of a belt,
	// and RotateY then, as a profile stands on its end.
	RotateX Num `json:"rotateX"`
	RotateY Num `json:"rotateY"`
	When    Num `json:"when"`
	// Span names a parameter that is set to how far the site lies from the nearest other site
	// of the same type alongside it, unless a station sets it, as the distance between the
	// heads of a through-beam sensor is that between the rails of a belt or a slide.
	Span string `json:"span,omitempty"`
	// Runs limits it to sites that run level, such as the side of a belt's profile, or
	// upright, such as a column, as a I/O block hangs on a bracket from the one and is screwed
	// flat onto the other.
	Runs string `json:"runs,omitempty"`
}

// Fits reports whether the mount fits a site that runs along dir, by its Runs.
func (m *MountDef) Fits(dir Point) bool {
	upright := math.Abs(dir.Z) > 0.7*length(dir)
	return m.Runs == "" || m.Runs == "upright" && upright || m.Runs == "level" && !upright
}

// ScrewPart is the part drawn for each screw of a mount with a Head.
const ScrewPart = "plate-screw"

// PlaceDef puts an instance onto a site of a module placed before it: On is that
// module and site, such as "stand/plate.slots", Slot picks one of a site's repeats,
// At is the position along it in mm and Turn a further turn in steps of 90 degrees.
// Mount picks the module's mount, by default the first that fits the site.
type PlaceDef struct {
	On    string `json:"on"`
	Slot  int    `json:"slot,omitempty"`
	At    Num    `json:"at"`
	Turn  Num    `json:"turn"`
	Mount string `json:"mount,omitempty"`
}

// StationParameterDef is a setting of a station that users can change. Kind and Description
// are as for a module's.
type StationParameterDef struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Unit        string    `json:"unit,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Description string    `json:"description,omitempty"`
	Options     []float64 `json:"options,omitempty"`
	Names       []string  `json:"names,omitempty"`
	Default     float64   `json:"default"`
	Min         float64   `json:"min"`
	Max         float64   `json:"max"`
	Step        float64   `json:"step,omitempty"`
	Module      string    `json:"module,omitempty"` // instance it is shown on

	// Only set when previewing a single module: a parameter without a label is for
	// whoever builds a station with it, and a flag is on or off.
	Advanced bool `json:"-"`
	Boolean  bool `json:"-"`
}

// InstanceDef places a module in a station.
type InstanceDef struct {
	ID       string            `json:"id"`
	Module   string            `json:"module"`
	Name     string            `json:"name,omitempty"`
	When     Num               `json:"when"`
	At       Vec3              `json:"at"`
	RotateX  Num               `json:"rotateX"`
	RotateY  Num               `json:"rotateY"`
	RotateZ  Num               `json:"rotateZ"`
	Params   map[string]Num    `json:"params,omitempty"`
	Texts    map[string]string `json:"texts,omitempty"`    // used as {name} in text fields
	SelectAs string            `json:"selectAs,omitempty"` // clicks select this instance instead
	// MountedOn is a moving node of the module around it that this one rides on, as a switch
	// on a cylinder that moves. At stays in that module's coordinates, at rest.
	MountedOn string `json:"mountedOn,omitempty"`
	// Place screws the instance onto a site instead of putting it at At.
	Place *PlaceDef `json:"place,omitempty"`
	// Optional marks a module that another one contains, such as a conveyor's sensor, as one
	// that a station may leave out.
	Optional bool `json:"optional,omitempty"`
	// Cut lets modules that cut rails cut a gap into this contained rail. A module that cuts
	// rails cannot stand in a rail without it.
	Cut bool `json:"cut,omitempty"`
	// Parts changes the modules this one contains, by their ids: where one is placed, its
	// settings, or that it is left out.
	Parts map[string]PartChange `json:"parts,omitempty"`
	// Rewire moves what the module's own wires join at one pin onto another, by references
	// inside the module, such as {"io.T.Q0": "io.T.Q3"} to run a conveyor forward from output
	// 3 of its I/O block. Moving two pins onto each other swaps them.
	Rewire map[string]string `json:"rewire,omitempty"`
}

// PartChange changes a module that another one contains. Place puts it elsewhere on the sites
// of the module around it (".rear"), and Params change its settings, both evaluated as the
// module around it sees them. Removed leaves an optional one out, with its wires.
type PartChange struct {
	Place   *PlaceDef      `json:"place,omitempty"`
	Params  map[string]Num `json:"params,omitempty"`
	Removed bool           `json:"removed,omitempty"`
}

// LinkDef moves workpieces between transports. Type is continue (at the end),
// branch (at a distance, while When holds or always without it, as for a fixed
// guide) or shareDrive (run with the
// drive and speed of To).
type LinkDef struct {
	Type      string `json:"type"`
	Transport string `json:"transport"`
	At        Num    `json:"at"`
	To        string `json:"to,omitempty"`
	When      string `json:"when,omitempty"`
	If        Num    `json:"if"` // leaves the link out when false, for a module's optional parts
}

// CableDef joins two connectors of the same type pin by pin.
type CableDef struct {
	From string `json:"from"`
	To   string `json:"to"`
	When Num    `json:"when"`
}

// WireDef joins two pins.
type WireDef struct {
	From string `json:"from"`
	To   string `json:"to"`
	When Num    `json:"when"`
}

// DockDef places station-wide widgets. Place is bottom or side.
type DockDef struct {
	Place   string      `json:"place"`
	Widgets []WidgetDef `json:"widgets"`
}

// PLCSignalDef names a bit of the PLC image for display.
// PLCMapDef lets a PLC program use other addresses than the station. A variable it
// does not mention uses the same address on both sides.
type PLCMapDef struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Inputs  []PLCBindingDef `json:"inputs,omitempty"`
	Outputs []PLCBindingDef `json:"outputs,omitempty"`
}

// PLCBindingDef connects a signal of the station to a variable of the PLC program.
type PLCBindingDef struct {
	Signal   string `json:"signal"`
	Variable string `json:"variable"`
	Invert   bool   `json:"invert,omitempty"`
}

type PLCSignalDef struct {
	Address     string `json:"address"`
	Name        string `json:"name"`
	Variable    string `json:"variable,omitempty"`
	Description string `json:"description,omitempty"`
}

// StationDef is the content of station.json.
type StationDef struct {
	Format      int                   `json:"format"`
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Workpieces  string                `json:"workpieces"`
	Parameters  []StationParameterDef `json:"parameters,omitempty"`
	Modules     []InstanceDef         `json:"modules"`
	Links       []LinkDef             `json:"links,omitempty"`
	Cables      []CableDef            `json:"cables,omitempty"`
	Wires       []WireDef             `json:"wires,omitempty"`
	Actions     []ActionDef           `json:"actions,omitempty"`
	Docks       []DockDef             `json:"docks,omitempty"`
	PLC         StationPLCDef         `json:"plc"`
	// PLCs are the PLCs besides the first one, of a line of stations on stands side by
	// side, each running its own: the modules its connectors reach, and its I/O table.
	PLCs []StationPLCDef `json:"plcs,omitempty"`
}

// StationPLCDef is a PLC of a station: the PLC module, such as the rack on a stand, the
// names of its signals by address, and the address maps for programs written for another
// wiring. Program is a built-in program, of the first PLC only.
type StationPLCDef struct {
	Program string         `json:"program,omitempty"`
	Module  string         `json:"module,omitempty"`
	Signals []PLCSignalDef `json:"signals,omitempty"`
	Maps    []PLCMapDef    `json:"maps,omitempty"`
}

// WorkpieceKind is one variant of a workpiece set.
type WorkpieceKind struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Material Material `json:"material"`
	Metal    bool     `json:"metal,omitempty"`
	// Reflectivity is the percentage of light it sends back to an optical sensor,
	// and Transparency the percentage it lets through a beam.
	Reflectivity float64 `json:"reflectivity,omitempty"`
	// Height overrides the set's height, for a set that differs in height.
	Height       float64 `json:"height,omitempty"`
	Transparency float64 `json:"transparency,omitempty"`
	// Family tells apart what is not interchangeable in one set, such as bodies and lids.
	Family string `json:"family,omitempty"`
	// Points overrides the set's profile. Height is then the height of that profile.
	Points [][2]float64 `json:"points,omitempty"`
	// Onto lets it join a workpiece of another family that it is set down or dropped onto,
	// as a lid onto a body.
	Onto *OntoDef `json:"onto,omitempty"`
	// Size makes it a rectangular block of that length and width in mm, as a pallet or a
	// cardboard box, instead of round with the set's diameter. Its length points along its
	// heading, which is the transport's where it is put on one.
	Size []float64 `json:"size,omitempty"`
	// Model is how it looks, as nodes of parts like a module's, drawn from the middle of its
	// base. A kind with a size and no model is a plain block of its material.
	Model []NodeDef `json:"model,omitempty"`
}

// OntoDef is what a workpiece kind joins: the family it goes onto, and how deep in mm it
// sinks into it.
type OntoDef struct {
	Family string  `json:"family"`
	Depth  float64 `json:"depth,omitempty"`
	// Free lets it go anywhere on top of the other one and stay where it was put, also onto
	// what was put there before, as boxes stacked on a pallet. Without it, it only goes onto
	// the middle of the other one's top, as a cap.
	Free bool `json:"free,omitempty"`
}

// MarkDef is something a process leaves on a workpiece, such as a hole or a filling, and
// how the 3D view shows it. Fill draws it as a filling inside the workpiece that rises that
// many mm per unit, and Hole as a hole of that diameter into its top.
type MarkDef struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Material *Material `json:"material,omitempty"`
	Fill     float64   `json:"fill,omitempty"`
	Hole     float64   `json:"hole,omitempty"`
}

// WorkpieceSet is the content of a workpieces file. Points is the profile of a
// three.js LatheGeometry.
type WorkpieceSet struct {
	Format   int             `json:"format"`
	ID       string          `json:"id"`
	Diameter float64         `json:"diameter"`
	Height   float64         `json:"height"`
	Points   [][2]float64    `json:"points"`
	Kinds    []WorkpieceKind `json:"kinds"`
	// Marks are what processes can leave on these workpieces.
	Marks []MarkDef `json:"marks,omitempty"`
}

// Material uses the names of three.js material classes and parameters.
type Material struct {
	Type      string  `json:"type"` // MeshLambertMaterial or MeshPhongMaterial
	Color     string  `json:"color"`
	Shininess int     `json:"shininess,omitempty"`
	Specular  string  `json:"specular,omitempty"`
	Opacity   float64 `json:"opacity,omitempty"`
}

// ConnectorType is one entry of connectors.json. Analog lists its analogue pins,
// of which the names starting with AQ or UA are outputs.
type ConnectorType struct {
	Label   string   `json:"label"`
	Inputs  int      `json:"inputs"`
	Outputs int      `json:"outputs"`
	Analog  []string `json:"analog,omitempty"`
	// Color is how the wiring view draws the connector and its cables, as #rrggbb.
	Color string `json:"color,omitempty"`
}
