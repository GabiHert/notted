package infra

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/GabiHert/notted/internal/application/service"
	"github.com/GabiHert/notted/internal/domain"
	"github.com/GabiHert/notted/internal/infra/paths"
	"github.com/GabiHert/notted/internal/infra/store"
	"github.com/GabiHert/notted/internal/infra/wails"
	"github.com/GabiHert/notted/internal/integration/askipc"
	"github.com/GabiHert/notted/internal/integration/audio"
	"github.com/GabiHert/notted/internal/integration/audio/wavinfo"
	"github.com/GabiHert/notted/internal/integration/claude"
	"github.com/GabiHert/notted/internal/integration/git"
	"github.com/GabiHert/notted/internal/integration/install"
	"github.com/GabiHert/notted/internal/integration/menubar"
	"github.com/GabiHert/notted/internal/integration/process"
	"github.com/GabiHert/notted/internal/integration/speech"
	"github.com/GabiHert/notted/internal/integration/system"
	wailsv2 "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
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
	vis := &wails.Visibility{}
	notifier := wails.NewNotifier(emitter)
	exe, err := os.Executable()
	if err != nil {
		log.Printf("executable: %v", err)
		exe = ""
	}
	forms := service.NewFormService(service.FormDeps{Meetings: st, Emit: emitter, Notify: notifier, Visible: vis.Visible,
		SessionsDir: paths.SessionsDir(), Executable: exe, Socket: paths.AskSocket()})
	askServer, err := askipc.Listen(paths.AskSocket(), forms)
	if err != nil {
		log.Printf("forms: %v", err) // the app runs on; ask then reports "Notted is not running"
	}
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
	archive := service.NewArchiveService(st, st, git.New(gitEnv(claude.LoginShellEnv)), emitter)
	summary := service.NewSummaryService(st, st, claude.PromptSource{EnvFn: claude.LoginShellEnv}, claude.Runner{EnvFn: claude.LoginShellEnv}, archive, emitter)
	instructions := service.NewInstructionsService(st, claude.PromptSource{EnvFn: claude.LoginShellEnv}, archive.ListProjects)
	term := process.NewManager(emitter)
	setup := service.NewSetupService(st, git.New(gitEnv(claude.LoginShellEnv)))
	update := service.NewUpdateService(service.UpdateDeps{
		Source:        st,
		Git:           git.New(gitEnv(claude.LoginShellEnv)),
		Stream:        &process.Streamer{EnvFn: gitEnv(claude.LoginShellEnv)},
		Install:       &install.Installer{},
		Signing:       install.Signing{}.Present,
		Recording:     recording.Recording,
		Executable:    os.Executable,
		Pid:           os.Getpid,
		Quit:          emitter.Quit,
		LogPath:       paths.UpdateLog,
		InstallSource: paths.InstallSource,
		Emit:          emitter,
	})
	permChecker := audio.NewPermissions(helperPath())
	permissions := service.NewPermissionsService(permChecker, system.Opener{})
	micTest := service.NewMicTestService(audio.NewMicTester(helperPath()), permChecker, recording.Recording, emitter)
	session := service.NewSessionService(st, st, term, claude.LoginShellEnv, claude.SessionEnv, claude.LookPath)
	review := service.NewReviewService(service.ReviewDeps{Store: st, Settings: st, Mode: st, Runner: claude.Runner{EnvFn: claude.LoginShellEnv},
		Terminal: term, Summary: summary, MCPConfig: forms.SessionConfig, Emit: emitter})
	session.SetMCPConfig(forms.SessionConfig)
	session.SetBusyCheck(service.SessionBusyCheck(review, summary))
	summary.SetMCPConfig(forms.SessionConfig)
	summary.SetBusyCheck(service.SummaryBusyCheck(review))
	deleter := service.NewDeleteService(service.DeleteDeps{Store: st, Recorder: recording, Transcription: trans, Summary: summary, Review: review, Session: session, AudioDir: paths.AudioDir})
	app := wails.New(emitter, wails.Deps{Recording: recording, Transcription: trans, Archive: archive, Summary: summary, Session: session, Instructions: instructions, Setup: setup, Update: update, Permissions: permissions, MicTest: micTest, Delete: deleter, Meetings: st, Models: models, Forms: forms, Review: review, Visibility: vis})

	statusItem := menubar.New()
	menuBar := service.NewMenuBarService(service.MenuBarDeps{
		Recorder:   recording,
		Meetings:   st,
		Item:       statusItem,
		ShowWindow: emitter.Show,
		Emit:       emitter,
		Context:    emitter.Context,
	})

	runErr := wailsv2.Run(&options.App{
		Title:            "Notted",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 251, G: 251, B: 248, A: 255},
		Mac:              &mac.Options{TitleBar: mac.TitleBarHiddenInset()},
		// Closing the window hides it; the menu bar icon and a running recording stay.
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			emitter.SetContext(ctx)
			notifier.Start()
			if err := recording.RecoverOnStartup(ctx); err != nil {
				log.Printf("recover recordings: %v", err)
			}
			recording.SetChunkListener(trans.Enqueue)
			recording.SetStoppedListener(trans.MeetingStopped)
			menuBar.Attach()
			statusItem.Install(menubar.Handlers{Start: menuBar.Start, Stop: menuBar.Stop, Open: emitter.Show, Quit: emitter.Quit})
			models.SetOnReady(func(domain.Language) { trans.ResumeWaiting() })
			if err := trans.Start(ctx); err != nil {
				log.Printf("start transcription: %v", err)
			}
		},
		OnShutdown: func(ctx context.Context) {
			forms.CloseAll()
			if askServer != nil {
				_ = askServer.Close()
			}
			review.StopAll()
			session.StopAll()
			_ = micTest.Stop()
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

// gitEnv wraps an environment source for git so API credentials never reach git or its hooks.
func gitEnv(src func() []string) func() []string {
	return func() []string { return claude.CleanEnv(src()) }
}
