// Copied from github.com/brienze1/quant (internal/application/service/job_manager.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.

package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

const (
	defaultTimeout = 15 * time.Minute
	readOnlyTools  = "Read Grep Glob"
	killGrace      = 300 * time.Millisecond
)

// Runner runs the person's own claude command headless, with read-only tools.
type Runner struct {
	EnvFn   func() []string // default LoginShellEnv
	Timeout time.Duration   // default 15 minutes
}

var _ port.ClaudeRunner = Runner{}

func buildArgs(r port.ClaudeRun, promptFile string) []string {
	args := []string{"-p"}
	if r.Resume {
		args = append(args, "--resume", r.SessionID)
	} else {
		args = append(args, "--session-id", r.SessionID)
	}
	tools := readOnlyTools
	if r.MCPConfig != "" {
		args = append(args, "--mcp-config", r.MCPConfig, "--permission-prompt-tool", domain.PermissionToolName)
		tools += " " + domain.AskToolName
	}
	if promptFile != "" {
		args = append(args, "--append-system-prompt-file", promptFile)
	}
	args = append(args, "--allowedTools", tools)
	if r.ProjectRoot != "" {
		args = append(args, "--add-dir", r.ProjectRoot)
	}
	return args
}

// humanDuration words a timeout: whole hours, else whole minutes.
func humanDuration(d time.Duration) string {
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return plural(int(d/time.Hour), "hour")
	}
	return plural(int(d/time.Minute), "minute")
}

// Run executes claude in the checkout, feeding r.Stdin whole, and returns its stdout.
func (rn Runner) Run(ctx context.Context, r port.ClaudeRun) (string, error) {
	envFn := rn.EnvFn
	if envFn == nil {
		envFn = LoginShellEnv
	}
	timeout := rn.Timeout
	if timeout <= 0 {
		timeout = r.Timeout
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	limit := "15 minutes"
	if r.Timeout > 0 {
		limit = humanDuration(r.Timeout)
	}
	env := WithToolTimeout(CleanEnv(envFn()))
	path, err := LookPath(env, "claude")
	if err != nil {
		return "", errors.New("Claude Code was not found. Install it and make sure the claude command runs in your terminal.")
	}

	promptFile := ""
	if r.PromptText != "" {
		f, err := os.CreateTemp("", "notted-prompt-*.md")
		if err != nil {
			return "", fmt.Errorf("could not write the prompt file: %w", err)
		}
		promptFile = f.Name()
		defer os.Remove(promptFile)
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			return "", fmt.Errorf("could not write the prompt file: %w", err)
		}
		if _, err := f.WriteString(r.PromptText); err != nil {
			f.Close()
			return "", fmt.Errorf("could not write the prompt file: %w", err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("could not write the prompt file: %w", err)
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, buildArgs(r, promptFile)...)
	cmd.Dir = r.Checkout
	cmd.Env = env
	cmd.Stdin = strings.NewReader(r.Stdin)
	// Own process group: on timeout or cancel the whole group gets SIGTERM, then SIGKILL after the grace period.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		err := syscall.Kill(-pid, syscall.SIGTERM)
		time.AfterFunc(killGrace, func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
		return err
	}
	cmd.WaitDelay = killGrace + time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", fmt.Errorf("claude run cancelled: %w", context.Canceled)
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("Claude Code did not finish within %s.", limit)
	}
	if err == nil {
		return stdout.String(), nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return "", fmt.Errorf("Claude Code could not start: %w", err)
	}
	both := strings.ToLower(stdout.String() + "\n" + stderr.String())
	if strings.Contains(both, "/login") || strings.Contains(both, "not logged in") {
		return "", errors.New("Claude Code is not logged in. Run claude in your terminal and log in, then try again.")
	}
	msg := fmt.Sprintf("Claude Code stopped with an error (exit %d):", exitErr.ExitCode())
	if t := lastLines(stderr.String(), 20); t != "" {
		msg += "\n" + t
	}
	if t := lastLines(stdout.String(), 20); t != "" {
		msg += "\n" + t
	}
	return "", errors.New(msg)
}
