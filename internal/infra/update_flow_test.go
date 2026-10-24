package infra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/application/service"
	"github.com/bhlabz/notted/internal/infra/store"
	"github.com/bhlabz/notted/internal/integration/git"
	"github.com/bhlabz/notted/internal/integration/process"
)

const flowMakefile = "build:\n\tmkdir -p build/bin/Notted.app/Contents/MacOS\n\ttouch build/bin/Notted.app/Contents/MacOS/Notted\n\t@echo built-ok\n"

type flowInstaller struct {
	mu    sync.Mutex
	calls [][]any
}

func (f *flowInstaller) Start(pid int, newApp, installedApp, logPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, []any{pid, newApp, installedApp, logPath})
	return nil
}

func (f *flowInstaller) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type flowEmitter struct {
	mu    sync.Mutex
	lines []string
}

func (e *flowEmitter) Emit(name string, payload any) {
	if name != "update:output" {
		return
	}
	m, _ := payload.(map[string]any)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lines = append(e.lines, m["line"].(string))
}

func (e *flowEmitter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.lines)
}

type flow struct {
	t     *testing.T
	svc   *service.UpdateService
	src   string
	seed  string
	app   string
	log   string
	inst  *flowInstaller
	emit  *flowEmitter
	quits atomic.Int32
}

func newFlow(t *testing.T, makefile string) *flow {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NOTTED_HOME", home)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitCfg := filepath.Join(base, "empty.gitconfig")
	if err := os.WriteFile(gitCfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	env := func() []string {
		return append(os.Environ(), "GIT_CONFIG_GLOBAL="+gitCfg, "GIT_CONFIG_NOSYSTEM=1")
	}
	remote := filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	src := filepath.Join(base, "src")
	sh(t, base, "init", "-q", "--bare", "-b", "main", remote)
	sh(t, base, "init", "-q", "-b", "main", seed)
	for name, text := range map[string]string{"wails.json": `{"name": "Notted"}`, ".gitignore": "build/\n", "Makefile": makefile} {
		if err := os.WriteFile(filepath.Join(seed, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh(t, seed, "add", "-A")
	f := &flow{t: t, src: src, seed: seed, inst: &flowInstaller{}, emit: &flowEmitter{}}
	f.commit(seed, "init")
	sh(t, seed, "remote", "add", "origin", remote)
	sh(t, seed, "push", "-q", "origin", "main")
	sh(t, base, "clone", "-q", remote, src)
	if err := os.WriteFile(filepath.Join(seed, "NEWS"), []byte("news\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, seed, "add", "-A")
	f.commit(seed, "news")
	sh(t, seed, "push", "-q", "origin", "main")

	exe := filepath.Join(base, "Apps", "Notted.app", "Contents", "MacOS", "Notted")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.app = filepath.Join(base, "Apps", "Notted.app")
	f.log = filepath.Join(home, "update.log")

	st, err := store.Open(filepath.Join(home, "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f.svc = service.NewUpdateService(service.UpdateDeps{
		Source:     st,
		Git:        git.New(env),
		Stream:     &process.Streamer{EnvFn: env},
		Install:    f.inst,
		Signing:    func(context.Context) bool { return false },
		Recording:  func() bool { return false },
		Executable: func() (string, error) { return exe, nil },
		Pid:        func() int { return 4242 },
		Quit:       func() { f.quits.Add(1) },
		LogPath:    func() string { return f.log },
		Emit:       f.emit,
	})
	if err := f.svc.SetSource(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *flow) commit(dir, msg string) {
	f.t.Helper()
	sh(f.t, dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", msg)
}

func (f *flow) waitState(want ...string) service.UpdateStatus {
	f.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var st service.UpdateStatus
	for time.Now().Before(deadline) {
		var err error
		st, err = f.svc.GetStatus(context.Background())
		if err != nil {
			f.t.Fatal(err)
		}
		for _, w := range want {
			if st.State == w {
				return st
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.t.Fatalf("timed out; status = %+v", st)
	return st
}

func contains(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func TestUpdateFlowPullsBuildsAndHandsOver(t *testing.T) {
	f := newFlow(t, flowMakefile)
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := f.waitState("installing", "failed")
	if st.State != "installing" {
		t.Fatalf("status = %+v", st)
	}
	if _, err := os.Stat(filepath.Join(f.src, "NEWS")); err != nil {
		t.Fatal("pull did not fast-forward:", err)
	}
	for _, want := range []string{"$ git pull --ff-only --no-rebase --no-autostash", "$ make build", "Fast-forward", "built-ok"} {
		if !contains(st.Lines, want) {
			t.Errorf("no line with %q in %q", want, st.Lines)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.quits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.quits.Load() != 1 {
		t.Fatalf("quits = %d", f.quits.Load())
	}
	f.inst.mu.Lock()
	defer f.inst.mu.Unlock()
	if len(f.inst.calls) != 1 {
		t.Fatalf("installer calls = %v", f.inst.calls)
	}
	c := f.inst.calls[0]
	if c[0] != 4242 || c[1] != filepath.Join(f.src, "build", "bin", "Notted.app") || c[2] != f.app || c[3] != f.log {
		t.Fatalf("installer call = %v", c)
	}
}

func TestUpdateFlowBuildFails(t *testing.T) {
	f := newFlow(t, "build:\n\t@echo boom\n\t@exit 2\n")
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := f.waitState("installing", "failed")
	want := "make build failed. Notted did not change. The output above shows why. The build runs without the network: if this update added dependencies, run make build once in Terminal in the Notted source folder, then update again."
	if st.State != "failed" || st.Message != want {
		t.Fatalf("status = %+v", st)
	}
	if !contains(st.Lines, "boom") {
		t.Errorf("no boom in %q", st.Lines)
	}
	time.Sleep(50 * time.Millisecond)
	if f.inst.count() != 0 || f.quits.Load() != 0 {
		t.Fatal("installer or Quit ran")
	}
}

func TestUpdateFlowDivergedPull(t *testing.T) {
	f := newFlow(t, flowMakefile)
	if err := os.WriteFile(filepath.Join(f.src, "LOCAL"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, f.src, "add", "-A")
	f.commit(f.src, "local")
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := f.waitState("installing", "failed")
	if st.State != "failed" || st.Message != "git pull failed. Notted did not change. The output above shows why." {
		t.Fatalf("status = %+v", st)
	}
	if _, err := os.Stat(filepath.Join(f.src, "build")); err == nil {
		t.Fatal("make ran")
	}
	if f.inst.count() != 0 {
		t.Fatal("installer ran")
	}
}

func TestUpdateFlowRefusesDirtyClone(t *testing.T) {
	f := newFlow(t, flowMakefile)
	if err := os.WriteFile(filepath.Join(f.src, "Makefile"), []byte("build:\n\t@echo changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := f.svc.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "M Makefile") || !strings.HasPrefix(err.Error(), "The Notted source folder has uncommitted changes.") {
		t.Fatalf("err = %v", err)
	}
	if f.emit.count() != 0 {
		t.Fatal("output emitted")
	}
	if _, err := os.Stat(filepath.Join(f.src, "NEWS")); err == nil {
		t.Fatal("NEWS appeared")
	}
}
