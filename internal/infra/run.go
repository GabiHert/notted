package infra

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/bhlabz/notted/internal/application/service"
	"github.com/bhlabz/notted/internal/infra/paths"
	"github.com/bhlabz/notted/internal/infra/store"
	"github.com/bhlabz/notted/internal/infra/wails"
	"github.com/bhlabz/notted/internal/integration/audio"
	"github.com/bhlabz/notted/internal/integration/audio/wavinfo"
	wailsv2 "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// helperPath is $NOTTED_AUDIOCAP if set, else the helper next to the app binary.
func helperPath() string {
	if v := os.Getenv("NOTTED_AUDIOCAP"); v != "" {
		return v
	}
	exe, err := os.Executable()
	if err != nil {
		return "audiocap"
	}
	return filepath.Join(filepath.Dir(exe), "audiocap")
}

// Run wires the app and starts the Wails window.
func Run(assets embed.FS) error {
	st, err := store.Open(paths.DBPath())
	if err != nil {
		return err
	}
	emitter := &wails.Emitter{}
	recording := service.NewRecordingService(st, audio.NewSupervisor(helperPath()), emitter, paths.AudioDir, wavinfo.Inspector{})
	app := wails.New(emitter, wails.Deps{Recording: recording, Meetings: st})

	runErr := wailsv2.Run(&options.App{
		Title:     "Notted",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			emitter.SetContext(ctx)
			if err := recording.RecoverOnStartup(ctx); err != nil {
				log.Printf("recover recordings: %v", err)
			}
		},
		OnShutdown: func(ctx context.Context) {
			recording.StopAll()
			if err := st.Close(); err != nil {
				log.Printf("close store: %v", err)
			}
		},
		Bind: []interface{}{app},
	})
	return runErr
}
