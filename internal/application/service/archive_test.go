package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/domain"
	"github.com/GabiHert/notted/internal/infra/store"
	"github.com/GabiHert/notted/internal/integration/git"
)

type archFixture struct {
	t       *testing.T
	root    string
	archive string
	st      *store.Store
	em      *recEmitter
	svc     *ArchiveService
	ctx     context.Context
	meet    domain.Meeting
	date    string
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newArchFixture(t *testing.T) *archFixture {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	archive := filepath.Join(root, "archive")
	runGit(t, root, "init", "-q", "--bare", "-b", "main", remote)
	runGit(t, root, "clone", "-q", remote, archive)
	runGit(t, archive, "config", "user.name", "Test")
	runGit(t, archive, "config", "user.email", "test@example.com")
	runGit(t, archive, "checkout", "-q", "-B", "main")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(archive, "bin"), 0o755))
	must(os.WriteFile(filepath.Join(archive, "bin", "meeting"), []byte("#!/bin/sh\n"), 0o755))
	must(os.MkdirAll(filepath.Join(archive, "instructions", "projects"), 0o755))
	must(os.WriteFile(filepath.Join(archive, "instructions", "projects", "acme.md"), []byte("acme\n"), 0o644))
	must(os.WriteFile(filepath.Join(archive, "instructions", "projects", "globex.md"), []byte("globex\n"), 0o644))
	must(os.WriteFile(filepath.Join(archive, "instructions", "projects", "README.txt"), []byte("x\n"), 0o644))
	must(os.MkdirAll(filepath.Join(archive, ".githooks"), 0o755))
	hook, err := os.ReadFile(filepath.Join("..", "..", "integration", "git", "testdata", "pre-commit"))
	must(err)
	must(os.WriteFile(filepath.Join(archive, ".githooks", "pre-commit"), hook, 0o755))
	runGit(t, archive, "config", "core.hooksPath", ".githooks")
	runGit(t, archive, "add", "-A")
	runGit(t, archive, "commit", "-q", "-m", "init")
	runGit(t, archive, "push", "-q", "-u", "origin", "main")

	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	must(err)
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	must(st.SaveSettings(ctx, domain.Settings{Profile: "ana", CheckoutPath: archive, ProjectRoots: map[string]string{}}))
	f := &archFixture{t: t, root: root, archive: archive, st: st, em: &recEmitter{}, ctx: ctx}
	f.svc = NewArchiveService(st, st, git.New(os.Environ), f.em)
	f.meet = f.newMeeting("m1", "Weekly sync — Q4")
	f.date = f.meet.StartedAt.In(time.Local).Format("2006-01-02")
	for _, s := range []domain.Segment{
		{MeetingID: "m1", Track: domain.TrackOthers, StartMs: 65_900, EndMs: 70_000, Text: "  third   line "},
		{MeetingID: "m1", Track: domain.TrackOthers, StartMs: 1_000, EndMs: 2_000, Text: "same tick others"},
		{MeetingID: "m1", Track: domain.TrackMic, StartMs: 1_000, EndMs: 2_000, Text: "same tick mic"},
	} {
		if _, err := st.AddSegment(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *archFixture) newMeeting(id, title string) domain.Meeting {
	f.t.Helper()
	m := domain.Meeting{ID: id, Title: title, StartedAt: time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC),
		Language: domain.LangPTBR, State: domain.StateSummarized}
	if err := f.st.CreateMeeting(f.ctx, m); err != nil {
		f.t.Fatal(err)
	}
	for _, err := range []error{
		f.st.SetProjectKey(f.ctx, id, "acme"),
		f.st.SetClaudeSessionID(f.ctx, id, "11111111-2222-3333-4444-555555555555"),
		f.st.SetSummary(f.ctx, id, "## Summary\n\nbody\n\n\n"),
		f.st.SetSuggestions(ctx0, id, "- task one"),
	} {
		if err != nil {
			f.t.Fatal(err)
		}
	}
	got, _ := f.st.GetMeeting(f.ctx, id)
	return got
}

var ctx0 = context.Background()

