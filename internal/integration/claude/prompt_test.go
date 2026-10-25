package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newSource(env []string) PromptSource {
	return PromptSource{EnvFn: func() []string { return env }}
}

func TestPromptReturnsStdoutAndPassesProfile(t *testing.T) {
	checkout, _, _ := fixture(t)
	env := []string{"PATH=/usr/bin:/bin", "ANTHROPIC_API_KEY=dummy", "ANTHROPIC_AUTH_TOKEN=dummy"}
	got, err := newSource(env).Prompt(context.Background(), checkout, "ana", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got != "PROMPT project=acme profile=ana" {
		t.Fatalf("got %q", got)
	}
}

func TestPromptStripsCredentials(t *testing.T) {
	checkout, _, out := fixture(t)
	env := []string{"PATH=/usr/bin:/bin", "STUB_OUT=" + out, "ANTHROPIC_API_KEY=dummy", "ANTHROPIC_AUTH_TOKEN=dummy"}
	if _, err := newSource(env).Prompt(context.Background(), checkout, "ana", "acme"); err != nil {
		t.Fatal(err)
	}
	e := read(t, out+".env")
	if strings.Contains(e, "ANTHROPIC_API_KEY") || strings.Contains(e, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatalf("credentials leaked:\n%s", e)
	}
	if !strings.Contains(e, "MEETINGS_PROFILE=ana") {
		t.Fatal("profile missing")
	}
}

func TestPromptFailureReportsLastStderrLine(t *testing.T) {
	checkout, _, _ := fixture(t)
	_, err := newSource([]string{"PATH=/usr/bin:/bin"}).Prompt(context.Background(), checkout, "ana", "missing")
	want := "bin/meeting prompt failed: No project file instructions/projects/missing.md."
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestPromptMissingBinMeeting(t *testing.T) {
	checkout := t.TempDir()
	_, err := newSource(nil).Prompt(context.Background(), checkout, "ana", "acme")
	want := "The meetings archive folder is missing bin/meeting. Choose the folder again in Settings > Instructions to set it up."
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
	// present but not executable
	if err := os.MkdirAll(filepath.Join(checkout, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "bin", "meeting"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newSource(nil).Prompt(context.Background(), checkout, "ana", "acme"); err == nil || err.Error() != want {
		t.Fatalf("non-executable: got %v", err)
	}
}
