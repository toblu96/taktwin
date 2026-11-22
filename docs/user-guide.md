# User guide

Taktwin starts on **Projects**, which lists your projects. Beside it, **Library** browses the collections and previews any module on its own, without a PLC. An open project has three views in the bar along the left edge: **Simulation** runs its station with its PLC, **Wiring** shows how the station's signals reach the PLC, and **Exercise** says what the exercise asks for, with its check. The bar also goes back to your projects. The gear at its foot opens **Settings**: **General** for your folder, **Collections**, **Updates** and **About**. A second click on the gear, or Esc, goes back.

In **Simulation**, the left side has the exercise or the station with what it is about, then the station's PLCs, each with a lamp for how it is connected, what it runs and its step, **Program** to write its program and **I/O test** to switch its outputs by hand. Below them are the station's own controls, such as putting workpieces in. The right side shows the station's inputs and outputs, live, with the I/O test, which switches the outputs by hand. Clicking a module in the 3D view shows it there instead, with what can be done with it while it runs: its controls, and its I/O connectors with their live pins; clicking beside it goes back to the inputs and outputs. A click on a workpiece picks it, with a box around it, a double click frames it as one on a module does, and **Take it off the station**, Delete or Backspace lifts it away, with what lies on it; while a check runs, workpieces stay where they are. **Wiring** shows the station's PLC, console, interfaces and I/O terminals as a diagram with the cables between them, and the one clicked on the right: a PLC with how it is connected, a controller outside the app or the soft PLC, and its signals; an I/O terminal with its pins, what each carries and the name the PLC gives it. The wiring itself is only looked at there, but a PLC can be connected. The panel below the 3D view has **Events**: what ran into what and which workpieces fell off, with the simulated time. The panel is the same on every page that has one, open or closed and as high: Events while the station runs, Problems while it is edited. ⌘J or Ctrl+J, or **Panel** in the status bar, shows and hides it. Every edge between these parts can be dragged, and the app keeps the sizes.

The top bar shows the open project, the simulated time and speed, pause and restart. The status bar along the bottom shows the app's version, with a dot when an update is ready; a click opens **Updates**. While a station runs it shows its PLC and connection, an I/O test with **End**, and the PLC's step; a click on the PLC shows it. On the right, **✓ Saved** shows the project's folder. What needs acknowledging, such as a collision or a workpiece that fell off, shows over the 3D view.

## Collections and your own stations

Stations, modules, workpieces and exercises are JSON files, described by the JSON Schemas in [`schema/`](../schema) and by [the collection format](../catalog/README.md). A collection is a folder of them, or a `.taktwin` zip of that folder, which **Settings › Collections** imports.

The app also reads stations from a `Taktwin` folder in your documents folder, wherever the system keeps that, or from the folder in `TAKTWIN_HOME`. **Settings** shows the exact path, opens it in the file manager and has **Reload**, which reads your files again. Put each station in `stations/<id>/station.json` there. They show next to the built-in ones when you make a project, and in the Library. A station that does not load is listed in red with the reason; one that loads but does not fit together shows its problems under **Problems**, in the panel below the middle, while it is edited.

## The builder

**Expert mode**, for teachers, is switched on in **Settings › General › Experimental**. It adds a switch to the top bar of an open project between running its views and editing them: edited, **Simulation** builds the station and **Wiring** rewires it. It shows more, but locks nothing away from students. Building puts a station together without editing files. A project made from an empty station opens there. A project on a collection's station builds on a copy of it that the project keeps, after asking, and no longer follows the collection's updates. The station of an exercise stays as the exercise has it. Then:

