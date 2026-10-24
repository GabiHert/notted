package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memSource struct {
	mu   sync.Mutex
	path string
	err  error
	sets int
}

func (m *memSource) GetSourcePath(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.path, m.err
}

func (m *memSource) SetSourcePath(ctx context.Context, p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.path = p
	m.sets++
	return nil
}

type scriptGit struct {
	mu        sync.Mutex
	branch    string
	detached  bool
	status    string
	statusErr bool
	calls     [][]string
}

func (g *scriptGit) Run(ctx context.Context, repo string, args ...string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, append([]string{repo}, args...))
	switch args[0] {
	case "symbolic-ref":
		if g.detached {
			return "", errors.New("exit status 1")
		}
		return g.branch + "\n", nil
	case "status":
		if g.statusErr {
			return g.status, errors.New("exit status 128")
		}
		return g.status, nil
	}
	return "", fmt.Errorf("unexpected git %v", args)
}

func (g *scriptGit) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.calls)
}

type streamStep struct {
	lines   []string
	err     error
	block   bool
	makeApp bool
	hook    func()
}

type streamCall struct {
	dir      string
	argv     []string
	extraEnv []string
}

type fakeStream struct {
	mu    sync.Mutex
	steps map[string]streamStep
	calls []streamCall
}

func (f *fakeStream) set(name string, s streamStep) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps[name] = s
}

func (f *fakeStream) callList() []streamCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]streamCall{}, f.calls...)
}

func (f *fakeStream) Stream(ctx context.Context, dir string, argv []string, extraEnv []string, onLine func(string)) error {
	f.mu.Lock()
	f.calls = append(f.calls, streamCall{dir, append([]string{}, argv...), append([]string{}, extraEnv...)})
	st := f.steps[argv[0]]
	f.mu.Unlock()
	if st.hook != nil {
		st.hook()
	}
	for _, l := range st.lines {
		onLine(l)
	}
	if st.makeApp {
		if err := os.MkdirAll(filepath.Join(dir, "build", "bin", "Notted.app", "Contents", "MacOS"), 0o755); err != nil {
			return err
		}
	}
	if st.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return st.err
}

type installCall struct {
	pid              int
	newApp, app, log string
}

type fakeInstaller struct {
	mu    sync.Mutex
	calls []installCall
	err   error
}

func (f *fakeInstaller) Start(pid int, newApp, installedApp, logPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, installCall{pid, newApp, installedApp, logPath})
	return f.err
}

func (f *fakeInstaller) callList() []installCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]installCall{}, f.calls...)
}

