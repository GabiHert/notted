package service

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GabiHert/notted/internal/domain"
	"github.com/GabiHert/notted/internal/infra/store"
	"github.com/GabiHert/notted/internal/integration/git"
)

var wantTemplateOrder = []string{
	"bin/meeting",
	"prompts/base.md",
	".claude/skills/meetings/SKILL.md",
	"instructions/README.md",
	"instructions/people/.gitkeep",
	"instructions/projects/.gitkeep",
	"meetings/.gitkeep",
	".githooks/pre-commit",
	".gitignore",
	"CLAUDE.md",
}

const gitignoreMarker = "# Notted: recordings and session logs never enter git"

// isolatedGit is the real git adapter with no user or system config, so no test depends on the machine's git setup.
func isolatedGit() *git.Git {
	return git.New(func() []string {
		return append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	})
}

func prepSvc() *SetupService { return NewSetupService(nil, isolatedGit()) }

// gitOut runs git with the isolated environment and returns trimmed output and the error.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// treeSnapshot maps every path under dir (outside .git) to its mode and content.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == ".git" {
			return filepath.SkipDir
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		body := ""
		if fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			body = string(b)
		}
		out[rel] = fi.Mode().String() + "|" + body
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInspectEmptyFolder(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	d := t.TempDir()
	got, err := prepSvc().InspectArchive(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != d || got.InRepo || got.RepoRoot != "" || got.Ready {
		t.Fatalf("got %+v", got)
	}
	if !reflect.DeepEqual(got.Missing, wantTemplateOrder) {
		t.Fatalf("missing %v", got.Missing)
	}
}

func TestInspectAndPrepareRefuseABadPath(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	file := filepath.Join(t.TempDir(), "f")
	put(t, file, "x")
	for _, p := range []string{"rel", filepath.Join(t.TempDir(), "nope"), file} {
		_, err := prepSvc().InspectArchive(context.Background(), p)
		wantErr(t, err, "That folder does not exist: "+p)
		wantErr(t, prepSvc().PrepareArchive(context.Background(), p), "That folder does not exist: "+p)
	}
}

func TestPrepareEmptyFolder(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	d := t.TempDir()
	svc := prepSvc()
	if err := svc.PrepareArchive(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := svc.InspectArchive(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready || !got.InRepo || len(got.Missing) != 0 {
		t.Fatalf("got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(d, ".git")); err != nil {
		t.Fatal(".git:", err)
	}
	if hp, err := gitOut(d, "config", "core.hooksPath"); err != nil || hp != ".githooks" {
		t.Fatalf("hooksPath %q %v", hp, err)
	}
	for _, p := range []string{"bin/meeting", ".githooks/pre-commit"} {
		fi, err := os.Stat(filepath.Join(d, p))
		if err != nil || fi.Mode().Perm() != 0o755 {
			t.Fatalf("%s: %v %v", p, fi, err)
		}
	}
	if read(t, filepath.Join(d, "CLAUDE.md")) != "Notted writes meeting folders under meetings/. Edit instructions/ and prompts/ freely; never edit meetings/ by hand outside a meeting session.\n" {
		t.Fatalf("CLAUDE.md %q", read(t, filepath.Join(d, "CLAUDE.md")))
	}
	if first, _, _ := strings.Cut(read(t, filepath.Join(d, ".gitignore")), "\n"); first != gitignoreMarker {
		t.Fatalf("first line %q", first)
	}
	before := treeSnapshot(t, d)
	if err := svc.PrepareArchive(ctx, d); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, d)) {
		t.Fatal("second run changed files")
	}
}

func TestPrepareKeepsExistingFiles(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	d := t.TempDir()
	put(t, filepath.Join(d, "README.md"), "mine\n")
	put(t, filepath.Join(d, "prompts", "base.md"), "custom base\n")
	put(t, filepath.Join(d, ".gitignore"), "node_modules/\n")
	svc := prepSvc()
	before, err := svc.InspectArchive(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{}
	for _, p := range wantTemplateOrder {
		switch p {
		case "prompts/base.md":
		case ".gitignore":
			want = append(want, ".gitignore (adds the recording rules)")
		default:
			want = append(want, p)
		}
	}
	if !reflect.DeepEqual(before.Missing, want) {
		t.Fatalf("missing %v", before.Missing)
	}
	if err := svc.PrepareArchive(ctx, d); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(d, "README.md")) != "mine\n" || read(t, filepath.Join(d, "prompts", "base.md")) != "custom base\n" {
		t.Fatal("existing file changed")
	}
	tpl, _ := archiveTemplate.ReadFile("archive_template/.gitignore")
	if got := read(t, filepath.Join(d, ".gitignore")); got != "node_modules/\n\n"+string(tpl) {
		t.Fatalf(".gitignore:\n%s", got)
	}
	snap := treeSnapshot(t, d)
	if err := svc.PrepareArchive(ctx, d); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snap, treeSnapshot(t, d)) {
		t.Fatal("second run changed files")
	}
	after, _ := svc.InspectArchive(ctx, d)
	if !after.Ready {
		t.Fatalf("not ready: %+v", after)
	}
}

func TestPrepareInsideAnotherRepository(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	parent := t.TempDir()
	if out, err := gitOut(parent, "init", "-q"); err != nil {
		t.Fatal(out)
	}
	sub := filepath.Join(parent, "archive")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := prepSvc()
	got, err := svc.InspectArchive(ctx, sub)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot, _ := filepath.EvalSymlinks(parent)
	if !got.InRepo || !got.Nested || got.RepoRoot != wantRoot || got.Ready {
		t.Fatalf("got %+v want root %s", got, wantRoot)
	}
	if err := svc.PrepareArchive(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(sub, ".git")); err == nil {
		t.Fatal("nested .git")
	}
	if hp, err := gitOut(parent, "config", "--get", "core.hooksPath"); err == nil {
		t.Fatalf("hooksPath set to %q", hp)
	}
	after, _ := svc.InspectArchive(ctx, sub)
	if !after.Ready || after.RepoRoot != wantRoot {
		t.Fatalf("after %+v", after)
	}
}

func TestPrepareTopOfExistingRepository(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	d := t.TempDir()
	if out, err := gitOut(d, "init", "-q"); err != nil {
		t.Fatal(out)
	}
	if err := prepSvc().PrepareArchive(ctx, d); err != nil {
		t.Fatal(err)
	}
	if hp, _ := gitOut(d, "config", "core.hooksPath"); hp != ".githooks" {
		t.Fatalf("hooksPath %q", hp)
	}

	e := t.TempDir()
	if out, err := gitOut(e, "init", "-q"); err != nil {
		t.Fatal(out)
	}
	if out, err := gitOut(e, "config", "core.hooksPath", "custom-hooks"); err != nil {
		t.Fatal(out)
	}
	if err := prepSvc().PrepareArchive(ctx, e); err != nil {
		t.Fatal(err)
	}
	if hp, _ := gitOut(e, "config", "core.hooksPath"); hp != "custom-hooks" {
		t.Fatalf("hooksPath %q", hp)
	}
}

func TestPrepareRefusesLinksOutsideTheFolder(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	d := t.TempDir()
	out := t.TempDir()
	if err := os.Symlink(out, filepath.Join(d, "prompts")); err != nil {
		t.Fatal(err)
	}
	err := prepSvc().PrepareArchive(ctx, d)
	if err == nil || !strings.HasPrefix(err.Error(), "Could not set up "+d+": ") {
		t.Fatalf("err %v", err)
	}
	if es, _ := os.ReadDir(out); len(es) != 0 {
		t.Fatalf("outside changed: %v", es)
	}

	d2 := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	put(t, target, "keep")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(d2, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(d2, "bin", "meeting")); err != nil {
		t.Fatal(err)
	}
	if err := prepSvc().PrepareArchive(ctx, d2); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(target)
	if read(t, target) != "keep" || fi.Mode().Perm() != 0o600 {
		t.Fatal("the link target changed")
	}
}

func TestPrepareFailureKeepsCreatedFiles(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	d := t.TempDir()
	put(t, filepath.Join(d, "instructions"), "a file, not a folder")
	err := prepSvc().PrepareArchive(context.Background(), d)
	if err == nil || !strings.HasPrefix(err.Error(), "Could not set up "+d+": ") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(filepath.Join(d, "bin", "meeting")); err != nil {
		t.Fatalf("created file removed: %v", err)
	}
}

// preparedArchive sets up an empty folder as an archive with a project file for acme and one exported meeting.
// When commitSetup is true the person has already committed the setup files.
func preparedArchive(t *testing.T, commitSetup bool) (*archFixture, string) {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	d := t.TempDir()
	if err := prepSvc().PrepareArchive(ctx, d); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(d, "instructions", "projects", "acme.md"), "acme\n")
	if commitSetup {
		gitOut(d, "add", "-A")
		if out, err := gitOut(d, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", "setup"); err != nil {
			t.Fatal(out)
		}
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveSettings(ctx, domain.Settings{Profile: "ana", CheckoutPath: d, ProjectRoots: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	f := &archFixture{t: t, st: st, ctx: ctx, archive: d, em: &recEmitter{}}
	f.svc = NewArchiveService(st, st, isolatedGit(), f.em)
	f.newMeeting("m1", "Weekly sync")
	if _, err := f.svc.ExportMeeting(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	return f, d
}

func TestCommitInPreparedFolderWithoutRemote(t *testing.T) {
	f, d := preparedArchive(t, true)
	res, err := f.svc.CommitAndPushMeeting(f.ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed {
		t.Fatal("pushed without a remote")
	}
	head, _ := gitOut(d, "rev-parse", "--short", "HEAD")
	if res.SHA == "" || res.SHA != head {
		t.Fatalf("sha %q head %q", res.SHA, head)
	}
}

func TestFirstCommitAddsTheUntrackedProjectFile(t *testing.T) {
	f, d := preparedArchive(t, false)
	res, err := f.svc.CommitAndPushMeeting(f.ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed || res.SHA == "" {
		t.Fatalf("result %+v", res)
	}
	out, _ := gitOut(d, "show", "--name-only", "--format=", "HEAD")
	names := strings.Fields(out)
	if len(names) != 5 {
		t.Fatalf("committed %v", names)
	}
	hasProject := false
	for _, n := range names {
		if n == "instructions/projects/acme.md" {
			hasProject = true
		} else if !strings.HasPrefix(n, "meetings/acme/") {
			t.Fatalf("unexpected %s", n)
		}
	}
	if !hasProject {
		t.Fatalf("project file not committed: %v", names)
	}
}

func TestCommitDoesNotReAddATrackedProjectFile(t *testing.T) {
	f, d := preparedArchive(t, true)
	put(t, filepath.Join(d, "instructions", "projects", "acme.md"), "acme edited\n")
	if _, err := f.svc.CommitAndPushMeeting(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	out, _ := gitOut(d, "show", "--name-only", "--format=", "HEAD")
	for _, n := range strings.Fields(out) {
		if !strings.HasPrefix(n, "meetings/acme/") {
			t.Fatalf("unexpected %s in %q", n, out)
		}
	}
	if st, _ := gitOut(d, "status", "--porcelain", "--", "instructions/projects/acme.md"); st != "M instructions/projects/acme.md" {
		t.Fatalf("project file status %q", st)
	}
}

func TestInspectAndPrepareMakeScriptsExecutable(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	d := t.TempDir()
	svc := prepSvc()
	if err := svc.PrepareArchive(ctx, d); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ rel, from, want string }{
		{"bin/meeting", "-rw-r--r--", "-rwxr-xr-x"},
		{".githooks/pre-commit", "-rw-------", "-rwx------"},
	} {
		p := filepath.Join(d, filepath.FromSlash(c.rel))
		mode := map[string]os.FileMode{"-rw-r--r--": 0o644, "-rw-------": 0o600}[c.from]
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(p)
		got, err := svc.InspectArchive(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if got.Ready || !reflect.DeepEqual(got.Missing, []string{c.rel + " (makes it executable)"}) {
			t.Fatalf("%s: %+v", c.rel, got)
		}
		if err := svc.PrepareArchive(ctx, d); err != nil {
			t.Fatal(err)
		}
		fi, _ := os.Stat(p)
		after, _ := os.ReadFile(p)
		if fi.Mode().String() != c.want || string(before) != string(after) {
			t.Fatalf("%s: mode %s", c.rel, fi.Mode())
		}
		if got, _ := svc.InspectArchive(ctx, d); !got.Ready {
			t.Fatalf("%s not ready: %+v", c.rel, got)
		}
	}
}

func TestInspectTopOfRepositoryIsNotNestedForAnyPathSpelling(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	d := t.TempDir()
	if out, err := gitOut(d, "init", "-q"); err != nil {
		t.Fatal(out)
	}
	for _, p := range []string{d, d + "/"} {
		got, err := prepSvc().InspectArchive(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		if !got.InRepo || got.Nested {
			t.Fatalf("%q: %+v", p, got)
		}
	}
	plain := t.TempDir()
	if got, _ := prepSvc().InspectArchive(context.Background(), plain); got.InRepo || got.Nested {
		t.Fatalf("no repository: %+v", got)
	}
}

func TestInspectRefusesALinkOutsideTheFolder(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	d := t.TempDir()
	out := t.TempDir()
	if err := os.Symlink(out, filepath.Join(d, "prompts")); err != nil {
		t.Fatal(err)
	}
	_, err := prepSvc().InspectArchive(context.Background(), d)
	wantErr(t, err, msgOutside)
	if es, _ := os.ReadDir(out); len(es) != 0 {
		t.Fatalf("outside changed: %v", es)
	}
}

func TestInspectWrapsAnUnreadableFolder(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	d := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(d, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("root reads any folder")
	}
	_, err := prepSvc().InspectArchive(context.Background(), d)
	if err == nil || !strings.HasPrefix(err.Error(), "Could not read "+d+": ") {
		t.Fatalf("err %v", err)
	}
}
