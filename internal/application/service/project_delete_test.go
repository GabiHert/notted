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
	"syscall"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
	"github.com/GabiHert/notted/internal/infra/store"
	"github.com/GabiHert/notted/internal/integration/git"
)

type gitSpy struct {
	mu    sync.Mutex
	calls [][]string
	inner *git.Git
}

func (g *gitSpy) Run(ctx context.Context, repo string, args ...string) (string, error) {
	g.mu.Lock()
	g.calls = append(g.calls, append([]string(nil), args...))
	g.mu.Unlock()
	return g.inner.Run(ctx, repo, args...)
}

type pdFixture struct {
	t       *testing.T
	archive string
	st      *store.Store
	audio   string
	active  *fakeActive
	em      *recEmitter
	spy     *gitSpy
	svc     *ProjectDeleteService
	ctx     context.Context
}

func newPDFixture(t *testing.T) *pdFixture {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	archive := t.TempDir()
	for _, rel := range []string{
		"instructions/projects/acme.md", "instructions/projects/globex.md",
		"instructions/people/ana/acme.md", "instructions/people/bruno/acme.md",
		"meetings/acme/2026/2026-10-01-sync/meeting.md", "meetings/acme/2026/2026-10-01-sync/summary.md",
		"meetings/acme/2026/2026-10-08-review/meeting.md",
		"meetings/globex/2026/2026-10-02-kickoff/meeting.md",
	} {
		put(t, filepath.Join(archive, filepath.FromSlash(rel)), rel+"\n")
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.SaveSettings(ctx, domain.Settings{Profile: "ana", CheckoutPath: archive, ProjectRoots: map[string]string{"acme": "/work/acme", "globex": "/work/globex"}}); err != nil {
		t.Fatal(err)
	}
	f := &pdFixture{t: t, archive: archive, st: st, audio: t.TempDir(), active: &fakeActive{}, em: &recEmitter{}, ctx: ctx,
		spy: &gitSpy{inner: git.New(os.Environ)}}
	stop := &fakeStopper{}
	del := NewDeleteService(DeleteDeps{Store: st, Recorder: f.active, Transcription: stop, Summary: stop, Session: stop,
		AudioDir: func(id string) string { return filepath.Join(f.audio, id) }})
	f.svc = NewProjectDeleteService(ProjectDeleteDeps{Store: st, Settings: st, Recorder: f.active, Meetings: del, Git: f.spy, Emit: f.em})
	f.meeting("m1", "acme", "meetings/acme/2026/2026-10-01-sync/", domain.StateSummarized)
	f.meeting("m2", "acme", "", domain.StateReady)
	f.meeting("m3", "globex", "meetings/globex/2026/2026-10-02-kickoff/", domain.StateSummarized)
	return f
}

func (f *pdFixture) meeting(id, key, folder string, state domain.MeetingState) {
	f.t.Helper()
	m := domain.Meeting{ID: id, Title: "Sync " + id, StartedAt: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC), Language: domain.LangEN, State: state}
	if err := f.st.CreateMeeting(f.ctx, m); err != nil {
		f.t.Fatal(err)
	}
	if err := f.st.SetProjectKey(f.ctx, id, key); err != nil {
		f.t.Fatal(err)
	}
	if folder != "" {
		if err := f.st.SetFolder(f.ctx, id, folder); err != nil {
			f.t.Fatal(err)
		}
	}
	put(f.t, filepath.Join(f.audio, id, "mic-000001.wav"), "wav")
}

func (f *pdFixture) abs(rel string) string { return filepath.Join(f.archive, filepath.FromSlash(rel)) }

func (f *pdFixture) exists(rel string) bool {
	_, err := os.Lstat(f.abs(rel))
	return err == nil
}

func (f *pdFixture) hasMeeting(id string) bool {
	_, err := f.st.GetMeeting(f.ctx, id)
	return err == nil
}

func (f *pdFixture) plan(key string) domain.ProjectDeletePlan {
	f.t.Helper()
	p, err := f.svc.DeletePlan(f.ctx, key)
	if err != nil {
		f.t.Fatalf("DeletePlan: %v", err)
	}
	return p
}

func (f *pdFixture) roots() map[string]string {
	st, _ := f.st.GetSettings(f.ctx)
	return st.ProjectRoots
}

