package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/toblu96/taktwin/internal/engine"
	"github.com/toblu96/taktwin/internal/simapp"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[engine.Snapshot](simapp.SnapshotEvent)
	application.RegisterEvent[engine.Snapshot](simapp.LibraryEvent)
	application.RegisterEvent[engine.Snapshot](simapp.BuilderEvent)
	application.RegisterEvent[simapp.CheckState](simapp.CheckEvent)
	application.RegisterEvent[string](simapp.ProjectCommandEvent)
}

// menu is the app's menu: a File menu to import a project file and to close the project, with
// the shortcuts of the system (CmdOrCtrl is ⌘ on a Mac and Ctrl elsewhere). Everything else
// about projects is on the app's start page. On a Mac it also has the app's, the Edit and the
// Window menu, without which copy and paste would not work in the app's text fields.
func menu(app *application.App) *application.Menu {
	m := app.NewMenu()
	if runtime.GOOS == "darwin" {
		m.AddRole(application.AppMenu)
	}
	command := func(name string) func(*application.Context) {
		return func(*application.Context) { app.Event.Emit(simapp.ProjectCommandEvent, name) }
	}
	file := m.AddSubmenu("File")
	file.Add("Import Project File…").SetAccelerator("CmdOrCtrl+O").OnClick(command("import"))
	file.Add("Close Project").SetAccelerator("CmdOrCtrl+W").OnClick(command("close"))
	if runtime.GOOS == "darwin" {
		m.AddRole(application.EditMenu)
		m.AddRole(application.WindowMenu)
	}
	return m
}

func main() {
	service, err := simapp.New()
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:        "Taktwin",
		Description: "PLC training simulator with a 3D digital twin of production stations",
		Services: []application.Service{
			application.NewService(service),
			application.NewService(simapp.NewLibraryService()),
			application.NewService(&simapp.SettingsService{}),
			application.NewService(simapp.NewBuilderService()),
			application.NewService(simapp.NewProgramService(service)),
			application.NewService(simapp.NewUpdateService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Menu.Set(menu(app))

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:              "Taktwin",
		UseApplicationMenu: true,
		Width:              1360,
		Height:             860,
		MinWidth:           1080,
		MinHeight:          700,
		BackgroundColour:   application.NewRGB(11, 31, 34),
		URL:                "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
