package infra

import (
	"context"
	"embed"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GabiHert/phonon/internal/application/service"
	"github.com/GabiHert/phonon/internal/domain"
	"github.com/GabiHert/phonon/internal/infra/migrate"
	"github.com/GabiHert/phonon/internal/infra/paths"
	"github.com/GabiHert/phonon/internal/infra/store"
	"github.com/GabiHert/phonon/internal/infra/themes"
	"github.com/GabiHert/phonon/internal/infra/wails"
	"github.com/GabiHert/phonon/internal/integration/askipc"
	"github.com/GabiHert/phonon/internal/integration/audio"
	"github.com/GabiHert/phonon/internal/integration/audio/wavinfo"
	"github.com/GabiHert/phonon/internal/integration/claude"
	"github.com/GabiHert/phonon/internal/integration/git"
	"github.com/GabiHert/phonon/internal/integration/menubar"
	"github.com/GabiHert/phonon/internal/integration/process"
	"github.com/GabiHert/phonon/internal/integration/speech"
	"github.com/GabiHert/phonon/internal/integration/system"
	"github.com/GabiHert/phonon/internal/update"
	wailsv2 "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// helperPath is $PHONON_AUDIOCAP if set, else the helper next to the app binary.
func helperPath() string {
	if v := os.Getenv("PHONON_AUDIOCAP"); v != "" {
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
	// wails build runs this to list the bound methods: give that run a throwaway data folder.
	cleanup, err := isolateHome(generatingBindings)
	if err != nil {
		return err
	}
	defer cleanup()
	// Phonon was called Notted: bring Notted's data over once, before anything opens it.
	plan, on := migrate.Default()
	if err := migrateNotted(plan, on, system.Alert); err != nil {
		return err
	}
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
		log.Printf("forms: %v", err) // the app runs on; ask then reports "Phonon is not running"
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
	upd := update.New(update.Deps{
		Version: Version, AppName: "Phonon", Repo: "GabiHert/notted",
		Env:  claude.LoginShellEnv,
		Emit: emitter.Emit,
		Dir:  paths.UpdateDir(), LogPath: paths.UpdateLog(),
		Quit: emitter.Quit,
	})
	upd.CleanLeftovers()
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
	deleter := service.NewDeleteService(service.DeleteDeps{Store: st, Recorder: recording, Transcription: trans, Summary: summary, Review: review, Session: session, Forms: forms, SessionsDir: paths.SessionsDir(), AudioDir: paths.AudioDir})
	projects := service.NewProjectDeleteService(service.ProjectDeleteDeps{Store: st, Settings: st, Recorder: recording, Meetings: deleter, Git: git.New(gitEnv(claude.LoginShellEnv)), Emit: emitter})
	themeStore := &themes.Store{Dir: paths.ThemesDir()}
	app := wails.New(emitter, wails.Deps{Recording: recording, Transcription: trans, Archive: archive, Summary: summary, Session: session, Instructions: instructions, Setup: setup, Update: upd, Permissions: permissions, MicTest: micTest, Delete: deleter, Projects: projects, Meetings: st, Models: models, Forms: forms, Review: review, Visibility: vis, Themes: themeStore})

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
		Title:            "Phonon",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: windowBackground(themeStore.Selected()),
		Mac:              &mac.Options{TitleBar: mac.TitleBarHiddenInset()},
		// Closing the window hides it; the menu bar icon and a running recording stay.
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			emitter.SetContext(ctx)
			go upd.Watch(ctx, time.Minute, 6*time.Hour)
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

// windowBackground is the native window colour before the page paints, so a dark theme does not
// flash light on launch.
func windowBackground(t themes.Theme) *options.RGBA {
	out, _ := exec.Command("/usr/bin/defaults", "read", "-g", "AppleInterfaceStyle").Output()
	r, g, b := themes.WindowBackground(t, strings.TrimSpace(string(out)) == "Dark")
	return &options.RGBA{R: r, G: g, B: b, A: 255}
}
