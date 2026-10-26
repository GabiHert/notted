package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
	"github.com/google/uuid"
)

type exporter interface {
	ExportMeeting(ctx context.Context, meetingID string) (ExportResult, error)
}

// SummaryService has Claude Code write a meeting's summary and task suggestions.
type SummaryService struct {
	store    port.MeetingStore
	settings port.SettingsStore
	prompts  port.PromptSource
	runner   port.ClaudeRunner
	export   exporter
	emit     port.EventEmitter
	newID    func() string

	mcpConfig func(meetingID string) (string, error) // nil = no Notted tools
	busy      func(meetingID string) string          // nil = never busy; "" = free

	mu     sync.Mutex
	active map[string]*summaryRun
	wg     sync.WaitGroup
}

// summaryRun is one meeting's running summary: cancel ends it, done closes when it has ended.
type summaryRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// NewSummaryService builds the summary service.
func NewSummaryService(store port.MeetingStore, settings port.SettingsStore, prompts port.PromptSource,
	runner port.ClaudeRunner, export exporter, emit port.EventEmitter) *SummaryService {
	return &SummaryService{store: store, settings: settings, prompts: prompts, runner: runner, export: export,
		emit: emit, newID: uuid.NewString, active: map[string]*summaryRun{}}
}

// SetMCPConfig sets the function that writes a meeting's tool server file. nil turns the tools off.
func (s *SummaryService) SetMCPConfig(fn func(meetingID string) (string, error)) { s.mcpConfig = fn }

// SetBusyCheck sets the check that refuses Generate while another run uses the conversation: "" = free, else the refusal.
func (s *SummaryService) SetBusyCheck(fn func(meetingID string) string) { s.busy = fn }

// Running reports whether a summary is being written for the meeting.
func (s *SummaryService) Running(meetingID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[meetingID] != nil
}

// Cancel ends the meeting's running summary, if any, and waits for it to stop. A cancelled run reports nothing.
func (s *SummaryService) Cancel(meetingID string) {
	s.mu.Lock()
	r := s.active[meetingID]
	s.mu.Unlock()
	if r == nil {
		return
	}
	r.cancel()
	<-r.done
}

// Wait blocks until no run is active. It is a test helper.
func (s *SummaryService) Wait() { s.wg.Wait() }

const suggestionsStdin = "Use the meetings skill's `suggest` format (its file is .claude/skills/meetings/SKILL.md) to write this meeting's suggestions.md from the summary you just wrote and the transcript above, checking the existing-work sources the instructions name. Do not write any file. Every Decision is `undecided`. Reply with the content of suggestions.md only: start with the line \"# Suggestions: <title>\" and add nothing before or after it."

func summaryStdin(m domain.Meeting, segs []domain.Segment) string {
	date := m.StartedAt.In(time.Local).Format("2006-01-02")
	return "Use the meetings skill's `process` format (its file is .claude/skills/meetings/SKILL.md) to write this meeting's summary.md, but do not write, stage or commit any file and do not run the save step. Reply with the content of summary.md only: start with the line \"# Summary: " + m.Title + "\" and add nothing before or after it.\n\n" +
		"Meeting: " + m.Title + "\n" +
		"Date: " + date + "\n" +
		"Project: " + m.ProjectKey + "\n" +
		"Language: " + string(m.Language) + "\n" +
		"Transcript lines: (mic) is the person who recorded; (others) is everyone else.\n\n" +
		transcriptLines(segs)
}

// extractFrom drops every line before the first line that starts with header.
func extractFrom(out, header string) (string, bool) {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, header) {
			return strings.TrimRight(strings.Join(lines[i:], "\n"), " \t\r\n"), true
		}
	}
	return "", false
}