func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func srcDir(t *testing.T) string {
	t.Helper()
	d := resolved(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(d, "wails.json"), []byte(`{"name": "Notted"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "Makefile"), []byte("build:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func appExeIn(t *testing.T, parent string) string {
	t.Helper()
	dir := filepath.Join(parent, "Notted.app", "Contents", "MacOS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "Notted")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func appExe(t *testing.T) string {
	t.Helper()
	return appExeIn(t, filepath.Join(resolved(t, t.TempDir()), "Apps"))
}

type uh struct {
	t           *testing.T
	svc         *UpdateService
	src         string
	exe         string
	source      *memSource
	git         *scriptGit
	stream      *fakeStream
	inst        *fakeInstaller
	emit        *recEmitter
	recording   atomic.Bool
	signing     atomic.Bool
	quits       atomic.Int32
	log         string
	installFile string
}

func newUH(t *testing.T) *uh {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	oldQuit := quitDelay
	quitDelay = 0
	t.Cleanup(func() { quitDelay = oldQuit })
	h := &uh{t: t, src: srcDir(t), exe: appExe(t), git: &scriptGit{branch: "main"}, inst: &fakeInstaller{}, emit: &recEmitter{},
		stream: &fakeStream{steps: map[string]streamStep{}}, log: "/tmp/notted-test/update.log"}
	h.source = &memSource{path: h.src}
	h.svc = NewUpdateService(UpdateDeps{
		Source:        h.source,
		Git:           h.git,
		Stream:        h.stream,
		Install:       h.inst,
		Signing:       func(ctx context.Context) bool { return h.signing.Load() },
		Recording:     h.recording.Load,
		Executable:    func() (string, error) { return h.exe, nil },
		Pid:           func() int { return 4242 },
		Quit:          func() { h.quits.Add(1) },
		LogPath:       func() string { return h.log },
		InstallSource: func() string { return h.installFile },
		Emit:          h.emit,
	})
	return h
}

func (h *uh) app() string {
	return filepath.Dir(filepath.Dir(filepath.Dir(resolved(h.t, h.exe))))
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitState(t *testing.T, svc *UpdateService, states ...string) UpdateStatus {
	t.Helper()
	var last UpdateStatus
	waitUntil(t, fmt.Sprintf("state %v (last %q)", states, last.State), func() bool {
		st, err := svc.GetStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		last = st
		for _, s := range states {
			if st.State == s {
				return true
			}
		}
		return false
	})
	return last
}

func TestSetSource(t *testing.T) {
	h := newUH(t)
	h.source.path = ""
	noWails := resolved(t, t.TempDir())
	otherName := srcDir(t)
	if err := os.WriteFile(filepath.Join(otherName, "wails.json"), []byte(`{"name": "Other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	noMake := srcDir(t)
	if err := os.Remove(filepath.Join(noMake, "Makefile")); err != nil {
		t.Fatal(err)
	}
	noGit := srcDir(t)
	if err := os.Remove(filepath.Join(noGit, ".git")); err != nil {
		t.Fatal(err)
	}
	notClone := "That folder is not a Notted source clone: "
	cases := []struct{ name, in, want string }{
		{"empty", "", "Choose your Notted source folder first."},
		{"blank", "  ", "Choose your Notted source folder first."},
		{"relative", "rel/x", "Use the full path of the folder, starting with /."},
		{"missing", "/no/such/notted-folder", "That folder does not exist: /no/such/notted-folder"},
		{"no wails.json", noWails, notClone + "wails.json is missing or does not name Notted."},
		{"other name", otherName, notClone + "wails.json is missing or does not name Notted."},
		{"no makefile", noMake, notClone + "Makefile is missing."},
		{"no git", noGit, "That folder is not a git clone: " + noGit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := h.svc.SetSource(context.Background(), c.in)
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if h.source.sets != 0 || h.source.path != "" {
				t.Fatalf("saved %q (%d sets)", h.source.path, h.source.sets)
			}
		})
	}
	if err := h.svc.SetSource(context.Background(), " "+h.src+"/ "); err != nil {
		t.Fatal(err)
	}
	if h.source.path != h.src {
		t.Fatalf("saved %q, want %q", h.source.path, h.src)
	}
	if h.git.callCount() != 0 {
		t.Fatal("SetSource ran git")
	}
}

func TestGetStatus(t *testing.T) {
	h := newUH(t)
	st, err := h.svc.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "idle" || st.Message != "" || st.Seq != 0 || st.Signing || st.SourcePath != h.src || st.InstalledApp != h.app() {
		t.Fatalf("status = %+v", st)
	}
	if st.Lines == nil || len(st.Lines) != 0 {
		t.Fatalf("lines = %#v", st.Lines)
	}
	h.signing.Store(true)
	h.exe = "/usr/local/bin/notted"
	st, _ = h.svc.GetStatus(context.Background())
	if st.InstalledApp != "" || !st.Signing {
		t.Fatalf("status = %+v", st)
	}
	h.source.err = errors.New("db down")
	if _, err := h.svc.GetStatus(context.Background()); err == nil || err.Error() != "db down" {
		t.Fatalf("err = %v", err)
	}
}

func TestStartRefusals(t *testing.T) {
	notRoot := os.Geteuid() != 0
	cases := []struct {
		name  string
		setup func(h *uh) string // returns the exact message
		git   bool               // the row reaches a git check
	}{
		{"no source", func(h *uh) string {
			h.source.path = ""
			return "Choose your Notted source folder first."
		}, false},
		{"invalid source", func(h *uh) string {
			h.source.path = "/no/such/notted-folder"
			return "That folder does not exist: /no/such/notted-folder"
		}, false},
		{"recording", func(h *uh) string {
			h.recording.Store(true)
			return "Stop the recording first. Notted quits and reopens to finish the update."
		}, false},
		{"not in a bundle", func(h *uh) string {
			h.exe = "/usr/local/bin/notted"
			return "Notted is not running from an app bundle, so it cannot replace itself. Run make build in your Notted source folder, copy build/bin/Notted.app to your Applications folder, and open that copy."
		}, false},
		{"app inside the source", func(h *uh) string {
			h.exe = appExeIn(h.t, filepath.Join(h.src, "build", "bin"))
			return "Notted is running from " + h.app() + ", inside its own source folder. Copy that app to your Applications folder, open the copy, and update from there."
		}, false},
		{"leftover old", func(h *uh) string {
			if err := os.Mkdir(h.app()+".old", 0o755); err != nil {
				h.t.Fatal(err)
			}
			return "A previous update left " + h.app() + ".old. Check that Notted opens, delete " + h.app() + ".old, then update again."
		}, false},
		{"unwritable", func(h *uh) string {
			if !notRoot {
				h.t.Skip("root can write anywhere")
			}
			dir := filepath.Dir(h.app())
			if err := os.Chmod(dir, 0o555); err != nil {
				h.t.Fatal(err)
			}
			h.t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
			return "Notted cannot write to " + dir + ", so it cannot replace " + h.app() + ". Move Notted to a folder you can write to, such as Applications."
		}, false},
		{"detached", func(h *uh) string {
			h.git.detached = true
			return "The Notted source folder is not on a branch. Switch it to main in Terminal, then update again."
		}, true},
		{"other branch", func(h *uh) string {
			h.git.branch = "feature-x"
			return "The Notted source folder is on feature-x, not main. Switch it to main in Terminal, then update again."
		}, true},
		{"status error", func(h *uh) string {
			h.git.statusErr = true
			h.git.status = "fatal: bad\n"
			return "git status failed in " + h.src + ":\nfatal: bad"
		}, true},
		{"tracked changes", func(h *uh) string {
			h.git.status = " M Makefile\n"
			return "The Notted source folder has uncommitted changes. Commit them, or undo them, in Terminal, then update again:\nM Makefile"
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newUH(t)
			want := c.setup(h)
			err := h.svc.Start(context.Background())
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v\nwant %q", err, want)
			}
			if !c.git && h.git.callCount() != 0 {
				t.Fatalf("git calls before the git checks: %v", h.git.calls)
			}
			time.Sleep(20 * time.Millisecond)
			if n := len(h.stream.callList()); n != 0 {
				t.Fatalf("%d stream calls", n)
			}
			if n := len(h.inst.callList()); n != 0 {
				t.Fatalf("%d installer calls", n)
			}
			st, _ := h.svc.GetStatus(context.Background())
			if st.State != "idle" || st.Message != "" || len(st.Lines) != 0 || st.Seq != 0 {
				t.Fatalf("status = %+v", st)
			}
			if len(h.emit.events) != 0 {
				t.Fatalf("events = %+v", h.emit.events)
			}
			// a refusal leaves the service ready for the next start
			h.git.detached, h.git.statusErr, h.git.status, h.git.branch = false, false, "", "main"
		})
	}
}

func TestStartSucceeds(t *testing.T) {
	h := newUH(t)
	h.stream.set("git", streamStep{lines: []string{"Updating 2a8d067..47d92f5", "Fast-forward"}})
	h.stream.set("make", streamStep{lines: []string{"built"}, makeApp: true})
	if err := h.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, h.svc, "installing")
	waitUntil(t, "quit", func() bool { return h.quits.Load() >= 1 })

	calls := h.stream.callList()
	want := []streamCall{{h.src, pullArgv, pullEnv}, {h.src, buildArgv, buildEnv}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("stream calls = %+v\nwant %+v", calls, want)
	}
	var states [][2]string
	for _, e := range h.emit.named("update:state") {
		states = append(states, [2]string{e.payload["state"].(string), e.payload["message"].(string)})
	}
	wantStates := [][2]string{{"pulling", ""}, {"building", ""}, {"installing", "Notted quits now and reopens when the new version is in place."}}
	if !reflect.DeepEqual(states, wantStates) {
		t.Fatalf("states = %v", states)
	}
	lines := []string{"$ git pull --ff-only --no-rebase --no-autostash", "Updating 2a8d067..47d92f5", "Fast-forward", "$ make build", "built"}
	outs := h.emit.named("update:output")
	if len(outs) != 5 {
		t.Fatalf("%d output events", len(outs))
	}
	for i, e := range outs {
		if e.payload["seq"] != i+1 || e.payload["line"] != lines[i] {
			t.Fatalf("event %d = %+v", i, e.payload)
		}
	}
	inst := h.inst.callList()
	if len(inst) != 1 || inst[0] != (installCall{4242, filepath.Join(h.src, "build", "bin", "Notted.app"), h.app(), h.log}) {
		t.Fatalf("installer calls = %+v", inst)
	}
	if h.quits.Load() != 1 {
		t.Fatalf("quits = %d", h.quits.Load())
	}
	st, _ := h.svc.GetStatus(context.Background())
	if !reflect.DeepEqual(st.Lines, lines) || st.Seq != 5 {
		t.Fatalf("status = %+v", st)
	}
}

