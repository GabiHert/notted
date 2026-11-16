package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
)

type chunkKey struct {
	meetingID string
	track     domain.Track
	index     int
}

// workItem is one unit for the worker: a chunk to transcribe, a readiness check for a meeting,
// or the next chunk of a meeting being transcribed again.
type workItem struct {
	chunk     domain.Chunk
	meetingID string // set for a readiness check and a re-transcription step
	check     bool
	redo      bool
}

func (it workItem) meeting() string {
	if it.check || it.redo {
		return it.meetingID
	}
	return it.chunk.MeetingID
}

// redoJob is a meeting being transcribed again: new segments are held here until every chunk is
// done, then swapped in for the old ones in one store transaction.
type redoJob struct {
	meeting domain.Meeting
	chunks  []domain.Chunk
	next    int
	segs    []domain.Segment
}

// RetranscribeProgress says how far a re-transcription is.
type RetranscribeProgress struct {
	Running bool
	Done    int // chunks transcribed
	Total   int
}

// Re-transcription errors the app shows as they are.
var (
	ErrRetranscribeRunning  = errors.New("This meeting is already being transcribed again.")
	ErrRetranscribeNotReady = errors.New("Only a finished meeting can be transcribed again.")
	ErrRetranscribeNoAudio  = errors.New("This meeting has no audio to transcribe.")
)

// TranscriptionService turns each closed chunk into labelled, timed segments with one worker,
// so the speech engine is never called from two goroutines at once.
type TranscriptionService struct {
	store  port.MeetingStore
	engine port.SpeechEngine
	emit   port.EventEmitter
	decode func(path string) ([]float32, int, error)

	mu      sync.Mutex
	idle    *sync.Cond
	items   []workItem
	parked  map[chunkKey]domain.Chunk
	redo    map[string]*redoJob
	pending int    // items queued or being processed
	busy    string // the meeting whose item the worker is processing, "" when none
	wake    chan struct{}
	stop    chan struct{}
	started bool
	done    chan struct{}
	drained func() // called when the worker has nothing left to do
}

