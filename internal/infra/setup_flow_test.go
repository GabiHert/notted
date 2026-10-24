package infra

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GabiHert/notted/internal/application/service"
	"github.com/GabiHert/notted/internal/infra/store"
	"github.com/GabiHert/notted/internal/integration/git"
)

func TestSetupCloneWithRealGit(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	base := t.TempDir()
	src := filepath.Join(base, "meetings-src")
	sh(t, base, "init", "-q", "-b", "main", src)
	files := map[string]string{
		"bin/meeting": "#!/bin/sh\n", "prompts/base.md": "BASE\n",
		"instructions/README.md": "R\n", ".githooks/pre-commit": "#!/bin/sh\n",
	}
	for p, text := range files {
		full := filepath.Join(src, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if p == "bin/meeting" || p == ".githooks/pre-commit" {
			mode = 0o755
		}
		if err := os.WriteFile(full, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	sh(t, src, "add", "-A")
	sh(t, src, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "init")

	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	parent := t.TempDir()
	got, err := service.NewSetupService(st, git.New(os.Environ)).CloneArchive(context.Background(), "file://"+src, parent)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(parent, "meetings-src") {
		t.Fatalf("got %q", got)
	}
	for p := range files {
		if _, err := os.Stat(filepath.Join(got, filepath.FromSlash(p))); err != nil {
			t.Fatal(err)
		}
	}
	if hp := sh(t, got, "config", "core.hooksPath"); hp != ".githooks" {
		t.Fatalf("hooksPath %q", hp)
	}
}
