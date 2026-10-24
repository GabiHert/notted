package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/domain"
	"github.com/google/uuid"
)

const (
	msgAlreadyRecording = "A recording is already running. Stop it first."
	msgMeetingNotFound  = "Meeting not found."
	msgSilentSystem     = "No system audio was heard in the last minute. If the other side was talking, check that Notted is allowed in System Settings > Privacy & Security > Screen & System Audio Recording."
	silentChunksToWarn  = 2
)

// userError carries user-facing text and, optionally, a sentinel to match with errors.Is.
type userError struct {
	msg string
	err error
}

func (e *userError) Error() string { return e.msg }
func (e *userError) Unwrap() error { return e.err }

type activeRecording struct {
	meetingID string
	rec       port.Recording
	ready     chan struct{} // closed once the meeting row exists (or the start failed)
	created   bool
	stopping  bool
	silentRun int
	warned    bool
	once      sync.Once
}

// RecordingService runs one recording at a time and keeps its meeting row in step with the helper.
type RecordingService struct {
	store    port.MeetingStore
	rec      port.AudioRecorder
	emit     port.EventEmitter
	now      func() time.Time
	newID    func() string
	audioDir func(meetingID string) string
	inspect  port.AudioInspector
	onChunk  func(domain.Chunk)
	onStop   func(meetingID string)
	mu       sync.Mutex
	active   *activeRecording
}

// NewRecordingService builds the service; audioDir maps a meeting id to its chunk folder.
func NewRecordingService(store port.MeetingStore, rec port.AudioRecorder, emit port.EventEmitter,
	audioDir func(string) string, inspect port.AudioInspector) *RecordingService {
	return &RecordingService{store: store, rec: rec, emit: emit, now: time.Now, newID: uuid.NewString, audioDir: audioDir, inspect: inspect}
}

// SetChunkListener attaches a function called after each stored chunk (transcription hooks in here).
func (s *RecordingService) SetChunkListener(fn func(domain.Chunk)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChunk = fn
}

// SetStoppedListener attaches a function called after every move of a meeting to `transcribing`.
func (s *RecordingService) SetStoppedListener(fn func(meetingID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStop = fn
}

// Start begins a recording and returns the new meeting once the helper reported `started`.
func (s *RecordingService) Start(ctx context.Context, title string, lang domain.Language) (domain.Meeting, error) {
	if lang != domain.LangPTBR && lang != domain.LangEN {
		return domain.Meeting{}, fmt.Errorf("Unknown language %q. Use pt-BR or en.", string(lang))
	}
	a := &activeRecording{meetingID: s.newID(), ready: make(chan struct{})} // fully built before it is published
	s.mu.Lock()
	if s.active != nil {
		s.mu.Unlock()
		return domain.Meeting{}, errors.New(msgAlreadyRecording)
	}
	s.active = a
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		if s.active == a {
			s.active = nil
		}
		s.mu.Unlock()
		close(a.ready)
	}

	now := s.now()
	if strings.TrimSpace(title) == "" {
		title = "Meeting " + now.Local().Format("2006-01-02 15:04")
	}
	id := a.meetingID
	dir := s.audioDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		release()
		return domain.Meeting{}, err
	}

	rec, err := s.rec.Start(dir, func(ev port.HelperEvent) { s.handleEvent(ctx, a, ev) })
	if err != nil {
		_ = os.Remove(dir) // only succeeds when empty
		release()
		return domain.Meeting{}, err
	}
	m := domain.Meeting{ID: id, Title: title, StartedAt: now.UTC(), Language: lang, State: domain.StateRecording}
	if err := s.store.CreateMeeting(ctx, m); err != nil {
		_ = rec.Stop()
		release()
		return domain.Meeting{}, err
	}
	s.mu.Lock()
	a.rec = rec
	a.created = true
	s.mu.Unlock()
	close(a.ready)
	s.emit.Emit("recording:state", map[string]any{"meetingId": id, "state": "recording"})
	go s.watch(context.WithoutCancel(ctx), a)
	return m, nil
}