func TestStartFailures(t *testing.T) {
	oldPull, oldBuild := pullTimeout, buildTimeout
	t.Cleanup(func() { pullTimeout, buildTimeout = oldPull, oldBuild })
	pullTimeout, buildTimeout = 50*time.Millisecond, 50*time.Millisecond
	buildFailed := "make build failed. Notted did not change. The output below shows why. The build runs without the network: if this update added dependencies, run make build once in Terminal in the Notted source folder, then update again."
	cases := []struct {
		name      string
		setup     func(h *uh)
		want      func(h *uh) string
		installer bool
	}{
		{"pull error", func(h *uh) { h.stream.set("git", streamStep{err: errors.New("exit status 1")}) },
			func(h *uh) string { return "git pull failed. Notted did not change. The output below shows why." }, false},
		{"pull timeout", func(h *uh) { h.stream.set("git", streamStep{block: true}) },
			func(h *uh) string { return "git pull did not finish within 5 minutes. Notted did not change." }, false},
		{"build error", func(h *uh) { h.stream.set("make", streamStep{err: errors.New("exit status 2")}) },
			func(h *uh) string { return buildFailed }, false},
		{"build timeout", func(h *uh) { h.stream.set("make", streamStep{block: true}) },
			func(h *uh) string { return "make build did not finish within 30 minutes. Notted did not change." }, false},
		{"app missing", func(h *uh) {},
			func(h *uh) string {
				return "make build finished, but " + h.src + "/build/bin/Notted.app is missing. Notted did not change."
			}, false},
		{"recording during build", func(h *uh) {
			h.stream.set("make", streamStep{makeApp: true, hook: func() { h.recording.Store(true) }})
		}, func(h *uh) string {
			return "The new Notted is built, but a recording is running. Stop the recording and click Update Notted again. Notted did not change."
		}, false},
		{"installer error", func(h *uh) {
			h.stream.set("make", streamStep{makeApp: true})
			h.inst.err = errors.New("boom")
		}, func(h *uh) string { return "Notted could not start the installer: boom. Notted did not change." }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newUH(t)
			c.setup(h)
			if err := h.svc.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			st := waitState(t, h.svc, "failed")
			if want := c.want(h); st.Message != want {
				t.Fatalf("message = %q\nwant %q", st.Message, want)
			}
			if n := len(h.inst.callList()); (n == 0) == c.installer {
				t.Fatalf("installer calls = %d", n)
			}
			time.Sleep(20 * time.Millisecond)
			if h.quits.Load() != 0 {
				t.Fatal("Quit was called")
			}
			h.recording.Store(false)
			if err := h.svc.Start(context.Background()); err != nil {
				t.Fatalf("second Start: %v", err)
			}
			waitState(t, h.svc, "failed")
		})
	}
}

