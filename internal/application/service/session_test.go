package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/domain"
	"github.com/bhlabz/notted/internal/infra/store"
)

type startCall struct {
	key, dir string
	argv     []string
	env      []string
}

type fakeTerm struct {
	calls   []startCall
	running map[string]bool
	stopped []string
	resizes int
}

func (f *fakeTerm) Start(key, dir string, argv, env []string) error {
	f.calls = append(f.calls, startCall{key, dir, argv, env})
	if f.running == nil {
		f.running = map[string]bool{}
	}
	f.running[key] = true
	return nil
}
func (f *fakeTerm) Write(key, data string) error         { return nil }
func (f *fakeTerm) Resize(key string, c, r uint16) error { f.resizes++; return nil }
func (f *fakeTerm) Stop(key string) error {
	f.stopped = append(f.stopped, key)
	delete(f.running, key)
	return nil
}
func (f *fakeTerm) Running(key string) bool { return f.running[key] }
func (f *fakeTerm) StopAll()                { f.stopped = append(f.stopped, "*") }

type sessFixture struct {
	st   *store.Store
	term *fakeTerm
	svc  *SessionService
	ctx  context.Context
	root string
}

func cleanForTest(env []string) []string {
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, "ANTHROPIC_API_KEY=") && !strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN=") {
			out = append(out, e)
		}
	}
	return out
}

func newSessFixture(t *testing.T) *sessFixture {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &sessFixture{st: st, term: &fakeTerm{}, ctx: context.Background(), root: t.TempDir()}
	if err := st.SaveSettings(f.ctx, domain.Settings{Profile: "ana", CheckoutPath: f.root,
		ProjectRoots: map[string]string{"acme": "/work/acme"}}); err != nil {
		t.Fatal(err)
	}
	env := func() []string { return []string{"PATH=/x", "ANTHROPIC_API_KEY=k", "ANTHROPIC_AUTH_TOKEN=t"} }
	look := func(_ []string, name string) (string, error) { return "/bin/" + name, nil }
	f.svc = NewSessionService(st, st, f.term, env, cleanForTest, look)
	return f
}

func (f *sessFixture) meeting(t *testing.T, id, project, sid string) {
	t.Helper()
	m := domain.Meeting{ID: id, Title: "Sync", StartedAt: time.Now().UTC(), Language: domain.LangEN, State: domain.StateSummarized}
	if err := f.st.CreateMeeting(f.ctx, m); err != nil {
		t.Fatal(err)
	}
	if project != "" {
		_ = f.st.SetProjectKey(f.ctx, id, project)
	}
	if sid != "" {
		if err := f.st.SetClaudeSessionID(f.ctx, id, sid); err != nil {
			t.Fatal(err)
		}
	}
}

func eq(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func TestSessionStartArgvWithRoot(t *testing.T) {
	f := newSessFixture(t)
	f.meeting(t, "m1", "acme", "sid-1")
	if err := f.svc.Start(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	c := f.term.calls[0]
	if want := []string{"/bin/claude", "--resume", "sid-1", "--add-dir", "/work/acme"}; !eq(c.argv, want) {
		t.Fatalf("argv = %v", c.argv)
	}
	if c.key != "m1" || c.dir != f.root {
		t.Fatalf("key/dir = %q %q", c.key, c.dir)
	}
	for _, e := range c.env {
		if strings.HasPrefix(e, "ANTHROPIC_") {
			t.Fatalf("credential left: %s", e)
		}
	}
	skip, bypass := "skip-"+"permissions", "bypass"+"Permissions"
	for _, a := range c.argv {
		if strings.Contains(a, skip) || strings.Contains(a, bypass) {
			t.Fatalf("forbidden argument %q", a)
		}
	}
}

func TestSessionStartNoRootNoAddDir(t *testing.T) {
	f := newSessFixture(t)
	f.meeting(t, "m1", "globex", "sid-2")
	if err := f.svc.Start(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/bin/claude", "--resume", "sid-2"}; !eq(f.term.calls[0].argv, want) {
		t.Fatalf("argv = %v", f.term.calls[0].argv)
	}
}

func TestSessionStartRefusals(t *testing.T) {
	f := newSessFixture(t)
	wantErr(t, f.svc.Start(f.ctx, "nope"), "Meeting not found.")
	f.meeting(t, "m1", "acme", "")
	wantErr(t, f.svc.Start(f.ctx, "m1"), "This meeting has no Claude Code session yet. Generate the summary first.")
	f.meeting(t, "m2", "acme", "sid")
	if err := f.st.SaveSettings(f.ctx, domain.Settings{Profile: "ana", ProjectRoots: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	wantErr(t, f.svc.Start(f.ctx, "m2"), "Set the meetings archive folder in Settings > Instructions first.")
	_ = f.st.SaveSettings(f.ctx, domain.Settings{Profile: "ana", CheckoutPath: f.root, ProjectRoots: map[string]string{}})
	f.svc.lookPath = func([]string, string) (string, error) { return "", errors.New("x") }
	wantErr(t, f.svc.Start(f.ctx, "m2"), "Claude Code was not found. Install it and make sure the claude command runs in your terminal.")
	if len(f.term.calls) != 0 {
		t.Fatal("a refused start reached the terminal")
	}
}

func TestSessionWriteResizeStopWhenNotRunning(t *testing.T) {
	f := newSessFixture(t)
	wantErr(t, f.svc.Write("m1", "x"), "The session is not running.")
	wantErr(t, f.svc.Resize("m1", 80, 24), "The session is not running.")
	if err := f.svc.Stop("m1"); err != nil {
		t.Fatal(err)
	}
	f.meeting(t, "m1", "acme", "sid")
	_ = f.svc.Start(f.ctx, "m1")
	if err := f.svc.Write("m1", "x"); err != nil {
		t.Fatal(err)
	}
	f.svc.StopAll()
	if f.term.stopped[len(f.term.stopped)-1] != "*" {
		t.Fatal("StopAll not forwarded")
	}
}

func TestSessionResizeOutOfRangeIsSilentNoop(t *testing.T) {
	f := newSessFixture(t)
	f.meeting(t, "m1", "acme", "sid")
	_ = f.svc.Start(f.ctx, "m1")
	for _, sz := range [][2]int{{0, 24}, {80, 0}, {-1, 5}, {70000, 5}} {
		if err := f.svc.Resize("m1", sz[0], sz[1]); err != nil {
			t.Fatalf("Resize(%v) = %v", sz, err)
		}
	}
	if f.term.resizes != 0 {
		t.Fatalf("resizes = %d", f.term.resizes)
	}
}
