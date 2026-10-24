// Copied from github.com/brienze1/quant (internal/application/service/job_manager.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.

package claude

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LoginShellEnv returns the environment of the user's login shell. An app started
// from Finder has a minimal PATH; the login shell's PATH is where claude lives.
func LoginShellEnv() []string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell, "-l", "-c", "env")
	output, err := cmd.Output()
	if err != nil {
		return os.Environ()
	}
	var env []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "=") {
			env = append(env, line)
		}
	}
	if len(env) == 0 {
		return os.Environ()
	}
	return env
}

// CleanEnv drops the API credentials so the run uses the person's subscription login.
func CleanEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") || strings.HasPrefix(e, "ANTHROPIC_AUTH_TOKEN=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// LookPath searches the PATH entry of env (not the app's own) for an executable file.
func LookPath(env []string, name string) (string, error) {
	var path string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			path = v
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", errors.New(name + " not found in PATH")
}
