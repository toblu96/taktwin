# Taktwin: notes for coding agents

Taktwin is a desktop app for PLC training. It simulates automated stations in 3D: conveyors, magazines, grippers, sensors and their wiring. Students program a PLC against the simulation: an external one over OPC UA, such as CODESYS, or the app's own Structured Text soft PLC. Teachers build stations from modules and write exercises whose runs the app checks line by line.

It is a [Wails v3](https://v3.wails.io/) app: a Go backend with a Vue 3 + TypeScript frontend built by Vite, for Windows and macOS. Releases come from GitHub Releases, and the app updates itself from them.

Read these before larger changes:

| Document | What is in it |
|---|---|
| [docs/development.md](docs/development.md) | Building, tests, how the code fits together, commits and releases |
| [docs/authoring.md](docs/authoring.md) | Making collections, modules, stations and exercises, step by step |
| [catalog/README.md](catalog/README.md) | The file formats in full, next to the JSON Schemas in [schema/](schema) |
| [docs/user-guide.md](docs/user-guide.md) | What the app does, as its users see it |
| [docs/adr/](docs/adr) | Design decisions, such as how signals reach an external PLC |
| [st/README.md](st/README.md) | The Structured Text compiler and runtime |

## Commands

```sh
wails3 dev                                       # the app with hot reload
wails3 task build                                # a native binary into bin/
go test ./internal/... ./cmd/... ./catalog/...   # Go tests; the root package also needs frontend/dist
(cd st && go test ./...)                         # the ST compiler, a Go module of its own
(cd frontend && npx vue-tsc --noEmit && npx vite build)
wails3 generate bindings -clean=true -ts -i      # after changing a Go service's methods or types
go run ./cmd/taktwin-schema                      # after changing the Go types of a file format
go run ./cmd/taktwin-cli -validate <folder or .taktwin>
```

The frontend's TypeScript bindings in `frontend/bindings/` are generated from the Go services and committed. Regenerate them whenever a service method or one of its types changes. The schemas in `schema/` are generated as well, and a test keeps them in step with the Go types.

## Rules

- **The repository is public and neutral.** It names no particular vendor's training system and holds no course material. `catalog/public_test.go` fails on the words it lists, in every tracked text file, this one included. Stations of a real training system live in a collection of their own, in a private repository with its own tests.
- **No third-party geometry.** Models are built from functional values such as sizes, positions and strokes. Never add, convert or trace meshes, textures or logos of a manufacturer.
- **No station-, module- or exercise-specific code in the app.** Everything is described in JSON and runs on the engine's general primitives: devices, transports, joints, expressions, and Structured Text `program` devices that modules bring. If a new station needs something the primitives cannot do, add or extend a primitive. Never write code for that station.
- **Never hardcode a path.** The user folder comes from Wails (`Documents/Taktwin` by default) or from `TAKTWIN_HOME`. Settings go to `TAKTWIN_CONFIG`, and `TAKTWIN_COLLECTIONS` adds collection folders for development and tests. Tests set all three to temporary folders.
- **Use the framework's own features** before writing your own: Wails menus with `CmdOrCtrl` accelerators, Wails dialogs, file drops and file associations. Don't write your own platform detection.
- **No `confirm()`, `alert()` or `prompt()` in the frontend.** Wails' macOS webview never shows them, and `confirm()` answers no at once. Ask through `ask()` or `choose()` in `frontend/src/ask.ts`, which use Wails' native dialogs.
- **No links out of the app, and no feedback form.** The repository's issues are not open to the public.
- **File formats carry a version.** Stations are format 5, modules and parts 2, exercises and projects 1 (`StationFormat`, `FormatVersion`, `ExerciseFormat`, `ProjectFormat`). The app refuses any other with a message saying so. A change that an older app cannot read raises the format, so the older app refuses the file rather than misreading it. That includes new files inside a project file, which older imports refuse.
- **Projects and collections are data only.** Imports refuse anything but the expected files, paths leading out of the folder, and too much data. Keep it that way: a file someone hands out must never run code on the computer. Structured Text runs only in the app's own sandboxed runtime.

## Style

- **Go:** `gofmt`. Doc comments are full sentences in plain English, saying what something is or does. Test names read as sentences, such as `TestAProjectKeepsItsWork`, with a comment saying what the test shows.
- **Frontend:** Prettier with `.prettierrc.json` (single quotes, no semicolons, lines up to 180 characters). Vue single-file components with `<script setup lang="ts">`. Shared state lives in plain modules with refs (`sim.ts`, `builder.ts`, `update.ts`), not in a store library.
- **Words in the app and the docs:** short, plain, British spelling, no jargon where a plain word does.
- **Commits and pull request titles** follow Conventional Commits, such as `feat(projects): …`, `fix(ui): …` or `docs: …`. The subject says what the change brings, and the body lists the details. Work on a branch named like the change (`feat/…`, `fix/…`, `docs/…`), never directly on `main`. Pull requests are squash-merged with their title.

## Seeing a UI change in the real app

Tests do not show whether a page works. Build the app in Wails' server mode, which serves the real frontend and Go services over HTTP. Then drive it with a headless Chromium through the DevTools protocol, and look at screenshots:

```sh
go build -tags server -o /tmp/taktwin-server .
TAKTWIN_HOME=$(mktemp -d) TAKTWIN_CONFIG=$(mktemp -d) WAILS_SERVER_PORT=8765 /tmp/taktwin-server
```

Then open `http://127.0.0.1:8765/`. Native dialogs, menus, file drops and file associations do not exist in server mode, and a headless browser answers `confirm()` with yes, so check those in the real app.

## Where things are

- `internal/model`: reads and validates the files, and resolves a station into numbers.
- `internal/sim` and `internal/engine`: the simulation.
- `internal/simapp`: the Wails services the frontend calls. Projects are in `projects.go`, exercise files in `exercisefile.go`, the builder in `builder.go`.
- `internal/link`: connections to external PLCs.
- `internal/plcprog`, `st/`: the soft PLC and its compiler.
- `frontend/src`: the pages in `components/`, the 3D view in `viewer/` (plain three.js), the programming tool in `ide/`.
- `catalog/base` and `catalog/reference`: the built-in collections.

[docs/development.md](docs/development.md) has the full map.
