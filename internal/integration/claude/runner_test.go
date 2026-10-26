package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

const (
	notFound  = "Claude Code was not found. Install it and make sure the claude command runs in your terminal."
	notLogged = "Claude Code is not logged in. Run claude in your terminal and log in, then try again."
)

func runnerEnv(binDir, out string, extra ...string) func() []string {
	return func() []string {
		e := []string{"PATH=" + binDir + ":/usr/bin:/bin", "STUB_OUT=" + out,
			"ANTHROPIC_API_KEY=dummy", "ANTHROPIC_AUTH_TOKEN=dummy", "KEEP_ME=yes"}
		return append(e, extra...)
	}
}

func argv(t *testing.T, out string) []string {
	return strings.Split(strings.TrimSuffix(read(t, filepath.Join(out, "argv")), "\n"), "\n")
}

func TestArgumentLists(t *testing.T) {
	const tools = "Read Grep Glob"
	cases := []struct {
		name string
		run  port.ClaudeRun
		want []string
	}{
		{"summary new", port.ClaudeRun{SessionID: "id1", PromptText: "P"},
			[]string{"-p", "--session-id", "id1", "--append-system-prompt-file", "<file>", "--allowedTools", tools}},
		{"summary resume root", port.ClaudeRun{SessionID: "id1", Resume: true, PromptText: "P", ProjectRoot: "/r"},
			[]string{"-p", "--resume", "id1", "--append-system-prompt-file", "<file>", "--allowedTools", tools, "--add-dir", "/r"}},
		{"suggestions", port.ClaudeRun{SessionID: "id1", Resume: true},
			[]string{"-p", "--resume", "id1", "--allowedTools", tools}},
		{"suggestions root", port.ClaudeRun{SessionID: "id1", Resume: true, ProjectRoot: "/r"},
			[]string{"-p", "--resume", "id1", "--allowedTools", tools, "--add-dir", "/r"}},
		{"summary new with tools", port.ClaudeRun{SessionID: "id1", PromptText: "P", MCPConfig: "/m.json"},
			[]string{"-p", "--session-id", "id1", "--mcp-config", "/m.json", "--permission-prompt-tool", "mcp__notted__permission",
				"--append-system-prompt-file", "<file>", "--allowedTools", tools + " mcp__notted__ask"}},
		{"suggestions with tools", port.ClaudeRun{SessionID: "id1", Resume: true, ProjectRoot: "/r", MCPConfig: "/m.json"},
			[]string{"-p", "--resume", "id1", "--mcp-config", "/m.json", "--permission-prompt-tool", "mcp__notted__permission",
				"--allowedTools", tools + " mcp__notted__ask", "--add-dir", "/r"}},
		{"review", port.ClaudeRun{SessionID: "id1", Resume: true, ProjectRoot: "/r", MCPConfig: "/m.json", Timeout: 24 * time.Hour},
			[]string{"-p", "--resume", "id1", "--mcp-config", "/m.json", "--permission-prompt-tool", "mcp__notted__permission",
				"--allowedTools", tools + " mcp__notted__ask", "--add-dir", "/r"}},
		{"review act without asking", port.ClaudeRun{SessionID: "id1", Resume: true, ProjectRoot: "/r", MCPConfig: "/m.json", Timeout: 24 * time.Hour, ActWithoutAsking: true},
			[]string{"-p", "--resume", "id1", "--permission-mode", domain.ReviewPermissionMode, "--mcp-config", "/m.json", "--permission-prompt-tool", "mcp__notted__permission",
				"--allowedTools", tools + " mcp__notted__ask", "--add-dir", "/r"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkout, binDir, out := fixture(t)
			c.run.Checkout = checkout
			r := &Runner{EnvFn: runnerEnv(binDir, out)}
			if _, err := r.Run(context.Background(), c.run); err != nil {
				t.Fatal(err)
			}
			got := argv(t, out)
			want := append([]string(nil), c.want...)
			for i := range want {
				if want[i] == "<file>" {
					want[i] = got[i]
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q\nwant %q", got, want)
			}
			needles := []string{"skip-" + "permissions"}
			if !c.run.ActWithoutAsking {
				needles = append(needles, "bypass"+"Permissions", "--permission-mode")
			}
			for _, a := range got {
				for _, n := range needles {
					if strings.Contains(a, n) {
						t.Fatalf("forbidden argument %q", a)
					}
				}
			}
		})
	}
}

func TestToolTimeoutOnceAndNoCredentials(t *testing.T) {
	checkout, binDir, out := fixture(t)
	r := &Runner{EnvFn: runnerEnv(binDir, out, "MCP_TOOL_TIMEOUT=5")}
	if _, err := r.Run(context.Background(), port.ClaudeRun{Checkout: checkout, SessionID: "i", Resume: true}); err != nil {
		t.Fatal(err)
	}
	env := read(t, filepath.Join(out, "env"))
	if n := strings.Count(env, "MCP_TOOL_TIMEOUT="); n != 1 || !strings.Contains(env, "MCP_TOOL_TIMEOUT=86400000\n") {
		t.Fatalf("MCP_TOOL_TIMEOUT count %d in %q", n, env)
	}
	if strings.Contains(env, "ANTHROPIC_API_KEY") || strings.Contains(env, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatal("credentials reached the child")
	}
}

func TestRunTimeoutMessageUsesRunTimeout(t *testing.T) {
	checkout, binDir, out := fixture(t)
	r := &Runner{EnvFn: runnerEnv(binDir, out, "STUB_SLEEP=2"), Timeout: 100 * time.Millisecond}
	_, err := r.Run(context.Background(), port.ClaudeRun{Checkout: checkout, SessionID: "i", Resume: true, Timeout: 24 * time.Hour})
	if err == nil || err.Error() != "Claude Code did not finish within 24 hours." {
		t.Fatalf("got %v", err)
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Hour: "1 hour", 24 * time.Hour: "24 hours", time.Minute: "1 minute", 15 * time.Minute: "15 minutes", 90 * time.Minute: "90 minutes",
	} {
		if got := humanDuration(d); got != want {
			t.Fatalf("%v: got %q want %q", d, got, want)
		}
	}
}

func TestSuggestionsHasNoPromptFile(t *testing.T) {
	checkout, binDir, out := fixture(t)
	r := &Runner{EnvFn: runnerEnv(binDir, out)}
	if _, err := r.Run(context.Background(), port.ClaudeRun{Checkout: checkout, SessionID: "i", Resume: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "prompt-path")); err == nil {
		t.Fatal("a prompt file was passed")
	}
}

func TestWorkingDirEnvStdinAndPromptFile(t *testing.T) {
	checkout, binDir, out := fixture(t)
	transcript := strings.Repeat("[00:01] Ana: héllo wörld, 日本語 line\n\n  indented\ttab\n", 4000)
	if len(transcript) < 200*1024 {
		t.Fatal("transcript too small")
	}
	r := &Runner{EnvFn: runnerEnv(binDir, out)}
	reply, err := r.Run(context.Background(), port.ClaudeRun{
		Checkout: checkout, SessionID: "id", PromptText: "PROMPT TEXT\nline2", Stdin: transcript})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "# Summary: stub" {
		t.Fatalf("reply %q", reply)
	}
	wantCwd, _ := filepath.EvalSymlinks(checkout)
	if got := strings.TrimSpace(read(t, filepath.Join(out, "cwd"))); got != wantCwd {
		t.Fatalf("cwd %q want %q", got, wantCwd)
	}
	env := read(t, filepath.Join(out, "env"))
	if strings.Contains(env, "ANTHROPIC_API_KEY") || strings.Contains(env, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatal("credentials reached the child")
	}
	if !strings.Contains(env, "KEEP_ME=yes") {
		t.Fatal("rest of the environment was dropped")
	}
	if read(t, filepath.Join(out, "stdin")) != transcript {
		t.Fatal("stdin differs")
	}
	if read(t, filepath.Join(out, "prompt")) != "PROMPT TEXT\nline2" {
		t.Fatal("prompt file content differs")
	}
	p := strings.TrimSpace(read(t, filepath.Join(out, "prompt-path")))
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("prompt file still exists: %v", err)
	}
}

