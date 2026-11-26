# User guide

Taktwin starts on **Projects**, which lists your projects. Beside it, **Library** browses the collections and previews any module on its own, without a PLC. An open project has three views in the bar along the left edge: **Simulation** runs its station with its PLC, **Wiring** shows how the station's signals reach the PLC, and **Exercise** says what the exercise asks for, with its check. The bar also goes back to your projects. The gear at its foot opens **Settings**: **General** for your folder, **Collections**, **Updates** and **About**. A second click on the gear, or Esc, goes back.

In **Simulation**, the left side has the exercise or the station with what it is about, then the station's PLCs, each with a lamp for how it is connected, what it runs and its step, and **Program** to write its program. Below them are the station's own controls, such as putting workpieces in. The right side shows the station's inputs and outputs, live. Clicking a module in the 3D view shows it there instead, with what can be done with it while it runs: its controls, and its I/O connectors with their live pins; clicking beside it goes back to the inputs and outputs. A click on a workpiece picks it, with a box around it, a double click frames it as one on a module does, and **Take it off the station**, Delete or Backspace lifts it away, with what lies on it; while a check runs, workpieces stay where they are. **Wiring** shows the station's PLC, console, interfaces and I/O terminals as a diagram with the cables between them, and the one clicked on the right: a PLC with how it is connected, a controller outside the app or the soft PLC, and its signals; an I/O terminal with its pins, what each carries and the name the PLC gives it. The wiring itself is only looked at there, but a PLC can be connected. The panel below the 3D view has **Events**: what ran into what and which workpieces fell off, with the simulated time. The panel is the same on every page that has one, open or closed and as high: Events while the station runs, Problems while it is edited. ⌘J or Ctrl+J, or **Panel** in the status bar, shows and hides it. Every edge between these parts can be dragged, and the app keeps the sizes.

The top bar shows the open project, the simulated time and speed, pause, restart and the **I/O test**. The status bar along the bottom shows the app's version, with a dot when an update is ready; a click opens **Updates**. On the right, **✓ Saved** shows the project's folder. What needs acknowledging, such as a collision or a workpiece that fell off, shows over the 3D view.

## Collections and your own stations

Stations, modules, workpieces and exercises are JSON files, described by the JSON Schemas in [`schema/`](../schema) and by [the collection format](../catalog/README.md). A collection is a folder of them, or a `.taktwin` zip of that folder, which **Settings › Collections** imports.

The app also reads stations from a `Taktwin` folder in your documents folder, wherever the system keeps that, or from the folder in `TAKTWIN_HOME`. **Settings** shows the exact path, opens it in the file manager and has **Reload**, which reads your files again. Put each station in `stations/<id>/station.json` there. They show next to the built-in ones when you make a project, and in the Library. A station that does not load is listed in red with the reason; one that loads but does not fit together shows its problems under **Problems**, in the panel below the middle, while it is edited.

## The builder

**Expert mode**, for teachers, is switched on in **Settings › General › Experimental**. It adds a switch to the top bar of an open project between running its views and editing them: edited, **Simulation** builds the station and **Wiring** rewires it. It shows more, but locks nothing away from students. Building puts a station together without editing files. A project made from an empty station opens there. Every project builds on its own copy of its station. The station of an exercise stays as the exercise has it. Then:

- The left side has the station's name and what it is about, then **In the station**, its modules as a tree, each stand with what stands on it, and **Add**, what can be put in, from one collection at a time, with a search.
- Under **Add**, click a module to pick it: every place it fits shows as a line, and a see-through copy follows the pointer, on the 5 mm grid or in whole mm with Shift. It is green where it fits and red where it does not, with the reason. A click puts it down, R turns it, Esc puts it back.
- A selected module shows arrows to move it along its rail or to the next one; what is screwed onto it goes along. **Fit** moves it to where the station has the fewest problems. Backspace removes it, with everything screwed onto it.
- Parts of a module, such as a conveyor's sensors, are selected and moved like any module but stay on it. Backspace takes one off, and **Put back** returns it.
- While building, blue lines show where workpieces travel and diamonds the special positions, such as where a gripper comes down; the selected module's are listed under "Lines up with".
- The right side has the module being added, with whether it fits where the pointer is, or the selected module: its place, with the slot, how far along, nudges and how it is turned, then **Build settings**, the parts taken off it, and **Physics** folded away. A setting changed from how the module comes has a button to put it back.
- What does not fit together shows under **Problems** below the 3D view, and updates with every change.
- A selected stand shows a **+** on each side where there is room, to put another stand beside it, so that a station grows into a line, each stand with its own PLC.
- **Wiring**, edited, changes the wiring: click a connector in the diagram, then another of its kind, to move its cable; an interface's or console's connector can be unplugged on the right. A terminal's pins swap what they carry, dragged onto another pin of their kind or clicked one after the other, and the name the PLC knows a signal by changes in its field. What does not fit together shows under **Problems** below the diagram.
- The project keeps every change, as `station.json` in its folder. Switching editing off runs the station as it is built; when it was changed, its PLCs are disconnected, as on a restart.

