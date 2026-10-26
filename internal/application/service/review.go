package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

type runningChecker interface{ Running(meetingID string) bool }

// SessionBusyCheck refuses the terminal session while a review or a summary runs.
func SessionBusyCheck(review, summary runningChecker) func(string) string {
	return func(id string) string {
		if review.Running(id) {
			return "Suggestions are being reviewed for this meeting. Open the session when the review finishes."
		}
		if summary.Running(id) {
			return "A summary is being written for this meeting. Open the session when it finishes."
		}
		return ""
	}
}

// SummaryBusyCheck refuses a summary while a review runs.
func SummaryBusyCheck(review runningChecker) func(string) string {
	return func(id string) string {
		if review.Running(id) {
			return "Suggestions are being reviewed for this meeting. Wait until the review finishes."
		}
		return ""
	}
}

const (
	reviewTimeout  = 24 * time.Hour
	reviewReplyMax = 4000
)

// ReviewDeps are the collaborators of ReviewService.
type ReviewDeps struct {
	Store     port.MeetingStore
	Settings  port.SettingsStore
	Mode      port.ReviewModeStore // nil: always off
	Runner    port.ClaudeRunner
	Terminal  runningChecker // port.TerminalManager satisfies it
	Summary   runningChecker
	MCPConfig func(meetingID string) (string, error)
	Emit      port.EventEmitter
	ReadFile  func(path string) ([]byte, error) // nil: os.ReadFile
}

// ReviewService has Claude Code review a meeting's suggestions through Notted's forms.
type ReviewService struct {
	store     port.MeetingStore
	settings  port.SettingsStore
	mode      port.ReviewModeStore
	runner    port.ClaudeRunner
	terminal  runningChecker
	summary   runningChecker
	mcpConfig func(meetingID string) (string, error)
	emit      port.EventEmitter
	readFile  func(path string) ([]byte, error)

	mu     sync.Mutex
	active map[string]*summaryRun
	wg     sync.WaitGroup
}

// NewReviewService builds the review service.
func NewReviewService(d ReviewDeps) *ReviewService {
	rf := d.ReadFile
	if rf == nil {
		rf = os.ReadFile
	}
	return &ReviewService{store: d.Store, settings: d.Settings, mode: d.Mode, runner: d.Runner, terminal: d.Terminal, summary: d.Summary,
		mcpConfig: d.MCPConfig, emit: d.Emit, readFile: rf, active: map[string]*summaryRun{}}
}

// Running reports whether a review runs for the meeting.
func (s *ReviewService) Running(meetingID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[meetingID] != nil
}

// Cancel ends the meeting's running review, if any, and waits for it to stop. A cancelled turn reports nothing.
func (s *ReviewService) Cancel(meetingID string) {
	s.mu.Lock()
	r := s.active[meetingID]
	s.mu.Unlock()
	if r == nil {
		return
	}
	r.cancel()
	<-r.done
}

// StopAll cancels every running review and waits for each to stop.
func (s *ReviewService) StopAll() {
	s.mu.Lock()
	runs := make([]*summaryRun, 0, len(s.active))
	for _, r := range s.active {
		runs = append(runs, r)
	}
	s.mu.Unlock()
	for _, r := range runs {
		r.cancel()
	}
	for _, r := range runs {
		<-r.done
	}
}

// Wait blocks until no review is active. It is a test helper.
func (s *ReviewService) Wait() { s.wg.Wait() }

// Start checks the meeting, then runs one headless review turn in the background.
func (s *ReviewService) Start(ctx context.Context, meetingID string) error {
	m, err := s.store.GetMeeting(ctx, meetingID)
	if errors.Is(err, domain.ErrNotFound) {
		return errors.New("Meeting not found.")
	}
	if err != nil {
		return err
	}
	if m.SuggestionsMD == "" || m.ClaudeSessionID == "" {
		return errors.New("This meeting has no suggestions yet. Generate the summary first.")
	}
	if m.Folder == "" {
		return errors.New("This meeting's files are not in the archive folder yet. Generate the summary again to write them.")
	}
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return err
	}
	if st.CheckoutPath == "" {
		return errors.New("Set the meetings archive folder in Settings > Instructions first.")
	}

	on := false
	if s.mode != nil {
		on, err = s.mode.GetReviewActWithoutAsking(ctx)
		if err != nil {
			return fmt.Errorf("Notted could not read the review setting: %w", err)
		}
	}

	s.mu.Lock()
	if s.terminal.Running(meetingID) {
		s.mu.Unlock()
		return errors.New("Close the Claude Code session on this page first. The review and the session use the same conversation.")
	}
	if s.summary.Running(meetingID) {
		s.mu.Unlock()
		return errors.New("A summary is being written for this meeting. Wait until it finishes.")
	}
	if s.active[meetingID] != nil {
		s.mu.Unlock()
		return errors.New("Suggestions are already being reviewed for this meeting.")
	}
	file, err := s.mcpConfig(meetingID)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("Notted could not prepare the session's tools: %w", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	r := &summaryRun{cancel: cancel, done: make(chan struct{})}
	s.active[meetingID] = r
	s.wg.Add(1)
	s.mu.Unlock()

	s.state(runCtx, meetingID, "running", "")
	run := port.ClaudeRun{Checkout: st.CheckoutPath, SessionID: m.ClaudeSessionID, Resume: true,
		ProjectRoot: st.ProjectRoots[m.ProjectKey], MCPConfig: file, Timeout: reviewTimeout, Stdin: reviewStdin(m), ActWithoutAsking: on}
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.active, meetingID)
			s.mu.Unlock()
			cancel()
			close(r.done)
			s.wg.Done()
		}()
		s.finish(runCtx, m, st.CheckoutPath, run)
	}()
	return nil
}

