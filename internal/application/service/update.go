package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
)

// UpdateStatus is what the update card shows.
type UpdateStatus struct {
	SourcePath   string
	InstalledApp string // "" when Notted does not run from an app bundle
	Signing      bool
	State        string // "idle", "pulling", "building", "installing" or "failed"
	Message      string
	Lines        []string // the current run's output, at most maxUpdateLines; never nil
	Seq          int      // sequence number of the last line ever emitted; 0 before any
}

// UpdateDeps are what the update service calls.
type UpdateDeps struct {
	Source     port.SourceStore
	Git        port.Git
	Stream     port.LineStreamer
	Install    port.AppInstaller
	Signing    func(ctx context.Context) bool
	Recording  func() bool
	Executable func() (string, error)
	Pid        func() int
	Quit       func()
	LogPath    func() string
	Emit       port.EventEmitter
}

// UpdateService pulls the person's Notted clone, builds it and hands the new app to the installer.
type UpdateService struct {
	d     UpdateDeps
	mu    sync.Mutex
	busy  bool
	state string
	msg   string
	lines []string
	seq   int
}

var (
	pullTimeout  = 5 * time.Minute
	buildTimeout = 30 * time.Minute
	quitDelay    = 1500 * time.Millisecond
)

const (
	maxUpdateLines = 5000
	wOK            = 0x2 // access(2) write permission
)

var (
	pullArgv  = []string{"git", "pull", "--ff-only", "--no-rebase", "--no-autostash"}
	pullEnv   = []string{"GIT_TERMINAL_PROMPT=0"}
	buildArgv = []string{"make", "build"}
	buildEnv  = []string{"NO_COLOR=1", "TERM=dumb", "GOPROXY=off", "GOTOOLCHAIN=local", "npm_config_offline=true", "GIT_TERMINAL_PROMPT=0"}
)

var (
	errUpdateBusy = errors.New("An update is already running.")
	errNoSource   = errors.New("Choose your Notted source folder first.")
)

const notSourceClone = "That folder is not a Notted source clone: "

type updateJob struct{ src, app string }

// NewUpdateService builds the service in the idle state.
func NewUpdateService(d UpdateDeps) *UpdateService {
	return &UpdateService{d: d, state: "idle", lines: []string{}}
}

// GetStatus returns the source folder, the installed app, the signing check and the current run.
func (s *UpdateService) GetStatus(ctx context.Context) (UpdateStatus, error) {
	src, err := s.d.Source.GetSourcePath(ctx)
	if err != nil {
		return UpdateStatus{}, err
	}
	app, _ := s.installedApp()
	signing := s.d.Signing(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	return UpdateStatus{SourcePath: src, InstalledApp: app, Signing: signing, State: s.state, Message: s.msg,
		Lines: append([]string{}, s.lines...), Seq: s.seq}, nil
}

// SetSource checks the folder and saves it as the Notted source; it never runs git.
func (s *UpdateService) SetSource(ctx context.Context, path string) error {
	// Hold the lock throughout so a Start cannot begin between the busy check and the save.
	// Nothing below calls back into the service or emits an event.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return errUpdateBusy
	}
	p := strings.TrimSpace(path)
	if err := checkSource(p); err != nil {
		return err
	}
	return s.d.Source.SetSourcePath(ctx, filepath.Clean(p))
}

func checkSource(p string) error {
	if p == "" {
		return errNoSource
	}
	if !filepath.IsAbs(p) {
		return errors.New("Use the full path of the folder, starting with /.")
	}
	if !isDir(p) {
		return fmt.Errorf("That folder does not exist: %s", p)
	}
	var w struct {
		Name string `json:"name"`
	}
	b, err := os.ReadFile(filepath.Join(p, "wails.json"))
	if err != nil || json.Unmarshal(b, &w) != nil || w.Name != "Notted" {
		return errors.New(notSourceClone + "wails.json is missing or does not name Notted.")
	}
	if fi, err := os.Stat(filepath.Join(p, "Makefile")); err != nil || !fi.Mode().IsRegular() {
		return errors.New(notSourceClone + "Makefile is missing.")
	}
	if _, err := os.Lstat(filepath.Join(p, ".git")); err != nil {
		return fmt.Errorf("That folder is not a git clone: %s", p)
	}
	return nil
}

