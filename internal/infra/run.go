package infra

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/bhlabz/notted/internal/application/service"
	"github.com/bhlabz/notted/internal/domain"
	"github.com/bhlabz/notted/internal/infra/paths"
	"github.com/bhlabz/notted/internal/infra/store"
	"github.com/bhlabz/notted/internal/infra/wails"
	"github.com/bhlabz/notted/internal/integration/audio"
	"github.com/bhlabz/notted/internal/integration/audio/wavinfo"
	"github.com/bhlabz/notted/internal/integration/claude"
	"github.com/bhlabz/notted/internal/integration/git"
	"github.com/bhlabz/notted/internal/integration/process"
	"github.com/bhlabz/notted/internal/integration/speech"
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
	engine := speech.New(speech.Config{ModelsDir: paths.ModelsDir()})
	detector := speech.NewDetector(paths.ModelsDir())
	models := speech.NewModelManager(paths.ModelsDir(), engine, detector)
	trans := service.NewTranscriptionService(st, engine, detector, emitter, func(p string) ([]float32, int, error) {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, 0, err
		}
		return wavinfo.DecodeWAV(b)
	})
	archive := service.NewArchiveService(st, st, git.New(claude.LoginShellEnv), emitter)
	summary := service.NewSummaryService(st, st, claude.PromptSource{EnvFn: claude.LoginShellEnv}, claude.Runner{EnvFn: claude.LoginShellEnv}, archive, emitter)
	term := process.NewManager(emitter)
	session := service.NewSessionService(st, st, term, claude.LoginShellEnv, claude.CleanEnv, claude.LookPath)
	app := wails.New(emitter, wails.Deps{Recording: recording, Transcription: trans, Archive: archive, Summary: summary, Session: session, Meetings: st, Models: models})

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
			recording.SetChunkListener(trans.Enqueue)
			recording.SetStoppedListener(trans.MeetingStopped)
			models.SetOnReady(func(domain.Language) { trans.ResumeWaiting() })
			if err := trans.Start(ctx); err != nil {
				log.Printf("start transcription: %v", err)
			}
		},
		OnShutdown: func(ctx context.Context) {
			session.StopAll()
			recording.StopAll()
			trans.Close()
			engine.Unload()
			if err := st.Close(); err != nil {
				log.Printf("close store: %v", err)
			}
		},
		Bind: []interface{}{app},
	})
	return runErr
}
