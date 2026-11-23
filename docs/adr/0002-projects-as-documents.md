# ADR 0002: Projects as documents

- **Status:** Accepted
- **Date:** 2026-11-23
- **Scope:** what a project file holds, where it lives and how it is saved; exercises as projects; how a PLC keeps its connection; what the user folder is for
- **Replaces:** parts of [ADR 0001](0001-signals-mapping-and-connection-drivers.md): its sections 5 (projects as overlays that follow updates), 6 (exercises as files of their own, installed into the user folder) and the choice of a connection from a list in section 7

## Context

A project today is a folder in `projects/` of the user folder. A `.taktwin-project` file is only a way to carry one: opening it unpacks a new copy into `projects/`, with a number when the name is taken. Several things that belong to the project are not in it:

- **The station**, unless it was built on: the project names a collection's station by id and follows its updates. On another computer the station may be missing, or be a different version.
- **The collections** it is built from. An exercise handed out brings the imported ones, and opening it installs them into the user folder.
- **The PLC's connection.** The project names a connection by id, with a few values such as the server's address. The connection itself, the bindings of every signal, is a file in `connections/` of the user folder or of a collection. The connection editor writes it into the user folder.

This failed in use. A teacher set up the bindings of a station to a FluidSIM circuit built with I/O port components, and sent the project file to a colleague. The file named the connection, but the connection had stayed on the teacher's computer. On the colleague's, the app did not find it and quietly fell back to another one, so nothing matched. Opening the file twice made two projects.

The connection list in the PLC's panel added to the confusion: it offered every connection file for the station, each a complete mapping of its own, while the project could only use one.

## Requirements

| # | Requirement |
|---|---|
| R1 | **A project is one file and holds all of its work:** station, wiring, settings, PLC connections, programs, and the collections it is built from. It can be moved, mailed or put on a shared drive, and opens the same everywhere. |
| R2 | **A project is edited where it is.** A file opened from the downloads folder is changed in that file, never copied into the user folder. |
| R3 | **Each PLC has one connection.** Switching between no PLC, the soft PLC and an external one keeps what each had. There is no list of mappings to choose from. |
| R4 | **Everything a project uses can be changed in it,** such as the PLC's address. |
| R5 | **An exercise is a project with an exercise definition.** It is the one thing a project file may hold in addition. There is one file type. |
| R6 | **Checks can be trusted by the teacher,** even though a project file is a zip anyone can edit. |

## Who uses this

| Who | Does | Needs |
|---|---|---|
| **Learner** | Opens an exercise from mail or a shared drive, works on it, hands it in | Double-click, work, send the same file back. Nothing to install, nothing to configure but the PLC's address |
| **Learner or tinkerer** | Makes a project from a station, plays, takes it home | Projects that open the same on another computer |
| **Teacher** | Builds a station, sets up a PLC connection, hands out a project or an exercise, checks hand-ins | Set it up once and share it as one file; checks that use the teacher's own definition |
| **Collection author** | Makes modules and stations, releases versions | Updates that reach projects only when their owners choose |

## Decision

### 1. The project file is the project

A `.taktwin-project` file is a zip of data only, as today. The app opens it where it is and saves every change back into it. There is no copy of the project in the user folder and no separate export: to share a project, share its file.