func TestProjectDeletePlanFull(t *testing.T) {
	f := newPDFixture(t)
	before := snapshot(t, f.archive)
	p := f.plan("acme")
	wantFiles := []string{"instructions/projects/acme.md", "instructions/people/ana/acme.md",
		"meetings/acme/2026/2026-10-01-sync/meeting.md", "meetings/acme/2026/2026-10-01-sync/summary.md", "meetings/acme/2026/2026-10-08-review/meeting.md"}
	wantFolders := []string{"meetings/acme/2026/2026-10-01-sync/", "meetings/acme/2026/2026-10-08-review/", "meetings/acme/2026/", "meetings/acme/"}
	if !reflect.DeepEqual(p.Files, wantFiles) || !reflect.DeepEqual(p.Folders, wantFolders) {
		t.Fatalf("plan = %+v", p)
	}
	if p.Key != "acme" || p.LocalMeetings != 2 || p.UnsavedMeetings != 1 || p.Recording {
		t.Fatalf("plan = %+v", p)
	}
	if !reflect.DeepEqual(before, snapshot(t, f.archive)) {
		t.Fatal("the plan changed the archive")
	}
	if len(f.spy.calls) != 0 {
		t.Fatalf("git calls: %v", f.spy.calls)
	}
}

func TestProjectDeletePlanOmitsAbsentItems(t *testing.T) {
	f := newPDFixture(t)
	if err := os.RemoveAll(f.abs("instructions/people/ana")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(f.abs("meetings/acme")); err != nil {
		t.Fatal(err)
	}
	p := f.plan("acme")
	if !reflect.DeepEqual(p.Files, []string{"instructions/projects/acme.md"}) || p.Folders == nil || len(p.Folders) != 0 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestProjectDeletePlanEmptyProfileSkipsPersonalFile(t *testing.T) {
	f := newPDFixture(t)
	st, _ := f.st.GetSettings(f.ctx)
	st.Profile = ""
	if err := f.st.SaveSettings(f.ctx, st); err != nil {
		t.Fatal(err)
	}
	for _, rel := range f.plan("acme").Files {
		if strings.HasPrefix(rel, "instructions/people/") {
			t.Fatalf("listed %s", rel)
		}
	}
}

func TestProjectDeletePlanRefusals(t *testing.T) {
	f := newPDFixture(t)
	for _, k := range []string{"", "  "} {
		_, err := f.svc.DeletePlan(f.ctx, k)
		wantErr(t, err, "Choose a project to delete.")
	}
	for _, k := range []string{"../x", "Acme", "a/b"} {
		_, err := f.svc.DeletePlan(f.ctx, k)
		wantErr(t, err, errProjectKey.Error())
	}
	_, err := f.svc.DeletePlan(f.ctx, "initech")
	wantErr(t, err, `There is nothing to delete for project "initech".`)
	st, _ := f.st.GetSettings(f.ctx)
	st.CheckoutPath = ""
	if err := f.st.SaveSettings(f.ctx, st); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.DeletePlan(f.ctx, "acme")
	wantErr(t, err, errNoArchive.Error())
}

func TestProjectDeletePlanListsWhatRemains(t *testing.T) {
	f := newPDFixture(t)
	if err := os.Remove(f.abs("instructions/projects/acme.md")); err != nil {
		t.Fatal(err)
	}
	p := f.plan("acme")
	if len(p.Files) != 4 || p.LocalMeetings != 2 || p.Files[0] != "instructions/people/ana/acme.md" {
		t.Fatalf("plan = %+v", p)
	}
}

func TestProjectDeleteHappyPath(t *testing.T) {
	f := newPDFixture(t)
	p := f.plan("acme")
	res, err := f.svc.Delete(f.ctx, "acme", p)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if res.DeletedMeetings != 2 || res.TrackedRemoved == nil || len(res.TrackedRemoved) != 0 {
		t.Fatalf("result = %+v", res)
	}
	for _, rel := range append(append([]string{}, p.Files...), p.Folders...) {
		if f.exists(strings.TrimSuffix(rel, "/")) {
			t.Fatalf("%s still there", rel)
		}
	}
	for _, rel := range []string{"instructions/people/bruno/acme.md", "instructions/projects/globex.md", "meetings/globex/2026/2026-10-02-kickoff/meeting.md"} {
		if !f.exists(rel) {
			t.Fatalf("%s was removed", rel)
		}
	}
	if f.hasMeeting("m1") || f.hasMeeting("m2") || !f.hasMeeting("m3") {
		t.Fatal("wrong meetings left")
	}
	for id, want := range map[string]bool{"m1": false, "m2": false, "m3": true} {
		_, err := os.Stat(filepath.Join(f.audio, id))
		if (err == nil) != want {
			t.Fatalf("audio of %s present=%v", id, err == nil)
		}
	}
	if !reflect.DeepEqual(f.roots(), map[string]string{"globex": "/work/globex"}) {
		t.Fatalf("roots = %v", f.roots())
	}
	if len(f.em.events) != 3 || f.em.events[0].name != "meeting:deleted" || f.em.events[1].name != "meeting:deleted" ||
		f.em.events[2].name != "project:deleted" || f.em.events[2].payload["key"] != "acme" {
		t.Fatalf("events = %+v", f.em.events)
	}
	ids := map[any]bool{f.em.events[0].payload["meetingId"]: true, f.em.events[1].payload["meetingId"]: true}
	if !ids["m1"] || !ids["m2"] {
		t.Fatalf("events = %+v", f.em.events)
	}
}

func TestProjectDeleteTrackedRemoved(t *testing.T) {
	f := newPDFixture(t)
	p := f.plan("acme")
	if _, err := f.svc.Delete(f.ctx, "acme", p); err != nil {
		t.Fatal(err)
	}
	if len(f.spy.calls) != 1 || f.spy.calls[0][0] != "ls-files" {
		t.Fatalf("git calls = %v", f.spy.calls)
	}

	g := newPDFixture(t)
	runGit(t, g.archive, "init", "-q", "-b", "main")
	runGit(t, g.archive, "config", "user.name", "Test")
	runGit(t, g.archive, "config", "user.email", "test@example.com")
	untracked := "meetings/acme/2026/2026-10-08-review/meeting.md"
	runGit(t, g.archive, "add", "-A")
	runGit(t, g.archive, "rm", "-q", "--cached", untracked)
	runGit(t, g.archive, "commit", "-q", "-m", "init")
	gp := g.plan("acme")
	res, err := g.svc.Delete(g.ctx, "acme", gp)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"instructions/projects/acme.md", "instructions/people/ana/acme.md", "meetings/acme/2026/2026-10-01-sync/meeting.md", "meetings/acme/2026/2026-10-01-sync/summary.md"}
	if !reflect.DeepEqual(res.TrackedRemoved, want) {
		t.Fatalf("TrackedRemoved = %v", res.TrackedRemoved)
	}
	if len(g.spy.calls) != 1 || strings.Join(g.spy.calls[0][:3], " ") != "ls-files -z --" {
		t.Fatalf("git calls = %v", g.spy.calls)
	}
	if out := runGit(t, g.archive, "diff", "--cached", "--name-only"); out != "" {
		t.Fatalf("something is staged: %s", out)
	}
	if out := runGit(t, g.archive, "diff", "--name-only"); !strings.Contains(out, "instructions/projects/acme.md") {
		t.Fatalf("deletions are not in the working tree diff: %q", out)
	}
	if out := runGit(t, g.archive, "log", "--oneline"); strings.Count(out, "\n") != 0 {
		t.Fatalf("extra commit: %s", out)
	}
}

func TestProjectDeleteMissingRootEntry(t *testing.T) {
	f := newPDFixture(t)
	st, _ := f.st.GetSettings(f.ctx)
	delete(st.ProjectRoots, "acme")
	if err := f.st.SaveSettings(f.ctx, st); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Delete(f.ctx, "acme", f.plan("acme")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.roots(), map[string]string{"globex": "/work/globex"}) {
		t.Fatalf("roots = %v", f.roots())
	}
}

func TestProjectDeleteRefusedWhileRecording(t *testing.T) {
	for name, arrange := range map[string]func(f *pdFixture){
		"state": func(f *pdFixture) {
			if err := f.st.UpdateState(f.ctx, "m2", domain.StateRecording); err != nil {
				f.t.Fatal(err)
			}
		},
		"active": func(f *pdFixture) { f.active.id = "m1" },
	} {
		t.Run(name, func(t *testing.T) {
			f := newPDFixture(t)
			arrange(f)
			p := f.plan("acme")
			if !p.Recording {
				t.Fatal("plan.Recording is false")
			}
			before := snapshot(t, f.archive)
			_, err := f.svc.Delete(f.ctx, "acme", p)
			wantErr(t, err, "Stop the recording before you delete this project.")
			if !reflect.DeepEqual(before, snapshot(t, f.archive)) || !f.hasMeeting("m1") || !f.hasMeeting("m2") {
				t.Fatal("something was removed")
			}
		})
	}
}

func TestProjectDeleteRefusedWhenPlanChanged(t *testing.T) {
	const msg = `Project "acme" changed since you reviewed it. Review the list again before deleting.`
	f := newPDFixture(t)
	p := f.plan("acme")
	put(t, f.abs("meetings/acme/2026/2026-10-01-sync/extra.md"), "x")
	before := snapshot(t, f.archive)
	_, err := f.svc.Delete(f.ctx, "acme", p)
	wantErr(t, err, msg)
	if !reflect.DeepEqual(before, snapshot(t, f.archive)) || !f.hasMeeting("m1") {
		t.Fatal("something was removed")
	}

	g := newPDFixture(t)
	gp := g.plan("acme")
	g.meeting("m4", "acme", "", domain.StateReady)
	_, err = g.svc.Delete(g.ctx, "acme", gp)
	wantErr(t, err, msg)
	if !g.hasMeeting("m1") || !g.hasMeeting("m4") {
		t.Fatal("a meeting was removed")
	}
}

func TestProjectDeleteRefusesLinksAndSpecialFiles(t *testing.T) {
	outsideMsg := "That path is outside the meetings archive folder."
	t.Run("file link", func(t *testing.T) {
		f := newPDFixture(t)
		out := filepath.Join(t.TempDir(), "secret.md")
		put(t, out, "keep")
		if err := os.Symlink(out, f.abs("meetings/acme/2026/link.md")); err != nil {
			t.Fatal(err)
		}
		f.refused(t, "acme", outsideMsg)
		if b, _ := os.ReadFile(out); string(b) != "keep" {
			t.Fatal("outside file touched")
		}
	})
	t.Run("folder link", func(t *testing.T) {
		f := newPDFixture(t)
		out := t.TempDir()
		put(t, filepath.Join(out, "x.md"), "keep")
		if err := os.RemoveAll(f.abs("meetings/acme")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(out, f.abs("meetings/acme")); err != nil {
			t.Fatal(err)
		}
		f.refused(t, "acme", outsideMsg)
		if _, err := os.Stat(filepath.Join(out, "x.md")); err != nil {
			t.Fatal("outside file touched")
		}
	})
	t.Run("project file link", func(t *testing.T) {
		f := newPDFixture(t)
		out := filepath.Join(t.TempDir(), "p.md")
		put(t, out, "keep")
		if err := os.Remove(f.abs("instructions/projects/acme.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(out, f.abs("instructions/projects/acme.md")); err != nil {
			t.Fatal(err)
		}
		f.refused(t, "acme", outsideMsg)
		if _, err := os.Stat(out); err != nil {
			t.Fatal("outside file touched")
		}
	})
	t.Run("fifo", func(t *testing.T) {
		f := newPDFixture(t)
		if err := syscall.Mkfifo(f.abs("meetings/acme/2026/pipe"), 0o600); err != nil {
			t.Fatal(err)
		}
		f.refused(t, "acme", "Notted cannot delete meetings/acme/2026/pipe: it is not a regular file.")
	})
	t.Run("meetings key is a file", func(t *testing.T) {
		f := newPDFixture(t)
		if err := os.RemoveAll(f.abs("meetings/acme")); err != nil {
			t.Fatal(err)
		}
		put(t, f.abs("meetings/acme"), "x")
		f.refused(t, "acme", "Notted cannot delete meetings/acme: it is not a folder.")
	})
}

// refused checks that the plan refuses and, given any plan, so does Delete, with nothing removed.
func (f *pdFixture) refused(t *testing.T, key, msg string) {
	t.Helper()
	before := snapshot(t, f.archive)
	_, err := f.svc.DeletePlan(f.ctx, key)
	wantErr(t, err, msg)
	_, err = f.svc.Delete(f.ctx, key, domain.ProjectDeletePlan{Key: key})
	wantErr(t, err, msg)
	if !reflect.DeepEqual(before, snapshot(t, f.archive)) || !f.hasMeeting("m1") || !f.hasMeeting("m2") {
		t.Fatal("something was removed")
	}
}

func TestProjectDeletePartialFailureThenRetry(t *testing.T) {
	f := newPDFixture(t)
	dir := f.abs("meetings/acme/2026/2026-10-08-review")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := f.svc.Delete(f.ctx, "acme", f.plan("acme"))
	if err == nil || !strings.HasSuffix(err.Error(), " Delete the project again to finish.") {
		t.Fatalf("error = %v", err)
	}
	if f.hasMeeting("m1") || f.hasMeeting("m2") {
		t.Fatal("local meetings should be gone already")
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := f.plan("acme")
	if p.LocalMeetings != 0 {
		t.Fatalf("plan = %+v", p)
	}
	if _, err := f.svc.Delete(f.ctx, "acme", p); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, ok := f.roots()["acme"]; ok || f.exists("meetings/acme") {
		t.Fatal("project not fully removed")
	}
}

func TestProjectDeleteMeetingFailureMessage(t *testing.T) {
	f := newPDFixture(t)
	f.svc.d.Meetings = failingDeleter{}
	_, err := f.svc.Delete(f.ctx, "acme", f.plan("acme"))
	if err == nil || !strings.HasPrefix(err.Error(), `Notted could not delete the meeting "Sync m`) || !strings.HasSuffix(err.Error(), " Delete the project again to finish.") {
		t.Fatalf("error = %v", err)
	}
}

type failingDeleter struct{}

func (failingDeleter) Delete(context.Context, string) error { return errors.New("boom.") }

type audioLeftDeleter struct{ inner meetingDeleter }

func (d audioLeftDeleter) Delete(ctx context.Context, id string) error {
	if err := d.inner.Delete(ctx, id); err != nil {
		return err
	}
	return fmt.Errorf("audio of %s: %w", id, ErrAudioLeft)
}

func TestProjectDeleteAudioLeftStillCountsAndContinues(t *testing.T) {
	f := newPDFixture(t)
	f.svc.d.Meetings = audioLeftDeleter{inner: f.svc.d.Meetings}
	res, err := f.svc.Delete(f.ctx, "acme", f.plan("acme"))
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if res.DeletedMeetings != 2 {
		t.Fatalf("DeletedMeetings = %d", res.DeletedMeetings)
	}
	if n := len(f.em.named("meeting:deleted")); n != 2 {
		t.Fatalf("meeting:deleted events = %d", n)
	}
	last := f.em.events[len(f.em.events)-1]
	if last.name != "project:deleted" || f.exists("meetings/acme") || f.exists("instructions/projects/acme.md") {
		t.Fatalf("the delete did not continue: %+v", f.em.events)
	}
}

func TestProjectDeleteOnlyLocalMeetingsRunsNoGit(t *testing.T) {
	f := newPDFixture(t)
	for _, rel := range []string{"instructions/projects/acme.md", "instructions/people/ana", "meetings/acme"} {
		if err := os.RemoveAll(f.abs(rel)); err != nil {
			t.Fatal(err)
		}
	}
	p := f.plan("acme")
	if len(p.Files) != 0 || len(p.Folders) != 0 || p.LocalMeetings != 2 {
		t.Fatalf("plan = %+v", p)
	}
	res, err := f.svc.Delete(f.ctx, "acme", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.spy.calls) != 0 {
		t.Fatalf("git calls: %v", f.spy.calls)
	}
	if res.DeletedMeetings != 2 || res.TrackedRemoved == nil || len(res.TrackedRemoved) != 0 {
		t.Fatalf("result = %+v", res)
	}
}

type failSaveSettings struct{ port.SettingsStore }

func (failSaveSettings) SaveSettings(context.Context, domain.Settings) error {
	return errors.New("disk full")
}

func TestProjectDeleteSettingsSaveFailure(t *testing.T) {
	f := newPDFixture(t)
	f.svc.d.Settings = failSaveSettings{f.st}
	_, err := f.svc.Delete(f.ctx, "acme", f.plan("acme"))
	wantErr(t, err, "The project's files are deleted, but its root folder setting could not be cleared: disk full")
	if f.exists("meetings/acme") || f.exists("instructions/projects/acme.md") {
		t.Fatal("files should be gone")
	}
}
