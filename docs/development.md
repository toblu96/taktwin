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

## How it fits together

| Path | What it does |
|---|---|
| `catalog/` | The app's own models, built in. `base/` holds what every collection can use and is always there; `reference/` is the app's own collection. See [catalog/README.md](../catalog/README.md) |
| `schema/` | JSON Schemas of the files of a collection, generated from the Go types |
| `internal/model` | Loads and validates model files, and resolves them into numbers |
| `internal/sim` | Runs any resolved station from a small set of device and transport primitives, and the controllers modules bring |
| `internal/plcio` | The PLC image, how a PLC names its variables, and the `Controller` interface |
| `internal/softplc` | The PLC used while none is connected, which keeps every output off |
| `internal/engine` | Fixed 2 ms simulation clock that couples station and PLC, with no UI dependency |
| `internal/exercise` | Checks a station against an exercise file line by line, in the app or in tests |
| `internal/opclink` | The station as OPC UA client of a PLC, implementing the `Controller` interface |
| `internal/opcsim` | An OPC UA server around any controller |
| `st/` | Structured Text compiler and runtime, a Go module of its own with no dependencies. See [st/README.md](../st/README.md) |
| `internal/plcprog` | A PLC's ST project in your folder, and the soft PLC that runs it behind an OPC UA server |
| `internal/simapp` | Wails services over the engine, the builder and the library |
| `internal/version` | The app's version; `scripts/set-version.sh` changes it with the build files |
| `frontend/src/viewer` | Renders any station in plain three.js |

The app has three pages. **Simulation** runs the station with its PLC. **Builder** puts stations together. **Library** browses the collections and previews any entry on its own engine, without a PLC.

## Commits, branches and releases

- **Commits and pull request titles** follow [Conventional Commits](https://www.conventionalcommits.org/): `feat(builder): …`, `fix: …`, `docs: …`, `refactor: …`, `test: …`, `ci: …`, `build: …`, `chore: …`. Pull requests are squash-merged with their title, which a check enforces, and the changelog of each release is made from them by [git-cliff](https://git-cliff.org) (`cliff.toml`).
- **Branches** are named like the change: `feat/…`, `fix/…`, `docs/…`, `ci/…`, `chore/…`.
- **Versions** follow [semantic versioning](https://semver.org); while it is 0.x, a minor version may change the file format or behaviour. `internal/version` holds the version, and a test keeps the build files in step with it.
- **To release**, run **Actions → prepare-release**. It opens the pull request `chore(release): v0.3.0` with the version worked out from the commits and the release notes; merging it tags the release, and the release workflow tests, builds the Windows installer and the macOS app, and publishes them with `SHA256SUMS` and the notes. A version such as `0.3.0-beta.1` makes a pre-release. See [.github/releases/README.md](../.github/releases/README.md).
- **Release notes** start with highlights written by hand, with screenshots, in `.github/releases/<tag>.md`, followed by the changelog and how to install. Preview them with `scripts/release-notes.sh v0.2.0`; see [.github/releases/README.md](../.github/releases/README.md).

## Rules

- **No third-party models or course material.** Geometry is built from functional values such as positions and strokes. Never add, convert or trace meshes, textures or logos of a manufacturer. Stations of a particular training system live in a collection of their own, outside this repository.
- **Keep `context/` out of git.**
