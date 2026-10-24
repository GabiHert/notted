package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
	"github.com/GabiHert/notted/internal/infra/store"
)

type fakeEngine struct {
	mu    sync.Mutex
	ready bool
	langs []domain.Language
	n     int
	fail  bool
	text  func(n int) string
}

func (e *fakeEngine) Ready(domain.Language) (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ready, "missing"
}

func (e *fakeEngine) setReady(v bool) { e.mu.Lock(); e.ready = v; e.mu.Unlock() }

func (e *fakeEngine) Transcribe(_ context.Context, _ []float32, _ int, lang domain.Language) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.n++
	e.langs = append(e.langs, lang)
	if e.fail {
		return "", errors.New("engine broke")
	}
	if e.text != nil {
		return e.text(e.n), nil
	}
	return fmt.Sprintf("text %d", e.n), nil
}

func (e *fakeEngine) calls() int { e.mu.Lock(); defer e.mu.Unlock(); return e.n }

type fakeDetector struct {
	ready bool
	def   []port.SpeechSpan
}

func (d *fakeDetector) Ready() (bool, string) { return d.ready, "missing" }

func (d *fakeDetector) Detect([]float32, int) ([]port.SpeechSpan, error) { return d.def, nil }

func decode30s(string) ([]float32, int, error) { return make([]float32, 16000*30), 16000, nil }

func span(startSample, n int) port.SpeechSpan {
	return port.SpeechSpan{StartSample: startSample, Samples: make([]float32, n)}
}

type transFixture struct {
	t    *testing.T
	st   *store.Store
	eng  *fakeEngine
	det  *fakeDetector
	em   *recEmitter
	svc  *TranscriptionService
	ctx  context.Context
	meet domain.Meeting
}

func newTransFixture(t *testing.T, lang domain.Language) *transFixture {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &transFixture{t: t, st: st, eng: &fakeEngine{ready: true}, det: &fakeDetector{ready: true, def: []port.SpeechSpan{span(16000, 32000)}}, em: &recEmitter{}, ctx: context.Background()}
	f.svc = NewTranscriptionService(st, f.eng, f.det, f.em, decode30s)
	f.meet = domain.Meeting{ID: "m1", Title: "t", StartedAt: time.Now().UTC(), Language: lang, State: domain.StateTranscribing}
	if err := st.CreateMeeting(f.ctx, f.meet); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *transFixture) addChunk(track domain.Track, idx int) domain.Chunk {
	f.t.Helper()
	c := domain.Chunk{MeetingID: f.meet.ID, Track: track, Index: idx, StartMs: int64(idx-1) * 30000, EndMs: int64(idx) * 30000, Path: fmt.Sprintf("/a/%s-%06d.wav", track, idx)}
	if err := f.st.AddChunk(f.ctx, c); err != nil {
		f.t.Fatal(err)
	}
	return c
}

