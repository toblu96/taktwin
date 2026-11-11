// Package catalog ships the models included with the app, as two library layers that come
// last, under any installed collection. Everything here is data; see README.md for the format.
package catalog

import (
	"embed"
	"io/fs"
)

//go:embed all:base all:reference
var files embed.FS

// Base holds what every collection can use: connector types, materials and shared parts.
// It is always there.
var Base = sub("base")

// Reference is the app's own collection of reference models, shown by default. It can be
// hidden in the settings like any other collection.
var Reference = sub("reference")

func sub(dir string) fs.FS {
	f, err := fs.Sub(files, dir)
	if err != nil {
		panic(err)
	}
	return f
}
