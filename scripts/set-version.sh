#!/bin/sh
# Sets the app's version, such as 0.2.0, in internal/version and the build files. Commit it as
# "chore(release): v0.2.0", then tag that commit v0.2.0 to release it.
set -eu
v=${1:?usage: scripts/set-version.sh 0.2.0}
v=${v#v}
echo "$v" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' || { echo "$v is no semantic version" >&2; exit 1; }
cd "$(dirname "$0")/.."
# Windows wants only numbers in the file version, the manifest and the installer: 0.2.0 of 0.2.0-beta.1.
core=${v%%-*}
sub() { perl -0pi -e "$1" "$2"; }
sub "s/const Version = \"[^\"]*\"/const Version = \"$v\"/" internal/version/version.go
sub "s/(\n  version: )\"[^\"]*\"/\$1\"$v\"/" build/config.yml
for f in build/darwin/Info.plist build/darwin/Info.dev.plist; do
  sub "s#(<key>CFBundle(?:Short)?Version(?:String)?</key>\s*<string>)[^<]*#\${1}$v#g" "$f"
done
sub "s/(\"file_version\": \")[^\"]*/\${1}$core/" build/windows/info.json
sub "s/(\"ProductVersion\": \")[^\"]*/\${1}$v/" build/windows/info.json
sub "s/(name=\"io\\.github\\.toblu96\\.taktwin\" version=\")[^\"]*/\${1}$core/" build/windows/wails.exe.manifest
sub "s/(!define INFO_PRODUCTVERSION \")[^\"]*/\${1}$core/" build/windows/nsis/wails_tools.nsh
echo "version $v"
