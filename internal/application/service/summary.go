package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/domain"
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

	mu     sync.Mutex
	active map[string]bool
	wg     sync.WaitGroup
}

// NewSummaryService builds the summary service.
func NewSummaryService(store port.MeetingStore, settings port.SettingsStore, prompts port.PromptSource,
	runner port.ClaudeRunner, export exporter, emit port.EventEmitter) *SummaryService {
	return &SummaryService{store: store, settings: settings, prompts: prompts, runner: runner, export: export,
		emit: emit, newID: uuid.NewString, active: map[string]bool{}}
}

// Wait blocks until no run is active. It is a test helper.
func (s *SummaryService) Wait() { s.wg.Wait() }

const suggestionsStdin = "Use the meetings skill's `suggest` format (its file is .claude/skills/meetings/SKILL.md) to write this meeting's suggestions.md from the summary you just wrote and the transcript above, checking the existing-work sources the instructions name. Do not write any file. Every Decision is `undecided`. Reply with the content of suggestions.md only: start with the line \"# Suggestions: <title>\" and add nothing before or after it."

func summaryStdin(m domain.Meeting, segs []domain.Segment) string {
	date := m.StartedAt.In(time.Local).Format("2006-01-02")
	return "Use the meetings skill's `process` format (its file is .claude/skills/meetings/SKILL.md) to write this meeting's summary.md, but do not write, stage or commit any file and do not run the save step. Reply with the content of summary.md only: start with the line \"# Summary: " + m.Title + "\" and add nothing before or after it.\n\n" +
		"Meeting: " + m.Title + "\n" +
		"Date: " + date + "\n" +
		"Client: " + m.ClientKey + "\n" +
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
	if m.ClientKey == "" {
		return errors.New("Choose a client for this meeting first.")
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
	s.mu.Lock()
	if s.active[meetingID] {
		s.mu.Unlock()
		return errors.New("A summary is already being written for this meeting.")
	}
	s.active[meetingID] = true
	s.wg.Add(1)
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		delete(s.active, meetingID)
		s.mu.Unlock()
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
		s.run(context.Background(), m, st, segs)
	}()
	return nil
}

func (s *SummaryService) state(id, pass, state, message string) {
	p := map[string]any{"meetingId": id, "pass": pass, "state": state}
	if message != "" {
		p["message"] = message
	}
	s.emit.Emit("summary:state", p)
}

func (s *SummaryService) run(ctx context.Context, m domain.Meeting, st domain.Settings, segs []domain.Segment) {
	id := m.ID
	prompt, err := s.prompts.Prompt(ctx, st.CheckoutPath, st.Profile, m.ClientKey)
	if err != nil {
		s.state(id, "summary", "error", err.Error())
		return
	}
	sid, resume := m.ClaudeSessionID, m.ClaudeSessionID != ""
	if !resume {
		sid = s.newID()
	}
	root := st.ProjectRoots[m.ClientKey]

	s.state(id, "summary", "running", "")
	out, err := s.runner.Run(ctx, port.ClaudeRun{Checkout: st.CheckoutPath, SessionID: sid, Resume: resume,
		PromptText: prompt, ProjectRoot: root, Stdin: summaryStdin(m, segs)})
	if err != nil {
		s.state(id, "summary", "error", err.Error())
		return
	}
	summary, found := extractFrom(out, "# Summary:")
	if !found {
		s.state(id, "summary", "error", "Claude Code returned no summary. Nothing was saved.")
		return
	}
	if !resume {
		if err := s.store.SetClaudeSessionID(ctx, id, sid); err != nil {
			s.state(id, "summary", "error", err.Error())
			return
		}
	}
	if err := s.store.SetSummary(ctx, id, summary); err != nil {
		s.state(id, "summary", "error", err.Error())
		return
	}
	s.state(id, "summary", "done", "")

	s.state(id, "suggestions", "running", "")
	out, err = s.runner.Run(ctx, port.ClaudeRun{Checkout: st.CheckoutPath, SessionID: sid, Resume: true,
		ProjectRoot: root, Stdin: strings.Replace(suggestionsStdin, "<title>", m.Title, 1)})
	if err != nil {
		s.state(id, "suggestions", "error", err.Error())
		return
	}
	suggestions, found := extractFrom(out, "# Suggestions:")
	if !found {
		s.state(id, "suggestions", "error", "Claude Code returned no suggestions. The summary is saved; generate again to retry.")
		return
	}
	if err := s.store.SetSuggestions(ctx, id, suggestions); err != nil {
		s.state(id, "suggestions", "error", err.Error())
		return
	}
	if err := s.store.UpdateState(ctx, id, domain.StateSummarized); err != nil {
		s.state(id, "suggestions", "error", err.Error())
		return
	}
	s.state(id, "suggestions", "done", "")
	if _, err := s.export.ExportMeeting(ctx, id); err != nil {
		s.state(id, "suggestions", "error", "The summary is saved in Notted, but writing the files failed: "+err.Error())
	}
}
