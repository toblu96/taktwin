package main

import (
	"embed"
	"log"

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
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Taktwin",
		Width:            1360,
		Height:           860,
		MinWidth:         1080,
		MinHeight:        700,
		BackgroundColour: application.NewRGB(11, 31, 34),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
