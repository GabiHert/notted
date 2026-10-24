package infra

import (
	"context"
	"strings"
	"testing"

	"github.com/bhlabz/notted/internal/integration/git"
)

func TestGitAdapterEnvDropsAnthropicCredentials(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	src := func() []string {
		return []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "ANTHROPIC_API_KEY=sk-secret", "ANTHROPIC_AUTH_TOKEN=tok", "KEEP=1"}
	}
	repo := t.TempDir()
	sh(t, repo, "init", "-q")
	out, err := git.New(gitEnv(src)).Run(context.Background(), repo, "-c", "alias.showenv=!env", "showenv")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "ANTHROPIC_") || strings.Contains(out, "sk-secret") {
		t.Fatalf("credentials reached git:\n%s", out)
	}
	if !strings.Contains(out, "KEEP=1") {
		t.Fatalf("env not passed on:\n%s", out)
	}
}
