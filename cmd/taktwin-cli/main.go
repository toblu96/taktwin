// Command taktwin-cli checks stations without a window.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/toblu96/taktwin/catalog"
	"github.com/toblu96/taktwin/internal/model"
	"github.com/toblu96/taktwin/internal/sim"
)

func main() {
	pkg := flag.String("validate", "", "validate a "+model.PackageExtension+" package, or a folder with a stations folder, and exit")
	collections := flag.String("collections", os.Getenv("TAKTWIN_COLLECTIONS"), "collections the stations build on besides the built-in ones, folders or "+model.PackageExtension+" files as a path list")
	flag.Parse()
	if *pkg == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := validate(*pkg, filepath.SplitList(*collections)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// validate loads every station in a package or folder against the modules of the given
// collections and the built-in ones, and prints what the station checks find in each.
func validate(path string, collections []string) error {
	layer, err := open(path)
	if err != nil {
		return err
	}
	stations := model.NewLibrary(layer).Stations()
	if len(stations) == 0 {
		return fmt.Errorf("%s contains no stations", path)
	}
	layers := []fs.FS{layer}
	for _, p := range collections {
		if p == "" {
			continue
		}
		c, err := open(p)
		if err != nil {
			return err
		}
		layers = append(layers, c)
	}
	lib := model.NewLibrary(append(layers, catalog.Reference, catalog.Base)...)
	for _, id := range stations {
		bundle, err := lib.Load(id)
		if err != nil {
			return err
		}
		plan, err := bundle.Resolve(nil)
		if err != nil {
			return err
		}
		fmt.Printf("ok    %s: %d modules\n", id, len(bundle.Station.Modules))
		for _, c := range sim.Check(plan) {
			fmt.Printf("check %s: %s\n", id, c.Text)
		}
	}
	return nil
}

// open opens a folder, or a package file, as a library layer.
func open(path string) (fs.FS, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return os.DirFS(path), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return model.ReadPackage(bytes.NewReader(data), int64(len(data)))
}
