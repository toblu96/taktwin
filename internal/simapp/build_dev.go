//go:build !production

package simapp

// releaseBuild is false when the app runs from the sources, which an update must not replace.
const releaseBuild = false
