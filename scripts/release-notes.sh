#!/bin/sh
# Writes the notes of a release to stdout, as the release workflow publishes them: the
# highlights in .github/releases/<tag>.md if there are any, how to update and install from
# .github/releases/updating.md, and the changelog git-cliff makes from the conventional commits
# since the tag before. See .github/releases/README.md.
#
#   scripts/release-notes.sh v0.2.0 > notes.md
#
# Before the tag exists, it previews the notes of the commits not released yet.
set -eu
if [ $# -ne 1 ]; then
  echo "usage: scripts/release-notes.sh <tag>, such as scripts/release-notes.sh v0.2.0" >&2
  exit 2
fi
tag=$1
case "$tag" in v*) ;; *) tag=v$tag ;; esac
version=${tag#v}
repo=${GITHUB_REPOSITORY:-toblu96/taktwin}
cd "$(dirname "$0")/.."
images="https://raw.githubusercontent.com/$repo/$tag/.github/releases/images/"

if [ -f ".github/releases/$tag.md" ]; then
  sed -e "s#](images/#]($images#g" -e "s#src=\"images/#src=\"$images#g" ".github/releases/$tag.md"
  printf '\n\n'
fi

sed -e "s/{tag}/$tag/g" -e "s/{version}/$version/g" .github/releases/updating.md
printf '\n## 👉 Changelog\n\n'

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  previous=$(git describe --tags --abbrev=0 --match 'v[0-9]*' "$tag^" 2>/dev/null || true)
  # The first release has no tag before it: everything up to it, which --latest takes.
  range="--latest"
  [ -n "$previous" ] && range="$previous..$tag"
  unreleased=""
else
  previous=$(git describe --tags --abbrev=0 --match 'v[0-9]*' HEAD 2>/dev/null || true)
  range=""
  unreleased="--unreleased --tag $tag"
fi
[ -n "$previous" ] && printf '[compare changes](https://github.com/%s/compare/%s...%s)\n\n' "$repo" "$previous" "$tag"

if command -v git-cliff >/dev/null 2>&1; then cliff=git-cliff; else cliff="npx --yes git-cliff@2"; fi
# shellcheck disable=SC2086
$cliff --config cliff.toml --strip header $unreleased $range
