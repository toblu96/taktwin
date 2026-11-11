package simapp

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// The updater takes the zipped app on macOS and the portable exe on Windows, from the files
// the release workflow publishes, never the disk image, the installer or the checksums.
func TestTheUpdaterPicksTheRightFile(t *testing.T) {
	var assets []github.ReleaseAsset
	for _, name := range []string{"SHA256SUMS", "Taktwin-0.2.0-macos-universal.dmg", "Taktwin-0.2.0-macos-universal.zip",
		"Taktwin-0.2.0-windows-x64-setup.exe", "Taktwin-0.2.0-windows-x64.exe"} {
		assets = append(assets, github.ReleaseAsset{Name: name})
	}
	for _, c := range []struct {
		platform, arch, want string
	}{
		{"darwin", "arm64", "Taktwin-0.2.0-macos-universal.zip"},
		{"darwin", "amd64", "Taktwin-0.2.0-macos-universal.zip"},
		{"windows", "amd64", "Taktwin-0.2.0-windows-x64.exe"},
		{"windows", "arm64", ""},
		{"linux", "amd64", ""},
	} {
		got := ""
		if i := matchAsset(updater.CheckRequest{Platform: c.platform, Arch: c.arch}, assets); i >= 0 {
			got = assets[i].Name
		}
		if got != c.want {
			t.Errorf("%s/%s: %q, want %q", c.platform, c.arch, got, c.want)
		}
	}
	if releasePage("0.2.0") != "https://github.com/toblu96/taktwin/releases/tag/v0.2.0" {
		t.Error(releasePage("0.2.0"))
	}
}
