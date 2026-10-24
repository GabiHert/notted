package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bhlabz/notted/internal/application/port"
)

var _ port.PromptSource = PromptSource{}

// PromptSource gets a meeting's prompt from the archive's own bin/meeting command.
type PromptSource struct {
	EnvFn func() []string // default LoginShellEnv
}

// Prompt runs `bin/meeting prompt --project <projectKey>` and returns its stdout unchanged.
func (p PromptSource) Prompt(ctx context.Context, checkout, profile, projectKey string) (string, error) {
	bin := filepath.Join(checkout, "bin", "meeting")
	st, err := os.Stat(bin)
	if err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		return "", errors.New("That folder is not a meetings archive checkout: bin/meeting is missing.")
	}
	envFn := p.EnvFn
	if envFn == nil {
		envFn = LoginShellEnv
	}
	cmd := exec.CommandContext(ctx, bin, "prompt", "--project", projectKey)
	cmd.Dir = checkout
	cmd.Env = append(CleanEnv(envFn()), "MEETINGS_PROFILE="+profile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", errors.New("bin/meeting prompt failed: " + lastLines(stderr.String(), 1))
	}
	return stdout.String(), nil
}

// lastLines returns the last n non-empty lines of s, joined by newlines.
func lastLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, "\r"))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