// ActWithoutAsking says whether the next review runs without permission prompts. Off when never saved.
func (s *ReviewService) ActWithoutAsking(ctx context.Context) (bool, error) {
	if s.mode == nil {
		return false, nil
	}
	on, err := s.mode.GetReviewActWithoutAsking(ctx)
	if err != nil {
		return false, fmt.Errorf("Notted could not read the review setting: %w", err)
	}
	return on, nil
}

// SetActWithoutAsking saves the switch. A running review keeps the mode it started with.
func (s *ReviewService) SetActWithoutAsking(ctx context.Context, on bool) error {
	if s.mode == nil {
		return errors.New("Notted could not save the review setting: no store.")
	}
	if err := s.mode.SetReviewActWithoutAsking(ctx, on); err != nil {
		return fmt.Errorf("Notted could not save the review setting: %w", err)
	}
	return nil
}

func (s *ReviewService) finish(ctx context.Context, m domain.Meeting, checkout string, run port.ClaudeRun) {
	out, err := s.runner.Run(ctx, run)
	if ctx.Err() != nil {
		return // cancelled: the meeting may be gone
	}
	if err != nil {
		s.state(ctx, m.ID, "error", err.Error())
		return
	}
	s.reload(ctx, m, checkout)
	reply := []rune(strings.TrimSpace(out))
	if len(reply) > reviewReplyMax {
		reply = reply[len(reply)-reviewReplyMax:]
	}
	s.state(ctx, m.ID, "done", string(reply))
}

// reload reads the meeting's suggestions.md from the archive and stores it, when it stays inside the archive.
func (s *ReviewService) reload(ctx context.Context, m domain.Meeting, checkout string) {
	path := filepath.Join(checkout, filepath.FromSlash(m.Folder), "suggestions.md")
	rel, err := filepath.Rel(checkout, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	b, err := s.readFile(path)
	if err != nil {
		return
	}
	content, found := extractFrom(string(b), "# Suggestions:")
	if !found {
		return
	}
	if err := s.store.SetSuggestions(ctx, m.ID, content); err != nil {
		log.Printf("review: reload suggestions %s: %v", m.ID, err)
	}
}

func (s *ReviewService) state(ctx context.Context, id, state, message string) {
	if ctx.Err() != nil {
		return
	}
	p := map[string]any{"meetingId": id, "state": state}
	if message != "" {
		p["message"] = message
	}
	s.emit.Emit("review:state", p)
}

// reviewStdin is the fixed review prompt. Only <title> and <folder> are replaced; the other angle brackets are for Claude.
func reviewStdin(m domain.Meeting) string {
	return strings.NewReplacer("<title>", m.Title, "<folder>", m.Folder).Replace(reviewPrompt)
}

const reviewPrompt = `Review this meeting's suggestions with me through Notted's ask tool (mcp__notted__ask). I am not reading a terminal, so ask me nothing outside ask.

Meeting: <title>
Meeting folder: <folder>

1. Read <folder>/suggestions.md. Take every suggestion whose Decision is undecided. If there is none, reply with the single line "No suggestions are waiting for a decision." and stop.
2. Call ask once. Title: "Review suggestions: <title>". One piece: kind multi, id "accept", question "Which suggestions do you accept? The ones you leave unchecked are rejected.", and one option per undecided suggestion, in order, with label "<id>: <task title>" and description "<verdict>. <why>".
3. If the status is dismissed, change nothing and reply with the single line "The review was dismissed. Nothing changed." Then stop.
4. Otherwise run the meetings skill's accept command (.claude/skills/meetings/SKILL.md) for <folder>: the checked suggestions are accepted and the unchecked ones are rejected. Read the form's note as my comment on the review.
5. Ask every yes-or-no question the skill or my instructions need through ask: one form with a text piece that shows what you will do (the paths, the file content, the diff) and one confirm piece. Only a confirm answered yes is a yes. A no or a dismissed form is a no.
6. Finish with one line per suggestion: its id, accepted or rejected, and its outcome.`
