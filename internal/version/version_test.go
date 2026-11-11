package version

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The build files say the same version as the app: what the installers, the macOS bundle and
// the Windows executable show is what the app knows itself as.
func TestTheBuildFilesHaveTheAppsVersion(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`).MatchString(Version) {
		t.Fatalf("%q is no semantic version", Version)
	}
	for file, pattern := range map[string]string{
		"build/config.yml":                   `(?m)^  version: "([^"]+)"`,
		"build/darwin/Info.plist":            `<key>CFBundle(?:Short)?Version(?:String)?</key>\s*<string>([^<]+)</string>`,
		"build/darwin/Info.dev.plist":        `<key>CFBundle(?:Short)?Version(?:String)?</key>\s*<string>([^<]+)</string>`,
		"build/windows/info.json":            `"(?:file_version|ProductVersion)": "([^"]+)"`,
		"build/windows/wails.exe.manifest":   `name="io\.github\.toblu96\.taktwin" version="([^"]+)"`,
		"build/windows/nsis/wails_tools.nsh": `!define INFO_PRODUCTVERSION "([^"]+)"`,
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		found := regexp.MustCompile(pattern).FindAllStringSubmatch(string(data), -1)
		if len(found) == 0 {
			t.Errorf("%s has no version", file)
		}
		for _, m := range found {
			// Windows takes only numbers, without a pre-release.
			core, _, _ := strings.Cut(Version, "-")
			if m[1] != Version && !(strings.HasPrefix(file, "build/windows/") && m[1] == core) {
				t.Errorf("%s says %s, the app %s: run scripts/set-version.sh %s", file, m[1], Version, Version)
			}
		}
	}
}
