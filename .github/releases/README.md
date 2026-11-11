# Release notes

`scripts/release-notes.sh` writes the notes of a release, and the release workflow publishes them. They are made of three parts:

1. **`<tag>.md`** here, such as `v0.2.0.md`, if there is one: news and highlights, written by hand. Start it with `## 👀 Highlights` and give each highlight a `###` heading. Put screenshots in `images/` and name them relatively, as `![The builder](images/v0.2.0-builder.png)`: the script points them at the files of the release's tag. GIFs and images uploaded to GitHub elsewhere work too.
2. **`updating.md`**: how to update and how to install, the same in every release. `{tag}` and `{version}` become `v0.2.0` and `0.2.0`.
3. **The changelog**, which git-cliff makes from the conventional commits since the tag before (see `cliff.toml`).

Preview the notes before tagging:

```sh
scripts/release-notes.sh v0.2.0 > /tmp/notes.md
```

The highlights file has to be on `main` before the tag, as the script reads it from the tagged commit.
