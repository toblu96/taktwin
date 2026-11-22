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
(cd frontend && npm run fmt && npm run lint && npx vue-tsc --noEmit && npx vite build)
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
- **New packages must be at least 7 days old** (`frontend/.npmrc`), and are pinned to an exact version. A release only days old is where a compromised package shows up.
- **Use the framework's own features** before writing your own: Wails menus with `CmdOrCtrl` accelerators, Wails dialogs, file drops and file associations. Don't write your own platform detection.
- **No `confirm()`, `alert()` or `prompt()` in the frontend.** Wails' macOS webview never shows them, and `confirm()` answers no at once. Ask through `ask()` or `choose()` in `frontend/src/ask.ts`, which use Wails' native dialogs.
- **No links out of the app, and no feedback form.** The repository's issues are not open to the public.
- **File formats carry a version.** Stations are format 5, modules and parts 2, exercises and projects 1 (`StationFormat`, `FormatVersion`, `ExerciseFormat`, `ProjectFormat`). The app refuses any other with a message saying so. A change that an older app cannot read raises the format, so the older app refuses the file rather than misreading it. That includes new files inside a project file, which older imports refuse.
- **Projects and collections are data only.** Imports refuse anything but the expected files, paths leading out of the folder, and too much data. Keep it that way: a file someone hands out must never run code on the computer. Structured Text runs only in the app's own sandboxed runtime.

## Style

- **Go:** `gofmt`. Doc comments are full sentences in plain English, saying what something is or does. Test names read as sentences, such as `TestAProjectKeepsItsWork`, with a comment saying what the test shows.
- **Frontend:** formatted with [oxfmt](https://oxc.rs/docs/guide/usage/formatter) and `frontend/.oxfmtrc.json` (single quotes, no semicolons, lines up to 180 characters), linted with [oxlint](https://oxc.rs/docs/guide/usage/linter): `npm run fmt` and `npm run lint` in `frontend/`, which CI checks. Where a lint rule is wrong for a line, disable it for that line only, with the reason (`// oxlint-disable-next-line rule -- why`). Vue single-file components with `<script setup lang="ts">`. Shared state lives in plain modules with refs (`sim.ts`, `builder.ts`, `update.ts`), not in a store library. Where the app is lives in the address, through vue-router (`router.ts`): navigate with the router, never with a ref of your own.
- **Words in the app and the docs:** short, plain, British spelling, no jargon where a plain word does.
- **Commits and pull request titles** follow Conventional Commits, such as `feat(projects): …`, `fix(ui): …` or `docs: …`. The subject says what the change brings, and the body lists the details. Work on a branch named like the change (`feat/…`, `fix/…`, `docs/…`), never directly on `main`. Pull requests are squash-merged with their title.

## The UI

The app is moving to a new look, page by page. New and reworked UI follows these rules:

- **Components:** `frontend/src/ui/` has the shared ones (`UiDialog`, `UiMenu`, `UiSelect`, `UiSegmented`, `UiTabs`, `UiIoList`…) on [UI SDK](https://ui-sdk.dev)'s headless Vue parts. Pages use these, never UI SDK directly, so a part can change library in one place. Add a new shared control there.
- **No UI SDK styles:** never import `@ui-sdk/core/web.css`. It is a full default theme whose rules (positions, focus rings, transitions) mix with ours; every style is our own.
- **Immediate states:** hover, focus and selection change at once, with no colour transitions. A control's look never changes its size: no bolder text for the selected item, which would shift its neighbours.
- **The focus ring:** `:focus-visible` draws one for every control, an outline 2px wide that stands 2px off it, so it never touches a coloured one. It is teal, and amber around an amber control (`--t-ring-color`), as `t-btn-primary` sets it. A button shows it only when reached with the keyboard; a text field shows it whenever it is being typed in. An outline is drawn over neighbouring elements, so only an ancestor that clips or scrolls can cut it: give the controls inside one `--t-ring` (4px) of room, or draw their ring inside them (`focus-visible:outline-offset-[-2px]`).
- **Keyboard:** every control can be reached with Tab. WebKit, the Mac's webview, leaves buttons out of the tab order, so `frontend/src/ui/keyboard.ts` gives each one a tabindex. Each option of a choice is a stop of its own, as in `UiSegmented`, rather than one stop with arrow keys. Leave a second control that does the same as its neighbour out with `tabindex="-1"`, as a card's name does beside its picture.
- **Styling:** Tailwind utilities with the tokens of `frontend/src/ui/theme.css`: `bg-s0`…`bg-s3` for surfaces, `text-fg`, `text-muted`, `border-line`, `border-control` for the edges of controls, `bg-accent` for the one main action, `text-focus` and `bg-highlight` for focus and selection, and the classes `t-btn`, `t-input`, `t-item`. No raw colours: every token has a dark and a light value.
- **Contrast:** every token pair people read or press meets WCAG 2.2 AA (text 4.5:1, control edges and focus 3:1). Check a changed page with axe-core in the browser.
- **No Tailwind reset yet:** buttons, lists and headings keep the browser's defaults, so components set margins, padding and line height themselves. Older pages' styles sit in the CSS layer `legacy`, below Tailwind, and map their colours onto the tokens.
- **Icons:** [Fluent UI System Icons](https://github.com/microsoft/fluentui-system-icons) (MIT), imported as `~icons/fluent/<name>-20-regular` and compiled in, never fetched.
- **The lab:** `npm run dev`, then `/lab.html`, shows every component and the layouts in dark and light.

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
