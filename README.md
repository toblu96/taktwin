# Taktwin

A lightweight digital twin of automated stations for PLC training. Stations are simulated in 3D from JSON descriptions of their modules, and are run by a PLC over OPC UA, by the soft PLC built into the app with its Structured Text editor, or by hand with the I/O test.

**Status:** in development, without releases yet. The app comes with its own reference collection, a pallet loading line with euro pallets, cartons, a conveyor, a pallet magazine and an XYZ gantry, which it opens with. Other collections of stations are imported as `.taktwin` packages. The auto-updater is not built yet.

## Getting started

You need Go 1.27+, a current Node LTS and the [Wails v3 CLI](https://v3.wails.io/).

```sh
wails3 dev          # run the app with hot reload
wails3 task build   # build a native binary into bin/
```

Run the station with the soft PLC from its panel, connect a PLC from the top bar, or start the I/O test to switch its outputs by hand. Click any module to see its controls in a popup and its pins and settings on the right.

## Checks

```sh
go test ./...                                    # engine, builder, the reference station and the file formats
go run ./cmd/taktwin-cli -validate my.taktwin    # or a folder with a stations folder
go run ./cmd/taktwin-schema                      # writes schema/ from the Go types
```

## Collections and your own stations

Stations, modules, workpieces and exercises are JSON files, described by the JSON Schemas in [`schema/`](schema) and by [catalog/README.md](catalog/README.md). A collection is a folder of them, or a `.taktwin` zip of that folder, which **Settings** imports.

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
- **Wiring** shows the PLCs, consoles, interfaces and I/O terminals with the cables between them. Click a connector, then another of its kind, to move its cable; click a pin to move what it carries. Each pin shows the PLC address it reaches and the name the PLC knows it by, which can be changed.
- **Save** writes it to your folder, and **Try in Simulation** opens it there.

## Running a station

### Testing I/O by hand

The I/O test cuts the program off from the I/O for the whole station. Start it in the PLC's panel or on any module with PLC outputs. Outputs can then be switched by hand, while inputs show live. Ending the test switches all outputs off.

### Connecting a PLC

The station is the OPC UA client, and the PLC is the server. Choose this on the PLC in the Simulation page, or from the button in the top bar. Each PLC of a station has its own connection.

The PLC must offer its variable list over OPC UA, named as the station's PLC module says in its `addresses`: for the reference collection's cabinet, the list `Bits` with `IB0Bit00` … for inputs and `QB0Bit00` … for outputs. In CODESYS, add a Symbol Configuration with OPC UA support to the application, tick the list, and download. Then connect to `opc.tcp://<host>:4840` with no security and an anonymous login. A PLC over OPC UA runs on the wall clock, so the simulation stays at 1× with it.

### Programming the soft PLC

Each PLC can also run a Structured Text program written in the app. In the PLC's panel, **Edit program** opens the editor over the 3D view, and **Open in window** opens it beside the simulation.

- **Ctrl+Space** (or Option+Space) lists what fits at the cursor: variables, the I/O list with each signal's description, POUs, types, standard functions and keywords, and members after a dot.
- **Build** (F11) lists mistakes with their file, line and column.
- **Download** (Alt+F8) loads the program into the soft PLC and connects the simulation to it over OPC UA, the same way it connects to a real PLC.
- **Run** (F5), **Stop** (Shift+F8) and **Reset** control the program.
- **Online**, the code shows each variable's value in a box after its name. Click a value to prepare a new one, then F7 forces it, Ctrl+F7 writes it once, and Alt+F7 releases all forces.
- **Watch** lists every variable live, and can prepare, write and force values in the same way.

A project lives in your folder under `programs/<station>/<plc>/`, one `.st` file for each POU, with `project.json` for the task. `Bits.st` is written from the station's I/O table each time the project opens. A new project starts from a template with a step chain; a collection can bring an example program for its station.

## How it fits together

| Path | What it does |
|---|---|
| `catalog/` | The app's own models, built in. `base/` holds what every collection can use and is always there; `reference/` is the app's own collection. See [catalog/README.md](catalog/README.md) |
| `schema/` | JSON Schemas of the files of a collection, generated from the Go types |
| `internal/model` | Loads and validates model files, and resolves them into numbers |
| `internal/sim` | Runs any resolved station from a small set of device and transport primitives, and the controllers modules bring |
| `internal/plcio` | The PLC image, how a PLC names its variables, and the `Controller` interface |
| `internal/softplc` | The PLC used while none is connected, which keeps every output off |
| `internal/engine` | Fixed 2 ms simulation clock that couples station and PLC, with no UI dependency |
| `internal/exercise` | Checks a station against an exercise file line by line, in the app or in tests |
| `internal/opclink` | The station as OPC UA client of a PLC, implementing the `Controller` interface |
| `internal/opcsim` | An OPC UA server around any controller |
| `st/` | Structured Text compiler and runtime, a Go module of its own with no dependencies. See [st/README.md](st/README.md) |
| `internal/plcprog` | A PLC's ST project in your folder, and the soft PLC that runs it behind an OPC UA server |
| `internal/simapp` | Wails services over the engine, the builder and the library |
| `internal/version` | The app's version; `scripts/set-version.sh` changes it with the build files |
| `frontend/src/viewer` | Renders any station in plain three.js |

The app has three pages. **Simulation** runs the station with its PLC. **Builder** puts stations together. **Library** browses the collections and previews any entry on its own engine, without a PLC.

## Rules for this repo

- **No third-party models or course material.** Geometry is built from functional values such as positions and strokes. Never add, convert or trace meshes, textures or logos of a manufacturer. Stations of a particular training system live in a collection of their own, outside this repository.
- **Keep `context/` out of git.**

## Contributing and releases

- **Commits and pull request titles** follow [Conventional Commits](https://www.conventionalcommits.org/): `feat(builder): …`, `fix: …`, `docs: …`, `refactor: …`, `test: …`, `ci: …`, `build: …`, `chore: …`. Pull requests are squash-merged with their title, which a check enforces, and the changelog of each release is made from them by [git-cliff](https://git-cliff.org) (`cliff.toml`).
- **Branches** are named like the change: `feat/…`, `fix/…`, `docs/…`, `ci/…`, `chore/…`.
- **Versions** follow [semantic versioning](https://semver.org); while it is 0.x, a minor version may change the file format or behaviour. `internal/version` holds the version, and a test keeps the build files in step with it.
- **To release**, run `scripts/set-version.sh 0.2.0`, commit that as `chore(release): v0.2.0` on `main`, and push the tag:

  ```sh
  git tag -m "Taktwin v0.2.0" v0.2.0 && git push origin v0.2.0
  ```

  The release workflow tests, builds the Windows installer and the macOS app, and publishes them with `SHA256SUMS` and the release notes. A tag such as `v0.2.0-beta.1` makes a pre-release.
- **Release notes** start with highlights written by hand, with screenshots, in `.github/releases/<tag>.md`, followed by how to update and the changelog. Preview them with `scripts/release-notes.sh v0.2.0`; see [.github/releases/README.md](.github/releases/README.md).

## License

Apache License 2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).
