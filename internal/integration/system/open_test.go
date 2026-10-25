package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenPassesTheURLToTheCommand(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_OPEN_ARGS", args)
	if err := (Opener{Command: filepath.Join("testdata", "fake-open.sh")}).Open("x-apple.systempreferences:a?b"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(args); strings.TrimSpace(string(b)) != "x-apple.systempreferences:a?b" {
		t.Fatalf("argv = %q", b)
	}
}

func TestOpenFailureIsUserFacing(t *testing.T) {
	t.Setenv("FAKE_OPEN_ARGS", filepath.Join(t.TempDir(), "args"))
	t.Setenv("FAKE_OPEN_FAIL", "1")
	err := (Opener{Command: filepath.Join("testdata", "fake-open.sh")}).Open("x")
	if err == nil || err.Error() != "Could not open System Settings. Open it yourself, then go to Privacy & Security." {
		t.Fatalf("err = %v", err)
	}
	if err := (Opener{Command: filepath.Join(t.TempDir(), "nope")}).Open("x"); err == nil {
		t.Fatal("a missing command must fail")
	}
}