func TestStartWhileRunning(t *testing.T) {
	old := pullTimeout
	t.Cleanup(func() { pullTimeout = old })
	pullTimeout = 300 * time.Millisecond
	h := newUH(t)
	h.stream.set("git", streamStep{block: true})
	if err := h.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	const busy = "An update is already running."
	if err := h.svc.Start(context.Background()); err == nil || err.Error() != busy {
		t.Fatalf("Start err = %v", err)
	}
	if err := h.svc.SetSource(context.Background(), h.src); err == nil || err.Error() != busy {
		t.Fatalf("SetSource err = %v", err)
	}
	if h.source.sets != 0 {
		t.Fatal("SetSource saved while running")
	}
	waitState(t, h.svc, "failed")
}

func TestNewRunClearsLines(t *testing.T) {
	h := newUH(t)
	h.stream.set("git", streamStep{lines: []string{"a"}, err: errors.New("exit status 1")})
	if err := h.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, h.svc, "failed")
	h.stream.set("git", streamStep{lines: []string{"b"}})
	h.stream.set("make", streamStep{lines: []string{"c"}, makeApp: true})
	if err := h.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitState(t, h.svc, "installing")
	waitUntil(t, "quit", func() bool { return h.quits.Load() >= 1 })
	st, _ := h.svc.GetStatus(context.Background())
	want := []string{"$ git pull --ff-only --no-rebase --no-autostash", "b", "$ make build", "c"}
	if !reflect.DeepEqual(st.Lines, want) || st.Seq != 2+4 {
		t.Fatalf("status = %+v", st)
	}
}

func TestLinesCap(t *testing.T) {
	h := newUH(t)
	var lines []string
	for i := 1; i <= 5003; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	h.stream.set("git", streamStep{lines: lines, err: errors.New("exit status 1")})
	if err := h.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := waitState(t, h.svc, "failed")
	if len(st.Lines) != 5000 || st.Lines[0] != "line 4" || st.Lines[4999] != "line 5003" || st.Seq != 5004 {
		t.Fatalf("len=%d first=%q last=%q seq=%d", len(st.Lines), st.Lines[0], st.Lines[len(st.Lines)-1], st.Seq)
	}
	if !strings.HasPrefix(st.Message, "git pull failed.") {
		t.Fatalf("message = %q", st.Message)
	}
}

