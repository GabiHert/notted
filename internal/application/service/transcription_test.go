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

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
	"github.com/GabiHert/phonon/internal/infra/store"
)

type fakeEngine struct {
	mu    sync.Mutex
	ready bool
	langs []domain.Language
	n     int
	fail  bool
	none  bool // no speech found
	text  func(n int) string
	// failAt makes the n-th call fail, when set.
	failAt int
	// before, when set, runs at the start of each call with the number of calls before it.
	before func(n int)
}

func (e *fakeEngine) Ready(domain.Language) (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ready, "missing"
}

func (e *fakeEngine) setReady(v bool) { e.mu.Lock(); e.ready = v; e.mu.Unlock() }

// Transcribe finds one segment from 1 s to 3 s into the chunk.
func (e *fakeEngine) Transcribe(_ context.Context, _ []float32, _ int, lang domain.Language) ([]port.SpeechSegment, error) {
	if e.before != nil {
		e.before(e.calls())
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.n++
	e.langs = append(e.langs, lang)
	if e.fail || e.n == e.failAt {
		return nil, errors.New("engine broke")
	}
	if e.none {
		return nil, nil
	}
	text := fmt.Sprintf("text %d", e.n)
	if e.text != nil {
		text = e.text(e.n)
	}
	return []port.SpeechSegment{{StartMs: 1000, EndMs: 3000, Text: text}}, nil
}

func (e *fakeEngine) calls() int { e.mu.Lock(); defer e.mu.Unlock(); return e.n }

func decode30s(string) ([]float32, int, error) { return make([]float32, 16000*30), 16000, nil }

type transFixture struct {
	t    *testing.T
	st   *store.Store
	eng  *fakeEngine
	em   *recEmitter
	svc  *TranscriptionService
	ctx  context.Context
	meet domain.Meeting
}

func newTransFixture(t *testing.T, lang domain.Language) *transFixture {
	t.Helper()
	t.Setenv("PHONON_HOME", t.TempDir())
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &transFixture{t: t, st: st, eng: &fakeEngine{ready: true}, em: &recEmitter{}, ctx: context.Background()}
	f.svc = NewTranscriptionService(st, f.eng, f.em, decode30s)
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
	// chunk 2 starts at 30000; the engine's segment runs from 1000 ms to 3000 ms into it
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
	f.eng.none = true
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackOthers, 1))
	f.svc.Wait()
	segs, _ := f.svc.Transcript(f.ctx, "m1")
	if len(segs) != 0 || f.eng.calls() != 1 || f.pending() != 0 {
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
	f.svc = NewTranscriptionService(f.st, f.eng, f.em, func(string) ([]float32, int, error) { return nil, 0, errors.New("bad wav") })
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

// blockingEngine holds the first Transcribe call until release is closed.
type blockingEngine struct {
	fakeEngine
	entered chan struct{}
	release chan struct{}
}

func (e *blockingEngine) Transcribe(ctx context.Context, s []float32, r int, l domain.Language) ([]port.SpeechSegment, error) {
	select {
	case e.entered <- struct{}{}:
	default:
	}
	<-e.release
	return e.fakeEngine.Transcribe(ctx, s, r, l)
}

func (f *transFixture) otherMeeting(id string) {
	f.t.Helper()
	m := domain.Meeting{ID: id, Title: "t", StartedAt: time.Now().UTC(), Language: domain.LangEN, State: domain.StateTranscribing}
	if err := f.st.CreateMeeting(f.ctx, m); err != nil {
		f.t.Fatal(err)
	}
}

func TestForgetDropsQueuedChunksAndChecksOfThatMeetingOnly(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.otherMeeting("m2")
	kept := domain.Chunk{MeetingID: "m2", Track: domain.TrackMic, Index: 1, EndMs: 30000, Path: "/a/m2.wav"}
	if err := f.st.AddChunk(f.ctx, kept); err != nil {
		t.Fatal(err)
	}
	gone := f.addChunk(domain.TrackMic, 1)
	f.svc.Enqueue(gone)
	f.svc.Enqueue(kept)
	f.svc.MeetingStopped("m1")
	f.svc.MeetingStopped("m2")

	f.svc.Forget("m1")
	f.svc.mu.Lock()
	left, pending := len(f.svc.items), f.svc.pending
	f.svc.mu.Unlock()
	if left != 2 || pending != 2 {
		t.Fatalf("queue = %d items, pending %d; want 2 and 2", left, pending)
	}

	if err := f.st.DeleteMeeting(f.ctx, "m1"); err != nil { // as Delete does; Start re-reads the store's pending chunks
		t.Fatal(err)
	}
	f.start()
	f.svc.Wait()
	if f.eng.calls() != 1 {
		t.Fatalf("engine calls = %d, want 1 (only the kept meeting)", f.eng.calls())
	}
}

func TestForgetDropsParkedChunks(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.eng.setReady(false)
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackMic, 1))
	f.svc.Wait() // parked for the missing model

	f.svc.Forget("m1")
	f.eng.setReady(true)
	f.svc.ResumeWaiting()
	f.svc.Wait()
	if f.eng.calls() != 0 {
		t.Fatalf("a forgotten chunk was transcribed (%d calls)", f.eng.calls())
	}
}

