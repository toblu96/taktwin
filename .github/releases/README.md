# Releases

1. **Actions → prepare-release → Run workflow.** It works out the next version from the conventional commits since the last release (a feature makes 0.3.0 of 0.2.1, anything else 0.2.2), or takes the one you give, such as `0.3.0-beta.1`. It sets the version, brings `CHANGELOG.md` up to date, adds `<tag>.md` for the highlights, and opens the pull request `chore(release): v…` with the release notes in it.
2. **Write the highlights** in that file on the pull request's branch, if the release has any.
3. **Merge the pull request.** `release-tag` tags it, and the tag builds the app for Windows and macOS and publishes the release with these notes.

The workflows push with the secret `RELEASE_TOKEN`, since what GitHub's built-in token pushes starts no other workflows: the pull request's checks would not run and the tag would build nothing. It is a fine-grained personal access token for this repository only, with read and write access to **Contents** and **Pull requests**. Renew it before it expires.

# Release notes

`scripts/release-notes.sh` writes the notes of a release, and the release workflow publishes them. They are made of three parts:

1. **`<tag>.md`** here, such as `v0.2.0.md`, if there is one: news and highlights, written by hand. Start it with `## 👀 Highlights` and give each highlight a `###` heading. Put screenshots in `images/` and name them relatively, as `![The builder](images/v0.2.0-builder.png)`: the script points them at the files of the release's tag. GIFs and images uploaded to GitHub elsewhere work too.
2. **`updating.md`**: how to update and how to install, the same in every release. `{tag}` and `{version}` become `v0.2.0` and `0.2.0`.
3. **The changelog**, which git-cliff makes from the conventional commits since the tag before (see `cliff.toml`).

Preview the notes before tagging:

```sh
scripts/release-notes.sh v0.2.0 > /tmp/notes.md
```

The highlights file has to be in the release's commit, as the script reads it from the tagged commit.
