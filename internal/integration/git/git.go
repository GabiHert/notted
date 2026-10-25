// Package git runs the person's git in an archive checkout.
package git

import (
	"context"
	"os/exec"
)

// Git implements port.Git with the git command.
type Git struct {
	envFn func() []string
}

// New builds a Git whose commands run with the environment envFn returns
// (the login shell's, so SSH_AUTH_SOCK and PATH reach git push).
func New(envFn func() []string) *Git { return &Git{envFn: envFn} }

// Run executes `git -C repo args...` and returns its combined output.
func (g *Git) Run(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(append([]string{}, g.envFn()...), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}