// Generate checks the meeting, then runs both passes in the background.
func (s *SummaryService) Generate(ctx context.Context, meetingID string) error {
	m, err := s.store.GetMeeting(ctx, meetingID)
	if errors.Is(err, domain.ErrNotFound) {
		return errors.New("Meeting not found.")
	}
	if err != nil {
		return err
	}
	if m.State == domain.StateRecording || m.State == domain.StateTranscribing {
		return errors.New("The transcript is not finished yet. Wait until transcription completes.")
	}
	if m.ProjectKey == "" {
		return errors.New("Choose a project for this meeting first.")
	}
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return err
	}
	if st.Profile == "" {
		return errors.New("Set your profile in Settings > Instructions first.")
	}
	if st.CheckoutPath == "" {
		return errors.New("Set the meetings archive folder in Settings > Instructions first.")
	}
	segs, err := s.store.ListSegments(ctx, meetingID)
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		return errors.New("The transcript is empty. Nothing to summarize.")
	}
	if s.busy != nil {
		if msg := s.busy(meetingID); msg != "" {
			return errors.New(msg)
		}
	}
	s.mu.Lock()
	if s.active[meetingID] != nil {
		s.mu.Unlock()
		return errors.New("A summary is already being written for this meeting.")
	}
	runCtx, cancel := context.WithCancel(context.Background())
	r := &summaryRun{cancel: cancel, done: make(chan struct{})}
	s.active[meetingID] = r
	s.wg.Add(1)
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		delete(s.active, meetingID)
		s.mu.Unlock()
		cancel()
		close(r.done)
		s.wg.Done()
	}

	// Read again inside the lock: a run that finished since the first read may have stored a session id.
	m, err = s.store.GetMeeting(ctx, meetingID)
	if err != nil {
		release()
		return err
	}

	go func() {
		defer release()
		s.run(runCtx, m, st, segs)
	}()
	return nil
}

func (s *SummaryService) state(ctx context.Context, id, pass, state, message string) {
	if ctx.Err() != nil {
		return // cancelled: the meeting may be gone
	}
	p := map[string]any{"meetingId": id, "pass": pass, "state": state}
	if message != "" {
		p["message"] = message
	}
	s.emit.Emit("summary:state", p)
}

func (s *SummaryService) run(ctx context.Context, m domain.Meeting, st domain.Settings, segs []domain.Segment) {
	id := m.ID
	prompt, err := s.prompts.Prompt(ctx, st.CheckoutPath, st.Profile, m.ProjectKey)
	if err != nil {
		s.state(ctx, id, "summary", "error", err.Error())
		return
	}
	mcpFile := ""
	if s.mcpConfig != nil {
		mcpFile, err = s.mcpConfig(id)
		if err != nil {
			s.state(ctx, id, "summary", "error", "Notted could not prepare the session's tools: "+err.Error())
			return
		}
	}
	sid, resume := m.ClaudeSessionID, m.ClaudeSessionID != ""
	if !resume {
		sid = s.newID()
	}
	root := st.ProjectRoots[m.ProjectKey]

	s.state(ctx, id, "summary", "running", "")
	out, err := s.runner.Run(ctx, port.ClaudeRun{Checkout: st.CheckoutPath, SessionID: sid, Resume: resume,
		PromptText: prompt, ProjectRoot: root, MCPConfig: mcpFile, Stdin: summaryStdin(m, segs)})
	if err != nil {
		s.state(ctx, id, "summary", "error", err.Error())
		return
	}
	summary, found := extractFrom(out, "# Summary:")
	if !found {
		s.state(ctx, id, "summary", "error", "Claude Code returned no summary. Nothing was saved.")
		return
	}
	if !resume {
		if err := s.store.SetClaudeSessionID(ctx, id, sid); err != nil {
			s.state(ctx, id, "summary", "error", err.Error())
			return
		}
	}
	if err := s.store.SetSummary(ctx, id, summary); err != nil {
		s.state(ctx, id, "summary", "error", err.Error())
		return
	}
	s.state(ctx, id, "summary", "done", "")

	s.state(ctx, id, "suggestions", "running", "")
	out, err = s.runner.Run(ctx, port.ClaudeRun{Checkout: st.CheckoutPath, SessionID: sid, Resume: true,
		ProjectRoot: root, MCPConfig: mcpFile, Stdin: strings.Replace(suggestionsStdin, "<title>", m.Title, 1)})
	if err != nil {
		s.state(ctx, id, "suggestions", "error", err.Error())
		return
	}
	suggestions, found := extractFrom(out, "# Suggestions:")
	if !found {
		s.state(ctx, id, "suggestions", "error", "Claude Code returned no suggestions. The summary is saved; generate again to retry.")
		return
	}
	if err := s.store.SetSuggestions(ctx, id, suggestions); err != nil {
		s.state(ctx, id, "suggestions", "error", err.Error())
		return
	}
	if err := s.store.UpdateState(ctx, id, domain.StateSummarized); err != nil {
		s.state(ctx, id, "suggestions", "error", err.Error())
		return
	}
	s.state(ctx, id, "suggestions", "done", "")
	if _, err := s.export.ExportMeeting(ctx, id); err != nil {
		s.state(ctx, id, "suggestions", "error", "The summary is saved in Notted, but writing the files failed: "+err.Error())
	}
}
