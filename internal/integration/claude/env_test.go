package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

const testMarker = "__NOTTED_ENV_BEGIN__"

// envBlock is what `printf marker; env -0` prints: the marker line, then NUL-ended entries.
func envBlock(entries ...string) string {
	return testMarker + "\n" + strings.Join(entries, "\x00") + "\x00"
}

type shellCall struct{ args []string }

// fakeShell answers the interactive call and the plain login call from two scripts.
func fakeShell(calls *[]shellCall, interactive, login func(ctx context.Context) ([]byte, error)) func(context.Context, string, ...string) ([]byte, error) {
	return func(ctx context.Context, shell string, args ...string) ([]byte, error) {
		*calls = append(*calls, shellCall{args: args})
		for _, a := range args {
			if a == "-i" {
				return interactive(ctx)
			}
		}
		return login(ctx)
	}
}

func isolate(t *testing.T) (home string) {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	home = t.TempDir()
	prev := fallbackDirs
	fallbackDirs = nil
	t.Cleanup(func() { fallbackDirs = prev })
	return home
}

func okOut(s string) func(context.Context) ([]byte, error) {
	return func(context.Context) ([]byte, error) { return []byte(s), nil }
}

func envHas(env []string, entry string) bool {
	for _, e := range env {
		if e == entry {
			return true
		}
	}
	return false
}

func TestMarkerParsingDropsNoiseBeforeTheMarker(t *testing.T) {
	home := isolate(t)
	out := "Welcome banner\nFOO=not-an-env-line-from-a-prompt\n" + envBlock("PATH=/a:/b", "HOME=/h")
	var calls []shellCall
	got := loginShellEnvWith(fakeShell(&calls, okOut(out), okOut("")), home, "/bin/zsh")
	want := []string{"PATH=/a:/b", "HOME=/h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(calls) != 1 {
		t.Fatalf("login fallback must not run when interactive works, calls %v", calls)
	}
	joined := strings.Join(calls[0].args, " ")
	if !strings.Contains(joined, "-l") || !strings.Contains(joined, "-i") || !strings.Contains(joined, testMarker) {
		t.Fatalf("interactive call args %v", calls[0].args)
	}
}

func TestOutputWithoutMarkerCountsAsNoEntries(t *testing.T) {
	home := isolate(t)
	var calls []shellCall
	login := envBlock("PATH=/login")
	got := loginShellEnvWith(fakeShell(&calls, okOut("PATH=/no-marker\n"), okOut(login)), home, "/bin/zsh")
	if !reflect.DeepEqual(got, []string{"PATH=/login"}) {
		t.Fatalf("got %v", got)
	}
	if len(calls) != 2 {
		t.Fatalf("calls %v", calls)
	}
}

func TestInteractiveFailureFallsBackToPlainLogin(t *testing.T) {
	home := isolate(t)
	var calls []shellCall
	fail := func(context.Context) ([]byte, error) { return nil, errors.New("boom") }
	got := loginShellEnvWith(fakeShell(&calls, fail, okOut("noise\n"+envBlock("PATH=/login"))), home, "/bin/zsh")
	if !reflect.DeepEqual(got, []string{"PATH=/login"}) {
		t.Fatalf("got %v", got)
	}
	for _, a := range calls[1].args {
		if a == "-i" {
			t.Fatalf("fallback must not be interactive: %v", calls[1].args)
		}
	}
}

func TestInteractiveTimeoutFallsBackToPlainLogin(t *testing.T) {
	home := isolate(t)
	prev := shellTimeout
	shellTimeout = 20 * time.Millisecond
	t.Cleanup(func() { shellTimeout = prev })
	var calls []shellCall
	hang := func(ctx context.Context) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }
	got := loginShellEnvWith(fakeShell(&calls, hang, okOut(envBlock("PATH=/login"))), home, "/bin/zsh")
	if !reflect.DeepEqual(got, []string{"PATH=/login"}) {
		t.Fatalf("got %v", got)
	}
}

func TestBothShellsFailingFallsBackToOsEnviron(t *testing.T) {
	home := isolate(t)
	t.Setenv("NOTTED_TEST_MARK", "yes")
	var calls []shellCall
	fail := func(context.Context) ([]byte, error) { return nil, errors.New("boom") }
	got := loginShellEnvWith(fakeShell(&calls, fail, fail), home, "/bin/zsh")
	if !envHas(got, "NOTTED_TEST_MARK=yes") {
		t.Fatalf("expected os.Environ fallback, got %d entries", len(got))
	}
}

