package claude

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCleanEnvDropsOnlyTheTwoKeys(t *testing.T) {
	in := []string{"A=1", "ANTHROPIC_API_KEY=x", "ANTHROPIC_API_KEY_X=keep", "B=2", "ANTHROPIC_AUTH_TOKEN=y", "ANTHROPIC_BASE_URL=keep"}
	want := []string{"A=1", "ANTHROPIC_API_KEY_X=keep", "B=2", "ANTHROPIC_BASE_URL=keep"}
	if got := CleanEnv(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestLookPathUsesTheEnvPathAndSkipsNonExecutables(t *testing.T) {
	dir1, dir2 := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir1, "claude"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "claude"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := LookPath([]string{"PATH=" + dir1 + ":" + dir2}, "claude")
	if err != nil || got != filepath.Join(dir2, "claude") {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := LookPath([]string{"PATH=" + dir1}, "claude"); err == nil {
		t.Fatal("expected not found")
	}
	if _, err := LookPath([]string{"HOME=/x"}, "claude"); err == nil {
		t.Fatal("expected not found without PATH")
	}
}
