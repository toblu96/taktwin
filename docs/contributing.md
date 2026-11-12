# Contributing

Issues and pull requests are welcome. For a larger change, open an issue first, so that we can agree on the approach.

## Commits, branches and releases

- **Commits and pull request titles** follow [Conventional Commits](https://www.conventionalcommits.org/): `feat(builder): …`, `fix: …`, `docs: …`, `refactor: …`, `test: …`, `ci: …`, `build: …`, `chore: …`. Pull requests are squash-merged with their title, which a check enforces, and the changelog of each release is made from them by [git-cliff](https://git-cliff.org) (`cliff.toml`).
- **Branches** are named like the change: `feat/…`, `fix/…`, `docs/…`, `ci/…`, `chore/…`.
- **Versions** follow [semantic versioning](https://semver.org); while it is 0.x, a minor version may change the file format or behaviour. `internal/version` holds the version, and a test keeps the build files in step with it.
- **To release**, run **Actions → prepare-release**. It opens the pull request `chore(release): v0.3.0` with the version worked out from the commits and the release notes; merging it tags the release, and the release workflow tests, builds the Windows installer and the macOS app, and publishes them with `SHA256SUMS` and the notes. A version such as `0.3.0-beta.1` makes a pre-release. See [.github/releases/README.md](../.github/releases/README.md).
- **Release notes** start with highlights written by hand, with screenshots, in `.github/releases/<tag>.md`, followed by the changelog and how to install. Preview them with `scripts/release-notes.sh v0.2.0`; see [.github/releases/README.md](../.github/releases/README.md).

## Security

Report a vulnerability privately, as [SECURITY.md](../SECURITY.md) describes.
