package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeUsesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOTTED_HOME", dir)
	if got := Home(); got != dir {
		t.Fatalf("Home() = %q, want %q", got, dir)
	}
}

func TestHomeDefaultsToDotNotted(t *testing.T) {
	t.Setenv("NOTTED_HOME", "")
	got := Home()
	if !strings.HasSuffix(got, "/.notted") {
		t.Fatalf("Home() = %q, want suffix /.notted", got)
	}
	if h, err := os.UserHomeDir(); err == nil && got != filepath.Join(h, ".notted") {
		t.Fatalf("Home() = %q, want under %q", got, h)
	}
}

func TestHelpersJoinUnderHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOTTED_HOME", dir)
	if got, want := DBPath(), filepath.Join(dir, "meetings.sqlite"); got != want {
		t.Fatalf("DBPath() = %q, want %q", got, want)
	}
	if got, want := AudioDir("m1"), filepath.Join(dir, "audio", "m1"); got != want {
		t.Fatalf("AudioDir() = %q, want %q", got, want)
	}
	if got, want := ModelsDir(), filepath.Join(dir, "models"); got != want {
		t.Fatalf("ModelsDir() = %q, want %q", got, want)
	}
}

func TestInstallSourceUnderHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOTTED_HOME", dir)
	if got, want := InstallSource(), filepath.Join(dir, "install-source"); got != want {
		t.Fatalf("InstallSource() = %q, want %q", got, want)
	}
}

func TestAskPathsUnderHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOTTED_HOME", dir)
	if got, want := RunDir(), filepath.Join(dir, "run"); got != want {
		t.Fatalf("RunDir() = %q, want %q", got, want)
	}
	if got, want := AskSocket(), filepath.Join(dir, "run", "ask.sock"); got != want {
		t.Fatalf("AskSocket() = %q, want %q", got, want)
	}
	if got, want := SessionsDir(), filepath.Join(dir, "sessions"); got != want {
		t.Fatalf("SessionsDir() = %q, want %q", got, want)
	}
}