func TestFailureMessages(t *testing.T) {
	run := func(t *testing.T, timeout time.Duration, extra ...string) error {
		checkout, binDir, out := fixture(t)
		r := &Runner{EnvFn: runnerEnv(binDir, out, extra...), Timeout: timeout}
		_, err := r.Run(context.Background(), port.ClaudeRun{Checkout: checkout, SessionID: "i", Resume: true})
		return err
	}
	t.Run("login", func(t *testing.T) {
		err := run(t, 0, "STUB_EXIT=1", "STUB_STDERR=Invalid API key · Please run /login")
		if err == nil || err.Error() != notLogged {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("not logged in in stdout", func(t *testing.T) {
		err := run(t, 0, "STUB_EXIT=1", "STUB_REPLY=You are Not Logged In")
		if err == nil || err.Error() != notLogged {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("exit 3", func(t *testing.T) {
		err := run(t, 0, "STUB_EXIT=3", "STUB_STDERR=boom happened", "STUB_REPLY=partial out")
		want := "Claude Code stopped with an error (exit 3):\nboom happened\npartial out"
		if err == nil || err.Error() != want {
			t.Fatalf("got %q", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		start := time.Now()
		err := run(t, 100*time.Millisecond, "STUB_SLEEP=2")
		if err == nil || err.Error() != "Claude Code did not finish within 15 minutes." {
			t.Fatalf("got %v", err)
		}
		if time.Since(start) > 1900*time.Millisecond {
			t.Fatal("did not stop the child promptly")
		}
	})
	t.Run("not found", func(t *testing.T) {
		checkout, _, _ := fixture(t)
		r := &Runner{EnvFn: func() []string { return []string{"PATH=" + t.TempDir()} }}
		_, err := r.Run(context.Background(), port.ClaudeRun{Checkout: checkout, SessionID: "i"})
		if err == nil || err.Error() != notFound {
			t.Fatalf("got %v", err)
		}
	})
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func waitGone(t *testing.T, childFile string) {
	t.Helper()
	b, err := os.ReadFile(childFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	for i := 0; i < 40; i++ {
		if !alive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("grandchild %d survived", pid)
}

func TestTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	checkout, binDir, out := fixture(t)
	child := filepath.Join(t.TempDir(), "child.pid")
	r := Runner{EnvFn: runnerEnv(binDir, out, "STUB_SLEEP=20", "STUB_CHILD="+child), Timeout: 500 * time.Millisecond}
	if _, err := r.Run(context.Background(), port.ClaudeRun{Checkout: checkout, SessionID: "i", Resume: true}); err == nil {
		t.Fatal("expected timeout")
	}
	waitGone(t, child)
}

func TestCancelReturnsCanceledAndKillsGroup(t *testing.T) {
	checkout, binDir, out := fixture(t)
	child := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(700*time.Millisecond, cancel)
	r := Runner{EnvFn: runnerEnv(binDir, out, "STUB_SLEEP=20", "STUB_CHILD="+child)}
	_, err := r.Run(ctx, port.ClaudeRun{Checkout: checkout, SessionID: "i", Resume: true})
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "exit") {
		t.Fatalf("got %v", err)
	}
	waitGone(t, child)
}

func TestRunUsesClaudeRunTimeoutAsDeadline(t *testing.T) {
	checkout, binDir, out := fixture(t)
	r := &Runner{EnvFn: runnerEnv(binDir, out, "STUB_SLEEP=5")}
	start := time.Now()
	_, err := r.Run(context.Background(), port.ClaudeRun{Checkout: checkout, SessionID: "i", Resume: true, Timeout: 300 * time.Millisecond})
	if err == nil || !strings.HasPrefix(err.Error(), "Claude Code did not finish within") {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("ClaudeRun.Timeout was not the deadline")
	}
}