- Click a module on the left to pick it: every place it fits shows as a line, and a see-through copy follows the pointer, on the 5 mm grid or in whole mm with Shift. It is green where it fits and red where it does not, with the reason. A click puts it down, R turns it, Esc puts it back.
- A selected module shows arrows to move it along its rail or to the next one; what is screwed onto it goes along. **Fit** moves it to where the station has the fewest problems. Backspace removes it, with everything screwed onto it.
- Parts of a module, such as a conveyor's sensors, are selected and moved like any module but stay on it. Backspace takes one off, and **Put back** returns it.
- While building, blue lines show where workpieces travel and diamonds the special positions, such as where a gripper comes down; the selected module's are listed under "Lines up with".
- The right side has the settings of the selected module: **Build settings** first, **Physics** below.
- The station checks have their own tab and update with every change.
- A selected stand shows a **+** on each side where there is room, to put another stand beside it, so that a station grows into a line, each stand with its own PLC.
- **Wiring**, edited, changes the wiring: click a connector in the diagram, then another of its kind, to move its cable; an interface's or console's connector can be unplugged on the right. A terminal's pins swap what they carry, dragged onto another pin of their kind or clicked one after the other, and the name the PLC knows a signal by changes in its field. What does not fit together shows under **Problems** below the diagram.
- The project keeps every change, as `station.json` in its folder. Switching editing off runs the station as it is built; when it was changed, its PLCs are disconnected, as on a restart.

## Projects

Everything you do in the simulation belongs to a project: the soft PLC's programs, how each PLC is connected, and the station's settings. A project is saved as it changes, so there is nothing to save.

The app starts on **Projects**: a card for each of your projects, the latest first, with a picture of its station as you left it and how its last exercise check went. Below them is the station each collection comes with, as an **Example**, until its collection is hidden in **Settings › Collections**. Clicking a project opens it; clicking an example makes a project of your own on it. With projects on stations and on exercises, tabs show either kind, and the filter field (⌘F / Ctrl+F) finds a project by its name, station or exercise.

- **New project** makes one from a station or from an exercise of your collections, or with an empty station to build. It is named after the station or exercise unless you give it a name, with a number where that name is taken. A project on an exercise has the station set up as the exercise says, and its check under **Exercise** in the panel.
- A card's **⋯** menu renames, duplicates, shows the project's folder, exports or deletes it.
- **Export…** writes a project into a `.taktwin-project` file, to hand it in or move it to another computer. **Open file…** opens such a file as a project of its own, as does dropping it onto the page; on a Mac also **File › Import Project File…** (⌘O).
- **‹ Projects** in the top bar closes the project; on a Mac also **File › Close Project** (⌘W). Closing disconnects every PLC and stops its soft PLCs, so download the program again after opening the project.

Only a Mac has a menu bar, as every Mac app does: besides the **File** menu it has **Taktwin › Settings…** (⌘,). On Windows everything is on the app's pages.

Opening a project restores its PLCs' connections without connecting them: press **Connect** in the PLC's panel. A project whose station or exercise is in none of your collections, for example because the collection is hidden, is listed with the reason and does not open. Projects are kept in the `projects` folder of your folder, one folder each, with `project.json`, the programs in `programs/<plc>/`, `thumbnail.jpg`, `station.json` once the project has a station of its own, and `exercise.json` and `instructions.md` for an exercise handed out. A project file holds only these, and an import refuses anything else.

### Exercises as one file

A teacher hands an exercise out as one file: in the card menu of a project on an exercise, **Hand out as exercise…** writes a `.taktwin-project` file with the station and the exercise as copies, the station's settings, the instructions, and the collections students may not have, those you imported. It asks whether this project's programs go along as a start; by default they do not, so that a solution does not go out by mistake. How the PLCs are connected and how checks went stay behind. A station that uses modules from your own folder cannot be handed out: put them into a collection first.

The instructions are Markdown: `instructions.md` in the project's folder, or beside the exercise in its collection (`exercises/<id>/instructions.md`). They show under the exercise, beside its **Check**.

Students open the file by double-clicking it, dropping it onto **Projects**, or with **Open file…**. When it brings a collection they do not have, or a newer one, the app asks to install it first; one it needs but does not bring must be imported in **Settings › Collections**. The project keeps the exercise's station as it is. Opening the file again asks whether to go on with the project made from it before or to start again.

To hand in, students use **Export…** on their project. A teacher opening that file gets a project named after the file, such as `Anna.taktwin-project`, and presses **Check**.

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

A PLC's program lives in the project you work in, under `programs/<plc>/`, one `.st` file for each POU, with `project.json` for the task. `Bits.st` is written from the station's I/O table each time the project opens. A new project starts from a template with a step chain; a collection can bring an example program for its station.
