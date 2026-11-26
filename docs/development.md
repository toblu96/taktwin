# Development

You need Go 1.27+, a current Node LTS and the [Wails v3 CLI](https://v3.wails.io/).

```sh
wails3 dev          # run the app with hot reload
wails3 task build   # build a native binary into bin/
```

## Checks

```sh
go test ./internal/... ./cmd/... ./catalog/...   # engine, builder, the reference station and the file formats
(cd st && go test ./...)                         # the Structured Text compiler, a module of its own
go run ./cmd/taktwin-cli -validate my.taktwin    # or a folder with a stations folder
go run ./cmd/taktwin-schema                      # writes schema/ from the Go types
```

The app's own package embeds the built frontend, so `go test ./...` from the root needs `frontend/dist` first; the packages above do not.

```sh
cd frontend
npx vue-tsc --noEmit                              # types
npx vite build                                    # frontend/dist
npm run fmt:check                                 # formatting with oxfmt, as .oxfmtrc.json says (npm run fmt fixes it)
npm run lint                                      # oxlint; warnings fail it
```

After changing the methods or types of a Go service, regenerate the frontend's bindings with `wails3 generate bindings -clean=true -ts -i` and commit them with the change. After changing the Go types of a file format, run `go run ./cmd/taktwin-schema` and commit `schema/`.

A collection of stations of a particular training system lives in a private repository with its own tests. Those tests use the app's internal packages, so its script copies them into a checkout of this repository and runs them there. When a service changes, run them as well, and update them in that repository on a branch of the same name.

### Seeing it work

Tests do not show whether a page works. `go build -tags server` builds the app in Wails' server mode, which serves the real frontend and services over HTTP at `WAILS_SERVER_PORT`. Run it with `TAKTWIN_HOME` and `TAKTWIN_CONFIG` set to temporary folders, and drive it with a headless Chromium through the DevTools protocol, looking at screenshots. Server mode has no native dialogs, menus, file drops or file associations, and a headless browser answers `confirm()` with yes, so check those in the real app.

## How it fits together

| Path | What it does |
|---|---|
| `catalog/` | The app's own models, built in. `base/` holds what every collection can use and is always there; `reference/` is the app's own collection. See [catalog/README.md](../catalog/README.md) |
| `schema/` | JSON Schemas of the files of a collection, generated from the Go types |
| `internal/model` | Loads and validates model files, and resolves them into numbers |
| `internal/sim` | Runs any resolved station from a small set of device and transport primitives, and the controllers modules bring |
| `internal/plcio` | The PLC image, its addresses, and the `Controller` interface |
| `internal/softplc` | The PLC used while none is connected, which keeps every output off |
| `internal/engine` | Fixed 2 ms simulation clock that couples station and PLC, with no UI dependency |
| `internal/exercise` | Checks a station against an exercise file line by line, in the app or in tests |
| `internal/link` | Couples a PLC of a station to a controller through a connection and a driver, implementing the `Controller` interface; no protocol of its own. See [ADR 0001](adr/0001-signals-mapping-and-connection-drivers.md) |
| `internal/link/opcua` | The OPC UA driver: a client of any server |
| `internal/opcsim` | An OPC UA server around any controller, the soft PLC's |
| `st/` | Structured Text compiler and runtime, a Go module of its own with no dependencies. See [st/README.md](../st/README.md) |
| `internal/plcprog` | A PLC's ST project in your folder, and the soft PLC that runs it behind an OPC UA server |
| `internal/simapp` | The Wails services the frontend calls: the simulation and its projects (`service.go`, `projects.go`), saving a project file in place (`document.go`), the list of files opened (`recent.go`), a project's original and handing it out (`original.go`), project files and exercise checks (`exercisefile.go`, `exercises.go`), the builder (`builder.go`, `wiring.go`), the library and collections (`library.go`, `collections.go`), connections (`connections.go`, `editor.go`), the programming tool (`programs.go`), settings and updates |
| `internal/version` | The app's version; `scripts/set-version.sh` changes it with the build files |
| `frontend/src` | The Vue app. `router.ts` says where the app is (projects, the open project simulated or built, library, settings) and guards those pages, `sim.ts` holds the open project, `builder.ts` the station being built, `update.ts` the updates; `ask.ts` asks in Wails' native dialogs |
| `frontend/src/ui` | The shared components on UI SDK and the design tokens (`theme.css`); `lab.html` shows them. See [AGENTS.md](../AGENTS.md#the-ui) |
| `frontend/src/components` | The pages and their parts: `ProjectsPage`; an open project's views, `ProjectWindow` for the simulation (`SimulationSide`, `ContextPanel` with a PLC's `PlcConnection`), `WiringPage` (`WiringSide`, `WiringDiagram`, `WiringDetails`, and `ConnectionEditor` for a PLC's bindings), `ExercisePage`, and `BuilderPage` for building; `PagePanel` below a page's middle; `TopBar`, `StatusBar`, `ActivityBar`; `LibraryPage`, `SettingsPage` |
| `frontend/src/viewer` | Renders any station in plain three.js, and the builder's handles and ghosts |
| `frontend/src/ide` | The programming tool for the soft PLC, loaded when first opened: `ProgramIde`, the code editor `StEditor` on CodeMirror 6 with Structured Text as a language of its own (`stLanguage.ts`), completion (`complete.ts`) and the watch list. Mistakes come from `ProgramService.Check`, which compiles the open files unsaved; Format Document from `st.Format`; the files left open from `ProgramService.EditorState`, kept in the settings folder by project |
| `frontend/bindings` | TypeScript bindings of the Go services, generated by Wails |

### Pages

The app starts on **Projects**, with **Library** beside it, which browses the collections and previews any entry on its own engine, without a PLC. An open project has three views, **Simulation**, **Wiring** and **Exercise**, each run or, in expert mode, edited: the simulation's edit is the builder, the wiring's rewiring (`router.ts` has their addresses). The gear opens **Settings**. The status bar along the bottom shows the version, whether the station is being built, and how the project's file is saved.

### Collections, projects and where a station comes from

The library is a stack of layers, front first: the user folder's own files, imported collections (`collections/` of the user folder), those named by `TAKTWIN_COLLECTIONS`, the built-in reference collection, and the base. Hidden collections are left out. A file is taken from the first layer that has it (`simapp.Library`).

Everything in the simulation belongs to a project: a `.taktwin-project` file, a zip of data only, wherever it is ([ADR 0002](adr/0002-projects-as-documents.md)). New ones are made in `projects/` of the user folder; the settings keep a list of those opened from elsewhere. A project file holds:

| File | What it is |
|---|---|
| `project.json` | Format 2: its id, what it is based on (a station, or an exercise and its station), the collections it carries with their versions, how its PLCs are connected with each one's external connection, its settings, its last check, when its original was set, and for an exercise `exerciseHash` |
| `station.json` | The project's copy of its station, made with the project and built on in it |
| `exercise.json`, `instructions.md` | For a project on an exercise, its copy of the exercise and its instructions |
| `programs/<plc>/` | The soft PLCs' Structured Text |
| `collections/<id>.taktwin` | Every collection the station, the exercise and their example programs come from, the base and the app's own among them, in the version the project was made with or last updated to |
| `original/` | The project's original: `project.json` without the last check, the station, the exercise, the instructions and the programs as they were when it was set, and in `original/collections/` the older versions of collections the project has since been updated from |
| `thumbnail.jpg` | A picture of the station as it was last closed |

The app works in a working copy of the open project, in `projects/<id>/` of the settings folder, so the rest of the app reads and writes a project as a folder (`simapp.document`). Every second it looks for changes, and writes a change once it has stayed for a look: into a temporary file beside the project file, renamed over it. Before writing, it compares the file's size and time with when it last read or wrote it, and leaves a file changed from elsewhere alone (`SaveConflict`). A file it cannot write opens read only (`SaveReadOnly`). `projects/<id>.json` beside the working copy says which file it belongs to and what was last saved, so that changes a crash left behind are offered back when the file is opened again. Closing a project writes the file and removes its working copy. The state goes to the frontend as `project:save` events.

`projectLibrary` puts the project's own `station.json` and `exercise.json` in front, then the collections it carries, then those installed for anything else, leaving out an installed collection whose id the project carries, also a hidden one. So the rest of the app loads a project's station like any other, and the same on every computer. Opening a project copies in what it lacks (`carry`): its station, its exercise, and the collections they come from, as installed here, for a new project and for a file made without them. `updatesOf` compares the carried versions with those installed, and `UpdateCollections` replaces them, letting the station and the exercise follow where the project's copies are those of the old version. A collection is only offered newer when its `version` is raised.

The original (`original.go`) is made when a project without one is opened, by `KeepAsOriginal`, and by `HandOut` for the copy it writes, with a new id. `Original` compares the project with it, part by part, and `PutBack` puts a part back: a PLC's connection or programs, the station, the settings, or everything, the collections' versions too. Before `UpdateCollections` replaces a collection the original uses, `keepOlder` keeps its package under `original/collections/`.

An exercise's project keeps `exerciseHash`, a sum of its `exercise.json` and `station.json` as the original has them; `ProjectInfo.ExerciseChanged` says when they differ. In expert mode, `StartCheck` first calls `teacherCheck`: when the installed collections have the exercise and the project's copies differ, their exercise and station go in front of the project's library as `project.teacher`, in memory only, and the station loads again with its PLCs connected (`reload(true)`).

## Commits, branches and releases

- **Commits and pull request titles** follow [Conventional Commits](https://www.conventionalcommits.org/): `feat(builder): …`, `fix: …`, `docs: …`, `refactor: …`, `test: …`, `ci: …`, `build: …`, `chore: …`. Pull requests are squash-merged with their title, which a check enforces, and the changelog of each release is made from them by [git-cliff](https://git-cliff.org) (`cliff.toml`).
- **Branches** are named like the change: `feat/…`, `fix/…`, `docs/…`, `ci/…`, `chore/…`.
- **Versions** follow [semantic versioning](https://semver.org); while it is 0.x, a minor version may change the file format or behaviour. `internal/version` holds the version, and a test keeps the build files in step with it.
- **To release**, run **Actions → prepare-release**. It opens the pull request `chore(release): v0.3.0` with the version worked out from the commits and the release notes; merging it tags the release, and the release workflow tests, builds the Windows installer and the macOS app, and publishes them with `SHA256SUMS` and the notes. A version such as `0.3.0-beta.1` makes a pre-release. See [.github/releases/README.md](../.github/releases/README.md).
- **Release notes** start with highlights written by hand, with screenshots, in `.github/releases/<tag>.md`, followed by the changelog and how to install. Preview them with `scripts/release-notes.sh v0.2.0`; see [.github/releases/README.md](../.github/releases/README.md).

## Rules

- **No third-party models or course material.** Geometry is built from functional values such as positions and strokes. Never add, convert or trace meshes, textures or logos of a manufacturer. Stations of a particular training system live in a collection of their own, outside this repository.
- **Keep `context/` out of git.**