// NewTranscriptionService builds the service; decode reads a WAV file into mono samples and their rate.
func NewTranscriptionService(store port.MeetingStore, engine port.SpeechEngine,
	emit port.EventEmitter, decode func(path string) ([]float32, int, error)) *TranscriptionService {
	s := &TranscriptionService{
		store: store, engine: engine, emit: emit, decode: decode,
		parked: map[chunkKey]domain.Chunk{},
		redo:   map[string]*redoJob{},
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

// SetDrainedListener calls fn, on the worker goroutine, each time the last queued item is done.
// Set it before Start.
func (s *TranscriptionService) SetDrainedListener(fn func()) {
	s.mu.Lock()
	s.drained = fn
	s.mu.Unlock()
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

// Forget drops a meeting's queued and parked work and any re-transcription, then waits for the item
// of that meeting the worker is processing, if any. After it returns nothing more is written for
// the meeting.
func (s *TranscriptionService) Forget(meetingID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.redo, meetingID)
	kept := s.items[:0]
	for _, it := range s.items {
		if it.meeting() == meetingID {
			s.pending--
			continue
		}
		kept = append(kept, it)
	}
	for i := len(kept); i < len(s.items); i++ {
		s.items[i] = workItem{}
	}
	s.items = kept
	for k := range s.parked {
		if k.meetingID == meetingID {
			delete(s.parked, k)
		}
	}
	if s.pending == 0 {
		s.idle.Broadcast()
	}
	for s.busy == meetingID {
		s.idle.Wait()
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
	s.busy = it.meeting()
	return it, true
}

func (s *TranscriptionService) finishItem() {
	s.mu.Lock()
	s.pending--
	s.busy = ""
	var drained func()
	if s.pending == 0 {
		drained = s.drained
	}
	s.idle.Broadcast() // Wait waits for an empty queue, Forget for the end of one item
	s.mu.Unlock()
	if drained != nil {
		drained()
	}
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
		switch {
		case it.check:
			s.checkReady(ctx, it.meetingID)
		case it.redo:
			s.redoStep(ctx, it.meetingID)
		default:
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

// chunkSegments transcribes a whole chunk and returns its segments with absolute times.
func (s *TranscriptionService) chunkSegments(ctx context.Context, m domain.Meeting, c domain.Chunk) ([]domain.Segment, error) {
	samples, rate, err := s.decode(c.Path)
	if err != nil {
		return nil, err
	}
	if rate <= 0 {
		return nil, fmt.Errorf("unsupported sample rate %d", rate)
	}
	if len(samples) == 0 {
		return nil, nil
	}
	found, err := s.engine.Transcribe(ctx, samples, rate, m.Language)
	if err != nil {
		return nil, err
	}
	var out []domain.Segment
	for _, f := range found {
		text := strings.Join(strings.Fields(f.Text), " ")
		if text == "" {
			continue
		}
		out = append(out, domain.Segment{MeetingID: c.MeetingID, Track: c.Track, StartMs: c.StartMs + f.StartMs, EndMs: c.StartMs + f.EndMs, Text: text})
	}
	return out, nil
}

func (s *TranscriptionService) transcribeChunk(ctx context.Context, m domain.Meeting, c domain.Chunk) error {
	segs, err := s.chunkSegments(ctx, m, c)
	if err != nil {
		return err
	}
	for _, seg := range segs {
		id, err := s.store.AddSegment(ctx, seg)
		if err != nil {
			return err
		}
		s.emit.Emit("transcript:segment", map[string]any{
			"meetingId": c.MeetingID,
			"segment": map[string]any{
				"id": id, "track": string(c.Track), "startMs": seg.StartMs, "endMs": seg.EndMs, "text": seg.Text,
			},
		})
	}
	return nil
}

// Retranscribe transcribes every chunk of a finished meeting again with the current model. The new
// segments are held aside and replace the old ones in one transaction once the last chunk is done;
// until then, and when anything fails, the old transcript stays. Progress arrives as
// transcript:retranscribe events. The meeting's state and summary are left alone.
func (s *TranscriptionService) Retranscribe(ctx context.Context, meetingID string) error {
	m, err := s.store.GetMeeting(ctx, meetingID)
	if err != nil {
		return err
	}
	if m.State != domain.StateReady && m.State != domain.StateSummarized {
		return ErrRetranscribeNotReady
	}
	if ok, msg := s.engine.Ready(m.Language); !ok {
		return fmt.Errorf("The speech models are not ready: %s.", msg)
	}
	chunks, err := s.store.ListChunks(ctx, meetingID)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return ErrRetranscribeNoAudio
	}
	s.mu.Lock()
	if s.redo[meetingID] != nil {
		s.mu.Unlock()
		return ErrRetranscribeRunning
	}
	s.redo[meetingID] = &redoJob{meeting: m, chunks: chunks}
	s.mu.Unlock()
	s.emitRedo(meetingID, "running", 0, len(chunks), "")
	s.push(workItem{meetingID: meetingID, redo: true})
	return nil
}

// RetranscribeStatus reports the meeting's re-transcription, Running false when there is none.
func (s *TranscriptionService) RetranscribeStatus(meetingID string) RetranscribeProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.redo[meetingID]
	if j == nil {
		return RetranscribeProgress{}
	}
	return RetranscribeProgress{Running: true, Done: j.next, Total: len(j.chunks)}
}

func (s *TranscriptionService) emitRedo(meetingID, state string, done, total int, msg string) {
	p := map[string]any{"meetingId": meetingID, "state": state, "done": done, "total": total}
	if msg != "" {
		p["error"] = msg
	}
	s.emit.Emit("transcript:retranscribe", p)
}

// redoStep transcribes the job's next chunk, then queues the step after it at the back of the
// queue, so a recording's chunks are not held up behind a long meeting. After the last chunk it
// swaps the segments in.
func (s *TranscriptionService) redoStep(ctx context.Context, meetingID string) {
	s.mu.Lock()
	j := s.redo[meetingID]
	s.mu.Unlock()
	if j == nil {
		return // forgotten
	}
	total := len(j.chunks)
	fail := func(err error) {
		s.mu.Lock()
		delete(s.redo, meetingID)
		s.mu.Unlock()
		log.Printf("transcription: re-transcribe %s: %v", meetingID, err)
		s.emitRedo(meetingID, "error", j.next, total, fmt.Sprintf("Transcribing again failed: %v. The old transcript is kept.", err))
	}
	c := j.chunks[j.next]
	segs, err := s.chunkSegments(ctx, j.meeting, c)
	if err != nil {
		fail(fmt.Errorf("%s part %d: %w", c.Track, c.Index, err))
		return
	}
	s.mu.Lock()
	if s.redo[meetingID] != j {
		s.mu.Unlock()
		return
	}
	j.segs = append(j.segs, segs...)
	j.next++
	done := j.next
	s.mu.Unlock()
	if done < total {
		s.emitRedo(meetingID, "running", done, total, "")
		s.push(workItem{meetingID: meetingID, redo: true})
		return
	}
	if err := s.store.ReplaceSegments(ctx, meetingID, j.segs); err != nil {
		fail(err)
		return
	}
	s.mu.Lock()
	delete(s.redo, meetingID)
	s.mu.Unlock()
	s.emitRedo(meetingID, "done", total, total, "")
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
