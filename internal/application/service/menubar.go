package service

import (
	"context"
	"log"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

// StatusItem is the menu bar icon; it shows the idle or the recording look.
type StatusItem interface {
	SetRecording(recording bool)
}

// MenuRecorder is the part of the recording service the menu bar drives.
type MenuRecorder interface {
	Start(ctx context.Context, title string, lang domain.Language) (domain.Meeting, error)
	Stop(ctx context.Context, meetingID string) error
	ActiveMeetingID() string
	SetStateListener(fn func(recording bool))
}

// MenuBarDeps are the collaborators of MenuBarService.
type MenuBarDeps struct {
	Recorder   MenuRecorder
	Meetings   port.MeetingStore
	Item       StatusItem
	ShowWindow func()
	Emit       port.EventEmitter
	Context    func() context.Context
}

// MenuBarService starts and stops recordings from the menu bar and keeps its icon in step.
type MenuBarService struct {
	d MenuBarDeps
}

// NewMenuBarService builds the service.
func NewMenuBarService(d MenuBarDeps) *MenuBarService { return &MenuBarService{d: d} }

// Attach makes the icon follow every recording, whichever side started it.
func (s *MenuBarService) Attach() {
	s.d.Recorder.SetStateListener(s.d.Item.SetRecording)
}

// Start begins a recording with the default title in the language of the newest meeting (pt-BR when there is none).
func (s *MenuBarService) Start() {
	ctx := s.d.Context()
	lang := domain.LangPTBR
	if ms, err := s.d.Meetings.ListMeetings(ctx); err != nil {
		log.Printf("menu bar: last language: %v", err)
	} else if len(ms) > 0 && ms[0].Language != "" {
		lang = ms[0].Language
	}
	if _, err := s.d.Recorder.Start(ctx, "", lang); err != nil {
		s.fail(err)
	}
}

// Stop ends the running recording.
func (s *MenuBarService) Stop() {
	id := s.d.Recorder.ActiveMeetingID()
	if id == "" {
		s.d.Item.SetRecording(false) // the icon was out of step: nothing is recording
		return
	}
	if err := s.d.Recorder.Stop(s.d.Context(), id); err != nil {
		s.fail(err)
	}
}

// fail shows the window and tells the UI what went wrong.
func (s *MenuBarService) fail(err error) {
	s.d.ShowWindow()
	s.d.Emit.Emit("menubar:error", map[string]any{"message": err.Error()})
}
