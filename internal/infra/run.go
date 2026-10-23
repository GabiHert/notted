package infra

import (
	"embed"

	"github.com/bhlabz/notted/internal/infra/wails"
	wailsv2 "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Run wires the app and starts the Wails window.
func Run(assets embed.FS) error {
	app := wails.New()
	return wailsv2.Run(&options.App{
		Title:     "Notted",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind:       []interface{}{app},
	})
}