| Card action today | Becomes |
|---|---|
| Open | Open the file in place |
| Duplicate | Save a copy… (the system's save dialog) |
| Rename | Rename the file, while it is closed |
| Show folder | Show the file in the file manager |
| Export… | Not needed: the file is the project |
| Delete | Move the file to the bin; for a file outside the projects folder, also **Remove from list** |
| Hand out as exercise… | **Hand out…**, for any project: a copy for others, whose original is the state it is handed out in (section 6) |

### 2. What a project file holds

```
Sorting with FluidSIM.taktwin-project   (zip, data only)
  project.json            format 2: id, name, PLCs and their connections, settings, last check, collections
  station.json            the project's station, always
  about.md                optional: how the station works, beside its station
  programs/<plc>/         the soft PLC's Structured Text
  collections/<id>.taktwin  every collection the station or its original is built from, in the version it uses
  exercise.json           an exercise only: the task's check
  instructions.md         an exercise only: the task
  original/               the project as it was made or handed out, to put back (section 6)
  thumbnail.jpg           a picture of the station as it was last closed
```

```json
{
  "format": 2,
  "id": "6f1c0b3e-8f5e-4b8a-9d55-2c1e8f0a7b42",
  "name": "Sorting with FluidSIM",
  "collections": [
    { "collection": "base", "version": "1.4.0" },
    { "collection": "training-stations", "version": "1.2.0" }
  ],
  "plcs": {
    "": {
      "mode": "external",
      "external": {
        "kind": "fluidsim-ports",
        "driver": "opcua",
        "settings": { "endpoint": "opc.tcp://192.168.64.3:62555" },
        "namespaces": { "fs": "http://example.com/fluidsim-namespace" },
        "generate": [ ],
        "bindings": [ ]
      }
    }
  },
  "parameters": { },
  "lastCheck": null
}
```

- **The station is always a copy.** A project made from a collection's station copies it in, and the collections it is built from go along under `collections/`, the app's built-in ones included. They are small: data only, no meshes.
- **The project's collections come first.** Its station loads from them, not from what is installed, so a project opens the same on every computer and after every update of the app or a collection.
- **Updates are offered, not applied.** When a collection installed here is newer than the project's copy, the project says so and offers to update. Updating replaces the copy in the file; the station's own changes stay, and what no longer fits is listed under **Problems**. A project whose collection is newer than this app understands is refused with a message, as a newer format is.
- **`id`** is made when the project is made and never changes, also when the file is renamed or copied. It ties the working copy (section 4) and the recent list to the file. **Save a copy** gives the copy a new id.
- Files hold no passwords. A connection's secret settings go to the system's keychain, as ADR 0001 says.

### 3. Where files live, and what the user folder is for

The user folder (`Documents/Taktwin`, the folder chosen in **Settings**, or `TAKTWIN_HOME`) keeps:

| Folder | What is in it |
|---|---|
| `collections/` | The collection store: collections imported in **Settings › Collections**, and the teacher's own collection, which the app can edit (expert mode). Stations, connection kinds and exercises meant for reuse go there |
| `projects/` | Where new project files are made. Files elsewhere are not moved here |

The app's settings folder (`TAKTWIN_CONFIG`) keeps, besides the settings, the **recent list**: the path, id and last opening of every project file opened. The working copies are kept beside the settings, in `projects/`.

The welcome screen lists the files in the user folder's `projects/` and the recent list, once each. A file that is gone from where it was is shown greyed out, with **Locate…** and **Remove from list**.

The user folder's `stations/` and `connections/` go away, and so does `programs/`, left from version 0.2. Section 9 says what becomes of their files.

### 4. Saving in place

A zip cannot be changed in part, and a file in a synced or downloads folder may be locked, synced half-way or changed from elsewhere. So:

1. **Opening** reads and checks the file as an import does today: only the expected files, no paths out of it, a size limit. It unpacks the file into a working copy in the settings folder, named by the project's id.
2. **Every change** goes into the working copy at once, as now. About a second after the last change, and when the project is closed, the app writes the file anew: into a temporary file in the same folder, then renamed over the old one, so a file is never left half-written.
3. **When the file changed on disk** since the app last read or wrote it (time and size differ), the app does not overwrite it. It asks: **Keep mine**, **Open theirs**, or **Save mine as a copy…**.
4. **When it cannot be written,** such as from a read-only share, the project opens read-only, with **Save a copy…** in a bar above it. A write that fails later, such as on a lock, is tried again; the status bar says the file is not saved yet, and closing asks what to do.
5. **After a crash,** a working copy newer than its file is offered back when the file is opened again.

### 5. One connection per PLC, kept per mode

A PLC is run in one of three modes: **none**, the **soft PLC**, or an **external** PLC. Each mode keeps its own state in the project, and switching modes loses nothing: going from external to none and back brings the same connection, bindings and address back. The soft PLC's state is its programs.

An external connection has a **kind**, which says how the PLC is reached and fills a new connection in:

| Kind | Driver | Starts with |
|---|---|---|
| CODESYS | `opcua` | The variable list the PLC module names, a binding for every signal |
| FluidSIM I/O ports | `opcua` | A binding for every bit by its address, to the circuit's input and output port components |
| OPC UA | `opcua` | No bindings |
| Modbus, later | `modbus` | As its driver says |

- **Kinds are technologies, not mappings.** A kind is a connection file without a `station`, in the built-in base collection, or the CODESYS connection the app makes from the PLC module. Adding one is a data change, or a driver for a new protocol. A mapping for one station and one circuit is made in a project, and shared by sharing the project.
- **`plc.maps` go too.** A map let a CODESYS program written for another wiring run on a station. That is a mapping as well: a teacher sets it up as a project's connection, of the kind CODESYS, and shares the project. Stations no longer carry maps, and connection files with a `station` are no longer offered.
- **The kind is chosen once,** when the external mode is first used. Choosing another later replaces the external connection, after asking. The original connection can be put back (section 6).
- **The connection is the project's own,** with its address, settings, namespaces and bindings, and every part of it can be changed in **Wiring**. The connection editor saves into the project, so the bindings travel with it.
- **Vars** (ADR 0001, section 7) remain for kinds: a kind may ask for values such as the server's address. They are filled in when the connection is made, and from then on they are ordinary settings of the project's connection.
- **A connection is never opened by itself** when a project is opened (ADR 0001, section 10). The PLC's panel shows where it goes, and the first connection to another computer asks first.

### 6. Every project keeps its original

A project file keeps, under `original/`, the state it started from: its station, its settings, its PLCs' modes and connections, its programs, and for an exercise the exercise. It is the project's first snapshot.

- **Set when the project is made,** from a station, an example, an exercise of a collection or an empty station.
- **Set anew by Hand out…:** the copy for others starts with the state it is handed out in as its original. A teacher who prepares a project and hands it out gives learners that prepared state to go back to. Sending the file itself sends its original as it is, so in expert mode **Keep this as the original** sets it to the state now.
- **Put back a part at a time,** so that undoing one mistake does not cost the rest of the work:
  - **a PLC's connection,** in **Wiring**, such as after a wrong address;
  - **a PLC's programs,** in the program editor;
  - **the station,** its building and wiring, in the edit views;
  - **the station's settings;**
  - **everything,** after asking.
- **Collections:** the original names the collection versions it was made with. After an update, `collections/` keeps the older version as long as the original uses it.

### 7. Exercises are projects with a definition

An exercise is a project file that also holds `exercise.json` and `instructions.md`. That is the only difference: an exercise defines more inside the same file. There is no other exercise file. A teacher makes one from a project; a project made from an exercise of a collection carries a copy of it. Learners open the file, work in it, and hand in that file.

- **The physical part stays locked,** as ADR 0001 decided: wiring, `changes` and symbols, except what the exercise unlocks. The connection and the programs are the learner's. What is locked needs no putting back.
- **The original carries a hash** of the exercise and its station in `project.json`. One that no longer matches shows a warning. It catches mistakes, not cheating: anyone can change the zip.
- **The teacher's check uses the teacher's definition.** When the teacher has an exercise with the same id, in a collection or in a file they handed it out from, a check of a hand-in runs with the teacher's `exercise.json` and station. If they differ from the hand-in's, the result says so. Handing out an exercise also puts it into the teacher's own collection, so that the teacher has it for checking.
- **Handing out an exercise** leaves the last check behind, and takes the programs along only when the teacher chooses it, so that a solution does not go out by mistake. The connections go along, so that learners need to change only the address.

### 8. Safety

The rules of AGENTS.md and ADR 0001, section 10, stay as they are. In addition:

| Risk | Measure |
|---|---|
| A file that runs code | Project files stay data only. Collections inside them are read as data and checked like any collection, and are not installed by opening the file |
| Writing somewhere unexpected | The app writes only the file that was opened, through a temporary file beside it, and its working copy in the settings folder |
| Losing work to a lock, a sync or a crash | Section 4: no half-written files, nothing overwritten that changed elsewhere, working copies offered back |
| A weakened check in a hand-in | Section 7: the teacher's own definition is used |

### 9. No migration

No release with projects is in use yet, so nothing is converted. The project format goes to 2, and the app refuses project files of format 1 with a message, as it refuses any other format. Project folders in `projects/`, and the user folder's `stations/`, `connections/` and `programs/`, are no longer read. `plc.maps` are dropped from the station format, and connection files with a `station` from the collections, such as those kept in a private collection, are made into projects by hand.

## Consequences

**Benefits**

- A project means one file, everywhere: what is shared is what was set up, connection included.
- Opening a file twice opens the same project, instead of making a numbered copy.
- Learners need nothing installed besides the app, and a teacher needs no collection release for a weekly exercise.
- Projects no longer change under their owners when a collection is updated.
- The PLC's panel loses the list of mappings; what remains is a choice of mode and, once, of a kind.

**Costs**

- Saving in place needs care: working copies, renames over the file, change detection, read-only and crash handling, with tests.
- Every project file carries its collections. They are small, but a fix in a module reaches a project only when its owner updates it.
- A shared project file holds one address per PLC. Two people working on the same file in a shared folder overwrite each other's; each should work on a copy.
- Format 2 project files cannot be opened by older apps, and this app no longer opens format 1.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Keep importing files into the user folder | Two places for one project, numbered copies, and a hand-in that is not the file the learner worked in |
| Projects as folders, shared as folders | Mail, learning platforms and downloads deal in files. A macOS package is a folder on Windows |
| Projects that follow collection updates (ADR 0001) | A project then opens differently on another computer or after an update. Offering updates keeps them identical and still lets fixes in |
| Connections chosen from a list per PLC | Every list entry was a mapping of its own, and the project's changes were saved into a separate file that did not travel. One connection per mode is enough, and the original brings the first one back |
| A separate exercise file (`.taktwin-exercise`, ADR 0001) | Two files for a learner to keep apart. An exercise is a project with a definition, so one file type suffices |
| Checking a hand-in with the definition inside it | Anyone can change a zip. The teacher's own definition is the one to trust |

## Decided in review

| Question | Decision |
|---|---|
| Does a project carry its collections? | Yes, each in the exact version it uses; updates are offered |
| Are collections inside a project installed when it is opened? | No; they are used from the file |
| One file type for projects and exercises? | Yes |
| What does a teacher's check of a hand-in use? | The teacher's own exercise definition, when there is one |
| How many connections does a PLC have? | One per mode, kept while switching modes. The kind is a technology, not a mapping |
| `plc.maps` | Dropped: a map is set up as a project's connection |
| Which projects can be put back? | Every project keeps its original, not only exercises; a part at a time or everything |
| What makes an exercise? | Only what it defines in addition: `exercise.json`, `instructions.md` and the locks |
| Are old projects converted? | No: no release with projects is in use yet |

## Open questions

- **More snapshots** than the original, to go back to a later state.

## Rollout

1. **Project files in place:** format 2, working copies, saving back, change detection, read-only and crash handling, the recent list and the welcome screen on files.
2. **Self-contained projects:** the station copy, the collections inside, and updates offered.
3. **The original:** `original/`, putting back a part or everything, Keep this as the original, and Hand out….
4. **One connection per PLC:** state per mode, kinds, the editor saving into the project, the PLC's panel without the list, and `plc.maps` dropped.
5. **Exercises:** the hash, the locks, the teacher's check with their own definition, and handing out without the last check.
6. **Documentation:** the user guide, the authoring guide, the collection format and AGENTS.md (the project format).
