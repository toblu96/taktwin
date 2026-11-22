# Collections and their files

Everything Taktwin knows about a station is in JSON files. The app reads them at startup, so changing a module, or adding a station built from existing modules, needs no code change. The JSON Schemas in [`schema/`](../schema) describe every file and field, with the same descriptions as this guide. Give a file its schema with `"$schema"` for completion and checks in an editor, for example `"$schema": "https://raw.githubusercontent.com/toblu96/taktwin/main/schema/module.schema.json"`. The schemas are generated from the Go types the app reads the files into (`go run ./cmd/taktwin-schema`), and a test keeps them in step.

## Collections

A collection is a folder of these files:

```
collection.json                 its id, name, version, description, and the station and workpieces it opens with
materials.json                  materials, by name
connectors.json                 connector types, by name, with how many inputs and outputs they carry
workpieces/<id>.json            a workpiece set: shape, kinds and their properties
parts/<id>.json                 3D geometry shared between modules, such as a connector
modules/<id>/module.json        a machine
accessories/<id>/module.json    what goes onto machines: sensors, I/O blocks, consoles, PLCs
stations/<id>/station.json      a station: which modules, where, how they are cabled and wired
exercises/<id>/exercise.json    a task on a station, and how a run of it is checked
exercises/<id>/instructions.md  what the exercise asks for, in Markdown, shown beside its check (optional)
connections/<id>.json           how a station's PLC connects to a controller outside the app
```

