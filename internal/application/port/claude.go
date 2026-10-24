package port

import "context"

// PromptSource assembles a meeting's prompt with the archive's own command.
type PromptSource interface {
	// Prompt runs `bin/meeting prompt --project <projectKey>` in checkout with MEETINGS_PROFILE=profile.
	Prompt(ctx context.Context, checkout, profile, projectKey string) (string, error)
}

// ClaudeRun describes one headless Claude Code run.
type ClaudeRun struct {
	Checkout    string // working directory (Claude Code keys sessions by it)
	SessionID   string // uuid
	Resume      bool   // false: --session-id; true: --resume
	PromptText  string // "" = no --append-system-prompt-file (the suggestions pass)
	ProjectRoot string // "" = no --add-dir
	Stdin       string // instruction + meeting details + whole transcript
}

// ClaudeRunner runs the person's own claude command and returns its stdout.
type ClaudeRunner interface {
	Run(ctx context.Context, r ClaudeRun) (stdout string, err error)
}