func (f *transFixture) start() {
	f.t.Helper()
	if err := f.svc.Start(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(f.svc.Close)
}

func (f *transFixture) state() domain.MeetingState {
	m, err := f.st.GetMeeting(f.ctx, f.meet.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return m.State
}

func (f *transFixture) pending() int {
	p, err := f.st.PendingChunks(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return len(p)
}

func TestTranscribesBothTracksWithAbsoluteTimesAndEvents(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.eng.text = func(n int) string { return fmt.Sprintf("  hello\n  world %d ", n) }
	f.start()
	mic := f.addChunk(domain.TrackMic, 2)
	oth := f.addChunk(domain.TrackOthers, 2)
	f.svc.Enqueue(mic)
	f.svc.Enqueue(oth)
	f.svc.Wait()

	segs, err := f.svc.Transcript(f.ctx, "m1")
	if err != nil || len(segs) != 2 {
		t.Fatalf("segs = %+v err=%v", segs, err)
	}
	if segs[0].Track != domain.TrackMic || segs[1].Track != domain.TrackOthers {
		t.Fatalf("tracks = %s,%s", segs[0].Track, segs[1].Track)
	}
	// chunk 2 starts at 30000; span starts at sample 16000 (1000 ms) and lasts 32000 samples (2000 ms)
	if segs[0].StartMs != 31000 || segs[0].EndMs != 33000 || segs[0].Text != "hello world 1" {
		t.Fatalf("mic segment = %+v", segs[0])
	}
	evs := f.em.named("transcript:segment")
	if len(evs) != 2 {
		t.Fatalf("events = %d", len(evs))
	}
	want := map[string]any{"id": segs[0].ID, "track": "mic", "startMs": int64(31000), "endMs": int64(33000), "text": "hello world 1"}
	if evs[0].payload["meetingId"] != "m1" || !reflect.DeepEqual(evs[0].payload["segment"], want) {
		t.Fatalf("payload = %#v", evs[0].payload)
	}
	if f.pending() != 0 {
		t.Fatalf("pending = %d", f.pending())
	}
}

func TestSameChunkQueuedTwiceTranscribedOnce(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.start()
	c := f.addChunk(domain.TrackMic, 1)
	f.svc.Enqueue(c)
	f.svc.Enqueue(c)
	f.svc.Wait()
	if f.eng.calls() != 1 {
		t.Fatalf("engine calls = %d", f.eng.calls())
	}
}

func TestLanguagePassedToEngine(t *testing.T) {
	for _, lang := range []domain.Language{domain.LangPTBR, domain.LangEN} {
		f := newTransFixture(t, lang)
		f.start()
		f.svc.Enqueue(f.addChunk(domain.TrackMic, 1))
		f.svc.Wait()
		if len(f.eng.langs) != 1 || f.eng.langs[0] != lang {
			t.Fatalf("%s: langs = %v", lang, f.eng.langs)
		}
	}
}

func TestNoSpeechAddsNoSegmentButMarksChunk(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.det.def = nil
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackOthers, 1))
	f.svc.Wait()
	segs, _ := f.svc.Transcript(f.ctx, "m1")
	if len(segs) != 0 || f.eng.calls() != 0 || f.pending() != 0 {
		t.Fatalf("segs=%d calls=%d pending=%d", len(segs), f.eng.calls(), f.pending())
	}
}

func TestEmptyTextSpanAddsNoSegment(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.eng.text = func(int) string { return "  \n " }
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackMic, 1))
	f.svc.Wait()
	if segs, _ := f.svc.Transcript(f.ctx, "m1"); len(segs) != 0 {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestDecodeErrorReportedOnceChunkSkippedMeetingReady(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.svc = NewTranscriptionService(f.st, f.eng, f.det, f.em, func(string) ([]float32, int, error) { return nil, 0, errors.New("bad wav") })
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackMic, 3))
	f.svc.MeetingStopped("m1")
	f.svc.Wait()
	var errs []string
	for _, ev := range f.em.named("recording:state") {
		if s, ok := ev.payload["error"].(string); ok {
			errs = append(errs, s)
			if ev.payload["state"] != "transcribing" {
				t.Fatalf("state = %v", ev.payload["state"])
			}
		}
	}
	if len(errs) != 1 || errs[0] != "Transcribing mic part 3 failed: bad wav. That part is skipped." {
		t.Fatalf("errors = %q", errs)
	}
	if f.pending() != 0 || f.state() != domain.StateReady {
		t.Fatalf("pending=%d state=%s", f.pending(), f.state())
	}
}

func TestEngineErrorReportedAndSkipped(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.eng.fail = true
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackOthers, 1))
	f.svc.Wait()
	var errs []any
	for _, ev := range f.em.named("recording:state") {
		if e, ok := ev.payload["error"]; ok {
			errs = append(errs, e)
		}
	}
	if len(errs) != 1 || errs[0] != "Transcribing others part 1 failed: engine broke. That part is skipped." {
		t.Fatalf("errors = %v", errs)
	}
	if f.pending() != 0 {
		t.Fatal("chunk still pending")
	}
}

