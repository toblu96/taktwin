package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

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
	application.RegisterEvent[string](simapp.MenuCommandEvent)
	application.RegisterEvent[[]string](simapp.FilesDroppedEvent)
	application.RegisterEvent[string](simapp.FileOpenedEvent)
	application.RegisterEvent[simapp.SaveState](simapp.SaveStateEvent)
}

// menu is the app's menu on a Mac, where every app has one: the app's menu with Settings, a
// File menu to import a project file and to close the project, and the Edit and Window
// menus, without which copy and paste would not work in the app's text fields. Elsewhere the
// window has no menu bar: everything is on the app's pages.
func menu(app *application.App) *application.Menu {
	command := func(name string) func(*application.Context) {
		return func(*application.Context) { app.Event.Emit(simapp.MenuCommandEvent, name) }
	}
	m := app.NewMenu()
	// The app menu as Wails' AppMenu role has it, with Settings below About as on a Mac.
	about := m.AddSubmenu("Taktwin")
	about.AddRole(application.About)
	about.AddSeparator()
	about.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(command("settings"))
	about.AddSeparator()
	about.AddRole(application.ServicesMenu)
	about.AddSeparator()
	about.AddRole(application.Hide)
	about.AddRole(application.HideOthers)
	about.AddRole(application.UnHide)
	about.AddSeparator()
	about.AddRole(application.Quit)
	file := m.AddSubmenu("File")
	file.Add("Import Project File…").SetAccelerator("CmdOrCtrl+O").OnClick(command("import"))
	file.Add("Close Project").SetAccelerator("CmdOrCtrl+W").OnClick(command("close"))
	m.AddRole(application.EditMenu)
	m.AddRole(application.WindowMenu)
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
			application.NewService(simapp.NewBuilderService(service)),
			application.NewService(simapp.NewProgramService(service)),
			application.NewService(simapp.NewUpdateService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// A project file double-clicked opens in the app; on a Mac the app's Info.plist and on
		// Windows its installer tie the files to it.
		FileAssociations: []string{simapp.ProjectExtension},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	if runtime.GOOS == "darwin" {
		app.Menu.Set(menu(app))
	}
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		service.Opened(e.Context().Filename())
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:              "Taktwin",
		UseApplicationMenu: true,
		Width:              1360,
		Height:             860,
		MinWidth:           1080,
		MinHeight:          700,
		BackgroundColour:   application.NewRGB(11, 31, 34),
		URL:                "/",
		// Project files dropped onto the projects page open there.
		EnableFileDrop: true,
	})
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		app.Event.Emit(simapp.FilesDroppedEvent, e.Context().DroppedFiles())
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
