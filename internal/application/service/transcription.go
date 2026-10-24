package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

type chunkKey struct {
	meetingID string
	track     domain.Track
	index     int
}

// workItem is one unit for the worker: a chunk to transcribe, or a readiness check for a meeting.
type workItem struct {
	chunk     domain.Chunk
	meetingID string // set for a readiness check
	check     bool
}

// TranscriptionService turns each closed chunk into labelled, timed segments with one worker,
// so the speech library is never called from two goroutines at once.
type TranscriptionService struct {
	store    port.MeetingStore
	engine   port.SpeechEngine
	detector port.SpeechDetector
	emit     port.EventEmitter
	decode   func(path string) ([]float32, int, error)

	mu      sync.Mutex
	idle    *sync.Cond
	items   []workItem
	parked  map[chunkKey]domain.Chunk
	pending int // items queued or being processed
	wake    chan struct{}
	stop    chan struct{}
	started bool
	done    chan struct{}
}

// NewTranscriptionService builds the service; decode reads a WAV file into mono samples and their rate.
func NewTranscriptionService(store port.MeetingStore, engine port.SpeechEngine, detector port.SpeechDetector,
	emit port.EventEmitter, decode func(path string) ([]float32, int, error)) *TranscriptionService {
	s := &TranscriptionService{
		store: store, engine: engine, detector: detector, emit: emit, decode: decode,
		parked: map[chunkKey]domain.Chunk{},
		wake:   make(chan struct{}, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	s.idle = sync.NewCond(&s.mu)
	return s
}

// Start launches the worker and queues every chunk still waiting in the store.
func (s *TranscriptionService) Start(ctx context.Context) error {
	chunks, err := s.store.PendingChunks(ctx)
	if err != nil {
		return err
	}
	meetings, err := s.store.ListMeetings(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("transcription already started")
	}
	s.started = true
	s.mu.Unlock()
	go s.run(ctx)
	for _, c := range chunks {
		s.Enqueue(c)
	}
	// A meeting left in `transcribing` may have no pending chunk (a crash after the last mark),
	// so every one gets a readiness check behind the chunks, whatever the wiring order.
	for _, m := range meetings {
		if m.State == domain.StateTranscribing {
			s.MeetingStopped(m.ID)
		}
	}
	return nil
}

// Close stops the worker; queued items stay in the store as pending chunks.
func (s *TranscriptionService) Close() {
	s.mu.Lock()
	started := s.started
	s.started = false
	s.mu.Unlock()
	if !started {
		return
	}
	close(s.stop)
	<-s.done
}

// Enqueue queues a closed chunk. It never blocks.
func (s *TranscriptionService) Enqueue(c domain.Chunk) { s.push(workItem{chunk: c}) }

// MeetingStopped queues a readiness check behind the meeting's chunks.
func (s *TranscriptionService) MeetingStopped(meetingID string) {
	s.push(workItem{meetingID: meetingID, check: true})
}

// ResumeWaiting queues the chunks parked for missing models again.
func (s *TranscriptionService) ResumeWaiting() {
	s.mu.Lock()
	var back []workItem
	for k, c := range s.parked {
		back = append(back, workItem{chunk: c})
		delete(s.parked, k)
	}
	s.mu.Unlock()
	for _, it := range back {
		s.push(it)
	}
}

// Wait blocks until every queued item is processed or parked.
func (s *TranscriptionService) Wait() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.pending > 0 {
		s.idle.Wait()
	}
}

// Transcript lists a meeting's segments in time order, mic before others on a tie.
func (s *TranscriptionService) Transcript(ctx context.Context, meetingID string) ([]domain.Segment, error) {
	return s.store.ListSegments(ctx, meetingID)
}

func (s *TranscriptionService) push(it workItem) {
	s.mu.Lock()
	s.items = append(s.items, it)
	s.pending++
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *TranscriptionService) next() (workItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) == 0 {
		return workItem{}, false
	}
	it := s.items[0]
	s.items[0] = workItem{}
	s.items = s.items[1:]
	return it, true
}

func (s *TranscriptionService) finishItem() {
	s.mu.Lock()
	s.pending--
	if s.pending == 0 {
		s.idle.Broadcast()
	}
	s.mu.Unlock()
}