func TestForgetWaitsForTheChunkBeingTranscribed(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	be := &blockingEngine{fakeEngine: fakeEngine{ready: true}, entered: make(chan struct{}, 1), release: make(chan struct{})}
	f.svc = NewTranscriptionService(f.st, be, f.em, decode30s)
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackMic, 1))
	<-be.entered

	returned := make(chan struct{})
	go func() { f.svc.Forget("m1"); close(returned) }()
	select {
	case <-returned:
		t.Fatal("Forget returned while the meeting's chunk was still being transcribed")
	case <-time.After(100 * time.Millisecond):
	}
	close(be.release)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Forget did not return after the chunk finished")
	}
}

func TestForgetDoesNotWaitForAnotherMeetingsChunk(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.otherMeeting("m2")
	be := &blockingEngine{fakeEngine: fakeEngine{ready: true}, entered: make(chan struct{}, 1), release: make(chan struct{})}
	f.svc = NewTranscriptionService(f.st, be, f.em, decode30s)
	f.start()
	c := domain.Chunk{MeetingID: "m2", Track: domain.TrackMic, Index: 1, EndMs: 30000, Path: "/a/m2.wav"}
	if err := f.st.AddChunk(f.ctx, c); err != nil {
		t.Fatal(err)
	}
	f.svc.Enqueue(c)
	<-be.entered
	done := make(chan struct{})
	go func() { f.svc.Forget("m1"); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Forget blocked on another meeting's chunk")
	}
	close(be.release)
}

