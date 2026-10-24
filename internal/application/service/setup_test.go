package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/domain"
)

type gitCall struct {
	repo string
	args []string
}

type fakeGit struct {
	calls   []gitCall
	mode    string // "", "fail", "noprompts", "wait", "hookfail"
	cloneFn func(repo, name string)
}

func (g *fakeGit) Run(ctx context.Context, repo string, args ...string) (string, error) {
	g.calls = append(g.calls, gitCall{repo, append([]string(nil), args...)})
	switch args[0] {
	case "clone":
		switch g.mode {
		case "fail":
			return "fatal: repository not found\n", os.ErrInvalid
		case "wait":
			<-ctx.Done()
			return "", ctx.Err()
		}
		name := args[len(args)-1]
		t := filepath.Join(repo, name)
		_ = os.MkdirAll(filepath.Join(t, "bin"), 0o755)
		_ = os.WriteFile(filepath.Join(t, "bin", "meeting"), []byte("#!/bin/sh\n"), 0o755)
		if g.mode != "noprompts" {
			_ = os.MkdirAll(filepath.Join(t, "prompts"), 0o755)
			_ = os.WriteFile(filepath.Join(t, "prompts", "base.md"), []byte("BASE"), 0o644)
			_ = os.MkdirAll(filepath.Join(t, "instructions"), 0o755)
		}
	case "config":
		if g.mode == "hookfail" {
			return "error: could not lock config\n", os.ErrInvalid
		}
	}
	return "", nil
}

func archiveDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	put(t, filepath.Join(d, "bin", "meeting"), "x")
	put(t, filepath.Join(d, "prompts", "base.md"), "BASE")
	put(t, filepath.Join(d, "instructions", "README.md"), "R")
	return d
}

func TestCheckArchive(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	if err := checkArchive(archiveDir(t)); err != nil {
		t.Fatal(err)
	}
	rel := "rel"
	wantErr(t, checkArchive(rel), "That folder does not exist: rel")
	d := t.TempDir()
	wantErr(t, checkArchive(d), "That folder is not a meetings archive checkout: bin/meeting is missing.")
	put(t, filepath.Join(d, "bin", "meeting"), "x")
	wantErr(t, checkArchive(d), "That folder is not a meetings archive checkout: prompts/base.md is missing.")
	put(t, filepath.Join(d, "prompts", "base.md"), "x")
	wantErr(t, checkArchive(d), "That folder is not a meetings archive checkout: instructions/ is missing.")
}

func TestSetSettingsChecksPrompts(t *testing.T) {
	f := newInsFixture(t)
	d := t.TempDir()
	put(t, filepath.Join(d, "bin", "meeting"), "x")
	wantErr(t, f.svc.SetSettings(f.ctx, domain.Settings{CheckoutPath: d}),
		"That folder is not a meetings archive checkout: prompts/base.md is missing.")
}

type setupFx struct {
	t    *testing.T
	root string
	set  *memSettings
	git  *fakeGit
	svc  *SetupService
	ctx  context.Context
}

func newSetupFx(t *testing.T, profile string) *setupFx {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	root := archiveDir(t)
	set := &memSettings{s: domain.Settings{Profile: profile, CheckoutPath: root, ProjectRoots: map[string]string{}}}
	g := &fakeGit{}
	return &setupFx{t: t, root: root, set: set, git: g, svc: NewSetupService(set, g), ctx: context.Background()}
}

func TestCloneArchiveAddressRefusals(t *testing.T) {
	f := newSetupFx(t, "")
	parent := t.TempDir()
	https := "https" + "://example.invalid/"
	wantErr(t, func() error { _, e := f.svc.CloneArchive(f.ctx, "  ", parent); return e }(), "Paste the archive's GitHub address first.")
	for _, a := range []string{"-c x", "a b", "ftp" + "://h/x", "-upload-pack=x", "https" + "://h/x\ny"} {
		_, err := f.svc.CloneArchive(f.ctx, a, parent)
		wantErr(t, err, "That is not a git address: "+strings.TrimSpace(a))
	}
	_, err := f.svc.CloneArchive(f.ctx, https, parent)
	wantErr(t, err, "That address does not end in a repository name: "+https)
	if len(f.git.calls) != 0 {
		t.Fatalf("git ran: %v", f.git.calls)
	}
}