type blockSource struct {
	*memSource
	entered chan struct{}
	release chan struct{}
}

func (b *blockSource) SetSourcePath(ctx context.Context, p string) error {
	close(b.entered)
	<-b.release
	return b.memSource.SetSourcePath(ctx, p)
}

func TestSetSourceRacingStart(t *testing.T) {
	h := newUH(t)
	oldSrc := h.src
	newSrc := srcDir(t)
	bs := &blockSource{memSource: h.source, entered: make(chan struct{}), release: make(chan struct{})}
	h.svc.d.Source = bs
	h.stream.set("make", streamStep{makeApp: true})

	setErr := make(chan error, 1)
	go func() { setErr <- h.svc.SetSource(context.Background(), newSrc) }()
	<-bs.entered

	startErr := make(chan error, 1)
	go func() { startErr <- h.svc.Start(context.Background()) }()
	time.Sleep(100 * time.Millisecond)
	close(bs.release)

	sErr, stErr := <-setErr, <-startErr
	if sErr != nil && sErr.Error() != "An update is already running." {
		t.Fatalf("SetSource err = %v", sErr)
	}
	if stErr == nil {
		waitUntil(t, "a pull", func() bool { return len(h.stream.callList()) > 0 })
		dir := h.stream.callList()[0].dir
		if sErr == nil && dir != newSrc {
			t.Fatalf("SetSource saved %s and returned nil, but the update pulled %s (old %s)", newSrc, dir, oldSrc)
		}
		waitUntil(t, "quit", func() bool { return h.quits.Load() >= 1 })
	}
}

func TestInstallSourceFallback(t *testing.T) {
	ctx := context.Background()
	write := func(h *uh, content string) {
		h.installFile = filepath.Join(t.TempDir(), "install-source")
		if err := os.WriteFile(h.installFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("file absent gives an empty source", func(t *testing.T) {
		h := newUH(t)
		h.source.path = ""
		h.installFile = filepath.Join(t.TempDir(), "missing")
		st, err := h.svc.GetStatus(ctx)
		if err != nil || st.SourcePath != "" {
			t.Fatalf("status = %+v, err = %v", st, err)
		}
		if err := h.svc.Start(ctx); err == nil || err != errNoSource {
			t.Fatalf("Start err = %v", err)
		}
	})
	t.Run("no install source dependency gives an empty source", func(t *testing.T) {
		h := newUH(t)
		h.source.path = ""
		h.installFile = ""
		st, _ := h.svc.GetStatus(ctx)
		if st.SourcePath != "" {
			t.Fatalf("source = %q", st.SourcePath)
		}
	})
	t.Run("a valid clone in the file is used", func(t *testing.T) {
		h := newUH(t)
		h.source.path = ""
		write(h, h.src+"\n")
		st, err := h.svc.GetStatus(ctx)
		if err != nil || st.SourcePath != h.src {
			t.Fatalf("status = %+v, err = %v", st, err)
		}
		if h.source.sets != 0 {
			t.Fatalf("source was copied into settings: %d sets", h.source.sets)
		}
		if err := h.svc.Start(ctx); err != nil {
			t.Fatalf("Start err = %v", err)
		}
		waitState(t, h.svc, "installing", "failed")
		if got := h.stream.callList(); len(got) == 0 || got[0].dir != h.src {
			t.Fatalf("pull did not run in the clone: %+v", got)
		}
		if h.source.sets != 0 {
			t.Fatalf("source was copied into settings: %d sets", h.source.sets)
		}
	})
	t.Run("an invalid path in the file is ignored", func(t *testing.T) {
		h := newUH(t)
		h.source.path = ""
		write(h, filepath.Join(t.TempDir(), "gone")+"\n")
		st, _ := h.svc.GetStatus(ctx)
		if st.SourcePath != "" {
			t.Fatalf("source = %q", st.SourcePath)
		}
		if err := h.svc.Start(ctx); err != errNoSource {
			t.Fatalf("Start err = %v", err)
		}
	})
	t.Run("the saved path wins", func(t *testing.T) {
		h := newUH(t)
		other := srcDir(t)
		h.source.path = other
		write(h, h.src+"\n")
		st, _ := h.svc.GetStatus(ctx)
		if st.SourcePath != other {
			t.Fatalf("source = %q, want %q", st.SourcePath, other)
		}
	})
}