## Projects

Everything you do in the simulation belongs to a project: the soft PLC's programs, how each PLC is connected, and the station's settings. A project is one `.taktwin-project` file, wherever it is: in your folder, in the downloads folder or on a shared drive. The app saves every change into that file shortly after it is made, so there is nothing to save, and the file can be moved, copied or sent as it is.

A project carries what it is built from: a copy of its station, and of its exercise, and every collection they come from, the app's own among them, in the version it was made with. It opens from them, so it opens the same on another computer, after an update of the app, and when those collections are hidden or not there. When a newer version of one of them is installed, the side bar of **Simulation** offers it under the station's description; the project takes it only on **Update…**. Its station then follows the new version, unless the project changed it, and stays as it was if it does not load with it.

The app starts on **Projects**: a card for each project file in the `projects` folder of your folder and for each one opened from elsewhere, the latest first, with a picture of its station as you left it and how its last exercise check went. A file from elsewhere says which folder it is in. Below them is the station each collection comes with, as an **Example**, until its collection is hidden in **Settings › Collections**. Clicking a project opens it; clicking an example makes a project of your own on it. With projects on stations and on exercises, tabs show either kind, and the filter field (⌘F / Ctrl+F) finds a project by its name, station or exercise.

- **New project** makes one from a station or from an exercise of your collections, or with an empty station to build, as a file in the `projects` folder of your folder. It is named after the station or exercise unless you give it a name, with a number where that name is taken. A project on an exercise has the station set up as the exercise says, and its check in the **Exercise** view.
- **Open file…** opens a project file where it is, as does dropping it onto the page or double-clicking it; on a Mac also **File › Import Project File…** (⌘O). Nothing is copied: the changes go into that file.
- A card's **⋯** menu renames the file, saves a copy of it elsewhere, hands it out, shows it in its folder, and deletes it, for good. A file opened from elsewhere is not deleted but taken off the list. One that was moved or deleted since is listed as missing, with **Locate…** to find it again.
- **‹ Projects** in the top bar closes the project; on a Mac also **File › Close Project** (⌘W). Closing writes the file, disconnects every PLC and stops its soft PLCs, so download the program again after opening the project.

The status bar's last item says how the file stands: **Saved**, **Saving** while a change is about to be written, or **Not saved** when it could not be, with the reason when you point at it. A click on it shows the file in its folder.

- **A file that cannot be written,** such as one on a read-only share, opens as **Read only**, with **Save a copy…** beside its name: the project goes on in the copy you save.
- **A file changed from elsewhere** while it is open, such as by a colleague on a shared drive, is not overwritten. The app asks whether to keep your project and write it over the file, open the file as it is now, or save yours as a copy.
- **Changes that could not be written** when the project is closed are saved as a copy or dropped, after asking.
- **After a crash,** opening the file again offers the changes it does not have yet.

Only a Mac has a menu bar, as every Mac app does: besides the **File** menu it has **Taktwin › Settings…** (⌘,). On Windows everything is on the app's pages.

Opening a project restores its PLCs' connections without connecting them: select the PLC in **Wiring** and press **Connect**. A project whose station cannot be found, neither in it nor in your collections, is listed with the reason and does not open. A project file is a zip with `project.json`, its station as `station.json`, the collections it carries in `collections/`, the programs in `programs/<plc>/`, its original in `original/`, `thumbnail.jpg`, and for an exercise `exercise.json` and `instructions.md`. It holds only these, and the app refuses a file with anything else. Project files of format 1, from earlier versions, no longer open.

### The original, and putting back

Every project keeps its original: the state it started from, which it can go back to. It is set when the project is made, and for a project handed out, the state it was handed out in. The **⋯** beside the project's name in the top bar puts back what differs from it, one part at a time, so that undoing one mistake does not cost the rest of the work:

- how a PLC is connected, such as after a wrong address;
- the programs of a PLC;
- the station, as built and wired, except an exercise's, which cannot be changed;
- the station's settings;
- everything, also the versions of the collections the project carries.

In expert mode, **Keep this as the original** makes the project as it is now its original, such as a station built from an empty one once it is finished. Putting back then goes back to that state.

**Hand out…**, in the same menu and in the card's, writes a copy of the project for others: as it is now, with its station, how its PLCs are connected and the collections it carries, and with this state as the copy's original. It asks whether the programs go along; by default they do not, so that a solution does not go out by mistake. How checks went stays behind. A station that uses modules from your own folder cannot be handed out: put them into a collection first.

### Exercises as one file

A teacher hands an exercise out as one file with **Hand out…** on a project on the exercise: the copy has the station and the exercise, the instructions, how the PLCs are connected, so that students only change the address, and the collections the project carries. Students who get lost put back a part, or everything, as it was handed out.

The instructions are Markdown: `instructions.md` in the project file, copied from beside the exercise in its collection (`exercises/<id>/instructions.md`). They show under the exercise, beside its **Check**.

Students open the file by double-clicking it, dropping it onto **Projects**, or with **Open file…**, and work in it where it is. They need no collection installed for it: it opens from those it carries. The project keeps the exercise's station as it is.

To hand in, students send that file, renamed if the teacher asks, such as `Anna.taktwin-project`. The teacher opens it and presses **Check**.

The station of an exercise is locked: it cannot be built on, rewired or put back, as the check counts on it. A project file is a zip anyone can change, so an exercise keeps a sum of itself and its station as handed out: when they no longer match, **Exercise** says so, and putting everything back makes them as they were. This catches mistakes, not cheating. So in expert mode, a check of a project on an exercise your collections have runs with your exercise and its station when the project's differ from them, in memory only, with the PLC still connected, and the result says so. The hand-in's file stays as it was.

## Running a station

### Testing I/O by hand

The I/O test cuts the programs of all the station's PLCs off from the I/O. Start and end it with **I/O test** in the top bar; while it runs, the button is amber, an amber frame runs round the 3D view, and the bar in its corner has **All outputs off**, which switches every output off, and **End test**. Outputs are switched by their lamps, in the inputs and outputs on the right or on a module's pins, and analogue outputs set to a value, while inputs show live. An external PLC keeps running on its own, with its inputs frozen. Ending the test switches all outputs off and restarts the programs; connecting a PLC ends it too.

### Connecting a PLC

Choose how a PLC is run in **Wiring**, with the PLC selected: none, the soft PLC, or an external one. Each PLC of a station keeps one connection to an external controller in the project, also while it runs as the soft PLC or with none, so that switching back brings the same connection, address and bindings back.

