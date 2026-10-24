package infra

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bhlabz/notted/internal/application/service"
	"github.com/bhlabz/notted/internal/domain"
	"github.com/bhlabz/notted/internal/infra/store"
	"github.com/bhlabz/notted/internal/integration/claude"
	"github.com/bhlabz/notted/internal/integration/git"
)

// An empty folder becomes an archive: profile and project files go in, and the folder's own bin/meeting builds the prompt.
func TestPreparedEmptyFolderBuildsAPrompt(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	setup := service.NewSetupService(st, git.New(func() []string {
		return append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	}))
	if err := setup.PrepareArchive(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSettings(ctx, domain.Settings{CheckoutPath: dir, ProjectRoots: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.CreateProfile(ctx, "ana"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSettings(ctx, domain.Settings{Profile: "ana", CheckoutPath: dir, ProjectRoots: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.CreateProject(ctx, "acme"); err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(filepath.Join(dir, "prompts", "base.md"))
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(base), "\n")
	got, err := claude.PromptSource{EnvFn: os.Environ}.Prompt(ctx, dir, "ana", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, first) {
		t.Fatalf("prompt lacks %q:\n%s", first, got)
	}
}

func TestPreparedBinMeetingRefusesOtherCommands(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	dir := t.TempDir()
	setup := service.NewSetupService(nil, git.New(os.Environ))
	if err := setup.PrepareArchive(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"new", "--project", "acme"}, "bin/meeting new is not in this version. Start meetings from Notted."},
		{nil, "usage: bin/meeting prompt --project <key>"},
	} {
		cmd := exec.Command(filepath.Join(dir, "bin", "meeting"), c.args...)
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 2 || strings.TrimSpace(string(out)) != c.want {
			t.Fatalf("%v: %v %q", c.args, err, out)
		}
	}
}