func (s *TranscriptionService) run(ctx context.Context) {
	defer close(s.done)
	for {
		it, ok := s.next()
		if !ok {
			select {
			case <-s.wake:
				continue
			case <-s.stop:
				return
			}
		}
		select {
		case <-s.stop:
			return
		default:
		}
		if it.check {
			s.checkReady(ctx, it.meetingID)
		} else {
			s.process(ctx, it.chunk)
		}
		s.finishItem()
	}
}

func (s *TranscriptionService) process(ctx context.Context, queued domain.Chunk) {
	cur, ok, err := s.currentChunk(ctx, queued)
	if err != nil {
		log.Printf("transcription: read chunk %s %d: %v", queued.Track, queued.Index, err)
		return
	}
	if !ok || cur.Transcribed {
		return
	}
	m, err := s.store.GetMeeting(ctx, cur.MeetingID)
	if err != nil {
		log.Printf("transcription: meeting %s: %v", cur.MeetingID, err)
		return
	}
	if ready, _ := s.engine.Ready(m.Language); !ready {
		s.park(cur)
		return
	}
	if ready, _ := s.detector.Ready(); !ready {
		s.park(cur)
		return
	}
	if err := s.transcribeChunk(ctx, m, cur); err != nil {
		s.emit.Emit("recording:state", map[string]any{
			"meetingId": m.ID,
			"state":     string(m.State),
			"error":     fmt.Sprintf("Transcribing %s part %d failed: %v. That part is skipped.", cur.Track, cur.Index, err),
		})
	}
	if err := s.store.MarkChunkTranscribed(ctx, cur.MeetingID, cur.Track, cur.Index); err != nil {
		log.Printf("transcription: mark chunk %s %d: %v", cur.Track, cur.Index, err)
		return
	}
	s.checkReady(ctx, cur.MeetingID)
}

func (s *TranscriptionService) currentChunk(ctx context.Context, c domain.Chunk) (domain.Chunk, bool, error) {
	chunks, err := s.store.ListChunks(ctx, c.MeetingID)
	if err != nil {
		return domain.Chunk{}, false, err
	}
	for _, x := range chunks {
		if x.Track == c.Track && x.Index == c.Index {
			return x, true, nil
		}
	}
	return domain.Chunk{}, false, nil
}

func (s *TranscriptionService) park(c domain.Chunk) {
	s.mu.Lock()
	s.parked[chunkKey{c.MeetingID, c.Track, c.Index}] = c
	s.mu.Unlock()
}

func (s *TranscriptionService) transcribeChunk(ctx context.Context, m domain.Meeting, c domain.Chunk) error {
	samples, rate, err := s.decode(c.Path)
	if err != nil {
		return err
	}
	if rate <= 0 {
		return fmt.Errorf("unsupported sample rate %d", rate)
	}
	spans, err := s.detector.Detect(samples, rate)
	if err != nil {
		return err
	}
	for _, sp := range spans {
		raw, err := s.engine.Transcribe(ctx, sp.Samples, rate, m.Language)
		if err != nil {
			return err
		}
		text := strings.Join(strings.Fields(raw), " ")
		if text == "" {
			continue
		}
		start := c.StartMs + int64(sp.StartSample)*1000/int64(rate)
		end := start + int64(len(sp.Samples))*1000/int64(rate)
		seg := domain.Segment{MeetingID: c.MeetingID, Track: c.Track, StartMs: start, EndMs: end, Text: text}
		id, err := s.store.AddSegment(ctx, seg)
		if err != nil {
			return err
		}
		s.emit.Emit("transcript:segment", map[string]any{
			"meetingId": c.MeetingID,
			"segment": map[string]any{
				"id": id, "track": string(c.Track), "startMs": start, "endMs": end, "text": text,
			},
		})
	}
	return nil
}

// checkReady moves a transcribing meeting to ready once none of its chunks is pending.
func (s *TranscriptionService) checkReady(ctx context.Context, meetingID string) {
	m, err := s.store.GetMeeting(ctx, meetingID)
	if err != nil || m.State != domain.StateTranscribing {
		return
	}
	chunks, err := s.store.ListChunks(ctx, meetingID)
	if err != nil {
		log.Printf("transcription: list chunks %s: %v", meetingID, err)
		return
	}
	for _, c := range chunks {
		if !c.Transcribed {
			return
		}
	}
	if err := s.store.UpdateState(ctx, meetingID, domain.StateReady); err != nil {
		log.Printf("transcription: ready %s: %v", meetingID, err)
		return
	}
	s.emit.Emit("recording:state", map[string]any{"meetingId": meetingID, "state": "ready"})
}
