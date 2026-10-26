package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

// SessionService runs a meeting's own Claude Code conversation in a terminal.
type SessionService struct {
	store    port.MeetingStore
	settings port.SettingsStore
	term     port.TerminalManager
	envFn    func() []string
	cleanEnv func(env []string) []string
	lookPath func(env []string, name string) (string, error)

	mcpConfig func(meetingID string) (string, error) // nil = no Notted tools
	busy      func(meetingID string) string          // nil = never busy; "" = free
}

// SetMCPConfig sets the function that writes a meeting's tool server file. nil turns the tools off.
func (s *SessionService) SetMCPConfig(fn func(meetingID string) (string, error)) { s.mcpConfig = fn }

// SetBusyCheck sets the check that refuses a start while another run uses the conversation: "" = free, else the refusal.
func (s *SessionService) SetBusyCheck(fn func(meetingID string) string) { s.busy = fn }

// NewSessionService builds the session service. cleanEnv removes the API credentials from the environment.
func NewSessionService(store port.MeetingStore, settings port.SettingsStore, term port.TerminalManager,
	envFn func() []string, cleanEnv func([]string) []string,
	lookPath func(env []string, name string) (string, error)) *SessionService {
	return &SessionService{store: store, settings: settings, term: term, envFn: envFn, cleanEnv: cleanEnv, lookPath: lookPath}
}

var errNotRunning = errors.New("The session is not running.")

// Start resumes the meeting's Claude Code session from the archive checkout. Claude Code's own permission prompts stay on.
func (s *SessionService) Start(ctx context.Context, meetingID string) error {
	m, err := s.store.GetMeeting(ctx, meetingID)
	if errors.Is(err, domain.ErrNotFound) {
		return errors.New("Meeting not found.")
	}
	if err != nil {
		return err
	}
	if m.ClaudeSessionID == "" {
		return errors.New("This meeting has no Claude Code session yet. Generate the summary first.")
	}
	if s.busy != nil {
		if msg := s.busy(meetingID); msg != "" {
			return errors.New(msg)
		}
	}
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return err
	}
	if st.CheckoutPath == "" {
		return errors.New("Set the meetings archive folder in Settings > Instructions first.")
	}
	env := s.cleanEnv(s.envFn())
	claudePath, err := s.lookPath(env, "claude")
	if err != nil {
		return errors.New("Claude Code was not found. Install it and make sure the claude command runs in your terminal.")
	}
	args := []string{claudePath, "--resume", m.ClaudeSessionID}
	if s.mcpConfig != nil {
		file, err := s.mcpConfig(meetingID)
		if err != nil {
			return fmt.Errorf("Notted could not prepare the session's tools: %w", err)
		}
		args = append(args, "--mcp-config", file, "--allowedTools", domain.AskToolName)
	}
	if root := st.ProjectRoots[m.ProjectKey]; root != "" {
		args = append(args, "--add-dir", root)
	}
	return s.term.Start(meetingID, st.CheckoutPath, args, env)
}

// Write sends keystrokes to the running session.
func (s *SessionService) Write(meetingID, data string) error {
	if !s.term.Running(meetingID) {
		return errNotRunning
	}
	return s.term.Write(meetingID, data)
}

// Resize sets the terminal size of the running session.
func (s *SessionService) Resize(meetingID string, cols, rows int) error {
	if !s.term.Running(meetingID) {
		return errNotRunning
	}
	if cols < 1 || rows < 1 || cols > 65535 || rows > 65535 {
		return nil // an out-of-range size is ignored
	}
	return s.term.Resize(meetingID, uint16(cols), uint16(rows))
}

// Stop ends the session's process. The Claude Code session stays resumable.
func (s *SessionService) Stop(meetingID string) error { return s.term.Stop(meetingID) }

// StopAll ends every session's process.
func (s *SessionService) StopAll() { s.term.StopAll() }
