# Making collections, modules, stations and exercises

Everything Taktwin simulates is described in JSON files, so a new station or exercise needs no change to the app. This guide goes through the steps. [catalog/README.md](../catalog/README.md) describes every file and field, and the JSON Schemas in [schema/](../schema) give an editor completion and checks. The reference collection in [catalog/reference](../catalog/reference) is a complete, small example of each kind of file.

| What | Made with | Lives in |
|---|---|---|
| Module or accessory: a machine or what goes onto one | an editor, as JSON | a collection's `modules/<id>/` or `accessories/<id>/` |
| Workpieces, parts, materials, connector types | an editor, as JSON | a collection |
| Station: modules placed, cabled and wired | the app's **Simulation** in expert mode, edited, or an editor | a project, or a collection's `stations/<id>/` |
| Exercise: a task on a station and its check | an editor, as JSON | a collection's `exercises/<id>/`, or a project handed out as a file |

## A collection to work in

A collection is a folder with a `collection.json` at its root:

```json
{
  "id": "bottling",
  "name": "Bottling line",
  "version": "0.1.0",
  "description": "Stations that fill and cap glass bottles.",
  "station": "filler",
  "workpieces": "bottles"
}
```

`station` is the station the start page offers as an example, and `workpieces` are what a new station and a module tried out on its own work with. Next to it go the folders `modules/`, `accessories/`, `parts/`, `workpieces/`, `stations/`, `exercises/`, `programs/` and `connections/`, as far as the collection has them. A collection may use everything of the built-in base, such as its connector types and materials, and the modules of collections installed beside it.

While you work on it, let the app read the folder as it is:

```sh
TAKTWIN_COLLECTIONS=~/work/bottling wails3 dev
```

The installed app reads a folder in `collections/` of your folder the same way. After changing a file, **Settings › General › Reload** reads everything again, and the Library shows what does not load, with the reason. Give every file its schema for completion in an editor, such as `"$schema": "https://raw.githubusercontent.com/toblu96/taktwin/main/schema/module.schema.json"`.

Check the whole collection before you share it:

```sh
go run ./cmd/taktwin-cli -validate ~/work/bottling
```

To share it, zip the folder's contents into a `.taktwin` file and import that in **Settings › Collections**. A newer version with the same `id` replaces the installed one, so raise `version` with every change you hand out. Within a minor version, never remove or rename a module's pins or parameters, so that stations and exercises made with the older version keep loading.

## Modules

A module is something you could unscrew from where it stands: a conveyor, a magazine, a sensor. Start from the one in the reference collection closest to what you need:

| Example | Shows |
|---|---|
| [accessories/pallet-stop](../catalog/reference/accessories/pallet-stop/module.json) | A small actuator: a setting, a mount, a model, a cylinder-like device and its pins |
| [accessories/photo-sensor](../catalog/reference/accessories/photo-sensor/module.json) | A sensor that sees workpieces, with a threshold |
| [modules/pallet-conveyor](../catalog/reference/modules/pallet-conveyor/module.json) | A transport that carries workpieces, with sites to screw others onto |
| [modules/xyz-gantry](../catalog/reference/modules/xyz-gantry/module.json) | Axes that move to positions, and a gripper |

Build a module in this order, and look at it in the Library after each step. The Library shows it on its own, with its settings, and lets you switch its inputs by hand.

1. **Settings** (`parameters`): sizes and speeds as named numbers. A setting with a `label` is one people can change: give it a `kind` (`build` or `physics`), a `description` of what it does, and its limits (`min` and `max`, or `options`). A test holds the app's own collection to this; do the same in yours.
2. **Model** (`model`): the 3D shape as a tree of named nodes, from boxes, cylinders and extrusions with sizes taken from the settings. Build it from functional values only, never from a manufacturer's meshes or drawings traced.
3. **Movement** (`joints`) and **working parts** (`devices`): cylinders, axes, grippers, sensors and lamps, each with the states the station's PLC reads and the inputs it drives.
4. **Workpieces** (`transport`): the paths that carry them, and where they are handed over.
5. **Terminal** (`terminal`): the connectors and pins, and which device each pin belongs to.
6. **Mounting** (`mounts`, `sites`, `role`): where it is screwed down and what can be screwed onto it, so that the builder can place it.

If the engine has no primitive for what the module does, a module can bring a controller of its own as a `program` device in Structured Text (see [Programs](../catalog/README.md#programs)). A mechanism that even that cannot describe needs a new primitive in the engine. That is a change to the app, made for every station, never code for one module.

## Stations

The easiest way is the app itself:

1. Switch on **Settings › General › Experimental › Expert mode**. **Projects › New project › Empty** then opens an empty station of a collection, edited. Or start a project on a station that is close to what you want, and switch the top bar to editing, which makes a copy that the project keeps.
2. Pick modules on the left and put them down on the lines that light up. Move them with the arrows, and set their settings on the right. **Problems**, in the panel below, lists what does not fit together.
3. **Wiring** shows the cables from the PLC to the I/O terminals, and the name the PLC knows each signal by.
4. Switching editing off runs it. **I/O test** in the top bar switches outputs by hand.

The project keeps the station as `station.json` in its file, a zip (**Show in folder** in the project's card menu). To make it part of a collection, copy that file out of it to the collection's `stations/<id>/station.json` and give it that `id`. A station can come with:

- **an example program** for its PLC, in `programs/<station>/<plc>/` of the collection, which the programming tool offers to load;
- **about.md**, how the station works, beside its `station.json`.

How a PLC is connected to a particular controller, such as the OPC UA variables of one CODESYS project, a FluidSIM circuit, or a PLC program written for another wiring, is set up in a project and shared with it. Switch on **Connect when the project opens** in the PLC's panel before **Hand out…**, and the students' copies connect as they open. A collection's `connections/` holds kinds of connection that fit any station, which name no station.

## Exercises

An exercise is a task on a station and the check of a run of it, line by line, as a grading sheet does. [exercises/load-a-pallet](../catalog/reference/exercises/load-a-pallet/exercise.json) is a complete example. Put an exercise into `exercises/<id>/exercise.json` of a collection, with:

- `station`, the station it is for;
- `rules`, saying where each kind of workpiece belongs at the end;
- `steps`, which place workpieces, press buttons, wait, and check expressions over the PLC's signals and the station's states, each check a line of the result;
- `changes`, when the exercise sets the station up its own way, such as signals on other pins of a terminal;
- an `instructions.md` beside it: the task in Markdown, which the app shows under the exercise.

Try it from the app: **New project › Exercise**, connect a PLC that solves it, such as the soft PLC with a program you trust, and press **Check**. A check runs on the wall clock, so a check of a long cycle takes as long as the cycle. Make sure it passes with a good program and fails with a wrong one before you hand it out.

To hand it out:

- **as one file:** **Hand out…** on the project on the exercise writes a `.taktwin-project` with the station and the exercise, the instructions, how the PLCs are connected and the collections the project carries, so students need nothing installed. Your programs go along only if you choose so. Students open it by double-clicking it, and can put it back as it was handed out.
- **with a collection:** exercises in a collection show under **New project › Exercise** wherever the collection is installed.

## Before sharing

- `taktwin-cli -validate` passes, and **Problems** is empty while the station is edited.
- Every check passes with a good program.
- `version` in `collection.json` is raised.
- Files for this repository name no particular vendor's training system: `catalog/public_test.go` refuses them. Such stations belong in a collection of their own.