func TestCloneArchiveDestination(t *testing.T) {
	f := newSetupFx(t, "")
	addr := "https" + "://example.invalid/acme/meetings.git"
	_, err := f.svc.CloneArchive(f.ctx, addr, " ")
	wantErr(t, err, "Choose the folder to clone into first.")
	_, err = f.svc.CloneArchive(f.ctx, addr, "rel")
	wantErr(t, err, "That folder does not exist: rel")
	missing := filepath.Join(t.TempDir(), "nope")
	_, err = f.svc.CloneArchive(f.ctx, addr, missing)
	wantErr(t, err, "That folder does not exist: "+missing)
	parent := t.TempDir()
	target := filepath.Join(parent, "meetings")
	put(t, filepath.Join(target, "x"), "x")
	_, err = f.svc.CloneArchive(f.ctx, addr, parent)
	wantErr(t, err, target+" already exists. Choose another folder, or choose the existing archive instead.")
	if len(f.git.calls) != 0 {
		t.Fatal("git ran")
	}
}

func TestCloneArchiveSuccessAndNames(t *testing.T) {
	cases := []struct{ addr, name string }{
		{"git@example.invalid:acme/meetings.git", "meetings"},
		{"file:///x/archive/", "archive"},
		{"https" + "://example.invalid/acme/meetings.git", "meetings"},
		{"ssh" + "://git@example.invalid/acme/notes", "notes"},
	}
	for _, c := range cases {
		f := newSetupFx(t, "")
		parent := t.TempDir()
		got, err := f.svc.CloneArchive(f.ctx, " "+c.addr+" ", parent)
		if err != nil {
			t.Fatalf("%s: %v", c.addr, err)
		}
		target := filepath.Join(parent, c.name)
		if got != target {
			t.Fatalf("got %q want %q", got, target)
		}
		want := []gitCall{
			{parent, []string{"clone", "--", c.addr, c.name}},
			{target, []string{"config", "core.hooksPath", ".githooks"}},
		}
		if !reflect.DeepEqual(f.git.calls, want) {
			t.Fatalf("calls %v", f.git.calls)
		}
	}
}

func TestCloneArchiveFailures(t *testing.T) {
	addr := "file:///x/meetings"
	f := newSetupFx(t, "")
	f.git.mode = "fail"
	_, err := f.svc.CloneArchive(f.ctx, addr, t.TempDir())
	wantErr(t, err, "git clone failed:\nfatal: repository not found")

	f = newSetupFx(t, "")
	f.git.mode = "hookfail"
	parent := t.TempDir()
	_, err = f.svc.CloneArchive(f.ctx, addr, parent)
	wantErr(t, err, "The archive was cloned to "+filepath.Join(parent, "meetings")+", but setting its git hooks failed:\nerror: could not lock config")

	f = newSetupFx(t, "")
	f.git.mode = "noprompts"
	parent = t.TempDir()
	_, err = f.svc.CloneArchive(f.ctx, addr, parent)
	wantErr(t, err, "That folder is not a meetings archive checkout: prompts/base.md is missing.")
	if _, serr := os.Stat(filepath.Join(parent, "meetings")); serr != nil {
		t.Fatal("clone should stay")
	}
}

func TestCloneArchiveTimeout(t *testing.T) {
	old := cloneTimeout
	cloneTimeout = 50 * time.Millisecond
	t.Cleanup(func() { cloneTimeout = old })
	f := newSetupFx(t, "")
	f.git.mode = "wait"
	_, err := f.svc.CloneArchive(f.ctx, "file:///x/meetings", t.TempDir())
	wantErr(t, err, "git clone did not finish within 10 minutes.")
}