func writeClaude(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMissingClaudeIsFoundInHomeLocalBinAndPrepended(t *testing.T) {
	home := isolate(t)
	want := writeClaude(t, filepath.Join(home, ".local", "bin"))
	writeClaude(t, filepath.Join(home, ".claude", "local"))
	var calls []shellCall
	out := envBlock("PATH=/usr/bin:/bin", "HOME="+home)
	env := loginShellEnvWith(fakeShell(&calls, okOut(out), okOut("")), home, "/bin/zsh")
	if !envHas(env, "PATH="+filepath.Join(home, ".local", "bin")+":/usr/bin:/bin") {
		t.Fatalf("PATH not prepended: %v", env)
	}
	got, err := LookPath(env, "claude")
	if err != nil || got != want {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestFallbackFoldersAreCheckedInOrder(t *testing.T) {
	home := isolate(t)
	second := writeClaude(t, filepath.Join(home, ".claude", "local"))
	extra := t.TempDir()
	writeClaude(t, extra)
	fallbackDirs = []string{extra}
	var calls []shellCall
	env := loginShellEnvWith(fakeShell(&calls, okOut(envBlock("PATH=/usr/bin")), okOut("")), home, "/bin/zsh")
	got, err := LookPath(env, "claude")
	if err != nil || got != second {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestFallbackFolderWithoutPathEntryStillGetsOne(t *testing.T) {
	home := isolate(t)
	writeClaude(t, filepath.Join(home, ".local", "bin"))
	var calls []shellCall
	env := loginShellEnvWith(fakeShell(&calls, okOut(envBlock("HOME=/h")), okOut("")), home, "/bin/zsh")
	if _, err := LookPath(env, "claude"); err != nil {
		t.Fatalf("expected found, env %v", env)
	}
}

func TestExistingPathHitIsLeftAlone(t *testing.T) {
	home := isolate(t)
	writeClaude(t, filepath.Join(home, ".local", "bin"))
	onPath := t.TempDir()
	writeClaude(t, onPath)
	var calls []shellCall
	out := envBlock("PATH=" + onPath + ":/usr/bin")
	env := loginShellEnvWith(fakeShell(&calls, okOut(out), okOut("")), home, "/bin/zsh")
	if !envHas(env, "PATH="+onPath+":/usr/bin") {
		t.Fatalf("PATH changed: %v", env)
	}
}

func TestNothingFoundLeavesPathUnchanged(t *testing.T) {
	home := isolate(t)
	var calls []shellCall
	env := loginShellEnvWith(fakeShell(&calls, okOut(envBlock("PATH=/usr/bin:/bin")), okOut("")), home, "/bin/zsh")
	if !reflect.DeepEqual(env, []string{"PATH=/usr/bin:/bin"}) {
		t.Fatalf("got %v", env)
	}
}

func TestCleanEnvStillStripsKeysFromLoginShellResult(t *testing.T) {
	home := isolate(t)
	var calls []shellCall
	out := envBlock("PATH=/usr/bin", "ANTHROPIC_API_KEY=x", "ANTHROPIC_AUTH_TOKEN=y", "B=2")
	env := CleanEnv(loginShellEnvWith(fakeShell(&calls, okOut(out), okOut("")), home, "/bin/zsh"))
	if !reflect.DeepEqual(env, []string{"PATH=/usr/bin", "B=2"}) {
		t.Fatalf("got %v", env)
	}
}

func TestRealShellFindsClaude(t *testing.T) {
	if os.Getenv("NOTTED_TEST_REAL_SHELL") != "1" {
		t.Skip("set NOTTED_TEST_REAL_SHELL=1 to run against the real login shell")
	}
	t.Setenv("NOTTED_HOME", t.TempDir())
	p, err := LookPath(LoginShellEnv(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("claude at", p)
}

func TestMultiLineValueDoesNotAddAPathEntry(t *testing.T) {
	home := isolate(t)
	var calls []shellCall
	out := envBlock("PATH=/usr/bin", "MOTD=hello\nPATH=/bogus\nmore", "B=2")
	env := loginShellEnvWith(fakeShell(&calls, okOut(out), okOut("")), home, "/bin/zsh")
	want := []string{"PATH=/usr/bin", "MOTD=hello\nPATH=/bogus\nmore", "B=2"}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("got %q", env)
	}
}

func TestPrependAndLookPathAgreeOnDuplicatePathEntries(t *testing.T) {
	home := isolate(t)
	want := writeClaude(t, filepath.Join(home, ".local", "bin"))
	var calls []shellCall
	out := envBlock("PATH=/first", "A=1", "PATH=/last")
	env := loginShellEnvWith(fakeShell(&calls, okOut(out), okOut("")), home, "/bin/zsh")
	n := 0
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			n++
		}
	}
	if n != 1 || !envHas(env, "PATH="+filepath.Join(home, ".local", "bin")+":/last") {
		t.Fatalf("got %q", env)
	}
	if got, err := LookPath(env, "claude"); err != nil || got != want {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestShellTimeoutKillsGrandchildHoldingThePipe(t *testing.T) {
	home := isolate(t)
	dir := t.TempDir()
	shell := filepath.Join(dir, "slowshell")
	script := "#!/bin/sh\n(sleep 30) &\nsleep 30\n"
	if err := os.WriteFile(shell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := shellTimeout
	shellTimeout = 300 * time.Millisecond
	t.Cleanup(func() { shellTimeout = prev })
	start := time.Now()
	env := loginShellEnvWith(execShell, home, shell)
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("call took %v, a grandchild held the pipe", d)
	}
	if len(env) == 0 {
		t.Fatal("expected os.Environ fallback")
	}
}

func TestWithToolTimeout(t *testing.T) {
	got := WithToolTimeout([]string{"A=1", "MCP_TOOL_TIMEOUT=5", "B=2", "MCP_TOOL_TIMEOUT=9"})
	want := []string{"A=1", "B=2", "MCP_TOOL_TIMEOUT=86400000"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", got)
	}
	if got := WithToolTimeout(nil); len(got) != 1 || got[0] != "MCP_TOOL_TIMEOUT=86400000" {
		t.Fatalf("got %v", got)
	}
}

func TestSessionEnv(t *testing.T) {
	got := SessionEnv([]string{"A=1", "ANTHROPIC_API_KEY=k", "ANTHROPIC_AUTH_TOKEN=t", "MCP_TOOL_TIMEOUT=5"})
	n := 0
	for _, e := range got {
		if strings.HasPrefix(e, "ANTHROPIC_") {
			t.Fatalf("credential left: %s", e)
		}
		if strings.HasPrefix(e, "MCP_TOOL_TIMEOUT=") {
			n++
			if e != "MCP_TOOL_TIMEOUT=86400000" {
				t.Fatalf("got %s", e)
			}
		}
	}
	if n != 1 || got[0] != "A=1" {
		t.Fatalf("got %v", got)
	}
}