func (s *RecordingService) handleEvent(ctx context.Context, a *activeRecording, ev port.HelperEvent) {
	if ev.Kind != "chunk" {
		return
	}
	<-a.ready // a chunk can arrive before Start has stored the meeting
	s.mu.Lock()
	created := a.created
	listener := s.onChunk
	s.mu.Unlock()
	if !created {
		return
	}
	c := domain.Chunk{MeetingID: a.meetingID, Track: ev.Track, Index: ev.Index, StartMs: ev.StartMs, EndMs: ev.EndMs, Path: ev.Path}
	if err := s.store.AddChunk(context.WithoutCancel(ctx), c); err != nil {
		log.Printf("recording: store chunk %s %d: %v", ev.Track, ev.Index, err)
		return
	}
	s.emit.Emit("recording:chunk", map[string]any{"meetingId": a.meetingID, "track": string(ev.Track), "index": ev.Index})
	if ev.Track == domain.TrackOthers {
		s.trackSilence(a, ev.Path)
	}
	if listener != nil {
		listener(c)
	}
}

func (s *RecordingService) trackSilence(a *activeRecording, path string) {
	info, err := s.inspect.Inspect(path)
	if err != nil || !info.Silent {
		a.silentRun = 0
		return
	}
	a.silentRun++
	if a.silentRun >= silentChunksToWarn && !a.warned {
		a.warned = true
		s.emit.Emit("recording:state", map[string]any{"meetingId": a.meetingID, "state": "recording", "warning": msgSilentSystem})
	}
}

// watch ends the recording when the helper exits without being asked to.
func (s *RecordingService) watch(ctx context.Context, a *activeRecording) {
	x := <-a.rec.Exited()
	s.mu.Lock()
	stopping := a.stopping
	s.mu.Unlock()
	if stopping {
		return
	}
	s.finish(ctx, a, x.Message)
}

// finish moves the recording's meeting to transcribing, once.
func (s *RecordingService) finish(ctx context.Context, a *activeRecording, errMsg string) {
	a.once.Do(func() {
		if err := s.markTranscribing(ctx, a.meetingID, errMsg); err != nil {
			log.Printf("recording: finish %s: %v", a.meetingID, err)
		}
		s.mu.Lock()
		if s.active == a {
			s.active = nil
		}
		s.mu.Unlock()
	})
}

func (s *RecordingService) markTranscribing(ctx context.Context, id, errMsg string) error {
	if err := s.store.SetEndedAt(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	if err := s.store.UpdateState(ctx, id, domain.StateTranscribing); err != nil {
		return err
	}
	payload := map[string]any{"meetingId": id, "state": "transcribing"}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	s.emit.Emit("recording:state", payload)
	s.mu.Lock()
	fn := s.onStop
	s.mu.Unlock()
	if fn != nil {
		fn(id)
	}
	return nil
}

// Stop ends the recording of meetingID and queues it for transcription.
func (s *RecordingService) Stop(ctx context.Context, meetingID string) error {
	m, err := s.store.GetMeeting(ctx, meetingID)
	if errors.Is(err, domain.ErrNotFound) {
		return &userError{msg: msgMeetingNotFound, err: domain.ErrNotFound}
	}
	if err != nil {
		return err
	}
	if m.State != domain.StateRecording {
		return nil
	}
	s.mu.Lock()
	a := s.active
	if a != nil && (a.meetingID != meetingID || !a.created) {
		a = nil
	}
	if a != nil {
		a.stopping = true
	}
	s.mu.Unlock()
	if a == nil {
		return s.markTranscribing(ctx, meetingID, "")
	}
	if err := a.rec.Stop(); err != nil {
		log.Printf("recording: stop helper: %v", err)
	}
	// Every chunk event was delivered before the helper's exit was reported.
	s.finish(ctx, a, "")
	return nil
}

// RecoverOnStartup queues every meeting left in `recording` (the app quit mid-recording) for transcription.
func (s *RecordingService) RecoverOnStartup(ctx context.Context) error {
	ms, err := s.store.ListMeetings(ctx)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.State != domain.StateRecording {
			continue
		}
		if err := s.markTranscribing(ctx, m.ID, ""); err != nil {
			return err
		}
	}
	return nil
}

// Recording reports whether a recording is running.
func (s *RecordingService) Recording() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active != nil
}

// StopAll stops the active recording; the app calls it on shutdown.
func (s *RecordingService) StopAll() {
	s.mu.Lock()
	a := s.active
	var id string
	if a != nil && a.created {
		id = a.meetingID
	}
	s.mu.Unlock()
	if id == "" {
		return
	}
	if err := s.Stop(context.Background(), id); err != nil {
		log.Printf("recording: stop all: %v", err)
	}
}
