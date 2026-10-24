package git

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRunUsesRepoAndReturnsCombinedOutput(t *testing.T) {
	dir := t.TempDir()
	g := New(func() []string { return os.Environ() })
	if out, err := g.Run(context.Background(), dir, "init", "-q"); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	if _, err := os.Stat(dir + "/.git"); err != nil {
		t.Fatalf("git ran outside -C dir: %v", err)
	}
	out, err := g.Run(context.Background(), dir, "checkout", "bogus-branch-xyz")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(out, "bogus") {
		t.Fatalf("combined output missing: %q", out)
	}
}

func TestRunEnvCarriesNoPrompt(t *testing.T) {
	dir := t.TempDir()
	g := New(func() []string { return []string{"PATH=" + os.Getenv("PATH"), "FOO=bar"} })
	if _, err := g.Run(context.Background(), dir, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Run(context.Background(), dir, "config", "alias.showenv", "!env"); err != nil {
		t.Fatal(err)
	}
	out, err := g.Run(context.Background(), dir, "showenv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "GIT_TERMINAL_PROMPT=0") || !strings.Contains(out, "FOO=bar") {
		t.Fatalf("env: %q", out)
	}
}
