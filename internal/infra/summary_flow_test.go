package infra

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/application/service"
	"github.com/bhlabz/notted/internal/domain"
	"github.com/bhlabz/notted/internal/infra/store"
	"github.com/bhlabz/notted/internal/integration/claude"
	"github.com/bhlabz/notted/internal/integration/git"
)

type nopEmitter struct{}

func (nopEmitter) Emit(string, any) {}

func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSummaryFlowWithStubClaude(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	root := t.TempDir()
	archive := filepath.Join(root, "archive")
	sh(t, root, "init", "-q", "-b", "main", archive)
	sh(t, archive, "config", "user.name", "Test")
	sh(t, archive, "config", "user.email", "test@example.com")
	td := filepath.Join("..", "integration", "claude", "testdata")
	copyFile(t, filepath.Join(td, "stub-meeting.sh"), filepath.Join(archive, "bin", "meeting"))
	if err := os.MkdirAll(filepath.Join(archive, "instructions", "clients"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "instructions", "clients", "acme.md"), []byte("acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, archive, "add", "-A")
	sh(t, archive, "commit", "-q", "-m", "init")

	binDir := filepath.Join(root, "bin")
	copyFile(t, filepath.Join(td, "stub-claude.sh"), filepath.Join(binDir, "claude"))
	out := filepath.Join(root, "out")
	t.Setenv("STUB_OUT", out)
	t.Setenv("STUB_REPLY", "# Summary: Weekly sync")
	t.Setenv("STUB_REPLY_SUGGEST", "# Suggestions: Weekly sync")
	envFn := func() []string {
		return []string{"PATH=" + binDir + ":/usr/bin:/bin", "HOME=" + root, "ANTHROPIC_API_KEY=sk-secret",
			"STUB_OUT=" + out, "STUB_REPLY=# Summary: Weekly sync", "STUB_REPLY_SUGGEST=# Suggestions: Weekly sync"}
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.SaveSettings(ctx, domain.Settings{Profile: "ana", CheckoutPath: archive, ProjectRoots: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	m := domain.Meeting{ID: "m1", Title: "Weekly sync", StartedAt: time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC),
		Language: domain.LangEN, State: domain.StateReady}
	if err := st.CreateMeeting(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := st.SetClientKey(ctx, "m1", "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddSegment(ctx, domain.Segment{MeetingID: "m1", Track: domain.TrackMic, StartMs: 1000, EndMs: 2000, Text: "hello"}); err != nil {
		t.Fatal(err)
	}

	arch := service.NewArchiveService(st, st, git.New(os.Environ), nopEmitter{})
	svc := service.NewSummaryService(st, st, claude.PromptSource{EnvFn: envFn}, claude.Runner{EnvFn: envFn}, arch, nopEmitter{})
	if err := svc.Generate(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	svc.Wait()

	got, _ := st.GetMeeting(ctx, "m1")
	if got.State != domain.StateSummarized || got.Folder == "" {
		t.Fatalf("meeting = %+v", got)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(archive, filepath.FromSlash(got.Folder), name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if !strings.HasPrefix(read("summary.md"), "# Summary: Weekly sync") ||
		!strings.HasPrefix(read("suggestions.md"), "# Suggestions: Weekly sync") ||
		!strings.Contains(read("transcript.md"), "(mic) hello") {
		t.Fatal("exported files have the wrong content")
	}
	argvB, _ := os.ReadFile(filepath.Join(out, "argv"))
	argv := strings.Fields(string(argvB))
	sid := ""
	for i, a := range argv {
		if a == "--resume" && i+1 < len(argv) {
			sid = argv[i+1]
		}
	}
	if sid == "" || got.ClaudeSessionID != sid || !strings.Contains(read("meeting.md"), `claude_session_id: "`+sid+`"`) {
		t.Fatalf("session id %q vs stored %q", sid, got.ClaudeSessionID)
	}
	if staged := sh(t, archive, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("staged files: %q", staged)
	}
	env, _ := os.ReadFile(filepath.Join(out, "env"))
	if strings.Contains(string(env), "ANTHROPIC_API_KEY") {
		t.Fatal("the stub saw the API key")
	}
}
