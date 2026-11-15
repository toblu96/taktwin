# User guide

Taktwin has four pages. **Simulation** runs a station with its PLC. **Builder** puts stations together. **Library** browses the collections and previews any module on its own, without a PLC. **Settings** manages collections, your folder and updates.

## Collections and your own stations

Stations, modules, workpieces and exercises are JSON files, described by the JSON Schemas in [`schema/`](../schema) and by [the collection format](../catalog/README.md). A collection is a folder of them, or a `.taktwin` zip of that folder, which **Settings** imports.

The app also reads stations from a `Taktwin` folder in your documents folder, wherever the system keeps that, or from the folder in `TAKTWIN_HOME`. **Settings** shows the exact path, opens it in the file manager and has **Reload**, which reads your files again. Put each station in `stations/<id>/station.json` there. They show in the station list next to the built-in ones, and in the Library. A station that does not load is listed in red with the reason; one that loads but does not fit together shows its problems under **Station checks**.

## The builder

The **Builder** page puts a station together without editing files. Start from an empty stand or a copy of any station, then:

- Click a module on the left to pick it: every place it fits shows as a line, and a see-through copy follows the pointer, on the 5 mm grid or in whole mm with Shift. It is green where it fits and red where it does not, with the reason. A click puts it down, R turns it, Esc puts it back.
- A selected module shows arrows to move it along its rail or to the next one; what is screwed onto it goes along. **Fit** moves it to where the station has the fewest problems. Backspace removes it, with everything screwed onto it.
- Parts of a module, such as a conveyor's sensors, are selected and moved like any module but stay on it. Backspace takes one off, and **Put back** returns it.
- While building, blue lines show where workpieces travel and diamonds the special positions, such as where a gripper comes down; the selected module's are listed under "Lines up with".
- The right side has the settings of the selected module: **Build settings** first, **Physics** below.
- The station checks have their own tab and update with every change.
- A selected stand shows a **+** on each side where there is room, to put another stand beside it, so that a station grows into a line, each stand with its own PLC.
- **Wiring** shows the PLCs, consoles, interfaces and I/O terminals with the cables between them. Click a connector, then another of its kind, to move its cable; click a pin to move what it carries. Each pin shows the PLC address it reaches, as the PLC module shows them, and the name the PLC knows it by, which can be changed.
- **Save** writes it to your folder, and **Try in Simulation** opens it there.

## Running a station

### Testing I/O by hand

The I/O test cuts the program off from the I/O for the whole station. Start it in the PLC's panel or on any module with PLC outputs. Outputs can then be switched by hand, and analogue outputs set to a value, while inputs show live. Ending the test switches all outputs off.

### Connecting a PLC

Choose how a PLC is run on the PLC in the Simulation page, or from the button in the top bar. Each PLC of a station has its own connection.

- **Soft PLC** runs the program written in the app's editor. It has an OPC UA server of its own on `opc.tcp://127.0.0.1:4841` (or the next free port), with a node for each signal named by its address, such as `cabinet.%IX0.3`, so any OPC UA client can watch it.
- **External** connects to a controller outside the app through a connection. Pick one and fill in its form:
  - **CODESYS** expects the variable list named as the station's PLC module says in its `addresses`: for the reference collection's cabinet, the list `Bits` with `IB0Bit00` … for inputs and `QB0Bit00` … for outputs. In CODESYS, add a Symbol Configuration with OPC UA support to the application, tick the list, and download. CODESYS names each variable on its OPC UA server as `ns=4;s=|var|<PLC name>.<application>.<list>.<variable>`, such as `ns=4;s=|var|CODESYS Control Win V3 x64.Application.Bits.IB0Bit00`, and the form asks for those parts: the PLC name is that of the runtime unless its configuration sets another, and the application and the list are named as in the CODESYS project's device tree. Any OPC UA client that browses the PLC's server shows the NodeIds, to check them.
    **Find on the server** looks for the list on the server the form names and fills in the PLC name, application and namespace.
  - **OPC UA server** is any server, with nothing bound yet: fill in its address and press **Edit bindings**.
  - **A connection file** in the `connections` folder of your folder, or of a collection, binds the station's signals to any server's nodes, such as those of a FluidSIM project. [catalog/README.md](../catalog/README.md#connections) describes the format.

**Edit bindings** opens the connection editor over the 3D view. It lists the PLC's signals with the node each is exchanged with, and on the right what the server offers. Select a signal and pick its node, and the next signal is selected; or type a NodeId. A binding follows the signal's symbol when an exercise moves it to another pin, or stays with its address. **Templates** bind every signal of a kind at once from a pattern, such as `nsu=urn:x;s=Bits.{shown}`; what is bound above takes their place. **JSON** shows the whole connection file, for its vars, settings and namespaces. Saving puts the connection into the `connections` folder of your folder: one of the app or of a collection is saved as a copy, which then takes its place there.

The station is the client, and the controller the server, with no security and an anonymous login so far. The panel shows how many signals are exchanged, which of them the controller does not have or does not allow, and which signals the connection leaves out. A PLC over a connection runs on the wall clock, so the simulation stays at 1× with it.

### Updating

Taktwin looks for a new version when it starts; **Settings → Updates** shows it, installs it and restarts. Updates come from the GitHub releases and are checked against their `SHA256SUMS`. Only a release build replaces itself: one run from the sources only checks.

### Programming the soft PLC

Each PLC can also run a Structured Text program written in the app. In the PLC's panel, **Edit program** opens the editor over the 3D view, and **Open in window** opens it beside the simulation.

- **Ctrl+Space** (or Option+Space) lists what fits at the cursor: variables, the I/O list with each signal's description, POUs, types, standard functions and keywords, and members after a dot.
- **Build** (F11) lists mistakes with their file, line and column.
- **Download** (Alt+F8) loads the program into the soft PLC and connects the simulation to it over OPC UA, the same way it connects to a real PLC.
- **Run** (F5), **Stop** (Shift+F8) and **Reset** control the program.
- **Online**, the code shows each variable's value in a box after its name. Click a value to prepare a new one, then F7 forces it, Ctrl+F7 writes it once, and Alt+F7 releases all forces.
- **Watch** lists every variable live, and can prepare, write and force values in the same way.

A project lives in your folder under `programs/<station>/<plc>/`, one `.st` file for each POU, with `project.json` for the task. `Bits.st` is written from the station's I/O table each time the project opens. A new project starts from a template with a step chain; a collection can bring an example program for its station.