// installedApp returns the .app bundle Notted runs from.
func (s *UpdateService) installedApp() (string, bool) {
	exe, err := s.d.Executable()
	if err != nil {
		return "", false
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	i := strings.LastIndex(exe, ".app/Contents/MacOS/")
	if i < 0 {
		return "", false
	}
	return exe[:i+len(".app")], true
}

// Start checks that an update is safe, then pulls, builds and installs in the background.
func (s *UpdateService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return errUpdateBusy
	}
	s.busy = true
	s.mu.Unlock()

	j, err := s.preflight(ctx)
	if err != nil {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.lines = []string{}
	s.setStateLocked("pulling", "")
	s.mu.Unlock()
	go s.run(j)
	return nil
}

func (s *UpdateService) preflight(ctx context.Context) (updateJob, error) {
	src, err := s.d.Source.GetSourcePath(ctx)
	if err != nil {
		return updateJob{}, err
	}
	if src == "" {
		return updateJob{}, errNoSource
	}
	if err := checkSource(src); err != nil {
		return updateJob{}, err
	}
	if s.d.Recording() {
		return updateJob{}, errors.New("Stop the recording first. Notted quits and reopens to finish the update.")
	}
	app, ok := s.installedApp()
	if !ok {
		return updateJob{}, errors.New("Notted is not running from an app bundle, so it cannot replace itself. Run make build in your Notted source folder, copy build/bin/Notted.app to your Applications folder, and open that copy.")
	}
	srcReal := src
	if r, err := filepath.EvalSymlinks(src); err == nil {
		srcReal = r
	}
	if rel, err := filepath.Rel(srcReal, app); err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, "../"))) {
		return updateJob{}, fmt.Errorf("Notted is running from %s, inside its own source folder. Copy that app to your Applications folder, open the copy, and update from there.", app)
	}
	if _, err := os.Lstat(app + ".old"); err == nil {
		return updateJob{}, fmt.Errorf("A previous update left %s.old. Check that Notted opens, delete %s.old, then update again.", app, app)
	}
	if dir := filepath.Dir(app); syscall.Access(dir, wOK) != nil {
		return updateJob{}, fmt.Errorf("Notted cannot write to %s, so it cannot replace %s. Move Notted to a folder you can write to, such as Applications.", dir, app)
	}
	out, err := s.d.Git.Run(ctx, src, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return updateJob{}, errors.New("The Notted source folder is not on a branch. Switch it to main in Terminal, then update again.")
	}
	if b := strings.TrimSpace(out); b != "main" {
		return updateJob{}, fmt.Errorf("The Notted source folder is on %s, not main. Switch it to main in Terminal, then update again.", b)
	}
	out, err = s.d.Git.Run(ctx, src, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return updateJob{}, fmt.Errorf("git status failed in %s:\n%s", src, strings.TrimSpace(out))
	}
	if t := strings.TrimSpace(out); t != "" {
		return updateJob{}, fmt.Errorf("The Notted source folder has uncommitted changes. Commit them, or undo them, in Terminal, then update again:\n%s", t)
	}
	return updateJob{src: src, app: app}, nil
}

func (s *UpdateService) run(j updateJob) {
	s.out("$ git pull --ff-only --no-rebase --no-autostash")
	if timedOut, err := s.stream(j.src, pullTimeout, pullArgv, pullEnv); err != nil {
		if timedOut {
			s.fail("git pull did not finish within 5 minutes. Notted did not change.")
		} else {
			s.fail("git pull failed. Notted did not change. The output below shows why.")
		}
		return
	}
	s.setState("building", "")
	s.out("$ make build")
	if timedOut, err := s.stream(j.src, buildTimeout, buildArgv, buildEnv); err != nil {
		if timedOut {
			s.fail("make build did not finish within 30 minutes. Notted did not change.")
		} else {
			s.fail("make build failed. Notted did not change. The output below shows why. The build runs without the network: if this update added dependencies, run make build once in Terminal in the Notted source folder, then update again.")
		}
		return
	}
	newApp := filepath.Join(j.src, "build", "bin", "Notted.app")
	if !isDir(filepath.Join(newApp, "Contents", "MacOS")) {
		s.fail(fmt.Sprintf("make build finished, but %s is missing. Notted did not change.", newApp))
		return
	}
	if s.d.Recording() {
		s.fail("The new Notted is built, but a recording is running. Stop the recording and click Update Notted again. Notted did not change.")
		return
	}
	s.setState("installing", "Notted quits now and reopens when the new version is in place.")
	if err := s.d.Install.Start(s.d.Pid(), newApp, j.app, s.d.LogPath()); err != nil {
		s.fail(fmt.Sprintf("Notted could not start the installer: %v. Notted did not change.", err))
		return
	}
	time.Sleep(quitDelay)
	s.d.Quit()
}

// stream runs one command with a time limit and reports whether the limit ended it.
func (s *UpdateService) stream(dir string, limit time.Duration, argv, env []string) (timedOut bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	err = s.d.Stream.Stream(ctx, dir, argv, env, s.out)
	return errors.Is(ctx.Err(), context.DeadlineExceeded), err
}

func (s *UpdateService) out(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	s.lines = append(s.lines, line)
	for len(s.lines) > maxUpdateLines {
		s.lines = s.lines[1:]
	}
	s.d.Emit.Emit("update:output", map[string]any{"seq": s.seq, "line": line})
}

func (s *UpdateService) setStateLocked(st, m string) {
	s.state, s.msg = st, m
	s.d.Emit.Emit("update:state", map[string]any{"state": st, "message": m})
}

func (s *UpdateService) setState(st, m string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setStateLocked(st, m)
}

func (s *UpdateService) fail(m string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setStateLocked("failed", m)
	s.busy = false
}