`modules/` and `accessories/` use the same format; the library lists them apart. A module folder can hold more than its `module.json`, such as the Structured Text source of a controller the module brings (see [Programs](#programs)).

The app layers collections. `catalog/base` is always there, with what every collection can use: the standard connector types, materials and shared parts. `catalog/reference` is the app's own collection, a pallet loading line, and the one it opens with. Collections the user imports in Settings are kept in `<user folder>/collections/`, and `TAKTWIN_COLLECTIONS` names collection folders for development. The user's own stations and modules are in the user folder itself, in the same folders. Materials and connector types of all layers add up; a module, part or station is taken from the first layer that has it.

A collection travels as a zip with the extension `.taktwin` that holds the folders at its root. It holds only JSON and Structured Text, which the app runs in its own simulation, so it cannot run code on the computer.

```sh
cd my-collection && zip -r ../my-collection.taktwin .
go run ./cmd/taktwin-cli -validate ../my-collection.taktwin
```

## module.json

A module is a physical unit you could unscrew from where it stands. Its coordinates are local (mm, z up), so a station can place it anywhere.

| Section | Defines |
|---|---|
| `parameters` | Named numbers a station can override. Those with a `label` are settings people can change, in the builder and the library; the others are the module's own business. A setting says its `kind`: `build` for how the module is put together, such as the length of a conveyor, or `physics` for how it behaves, such as its speed. It has a `description` of what it does, and limits of its own: `min`, `max` and `step`, or `options` with `names`. A value outside them does not load |
| `needs` | Modules it does not work without, such as a conveyor's motor controller: `module`, `min` (default 1) and `why`. They may still be taken off, and the station checks say what is missing |
| `model` | The 3D shape, as a tree of named nodes |
| `joints` | How nodes move |
| `devices` | The working parts: cylinders, axes, sensors, grippers, lamps, buttons, controllers |
| `transport` | Paths that carry workpieces |
| `terminal` | The connectors and pins, and what each pin does |
| `actions` | Functions of the model that people can trigger, such as putting a workpiece on |
| `controls` | What appears when the module is clicked |
| `modules`, `wires`, `cables`, `links` | Modules it contains, and how they join up inside it. A link with `if` is left out when that is false, for optional parts |
| `sites`, `mounts` | Where other modules can be screwed to it, and where it is screwed down itself. See [Mounting](#mounting) |
| `role` | What the module is to the builder beyond something to screw on. See [Roles](#roles) |
| `addresses` | For a PLC: how its program names the variables of its image. See [PLCs](#plcs) |
| `plc` | For a PLC: how it shows its addresses. See [PLCs](#plcs) |
| `group` | Where the library lists an accessory: Sensors, Actuators, PLC and I/O, or Mechanical |

A `description` on a module is what people read in the library, so say what it is and what it does. The library previews every entry on its own, without a PLC. It does so with the workpieces its collection names, and offers on each transport only the kinds of the `family` that the module's own `placeWorkpiece` actions on that transport name, such as cartons on a carton conveyor; without one, every kind.

Any number can be an expression over parameters, such as `"length - 47"` or `"radius * sin(45)"`. Angles are degrees. Expressions know `+ - * /`, comparisons, `&&`, `||` and `!` (true is 1, and anything from 0.5 counts as true), and `sin`, `cos`, `asin`, `sqrt`, `abs`, `min` and `max`. Anything with `"when"` only exists if the expression isn't zero. A station can pass text with `texts`, used as `{name}` in text fields.

A module can contain other modules, as a conveyor contains its motor controller. Each stays separately clickable, under the id `parent/child`, and moves with its parent. One with `optional` may be left out by a station, with the module's wires to it. Inside the module, references name a contained module or the module's own connector, such as `io.X1` or `X1`.

### model

Nodes follow three.js `Object3D`, and logic only ever refers to them by `name`. A node's shape can be refined without touching how the module behaves.

```json
{ "name": "arm", "position": [20, -24, 0], "rotation": [0, 0, 45], "children": [] }
{ "position": [23, -38.5, 10], "geometry": { "type": "BoxGeometry", "width": 22, "height": 25, "depth": 24 }, "material": "steel" }
```

| Taken from three.js | Taktwin extension |
|---|---|
| `position`, `children`, `name` | `rotation` is in degrees, and z is up |
| `BoxGeometry`, `PlaneGeometry`, `SphereGeometry` and their parameters | `CylinderGeometry` is given by `from`, `to`, `radius` and optional `radialSegments` |
| `ExtrudeGeometry` with `depth`, along +z | Its shape is given by `points` and `holes` |
| `MeshLambertMaterial`, `MeshPhongMaterial` and their parameters | `SectorGeometry`: a curved plate with `center`, `inner`, `outer`, `start`, `end`, `height` |
| `LatheGeometry` points, for workpieces | `part`, `label`, `belt`, `repeat`, `noPick`, `when` |

A `label` has `text`, `size` and `color`, and a `background` to draw it on a plate.

A node with `"part": "plate-screw"` draws that shared part at its position and rotation. A part is a file with `format`, `id`, `name` and a `model`, so improving it improves every module that uses it. A node in a part with the material `inherit` takes the material of the node that draws it.

Build geometry from functional dimensions and these shapes. Do not convert or trace meshes, textures or logos of a manufacturer.

### joints

A joint maps a state from 0 to 1 onto a movement of a node. `range` is in degrees for `revolute` and mm for `prismatic`. `axis` and `origin` are in the node's own coordinates.

```json
{ "node": "arm", "type": "revolute", "axis": [0, 0, 1], "origin": [20, -24, 0], "range": [0, 45], "driver": "cylinder.position" }
```

### devices

The device types are implemented once in `internal/sim`. Anything built from them is only data.

| `type` | What it does | Inputs | States |
|---|---|---|---|
| `cylinder` | Moves `stroke` mm at `speed` mm/s. `rest` is its power-off end. A `monostable` valve returns when its coil drops; a `bistable` one holds its position. `limited` stops it gently on what it presses on; `stop` and `stopAt` hold it part way out | `extend`, `retract`, depending on valve and rest | `position` (0-1), `extended`, `retracted`, `commanded`, `midway` |
| `reedSwitch` | End switch on a `cylinder`, at `retracted` or `extended`. A switch anywhere else along a stroke is a `logic` over its `position` | | `on` |
| `axis` | A motor that moves `stroke` mm (or degrees) at `speed` per second while `forward` or `backward` is on. `forward` and `backward` given as names make it follow those states instead. With `wrap` it turns on and on. With `targets` (up to 8) it is a positioning axis: as `go` comes on it drives to the target that `record0` to `record2` select in binary. With `setpoint` it drives to the value of its input `setpoint` as `go` comes on. `enable` adds a controller's enable, ready, homed and error | `forward`, `backward`; `go`, `record0`-`record2`, `setpoint`, `enable` | `position` (0-1), `value`, `start`, `end`, `moving`; `done`, `ready`, `homed`, `error` |
| `motorController` | DC motor controller. One direction input runs the motor, both brake it and none stops it. `creep` is half speed | `clockwise`, `counterclockwise`, `creep` | `forward`, `backward`, `running`, `factor`, `ready` |
| `sensor` | Beam with `origin`, `direction` and `reach`. `detect` is `metal`, `reflection` (a workpiece reflects at least `threshold` percent of the light) or `beam` (less than `threshold` percent gets through it). `has` limits it to workpieces with a mark or a joined part. `mirror` folds its beam for a second `reach`. `offDelay` holds it on; `fault` adds a stuck-low fault | | `detected`, `free` |
| `distanceSensor` | Measures straight down from `origin`, between `min` and `reach` mm, and sees `surface` mm when nothing is there. Its `value` is 0 to 10 V over that range, and it switches within `threshold` mm | | `detected`, `value`, `distance` |
| `gripper` | Holds a workpiece at its `tip` node: with the jaws of its `cylinder`, or without one as a suction cup or, with `"detect": "metal"`, a magnet, which holds what its tip touches from above within `reach` mm | `on`, without jaws | `holding` |
| `pusher` | Its `cylinder` pushes the lowest workpiece of the `from` transport along the `to` transport, as a magazine does, or without `from` whatever stands at the start of `to`, as an ejector | | |
| `solid` | A `box` that moves with its `node`, which workpieces cannot enter (see below) | | |
| `process` | Works on the workpiece under `origin`, within `reach` mm below it: after `time` seconds on, sets its `mark` to `amount`, or with `adds` adds it | `on` | `busy`, `done` |
| `button`, `key` | Operated by pressing `node` in 3D. A button follows the pointer; a key flips | | `active` |
| `lamp` | Colours `node` with `on` or `off`. Lit by `state`, by a pin that drives its `on` input, or always with `alwaysOn`. `glow` is the diameter in mm of its halo while lit | `on` | `on` |
| `logic` | A state worked out from others, as a relay circuit would. `expr` is an expression over states of the module and its parts, such as `"gate.extended && !stop.detected"`. Or `set` and `reset` make it a latch, `reset` winning. `onDelay` and `offDelay` hold off a change, in seconds. `drives` sets an input of a device of the module to it, unless a PLC output drives that input | | `on`, `value` |
| `safetyRelay` | The station's safety circuit. It opens while `reset` holds, such as an emergency stop being pressed, and closes again only while `set` holds and `reset` does not. While any safety relay is open, PLC outputs through a `switched` connector are off, and positioning axes stop | | `ok` |
| `program` | A controller of the module's own, in Structured Text. See [Programs](#programs) | its `VAR_INPUT` variables | its `VAR_OUTPUT` variables |
| `plc` | Marks the module as the PLC, so clicking it shows the whole PLC image | | |

A `solid` is a `box` that moves with its `node`, or stays put without one. Workpieces cannot enter it and it cannot move into them, but they can always move away. A workpiece running into it just stops. A solid moving into a workpiece stops too, and that is a collision, which the top bar shows with its `label`. Its node may only turn about the vertical. Where a box fits a part badly, an `outline` gives its shape from above. With `shoves` it pushes a workpiece back along its transport instead of stopping.

Everything else collides without being declared. Each fixed node with a box, cylinder or extruded geometry becomes an obstacle, except with `noPick`. A solid or a carried workpiece that moves into an obstacle or into a solid of another top-level module stops there and reports it. Parts of the same top-level module ignore each other.

### Programs

A module can bring its own controller, as a machine with a built-in control does, written in IEC 61131-3 Structured Text. Such a mechanism needs no new code in the app: the module is complete with its `.st` file.

```json
{
  "type": "program", "id": "ctrl", "source": "controller.st", "program": "Controller", "cycle": 0.002,
  "reads": { "atStart": "x.start", "taken[0..7]": "place{}.count", "beltBusy": "/{belt}.surface.count" },
  "writes": { "xTarget": "x.setpoint", "xGo": "x.go", "grip": "gripper.cylinder.extend" }
}
```

The app compiles `source`, from the module's folder, when it reads the module, and reports a mistake with its file, line and column. It runs the `PROGRAM` named by `program` (the first one without) every `cycle` seconds (0.01 without) on the simulation's clock.

- The program's `VAR_INPUT` variables are inputs of the device that pins can drive, and its `VAR_OUTPUT` variables are states that pins can read, such as `ctrl.busy`.
- `reads` gives variables the values of states of the module before each cycle. A state of the station starts with `/` and uses the names the station passes in `texts`, such as `/{belt}.surface.count`. Where the station names none, the variable keeps its initial value.
- `writes` sets inputs of the module's devices to variables after each cycle.
- A name ending in `[a..b]` binds the elements of an array, with `{}` in the state standing for the index.
- A BOOL is 0 or 1, and a TIME counts seconds.
- A transport's `count` is how many workpieces are on it. Its `before` is on while another transport takes them over at its start.

### transport

A path is a `line` or an `arc`. `drive` names the state that runs it; without one, gravity always moves it. With `open`, workpieces that run off either end fall. A falling workpiece lands on a straight transport it falls onto. One that reaches the floor is lost, which the top bar reports until someone acknowledges it. `reverse` names the state that runs it backwards, and `factor` one that scales its speed. `shared` means it runs with the drive and speed of another transport. `freeFrom` is the distance from which it keeps moving without its drive.

Modules hand workpieces to each other where they are mounted, without station links. A transport with `branch` starts on another module's transport and takes workpieces off it while `branch.when` holds. A transport that ends where another module's transport starts, within 8 mm, goes on onto it. Where an open end stops less than 25 mm short of the start of another module's transport, straight ahead and at the same height, workpieces cross onto it, as between two conveyors side by side.

A transport with `node` rides on that node, such as a lift's platform or a nest on a turntable, and moves with its joints. Where its ends meet another transport is worked out as it moves.

### terminal

Every module declares its connectors, and every pin is fixed. Pins named `I<n>` carry signals to the PLC and pins named `Q<n>` carry signals from it.

```json
"terminal": { "connectors": [{ "id": "X1", "type": "m23-io", "pins": [
  { "pin": "Q0", "function": "Z axis down", "drives": "z.extend" },
  { "pin": "Q2", "function": "Vacuum on", "drives": "cup.on" },
  { "pin": "I0", "function": "Z axis is up", "reads": "zTop.on" },
  { "pin": "Q3", "spare": true }
]}]}
```

| Pin field | Meaning |
|---|---|
| `drives` | The device input this pin controls |
| `reads` | The state this pin reports. `invert` flips it, as for a normally closed button |
| `constant` | A fixed level |
| `spare` | Deliberately unused |
| `symbol` | The name the PLC gives the signal of this pin, such as `B1`, where the station names none |

- A connector's `type` names an entry of `connectors.json`, which gives its pins. The `wires` type has freely named pins, for parts with loose wires.
- `bridges` connect two connectors inside a module: digital pin n of `a` joins pin n + `offset` of `b`. With `pins` a bridge joins only the named pins, such as an analogue signal.
- A connector type can list `analog` pins. They travel through cables and bridges like digital ones.
- `byte` marks a PLC connector: its pins are the bits of that byte of the image. With `analog` (formerly `analogIn`), its `UE` pins are the PLC's analogue inputs and its `UA` pins its analogue outputs, `UE1` and `UA1` channel 0; the PLC module places them in the image (see [PLCs](#plcs)).
- `internal` leaves a connector out of the pin tables, where it only repeats another's signals.
- `switched` feeds the outputs through it from the safety circuit, as the actuator supply of an I/O block.
- `prefer` names the connector the builder cables it to first.

### actions and controls

An action is a function of the model, and it runs in the simulation. `clearTransport` removes the workpieces on a transport. `placeWorkpiece` adds one `at` a distance, of the given `family` if it has one, refused while `blockedBy` is on. Either can name a point of the module with `over` instead of a transport. With `dock`, a station made in the builder offers it at the bottom of the screen.

A control is a widget. It opens in a popup on the module when clicked, or in the right-hand panel with `"show": "panel"`.

| `type` | Shows |
|---|---|
| `fill` | Fill level of a `transport`, with `capacity`. `expects` flags wrong workpiece kinds |
| `button` | Runs an `action` with `args`. `disabledWhen` greys it out; `swatch` adds a colour |
| `push` | Holds a `button` device while pressed. `state` lights it |
| `switch` | Sets a `key` device, with two `labels` |
| `lamp` | Shows a `state` |
| `group` | Titled set of `widgets` |

## PLCs

A module with the role `plc` and a `plc` device is a PLC. Its connectors with a `byte` are its I/O, and its `addresses` say how its program names the variables of its image, with `{byte}`, `{bit}` (or `{bit:2}` for two digits) and `{channel}`:

```json
"addresses": { "inputs": "IB{byte}Bit{bit:2}", "outputs": "QB{byte}Bit{bit:2}", "analog": "AI{channel}", "list": "Bits" }
```

The soft PLC declares them in its `Bits.st`, and the app's connection to CODESYS expects them in the variable list `list` (see [Connections](#connections)).

Stations give addresses as IEC 61131-3 direct addresses: `%IX0.3` is bit 3 of input byte 0, `%QX0.3` the same output bit, `%IB2` a byte, `%IW8` a word and `%ID8` a double word, each at its first byte. Words and double words are stored with their low byte first, as CODESYS on a PC stores them. Older station files give the names of `addresses` instead, such as `IB0Bit03` and `AI0`, which still works.

`plc.image` says how many bytes of inputs and of outputs the PLC has, up to 64 each; without it, 16 each. A connector's `byte` must be inside it.

`plc.analog` places the analogue channels in the image and says how their values appear there. Inside the simulation an analogue value is what it is on the wire, such as 7.3 V; the PLC reads and writes it as the INT its range stands for, the way analogue modules give them, or as a REAL holding the value itself:

```json
"plc": {
  "image": { "inputs": 24, "outputs": 16 },
  "analog": {
    "inputs": [
      { "pin": "UE1", "address": "%IW8", "range": [0, 10], "unit": "V", "raw": [0, 27648] },
      { "pin": "UE2", "address": "%ID12", "type": "REAL", "range": [0, 100], "unit": "mm" }
    ],
    "outputs": [ { "pin": "UA1", "address": "%QW8" } ]
  }
}
```

| Field | Meaning |
|---|---|
| `pin` | The PLC's pin, `UE<n>` for an input and `UA<n>` for an output |
| `address` | Its place: a word, as `%IW8`, for an `INT`; a double word, as `%ID12`, for a `REAL` |
| `type` | `INT` (the raw value, without it) or `REAL` (the value itself) |
| `range`, `unit` | The value on the wire, `[0, 10]` V without them |
| `raw` | The INT that stands for `range`, `[0, 27648]` without it, held to the range of an INT |

Without `plc.analog`, channel n is the INT at the word at byte 8 + 2n, after 8 bytes of bits: `UE1` to `UE4` at `%IW8` to `%IW14`, `UA1` to `UA4` at `%QW8` to `%QW14`, with 0 to 27648 for 0 to 10 V. A module that places some channels must place all its connectors have. A channel may not share a byte with a connector's bits or another channel.

How a PLC shows its addresses on screen is up to its module. `plc.addressFormat` has a template for input and output bits, with `{byte}` and `{bit}` (or `{bit:2}`), and for input and output words, with `{byte}` and, for analogue inputs, `{channel}`:

```json
"plc": { "addressFormat": { "input": "E{byte}.{bit}", "output": "A{byte}.{bit}", "inputWord": "EW{byte}" } }
```

Without it, the templates of `addresses` show them, as `IB0Bit03` and `AI0`. An address without a template is shown as it is.

## Connections

A connection says how a PLC of a station is coupled to a controller outside the app, such as a CODESYS PLC or a FluidSIM project: which driver talks to it, how it is reached, and which item of it each signal of the station is exchanged with. The station itself never changes for it. The PLC panel offers the app's own connections, one to CODESYS, one for each `plc.maps` entry of the station and an empty one to any OPC UA server, and the files in `connections/` of the user's folder and of the collections. Its connection editor writes such files into the user's folder.

```json
{
  "id": "fluidsim",
  "name": "FluidSIM project",
  "station": "pallet-loading",
  "driver": "opcua",
  "vars": {
    "type": "object",
    "required": ["host", "port", "project"],
    "properties": {
      "host": { "type": "string", "title": "Computer running FluidSIM", "default": "127.0.0.1" },
      "port": { "type": "integer", "title": "Port of its OPC UA server", "minimum": 1, "maximum": 65535, "default": 4840 },
      "project": { "type": "string", "title": "FluidSIM project name", "default": "Task 2", "minLength": 1 }
    }
  },
  "settings": { "endpoint": "opc.tcp://{host}:{port}" },
  "namespaces": { "fs": "<the namespace URI the server gives for its components>" },
  "bindings": [
    { "signal": "B1", "item": { "nodeId": "nsu={fs};s={project}.B1.state_man" } },
    { "address": "%QX0.0", "item": { "nodeId": "nsu={fs};s={project}.MB3.state" } }
  ],
  "generate": [
    { "signals": "outputs", "item": { "nodeId": "nsu={fs};s={project}.{symbol}.state" } }
  ]
}
```

| Field | Meaning |
|---|---|
| `id`, `name`, `description` | How the panel lists it. An id already taken by a connection listed earlier is left out |
| `station`, `plc` | The station and PLC module instance it fits. Without `station` it fits every station; without `plc`, the first PLC |
| `driver` | `opcua` |
| `vars` | A [JSON Schema](https://json-schema.org/) (draft 2020-12) of an object: one property per value someone fills in, from which the panel makes its form. `title`, `description`, `default`, `examples`, `required`, `minimum`, `maximum`, `multipleOf`, `pattern`, `enum` and `oneOf` with `const` and `title` shape the form; `writeOnly` marks a secret, which the app does not keep. A schema may not refer to another file |
| `settings` | The driver's. Texts may use `{var}` |
| `namespaces` | Names for texts, such as a namespace URI, used as `{name}` like a var |
| `bindings` | One signal each, by its symbol (`signal`) or its `address`, and the driver's `item` for it. `invert` exchanges a bit the other way round; `scale` (`from` and `to`, two numbers each) converts an analogue value. An input may go to more than one item; an output may come from only one |
| `generate` | Makes a binding for each signal of a kind, `inputs` or `outputs` (bits), `analogInputs`, `analogOutputs`, `analog` (both) or `all`, from an item whose texts may also use `{address}`, `{shown}`, `{symbol}`, `{byte}`, `{bit}`, `{bit:2}` and `{channel}` |

Direction comes from the signal: the app writes a PLC input to its item and reads a PLC output from its item. An explicit binding takes the place of a generated one that writes the same: for an input, the same item; for an output, the same signal. In the example, every output comes from the node named like its symbol, except `%QX0.0`, which comes from `MB3`. A signal without a binding is not exchanged, which the panel lists.

A symbol binding (`"signal": "B1"`) follows the signal when an exercise moves it to another pin, as a component of a co-simulation should. An address binding (`"address": "%IX0.3"`) stays with the address, whatever is wired there, as a real PLC's input would.

### opcua

The OPC UA driver is a client of any server. Its settings:

| Setting | Meaning |
|---|---|
| `endpoint` | The server, as `opc.tcp://host:port` |
| `security` | `policy` and `mode`, both `None` so far |
| `auth` | `type`: `anonymous` so far |
| `outputs` | `exchange`: `poll`; `interval` in milliseconds, 5 to 1000, 10 without it |

An item is a node, by `nodeId` or by browse `path`:

- `nodeId` as the server writes it, `ns=<index>;s=<name>`, or with the namespace's URI, `nsu=<URI>;s=<name>`, which holds even if the server puts the namespace at another index. `i=`, `g=` and `b=` identifiers work too.
- `path`, such as `/2:Projects/2:Task 2/2:B1`: browse names with the index of their namespace, followed from the Objects folder, or from the node `from`. With `namespace`, a namespace URI, names without an index are of that namespace, whatever index the server gives it: `{ "path": "/Projects/Task 2/PLC/M1", "namespace": "{fs}" }`. A path finds a node whose NodeId the server numbers as it pleases, such as the outputs of a FluidSIM GRAFCET PLC.

When it connects, the driver checks every node once: that it exists, can be written for an input and read for an output, and holds a Boolean or a number. A node that fails is left out and listed, and the others are exchanged. Values are written and read as the node's own type: a bit as `true` and `false` or `1` and `0`; an analogue value as the PLC has it in its image into an integer, such as 0 to 27648, and as it is on the wire into a float, such as volts, unless the binding scales it. Inputs are written when they change, outputs read on every interval. When the connection is lost, the PLC's outputs go off, and the driver connects again.

## Roles

The builder knows no module by its id. What it does beyond screwing modules on goes by their `role`:

| `role` | Is | The builder |
|---|---|---|
| `station` | What a station stands on, such as the reference collection's base plate | Puts it on the floor, beside others, with the `floor` size `width` and `depth`. One added beside it starts with the settings in `floor.beside`. Everything screwed onto it belongs to that station |
| `plc` | The station's PLC | Allows one per stand. Cables each console, interface and terminal with a socket of a kind the PLC has to a free connector of it |
| `console` | The operator panel | Cables it to the PLC |
| `interface` | Passes the PLC's signals on to terminals | Cables the connectors that fit the PLC to it, and the others to the nearest free socket of the same type |
| `terminal` | Where devices are wired, such as an I/O block | Wires an accessory put onto a module to the connectors of its terminals that are not `internal` |

A new station starts with a module of the `station` role, preferably of the collection it is started from, and a PLC and a console of that collection, each where it first fits on the stand without overlapping another.

## Mounting

A station is put together the way a real one is screwed together. A module offers `sites`, such as the rails of the reference collection's base plate (`floor-rail`), and other modules have `mounts` that are screwed onto sites of the same type.

A site runs from `from` along `dir` (default x), and a position on it counts in mm within `range`. With `repeat` it is several parallel sites, its slots. A module on a site stands on the site's frame: x along `dir`, z towards `up` (default up) and y into the face it is screwed to, turned further about z by the site's `turn`. A site may slope or run upright. The sites of a contained module are named by its id, as `conveyor/frame.side`.

A module's `mounts` say how it is screwed down: `at` is its point that sits on the site, `turn` how it faces there, `rotateX` and `rotateY` stand it up first, and each of its `screws` must find a site of the same type on the same module, within 0.1 mm. A mount with `when` exists only while that holds. A mount with `span` names a parameter that is set to how far the site lies from the nearest other site of the same type alongside it, as the two heads of a through-beam sensor span a belt. A module can have several mounts, and the first whose screws all fit is used. A mount with `runs` fits only sites that run `level` or `upright`. With `head`, each screw is drawn as the `plate-screw` part.

A station places an instance with `place`: `on` names a module placed before it and one of its sites, `slot` picks a repeat, `at` is the position along it and `turn` turns it further in steps of 90°.

```json
{ "id": "base", "module": "base-plate" },
{ "id": "conveyor", "module": "pallet-conveyor", "place": { "on": "base.rails", "slot": 14, "at": -2400 } }
```

A module's own parts are changed with `parts`, by their ids: `place` puts one elsewhere on the sites of the module, `params` change its settings, and `removed` leaves out an `optional` one. A station can move what a module's own wires join at one of its pins onto another with `rewire`, by references inside the module, such as `"rewire": { "io.T.Q0": "io.T.Q3" }`.

A module with `cutsRails` cuts a gap into every rail it stands in, 3 mm wider than it on each side, so that workpieces can pass. What is left of the rail are pieces with end caps, and a site with `rail` follows that rail, so nothing can be screwed on where the gap is. Only a contained rail with `cut` may be cut.

Moving a module moves everything screwed to it. A transport with `handsOver` has to lead onto another module's transport.

## Workpieces

A workpiece set gives the shape of its workpieces and their `kinds`. Round workpieces share the set's `diameter`, `height` and `points` (a three.js lathe profile); a kind can have its own `height` and `points`. A kind with a `size` is a rectangular block of that length and width, such as a pallet or a carton, whose length points along its transport, and a `model` draws it from nodes like a module's.

A kind's `metal`, `reflectivity` and `transparency` are what sensors see. Its `family` tells apart what is not interchangeable, such as pallets and cartons. A kind with `onto` joins a workpiece of that family that it is set down on by a gripper or drops onto, and sinks `depth` mm into it; the one below then carries it. With `free` it may go anywhere on top and stay where it was put, also onto what was put there before, as cartons stacked on a pallet. Without, it only goes onto the middle of the other one's top, as a lid.

The set lists its `marks`, what a `process` device can leave on a workpiece: an `id`, a `label`, and how the 3D view shows it, as a filling of `material` that rises `fill` mm per unit, or a `hole` of that diameter. A `sensor` with `has` sees only workpieces with that mark, or with a joined part of that family or kind.

## station.json

A station file says its `format`, which is 5; the app reads no other. Modules and parts are format 2.

| Section | Defines |
|---|---|
| `parameters` | Settings users can change while it runs. Module `params` can use them in expressions |
| `modules` | Instances: `id`, `module`, `place` or a position `at` with `rotateX`, `rotateZ`, and `params`, `texts`, `parts`. `selectAs` makes clicks select another instance. `mountedOn` puts it on a moving node of the module around it |
| `links` | Only where modules do not meet by themselves: `continue` at the end, `branch` at a distance while `when` holds, and `shareDrive` |
| `cables` | Join two connectors of the same type, pin by pin |
| `wires` | Join two pins |
| `actions` | Station-wide functions, in the same form as a module's |
| `docks` | Station-wide widgets, at `bottom` or `side` |
| `plc.signals` | Names for the signals of the PLC image, by address, such as `%IX0.0`. A signal the station does not name takes the `symbol` of a pin on its way, or else the name `instance.pin` |
| `plc.maps` | Other addresses for a PLC program written for another wiring: each entry connects a `signal` to a `variable` of the list, optionally inverted. The PLC panel offers each as a CODESYS connection |
| `plcs` | The PLCs besides the first one, of stations side by side, each with its `module` and `signals`. Every PLC has an image of its own, its own program or OPC UA connection, and its own I/O test |

Signals are referenced as `instance.device.state` or `instance.connector.pin`. Everything joined by cables, wires and bridges is one electrical net with one value, so a signal travels from a device through its terminals to the PLC as on a real station.

A PLC bit the station does not name gets the name of the module pin it reaches, as `instance.pin`, with the module's name and the pin's function as its description.

Loading checks every reference, address, node and material. It refuses a signal with two sources, a device input wired to something other than the PLC, and links between transports more than 8 mm apart. A mistake gives an error naming the file instead of a station that misbehaves.

Some things load but do not fit together. The app checks each station when it starts and shows what it finds under Problems, in the panel below the 3D view: a module that lacks what it `needs`, a `handsOver` transport leading nowhere, a pusher pushing to where no transport is, a gripper coming down at a place without one, an actuator that no PLC output reaches, a sensor that reaches no PLC input, and a part that runs into a fixed part of the station with only one PLC output on.

## exercise.json

An exercise is a task on a station, and the check of a run of it, line by line as a grading sheet does. It has no program: the station runs with whatever PLC is connected. **Check** resets the station and goes through the steps on the wall clock, stopping at the first line that fails.

| Field | Meaning |
|---|---|
| `station` | The station it is for |
| `rules` | Where each workpiece belongs, by `kind`, `family` and `upsideDown`: `endsOn` a transport, or `waitsAt` one, `at` mm along it or at its end, to be taken away by hand. `place` names that place for `{place}` in a line |
| `conditions` | Named expressions, used by name in steps and other conditions |
| `steps` | What is done and checked, in order |
| `changes` | How the exercise sets the station up: `params` of its modules or their parts, `pins` swapped on a terminal, and `docks` that replace the station's |

| Step | Does |
|---|---|
| `place` + `kind` | Runs a placeWorkpiece action, such as `"conveyor.put"`; `upsideDown` turns the workpiece over |
| `press`, or `set` + `on` | Presses a button or key for a moment, or turns it |
| `wait` | Lets seconds pass |
| `take` | Takes away by hand what waits where its rule says |
| `until` + `within` | A line: the expression holds within that many seconds |
| `holds` + `for` | A line: the expression keeps holding for that many seconds |
| `repeat` + `steps` | The steps once per kind listed, with `{kind}` and `{place}` filled in |

Expressions in steps know the PLC's signals as `out.M1` and `in.B1`, states such as `gantry.x.position`, `step` (the PLC's step, or -1), `lost`, the conditions, and the functions `count("conveyor.surface")` or `count("conveyor.surface", "pallet")`, `at("conveyor.surface", 53.2, 0.5)`, `hits` (collisions so far), `loads("conveyor.surface")` and `rules()`, which holds when every workpiece placed so far is where its rule wants it. `exercises/` of the reference collection has an example.