func TestDrainedListenerRunsOnceTheQueueIsEmptyAfterTheLastChunk(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	drained := make(chan int, 8)
	f.svc.SetDrainedListener(func() { drained <- f.eng.calls() })
	f.start()
	f.svc.Enqueue(f.addChunk(domain.TrackMic, 1))
	f.svc.Enqueue(f.addChunk(domain.TrackOthers, 1))
	f.svc.MeetingStopped(f.meet.ID)
	f.svc.Wait()
	select {
	case calls := <-drained:
		// Items may drain the queue more than once as they arrive; the last drain follows every call.
		for len(drained) > 0 {
			calls = <-drained
		}
		if calls != 2 {
			t.Fatalf("drained after %d engine calls, want 2", calls)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("drained listener was not called")
	}
}

// finishedMeeting makes m1 a transcribed meeting in state st with two chunks and old segments.
func (f *transFixture) finishedMeeting(st domain.MeetingState) {
	f.t.Helper()
	for _, c := range []domain.Chunk{f.addChunk(domain.TrackMic, 1), f.addChunk(domain.TrackOthers, 1)} {
		if err := f.st.MarkChunkTranscribed(f.ctx, c.MeetingID, c.Track, c.Index); err != nil {
			f.t.Fatal(err)
		}
		if _, err := f.st.AddSegment(f.ctx, domain.Segment{MeetingID: "m1", Track: c.Track, StartMs: 0, EndMs: 500, Text: "old " + string(c.Track)}); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := f.st.UpdateState(f.ctx, "m1", st); err != nil {
		f.t.Fatal(err)
	}
	if st == domain.StateSummarized {
		if err := f.st.SetSummary(f.ctx, "m1", "# summary"); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *transFixture) texts() []string {
	f.t.Helper()
	segs, err := f.st.ListSegments(f.ctx, "m1")
	if err != nil {
		f.t.Fatal(err)
	}
	var out []string
	for _, sg := range segs {
		out = append(out, sg.Text)
	}
	return out
}

func (f *transFixture) redoEvents() []string {
	var out []string
	for _, ev := range f.em.named("transcript:retranscribe") {
		out = append(out, fmt.Sprintf("%v %v/%v", ev.payload["state"], ev.payload["done"], ev.payload["total"]))
	}
	return out
}

func TestRetranscribeKeepsTheOldTranscriptUntilEveryChunkIsDone(t *testing.T) {
	f := newTransFixture(t, domain.LangPTBR)
	f.finishedMeeting(domain.StateSummarized)
	var midway []string
	f.eng.before = func(n int) {
		if n == 1 {
			midway = f.texts() // the second chunk is starting: the first one's new text is held aside
		}
	}
	f.start()
	f.svc.Wait()
	if err := f.svc.Retranscribe(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if want := []string{"old mic", "old others"}; !reflect.DeepEqual(midway, want) {
		t.Fatalf("midway = %v, want %v", midway, want)
	}
	if got, want := f.texts(), []string{"text 1", "text 2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after = %v, want %v", got, want)
	}
	segs, _ := f.st.ListSegments(f.ctx, "m1")
	if segs[0].Track != domain.TrackMic || segs[0].StartMs != 1000 || segs[0].EndMs != 3000 || segs[1].Track != domain.TrackOthers {
		t.Fatalf("segments = %+v", segs)
	}
	if got, want := f.redoEvents(), []string{"running 0/2", "running 1/2", "done 2/2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	m, _ := f.st.GetMeeting(f.ctx, "m1")
	if m.State != domain.StateSummarized || m.SummaryMD != "# summary" {
		t.Fatalf("meeting changed: state %s summary %q", m.State, m.SummaryMD)
	}
	if len(f.eng.langs) != 2 || f.eng.langs[0] != domain.LangPTBR {
		t.Fatalf("langs = %v", f.eng.langs)
	}
	if p := f.svc.RetranscribeStatus("m1"); p.Running {
		t.Fatalf("status after done: %+v", p)
	}
	if len(f.em.named("transcript:segment")) != 0 {
		t.Fatal("re-transcription emitted live segment events")
	}
}

func TestRetranscribeFailureKeepsTheOldTranscript(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.finishedMeeting(domain.StateReady)
	f.eng.failAt = 2
	f.start()
	if err := f.svc.Retranscribe(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if got, want := f.texts(), []string{"old mic", "old others"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after a failure = %v, want %v", got, want)
	}
	evs := f.em.named("transcript:retranscribe")
	last := evs[len(evs)-1].payload
	if last["state"] != "error" || last["error"] != "Transcribing again failed: others part 1: engine broke. The old transcript is kept." {
		t.Fatalf("last event = %v", last)
	}
	if p := f.svc.RetranscribeStatus("m1"); p.Running {
		t.Fatalf("status after a failure: %+v", p)
	}
	// It can run again after a failure.
	f.eng.failAt = 0
	if err := f.svc.Retranscribe(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if got := f.texts(); len(got) != 2 || got[0] == "old mic" {
		t.Fatalf("second run = %v", got)
	}
}

func TestRetranscribeRefusals(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	for _, st := range []domain.MeetingState{domain.StateRecording, domain.StateTranscribing} {
		if err := f.st.UpdateState(f.ctx, "m1", st); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.Retranscribe(f.ctx, "m1"); !errors.Is(err, ErrRetranscribeNotReady) {
			t.Fatalf("%s: err = %v", st, err)
		}
	}
	if err := f.st.UpdateState(f.ctx, "m1", domain.StateReady); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Retranscribe(f.ctx, "m1"); !errors.Is(err, ErrRetranscribeNoAudio) {
		t.Fatalf("no chunks: err = %v", err)
	}
	if err := f.svc.Retranscribe(f.ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing meeting: err = %v", err)
	}
	f.addChunk(domain.TrackMic, 1)
	f.eng.setReady(false)
	if err := f.svc.Retranscribe(f.ctx, "m1"); err == nil {
		t.Fatal("ran without models")
	}
	f.eng.setReady(true)
	// The worker is not started, so the first job stays queued.
	if err := f.svc.Retranscribe(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if p := f.svc.RetranscribeStatus("m1"); !p.Running || p.Done != 0 || p.Total != 1 {
		t.Fatalf("status = %+v", p)
	}
	if err := f.svc.Retranscribe(f.ctx, "m1"); !errors.Is(err, ErrRetranscribeRunning) {
		t.Fatalf("second: err = %v", err)
	}
}

func TestForgetStopsARetranscription(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.finishedMeeting(domain.StateReady)
	be := &blockingEngine{fakeEngine: fakeEngine{ready: true}, entered: make(chan struct{}, 1), release: make(chan struct{})}
	f.svc = NewTranscriptionService(f.st, be, f.em, decode30s)
	f.start()
	if err := f.svc.Retranscribe(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	<-be.entered
	returned := make(chan struct{})
	go func() { f.svc.Forget("m1"); close(returned) }()
	time.Sleep(50 * time.Millisecond)
	close(be.release)
	<-returned
	f.svc.Wait()
	if be.calls() != 1 {
		t.Fatalf("engine calls = %d, want the job to stop after the chunk in flight", be.calls())
	}
	if got, want := f.texts(), []string{"old mic", "old others"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %v, want %v", got, want)
	}
	if p := f.svc.RetranscribeStatus("m1"); p.Running {
		t.Fatalf("status = %+v", p)
	}
}

func TestRetranscribeLetsARecordingsChunksThrough(t *testing.T) {
	f := newTransFixture(t, domain.LangEN)
	f.finishedMeeting(domain.StateReady)
	f.otherMeeting("m2")
	live := domain.Chunk{MeetingID: "m2", Track: domain.TrackMic, Index: 1, EndMs: 30000, Path: "/a/m2.wav"}
	var order []string
	f.eng.before = func(n int) {
		if n == 0 { // a recording's chunk closes while the first re-transcription chunk runs
			if err := f.st.AddChunk(f.ctx, live); err != nil {
				t.Error(err)
			}
			f.svc.Enqueue(live)
		}
	}
	f.start()
	f.svc.Wait()
	if err := f.svc.Retranscribe(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	for _, id := range []string{"m1", "m2"} {
		segs, _ := f.st.ListSegments(f.ctx, id)
		for _, sg := range segs {
			order = append(order, id+":"+sg.Text)
		}
	}
	// call 1 is m1 mic, call 2 the live chunk, call 3 m1 others
	if want := []string{"m1:text 1", "m1:text 3", "m2:text 2"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