func TestListProfiles(t *testing.T) {
	f := newSetupFx(t, "")
	ppl := filepath.Join(f.root, "instructions", "people")
	got, err := f.svc.ListProfiles(f.ctx)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %v %v", got, err)
	}
	for _, n := range []string{"bruno.md", "ana.md", "ana.local.md", "Bad.md", "notes.txt"} {
		put(t, filepath.Join(ppl, n), "x")
	}
	put(t, filepath.Join(ppl, "ana", "acme.md"), "x")
	put(t, filepath.Join(f.root, "secret.md"), "x")
	if err := os.Symlink(filepath.Join(f.root, "secret.md"), filepath.Join(ppl, "evil.md")); err != nil {
		t.Fatal(err)
	}
	got, err = f.svc.ListProfiles(f.ctx)
	if err != nil || !reflect.DeepEqual(got, []string{"ana", "bruno"}) {
		t.Fatalf("got %v %v", got, err)
	}
	f.set.s.CheckoutPath = ""
	_, err = f.svc.ListProfiles(f.ctx)
	wantErr(t, err, "Set the meetings archive folder in Settings > Instructions first.")
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCreateProfile(t *testing.T) {
	f := newSetupFx(t, "")
	got, err := f.svc.CreateProfile(f.ctx, " carla ")
	if err != nil || got != "instructions/people/carla.md" {
		t.Fatalf("%q %v", got, err)
	}
	want := "# Person: carla\n\n## Role\n\n## Style\n\n## Tools\n"
	p := filepath.Join(f.root, got)
	if read(t, p) != want {
		t.Fatalf("template %q", read(t, p))
	}
	put(t, p, "mine")
	_, err = f.svc.CreateProfile(f.ctx, "carla")
	wantErr(t, err, "instructions/people/carla.md already exists. Choose it from the list instead.")
	if read(t, p) != "mine" {
		t.Fatal("overwritten")
	}
	_, err = f.svc.CreateProfile(f.ctx, " ")
	wantErr(t, err, "Enter a name for the new profile.")
	_, err = f.svc.CreateProfile(f.ctx, "Ana Lima")
	wantErr(t, err, `Profile must use lowercase letters, digits and dashes, like "ana" or "ana-lima".`)
	f.set.s.CheckoutPath = ""
	_, err = f.svc.CreateProfile(f.ctx, "x")
	wantErr(t, err, "Set the meetings archive folder in Settings > Instructions first.")
}

func TestCreateProfileThroughLink(t *testing.T) {
	f := newSetupFx(t, "")
	out := t.TempDir()
	if err := os.Symlink(out, filepath.Join(f.root, "instructions", "people")); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.CreateProfile(f.ctx, "carla")
	wantErr(t, err, msgOutside)
	if es, _ := os.ReadDir(out); len(es) != 0 {
		t.Fatalf("outside changed: %v", es)
	}
}

func TestCreateProject(t *testing.T) {
	f := newSetupFx(t, "")
	_, err := f.svc.CreateProject(f.ctx, "acme")
	wantErr(t, err, "Set your profile in Settings > Instructions first.")
	f.set.s.Profile = "ana"
	_, err = f.svc.CreateProject(f.ctx, " ")
	wantErr(t, err, "Enter a key for the new project.")
	_, err = f.svc.CreateProject(f.ctx, "Acme Web")
	wantErr(t, err, `Project key must use lowercase letters, digits and dashes, like "acme" or "acme-web".`)

	got, err := f.svc.CreateProject(f.ctx, "acme")
	if err != nil || !reflect.DeepEqual(got, []string{"instructions/projects/acme.md", "instructions/people/ana/acme.md"}) {
		t.Fatalf("%v %v", got, err)
	}
	if read(t, filepath.Join(f.root, got[0])) != "# Project: acme\n\n## Key\nacme\n\n## Name\n\n## Aliases\n\n## About\n\n## Existing work\n" {
		t.Fatal("project template")
	}
	if read(t, filepath.Join(f.root, got[1])) != "# ana — acme\n\n## Project root\n\n## Tools\n" {
		t.Fatal("person-project template")
	}
	_, err = f.svc.CreateProject(f.ctx, "acme")
	wantErr(t, err, `Project "acme" and your file for it already exist.`)

	if err := os.Remove(filepath.Join(f.root, got[1])); err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.CreateProject(f.ctx, "acme")
	if err != nil || !reflect.DeepEqual(again, []string{got[1]}) {
		t.Fatalf("%v %v", again, err)
	}
}

func TestCreateProjectThroughLinks(t *testing.T) {
	f := newSetupFx(t, "ana")
	out := t.TempDir()
	if err := os.Symlink(out, filepath.Join(f.root, "instructions", "people")); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.CreateProject(f.ctx, "acme")
	wantErr(t, err, msgOutside)
	if es, _ := os.ReadDir(out); len(es) != 0 {
		t.Fatalf("outside changed: %v", es)
	}
	g := newSetupFx(t, "ana")
	if err := os.Symlink(filepath.Join(g.root, "prompts"), filepath.Join(g.root, "instructions", "projects")); err != nil {
		t.Fatal(err)
	}
	_, err = g.svc.CreateProject(g.ctx, "acme")
	wantErr(t, err, msgOutside)
	if read(t, filepath.Join(g.root, "prompts", "base.md")) != "BASE" {
		t.Fatal("base changed")
	}
	if es, _ := os.ReadDir(filepath.Join(g.root, "prompts")); len(es) != 1 {
		t.Fatalf("prompts changed: %v", es)
	}
}