func (f *archFixture) read(rel string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.archive, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func wantErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestArchiveListAndSetProject(t *testing.T) {
	f := newArchFixture(t)
	keys, err := f.svc.ListProjects(f.ctx)
	if err != nil || strings.Join(keys, ",") != "acme,globex" {
		t.Fatalf("keys %v %v", keys, err)
	}
	wantErr(t, f.svc.SetMeetingProject(f.ctx, "m1", "nope"), `Unknown project "nope". Add instructions/projects/nope.md to the archive first.`)
	wantErr(t, f.svc.SetMeetingProject(f.ctx, "m1", "globex"), "This meeting's project is fixed once its summary exists.")
	if err := f.svc.SetMeetingProject(f.ctx, "m1", "acme"); err != nil {
		t.Fatalf("same key must be a no-op: %v", err)
	}
	f.newMeeting2NoSession("m9")
	if err := f.svc.SetMeetingProject(f.ctx, "m9", "globex"); err != nil {
		t.Fatal(err)
	}
	m, _ := f.st.GetMeeting(f.ctx, "m9")
	if m.ProjectKey != "globex" {
		t.Fatalf("project %q", m.ProjectKey)
	}
	wantErr(t, f.svc.SetMeetingProject(f.ctx, "zzz", "acme"), "Meeting not found.")
}

func (f *archFixture) newMeeting2NoSession(id string) {
	f.t.Helper()
	m := domain.Meeting{ID: id, Title: "x", StartedAt: time.Now().UTC(), Language: domain.LangEN, State: domain.StateReady}
	if err := f.st.CreateMeeting(f.ctx, m); err != nil {
		f.t.Fatal(err)
	}
}

func TestArchiveCheckoutMessages(t *testing.T) {
	f := newArchFixture(t)
	if err := f.st.SaveSettings(f.ctx, domain.Settings{Profile: "ana", ProjectRoots: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.ListProjects(f.ctx)
	wantErr(t, err, "Set the meetings archive folder in Settings > Instructions first.")
	if err := f.st.SaveSettings(f.ctx, domain.Settings{Profile: "ana", CheckoutPath: t.TempDir(), ProjectRoots: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.ListProjects(f.ctx)
	wantErr(t, err, "The meetings archive folder is missing bin/meeting. Choose the folder again in Settings > Instructions to set it up.")
}

const goldenMeeting = `---
date: %DATE%
project: acme
title: "Weekly sync — Q4"
participants: []
source: app
language: pt-BR
meetily_id: ""
claude_session_id: "11111111-2222-3333-4444-555555555555"
session_owner: "ana"
---

# Weekly sync — Q4

Participants are typed into ` + "`participants`" + ` above by hand. Notted gives only ` + "`mic`" + ` (the person
who recorded) and ` + "`others`" + `.
`

func TestArchiveExport(t *testing.T) {
	f := newArchFixture(t)
	res, err := f.svc.ExportMeeting(f.ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	folder := "meetings/acme/" + f.date[:4] + "/" + f.date + "-weekly-sync-q4"
	if res.Folder != folder || len(res.Files) != 4 || res.Files[0] != folder+"/meeting.md" {
		t.Fatalf("result %+v", res)
	}
	if got := f.read(folder + "/meeting.md"); got != strings.ReplaceAll(goldenMeeting, "%DATE%", f.date) {
		t.Fatalf("meeting.md:\n%s", got)
	}
	wantT := "# Transcript: Weekly sync — Q4\n\n[00:00:01] (mic) same tick mic\n[00:00:01] (others) same tick others\n[00:01:05] (others) third line\n"
	if got := f.read(folder + "/transcript.md"); got != wantT {
		t.Fatalf("transcript:\n%q", got)
	}
	if got := f.read(folder + "/summary.md"); got != "## Summary\n\nbody\n" {
		t.Fatalf("summary %q", got)
	}
	if got := f.read(folder + "/suggestions.md"); got != "- task one\n" {
		t.Fatalf("suggestions %q", got)
	}
	evs := f.em.named("export:ready")
	if len(evs) != 1 || evs[0].payload["folder"] != folder || evs[0].payload["meetingId"] != "m1" {
		t.Fatalf("events %+v", evs)
	}
	if files, _ := evs[0].payload["files"].([]string); len(files) != 4 || files[3] != folder+"/suggestions.md" {
		t.Fatalf("files %+v", evs[0].payload["files"])
	}
	if got := runGit(t, f.archive, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("staged: %q", got)
	}
	if st := runGit(t, f.archive, "status", "--porcelain"); !strings.HasPrefix(st, "?? meetings/") {
		t.Fatalf("status %q", st)
	}
	m, _ := f.st.GetMeeting(f.ctx, "m1")
	if m.Folder != folder {
		t.Fatalf("folder %q", m.Folder)
	}

	f.newMeeting("m2", "Weekly sync — Q4")
	res2, err := f.svc.ExportMeeting(f.ctx, "m2")
	if err != nil || res2.Folder != folder+"-2" {
		t.Fatalf("second %+v %v", res2, err)
	}
	if err := os.WriteFile(filepath.Join(f.archive, folder, "summary.md"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	res3, err := f.svc.ExportMeeting(f.ctx, "m1")
	if err != nil || res3.Folder != folder {
		t.Fatalf("re-export %+v %v", res3, err)
	}
	if f.read(folder+"/summary.md") != "## Summary\n\nbody\n" {
		t.Fatal("re-export did not rewrite")
	}
	if _, err := os.Stat(filepath.Join(f.archive, folder+"-3")); err == nil {
		t.Fatal("created -3")
	}
}

func TestArchiveExportNotReady(t *testing.T) {
	f := newArchFixture(t)
	f.newMeeting2NoSession("m9")
	_, err := f.svc.ExportMeeting(f.ctx, "m9")
	wantErr(t, err, "Nothing to export yet. Generate the summary first.")
	_, err = f.svc.ExportMeeting(f.ctx, "nope")
	wantErr(t, err, "Meeting not found.")
}

func TestArchiveCommitKeepsOtherStagedWork(t *testing.T) {
	f := newArchFixture(t)
	_, err := f.svc.CommitAndPushMeeting(f.ctx, "m1")
	wantErr(t, err, "Export the meeting first.")
	res, err := f.svc.ExportMeeting(f.ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.archive, "notes.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, f.archive, "add", "notes.txt")
	res2, err := f.svc.CommitAndPushMeeting(f.ctx, "m1")
	if err != nil || res2.SHA == "" || !res2.Pushed {
		t.Fatalf("commit: %+v %v", res2, err)
	}
	names := strings.Fields(runGit(t, f.archive, "show", "--name-only", "--format=", "HEAD"))
	if len(names) != 4 {
		t.Fatalf("committed %v", names)
	}
	for _, n := range names {
		if !strings.HasPrefix(n, res.Folder+"/") {
			t.Fatalf("outside folder: %s", n)
		}
	}
	if got := runGit(t, f.archive, "diff", "--cached", "--name-only"); got != "notes.txt" {
		t.Fatalf("staged after: %q", got)
	}
	if local, remote := runGit(t, f.archive, "rev-parse", "HEAD"), runGit(t, f.archive, "rev-parse", "origin/main"); local != remote {
		t.Fatalf("not pushed %s %s", local, remote)
	}
	if remote := runGit(t, f.root, "--git-dir", filepath.Join(f.root, "remote.git"), "rev-parse", "main"); remote != runGit(t, f.archive, "rev-parse", "HEAD") {
		t.Fatal("remote main lacks the commit")
	}
	_, err = f.svc.CommitAndPushMeeting(f.ctx, "m1")
	wantErr(t, err, "Nothing changed in "+res.Folder+" since the last commit.")
}

func TestArchiveHookRejection(t *testing.T) {
	f := newArchFixture(t)
	res, err := f.svc.ExportMeeting(f.ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.archive, res.Folder, "x.wav"), []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CommitAndPushMeeting(f.ctx, "m1")
	if err == nil || !strings.HasPrefix(err.Error(), "The commit was rejected:\n") || !strings.Contains(err.Error(), "pre-commit: rejected") {
		t.Fatalf("err %v", err)
	}
	if got := runGit(t, f.archive, "diff", "--cached", "--name-only", "--", res.Folder); got != "" {
		t.Fatalf("still staged: %q", got)
	}
}

func TestArchiveHookRejectsUnknownProject(t *testing.T) {
	f := newArchFixture(t)
	if err := f.st.SetProjectKey(f.ctx, "m1", "initech"); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.ExportMeeting(f.ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CommitAndPushMeeting(f.ctx, "m1")
	if err == nil || !strings.Contains(err.Error(), `project "initech" has no instructions/projects/initech.md`) {
		t.Fatalf("err %v", err)
	}
	if got := runGit(t, f.archive, "diff", "--cached", "--name-only", "--", res.Folder); got != "" {
		t.Fatalf("still staged: %q", got)
	}
}

func TestArchivePushFailureKeepsCommit(t *testing.T) {
	f := newArchFixture(t)
	if _, err := f.svc.ExportMeeting(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	runGit(t, f.archive, "remote", "set-url", "origin", filepath.Join(f.root, "missing.git"))
	_, err := f.svc.CommitAndPushMeeting(f.ctx, "m1")
	short := runGit(t, f.archive, "rev-parse", "--short", "HEAD")
	if err == nil || !strings.HasPrefix(err.Error(), "The commit "+short+" was saved locally but not pushed:\n") {
		t.Fatalf("err %v", err)
	}
	if subj := runGit(t, f.archive, "log", "-1", "--format=%s"); !strings.HasPrefix(subj, "meeting(acme): "+f.date+" Weekly sync") {
		t.Fatalf("subject %q", subj)
	}
}

func (f *archFixture) deletionSetup() []string {
	f.t.Helper()
	for rel, body := range map[string]string{
		"instructions/people/ana/acme.md":               "ana\n",
		"instructions/people/bruno/acme.md":             "bruno\n",
		"meetings/acme/2026/2026-03-10-sync/meeting.md": "m\n",
		"meetings/acme/2026/2026-03-10-sync/summary.md": "s\n",
	} {
		p := filepath.Join(f.archive, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	runGit(f.t, f.archive, "add", "-A")
	runGit(f.t, f.archive, "commit", "-q", "-m", "add project files")
	runGit(f.t, f.archive, "push", "-q", "-u", "origin", "main")
	for _, rel := range []string{"instructions/projects/acme.md", "instructions/people/ana/acme.md", "meetings/acme"} {
		if err := os.RemoveAll(filepath.Join(f.archive, filepath.FromSlash(rel))); err != nil {
			f.t.Fatal(err)
		}
	}
	return []string{"instructions/projects/acme.md", "instructions/people/ana/acme.md",
		"meetings/acme/2026/2026-03-10-sync/meeting.md", "meetings/acme/2026/2026-03-10-sync/summary.md"}
}

func TestCommitProjectDeletionCommitsOnlyTheRemovedPaths(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	if err := os.WriteFile(filepath.Join(f.archive, "notes.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, f.archive, "add", "notes.txt")
	if err := os.WriteFile(filepath.Join(f.archive, "instructions/projects/globex.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths); err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(runGit(t, f.archive, "show", "--name-status", "--format=", "HEAD"))
	want := []string{}
	for _, p := range paths {
		want = append(want, "D", p)
	}
	if len(got) != len(want) {
		t.Fatalf("show = %v", got)
	}
	seen := map[string]bool{}
	for i := 0; i < len(got); i += 2 {
		if got[i] != "D" {
			t.Fatalf("status %v", got)
		}
		seen[got[i+1]] = true
	}
	for _, p := range paths {
		if !seen[p] {
			t.Fatalf("missing %s in %v", p, got)
		}
	}
	if s := runGit(t, f.archive, "log", "-1", "--format=%s"); s != "project(acme): delete" {
		t.Fatalf("subject %q", s)
	}
	if s := runGit(t, f.archive, "diff", "--cached", "--name-only"); s != "notes.txt" {
		t.Fatalf("staged %q", s)
	}
	if s := runGit(t, f.archive, "diff", "--name-only"); s != "instructions/projects/globex.md" {
		t.Fatalf("unstaged %q", s)
	}
	runGit(t, f.archive, "ls-files", "--error-unmatch", "instructions/people/bruno/acme.md")
	if f.read("instructions/people/bruno/acme.md") != "bruno\n" {
		t.Fatal("bruno file changed")
	}
	if a, b := runGit(t, f.archive, "rev-parse", "HEAD"), runGit(t, f.root, "--git-dir=remote.git", "rev-parse", "main"); a != b {
		t.Fatalf("remote %s != local %s", b, a)
	}
}

func TestCommitProjectDeletionHookAccepts(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	if got := runGit(t, f.archive, "config", "core.hooksPath"); got != ".githooks" {
		t.Fatalf("hooksPath %q", got)
	}
	before := runGit(t, f.archive, "rev-list", "--count", "HEAD")
	err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths)
	if err != nil {
		if strings.Contains(err.Error(), "pre-commit: rejected") {
			t.Fatalf("hook rejected: %v", err)
		}
		t.Fatal(err)
	}
	if after := runGit(t, f.archive, "rev-list", "--count", "HEAD"); after == before {
		t.Fatal("no commit")
	}
}

func TestCommitProjectDeletionWithoutOrigin(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	runGit(t, f.archive, "remote", "remove", "origin")
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths); err != nil {
		t.Fatal(err)
	}
	if s := runGit(t, f.archive, "log", "-1", "--format=%s"); s != "project(acme): delete" {
		t.Fatalf("subject %q", s)
	}
}

func TestCommitProjectDeletionPushFailureKeepsCommit(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	runGit(t, f.archive, "remote", "set-url", "origin", filepath.Join(f.root, "missing.git"))
	err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths)
	sha := runGit(t, f.archive, "rev-parse", "--short", "HEAD")
	if err == nil || !strings.HasPrefix(err.Error(), "The commit "+sha+" was saved locally but not pushed:\n") {
		t.Fatalf("error = %v", err)
	}
	if s := runGit(t, f.archive, "log", "-1", "--format=%s"); s != "project(acme): delete" {
		t.Fatalf("subject %q", s)
	}
	count := runGit(t, f.archive, "rev-list", "--count", "HEAD")
	runGit(t, f.archive, "remote", "set-url", "origin", filepath.Join(f.root, "remote.git"))
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths); err != nil {
		t.Fatal(err)
	}
	if c := runGit(t, f.archive, "rev-list", "--count", "HEAD"); c != count {
		t.Fatalf("count %s != %s", c, count)
	}
	if a, b := runGit(t, f.archive, "rev-parse", "HEAD"), runGit(t, f.root, "--git-dir=remote.git", "rev-parse", "main"); a != b {
		t.Fatalf("remote %s != local %s", b, a)
	}
}

func TestCommitProjectDeletionRefusesBadInput(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	nothing := `There is nothing to commit for project "acme".`
	notOne := func(p string) string {
		return `Notted can commit only the deleted files of project "acme". ` + p + ` is not one of them.`
	}
	cases := []struct {
		name, key string
		paths     []string
		want      string
	}{
		{"empty key", "", paths, errProjectKey.Error()},
		{"upper key", "Acme", paths, errProjectKey.Error()},
		{"no paths", "acme", nil, nothing},
	}
	for _, p := range []string{"instructions/projects/globex.md", "instructions/people/bruno/acme.md", "meetings/globex/2026/x/meeting.md",
		"meetings/acme/../globex/x.md", "meetings/acme/", "./meetings/acme/a.md", "/etc/hosts", `meetings/acme\a.md`, "notes.txt"} {
		cases = append(cases, struct {
			name, key string
			paths     []string
			want      string
		}{p, "acme", []string{paths[0], p}, notOne(p)})
	}
	headBefore := runGit(t, f.archive, "rev-parse", "HEAD")
	stagedBefore := runGit(t, f.archive, "diff", "--cached", "--name-only")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantErr(t, f.svc.CommitProjectDeletion(f.ctx, c.key, c.paths), c.want)
			if runGit(t, f.archive, "rev-parse", "HEAD") != headBefore || runGit(t, f.archive, "diff", "--cached", "--name-only") != stagedBefore {
				t.Fatal("git state changed")
			}
		})
	}
	t.Run("archive not set", func(t *testing.T) {
		if err := f.st.SaveSettings(f.ctx, domain.Settings{Profile: "ana", ProjectRoots: map[string]string{}}); err != nil {
			t.Fatal(err)
		}
		wantErr(t, f.svc.CommitProjectDeletion(f.ctx, "acme", paths), "Set the meetings archive folder in Settings > Instructions first.")
	})
}

func TestCommitProjectDeletionRejectedUnstages(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	paths = append(paths, "meetings/acme/2026/2026-03-10-sync/ghost.md")
	err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths)
	if err == nil || !strings.HasPrefix(err.Error(), "The commit was rejected:\n") {
		t.Fatalf("error = %v", err)
	}
	if s := runGit(t, f.archive, "diff", "--cached", "--name-only", "--", "meetings/acme", "instructions"); s != "" {
		t.Fatalf("still staged: %q", s)
	}
}

func TestCommitProjectDeletionNothingStagedMakesNoCommit(t *testing.T) {
	f := newArchFixture(t)
	f.deletionSetup()
	const kept = "meetings/acme/2026/2026-03-10-sync/meeting.md"
	runGit(t, f.archive, "checkout", "--", kept)
	before := runGit(t, f.archive, "rev-list", "--count", "HEAD")
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", []string{kept}); err != nil {
		t.Fatal(err)
	}
	if c := runGit(t, f.archive, "rev-list", "--count", "HEAD"); c != before {
		t.Fatalf("commit count %s != %s", c, before)
	}
}

func TestCommitProjectDeletionRetryRejectsUntrackedPath(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths); err != nil {
		t.Fatal(err)
	}
	err := f.svc.CommitProjectDeletion(f.ctx, "acme", []string{"meetings/acme/2026/x/ghost.md"})
	if err == nil || !strings.HasPrefix(err.Error(), "The commit was rejected:\n") {
		t.Fatalf("error = %v", err)
	}
}

func TestCommitProjectDeletionDoesNotMistakeAnEarlierCommitForTheRetry(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths[:1]); err != nil {
		t.Fatal(err)
	}
	count := runGit(t, f.archive, "rev-list", "--count", "HEAD")
	runGit(t, f.archive, append([]string{"add", "-A", "--"}, paths[1:]...)...)
	err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths[1:])
	if err == nil || !strings.HasPrefix(err.Error(), "The commit was rejected:\n") {
		t.Fatalf("first call error = %v", err)
	}
	if s := runGit(t, f.archive, "diff", "--cached", "--name-only"); s != "" {
		t.Fatalf("still staged: %q", s)
	}
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths[1:]); err != nil {
		t.Fatal(err)
	}
	if c := runGit(t, f.archive, "rev-list", "--count", "HEAD"); c == count {
		t.Fatal("second call made no commit")
	}
	got := runGit(t, f.archive, "show", "--name-only", "--format=", "HEAD")
	if len(strings.Fields(got)) != len(paths[1:]) {
		t.Fatalf("HEAD deleted %q", got)
	}
}

func TestCommitProjectDeletionRetryHandlesNonASCIIPath(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	const rel = "meetings/acme/2026/2026-03-11-reuniao/ata-reunião final.md"
	full := filepath.Join(f.archive, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("ata\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, f.archive, "add", "--", rel)
	runGit(t, f.archive, "commit", "-q", "-m", "add ata")
	runGit(t, f.archive, "push", "-q", "origin", "main")
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	paths = append(paths, rel)
	runGit(t, f.archive, "remote", "set-url", "origin", filepath.Join(f.root, "missing.git"))
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths); err == nil || !strings.Contains(err.Error(), "saved locally but not pushed") {
		t.Fatalf("first call error = %v", err)
	}
	count := runGit(t, f.archive, "rev-list", "--count", "HEAD")
	runGit(t, f.archive, "remote", "set-url", "origin", filepath.Join(f.root, "remote.git"))
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths); err != nil {
		t.Fatal(err)
	}
	if c := runGit(t, f.archive, "rev-list", "--count", "HEAD"); c != count {
		t.Fatalf("count %s != %s", c, count)
	}
	if a, b := runGit(t, f.archive, "rev-parse", "HEAD"), runGit(t, f.root, "--git-dir=remote.git", "rev-parse", "main"); a != b {
		t.Fatalf("remote %s != local %s", b, a)
	}
}

func TestCommitProjectDeletionRetryNeedsEveryPathDeletedByHead(t *testing.T) {
	f := newArchFixture(t)
	paths := f.deletionSetup()
	if err := f.svc.CommitProjectDeletion(f.ctx, "acme", paths); err != nil {
		t.Fatal(err)
	}
	err := f.svc.CommitProjectDeletion(f.ctx, "acme", []string{paths[0], "meetings/acme/2026/x/ghost.md"})
	if err == nil || !strings.HasPrefix(err.Error(), "The commit was rejected:\n") {
		t.Fatalf("error = %v", err)
	}
}