func TestSegmentsOrderedAfterOutOfOrderCompletion(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.start()
	for _, c := range []domain.Chunk{f.addChunk(domain.TrackOthers, 2), f.addChunk(domain.TrackMic, 2), f.addChunk(domain.TrackMic, 1)} {
		f.svc.Enqueue(c)
	}
	f.svc.Wait()
	segs, _ := f.svc.Transcript(f.ctx, "m1")
	if len(segs) != 3 || segs[0].StartMs != 1000 || segs[1].Track != domain.TrackMic || segs[2].Track != domain.TrackOthers {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestSimulatedRecordingLeavesAtMostOnePendingPerTrack(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	if err := f.st.UpdateState(f.ctx, "m1", domain.StateRecording); err != nil {
		t.Fatal(err)
	}
	f.start()
	for idx := 1; idx <= 2; idx++ {
		f.svc.Enqueue(f.addChunk(domain.TrackMic, idx))
		f.svc.Enqueue(f.addChunk(domain.TrackOthers, idx))
	}
	f.svc.Wait()
	if f.pending() != 0 || f.state() != domain.StateRecording {
		t.Fatalf("while recording: pending=%d state=%s", f.pending(), f.state())
	}
	// stop: the last chunk of each track closes, the state flips, the readiness check is queued
	c1, c2 := f.addChunk(domain.TrackMic, 3), f.addChunk(domain.TrackOthers, 3)
	f.svc.Enqueue(c1)
	f.svc.Enqueue(c2)
	if err := f.st.UpdateState(f.ctx, "m1", domain.StateTranscribing); err != nil {
		t.Fatal(err)
	}
	f.svc.MeetingStopped("m1")
	f.svc.Wait()
	if f.pending() != 0 || f.state() != domain.StateReady {
		t.Fatalf("after stop: pending=%d state=%s", f.pending(), f.state())
	}
	var ready int
	for _, ev := range f.em.named("recording:state") {
		if ev.payload["state"] == "ready" && ev.payload["meetingId"] == "m1" {
			ready++
		}
	}
	if ready != 1 {
		t.Fatalf("ready events = %d", ready)
	}
}

func TestParkedWhileEngineNotReadyThenResumed(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.eng.setReady(false)
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackMic, 1))
	f.svc.MeetingStopped("m1")
	f.svc.Wait()
	if f.pending() != 1 || f.state() != domain.StateTranscribing || f.eng.calls() != 0 {
		t.Fatalf("parked: pending=%d state=%s calls=%d", f.pending(), f.state(), f.eng.calls())
	}
	f.eng.setReady(true)
	f.svc.ResumeWaiting()
	f.svc.Wait()
	if f.pending() != 0 || f.state() != domain.StateReady || f.eng.calls() != 1 {
		t.Fatalf("resumed: pending=%d state=%s calls=%d", f.pending(), f.state(), f.eng.calls())
	}
}

func TestParkedWhenDetectorNotReady(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.det.ready = false
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackMic, 1))
	f.svc.Wait()
	if f.pending() != 1 {
		t.Fatalf("pending = %d", f.pending())
	}
}

func TestStartQueuesEveryPendingChunk(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.addChunk(domain.TrackMic, 1)
	f.addChunk(domain.TrackOthers, 1)
	f.addChunk(domain.TrackMic, 2)
	f.start()
	f.svc.Wait()
	if f.pending() != 0 || f.eng.calls() != 3 {
		t.Fatalf("pending=%d calls=%d", f.pending(), f.eng.calls())
	}
}

func TestEnqueueNeverBlocksWhenQueueFull(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	// the worker is not started, so nothing drains the queue
	done := make(chan struct{})
	go func() {
		for i := 1; i <= 3000; i++ {
			f.svc.Enqueue(domain.Chunk{MeetingID: "m1", Track: domain.TrackMic, Index: i, Path: "/x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Enqueue blocked")
	}
}

func TestStartMakesTranscribingMeetingWithNoPendingChunksReady(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.start()
	f.svc.Wait()
	if f.state() != domain.StateReady {
		t.Fatalf("state = %s", f.state())
	}
	var ready int
	for _, ev := range f.em.named("recording:state") {
		if ev.payload["state"] == "ready" && ev.payload["meetingId"] == "m1" {
			ready++
		}
	}
	if ready != 1 {
		t.Fatalf("ready events = %d", ready)
	}
}

func TestStartRecoversCrashBetweenMarkAndStateUpdate(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	c := f.addChunk(domain.TrackMic, 1)
	if err := f.st.MarkChunkTranscribed(f.ctx, c.MeetingID, c.Track, c.Index); err != nil {
		t.Fatal(err)
	}
	f.start()
	f.svc.Wait()
	if f.state() != domain.StateReady || f.eng.calls() != 0 {
		t.Fatalf("state=%s calls=%d", f.state(), f.eng.calls())
	}
}