- **Soft PLC** runs the program written in the app's editor. It has an OPC UA server of its own on `opc.tcp://127.0.0.1:4841` (or the next free port), with a node for each signal named by its address, such as `cabinet.%IX0.3`, so any OPC UA client can watch it.
- **External** connects to a controller outside the app through the project's connection. The first time, choose what kind of controller it is; the project then keeps the connection as its own, and **Change…** starts a new one of another kind in its place, after asking. Fill in its form, such as the server's address: the project keeps what it has as it is typed.
  - **CODESYS** expects the variable list named as the station's PLC module says in its `addresses`: for the reference collection's cabinet, the list `Bits` with `IB0Bit00` … for inputs and `QB0Bit00` … for outputs. In CODESYS, add a Symbol Configuration with OPC UA support to the application, tick the list, and download. CODESYS names each variable on its OPC UA server as `ns=4;s=|var|<PLC name>.<application>.<list>.<variable>`, such as `ns=4;s=|var|CODESYS Control Win V3 x64.Application.Bits.IB0Bit00`, and the form asks for those parts: the PLC name is that of the runtime unless its configuration sets another, and the application and the list are named as in the CODESYS project's device tree. Any OPC UA client that browses the PLC's server shows the NodeIds, to check them.
    **Find on the server** looks for the list on the server the form names and fills in the PLC name, application and namespace.
  - **OPC UA server** is any server, with nothing bound yet: fill in its address and press **Edit bindings**.
  - **FluidSIM: input and output ports** is for a FluidSIM circuit that is the controller, with input and output port components: each input of the PLC goes to the same bit of an input port, `%IX1.3` to `EB1.bit.3`, and each output comes from the same bit of an output port. Ports the circuit does not have are left out quietly for signals the station does not use.
  - A collection can bring other kinds, as connection files that name no station. [catalog/README.md](../catalog/README.md#connections) describes the format.

**Edit bindings**, in **Wiring**, puts the PLC's bindings where the diagram is: its signals with the node each is exchanged with, and in the side bar what the server offers, opened node by node. A signal in use without a node shows in red; one that nothing of the station reaches is not needed, though it can be bound. A bound signal's node is cleared with the button at the end of its row. A connection saves with what it binds so far. Bindings are kept in the project when **Save** is pressed, not with each change: until then the editor says they are not saved, and closing it or leaving the wiring asks whether to save them. The PLC exchanges with saved bindings once it is connected again. Select a signal and pick its node, and the next signal is selected; or type a NodeId. A binding follows the signal's symbol when an exercise moves it to another pin, or stays with its address. **Templates** bind every signal of a kind at once from a pattern, such as `nsu=urn:x;s=Bits.{shown}`; what is bound above takes their place. **JSON** shows the whole connection, for its vars, settings and namespaces. A mapping for one station and one controller, such as a FluidSIM circuit with its own component names, or a CODESYS program written for another wiring, is made this way in a project, and shared by sharing the project file.

The station is the client, and the controller the server, with no security and an anonymous login so far. The panel shows how many signals are exchanged, which of them the controller does not have or does not allow, and which signals the connection leaves out. A PLC over a connection runs on the wall clock, so the simulation stays at 1× with it.

### Updating

Taktwin looks for a new version when it starts; **Settings → Updates** shows it, installs it and restarts. Updates come from the GitHub releases and are checked against their `SHA256SUMS`. Only a release build replaces itself: one run from the sources only checks.

### Programming the soft PLC

Each PLC can also run a Structured Text program written in the app. In the PLC's card, **Program** opens the editor over the 3D view, and **Open in window** opens it beside the simulation. It opens as you left it in this project: the same files open, the same one in front, at the same place. That is remembered on this computer, not in the project file.

The editor works as VS Code does with text, and as CODESYS does with Structured Text:

- **Typing:** Enter after `IF … THEN`, `CASE … OF`, `FOR … DO`, `WHILE … DO`, `REPEAT`, `VAR` or `STRUCT` puts the block's end below, and the end of a block moves to where its start is as it is typed. Tab indents, Shift+Tab outdents.
- **Lines and cursors:** Alt+↑ and Alt+↓ move the line or the selected lines, Shift+Alt+↑ and ↓ copy them; Ctrl+/ (⌘/ on a Mac) comments them in or out. ⌘D or Ctrl+D selects the next place with the same text, for typing in several places at once.
- **Finding:** Ctrl+F (⌘F) searches and replaces in the file. Blocks and comments over several lines fold with the arrows beside the line numbers.
- **Ctrl+Space** (or Alt+Space) lists what fits at the cursor: variables, the I/O list with each signal's description, POUs, types, standard functions and keywords, and members after a dot. Pointing at a name shows its declaration.
- **Mistakes** are underlined as you type, and listed under **Messages** with their file, line and column; a click goes there. For a name that is not declared, pointing at it, or Ctrl+Shift+M (⌘⇧M) for the list, offers to declare it as `BOOL`, `INT` or `TIME`.
- **Format** (Shift+Alt+F) lays the file out as CODESYS's Format Document does: keywords in capitals, spaces around assignments and operators, and blocks indented. It leaves a file with a syntax error alone, and comments as they are.
- **Build** (F11) checks the project as saved. **Download** (Alt+F8) loads the program into the soft PLC and connects the simulation to it over OPC UA, the same way it connects to a real PLC.
- **Run** (F5), **Stop** (Shift+F8) and **Reset** control the program.
- **Online**, the code shows each variable's value in a box after its name, and can still be edited; **Hide values** above it takes the boxes away. Click a value to prepare a new one, then F7 forces it, Ctrl+F7 writes it once, and Alt+F7 releases all forces.
- **Watch** lists every variable live, and can prepare, write and force values in the same way.
- The **⋯** beside **POUs** renames or deletes the POU in front, loads the station's example program, and puts the PLC's programs back as the project's original has them.

A PLC's program lives in the project you work in, under `programs/<plc>/`, one `.st` file for each POU, with `project.json` for the task. `Bits.st` is written from the station's I/O table each time the project opens. A new project starts from a template with a step chain; a collection can bring an example program for its station.
