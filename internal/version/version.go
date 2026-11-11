// Package version is the version of the app, as releases are tagged: v0.1.0 is Version
// "0.1.0". scripts/set-version.sh changes it here and in the build files, which a test keeps
// in step, and the release workflow builds only a tag that matches it.
package version

// Version is the app's semantic version, without the v.
const Version = "0.1.0"
