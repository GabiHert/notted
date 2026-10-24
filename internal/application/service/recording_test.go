package service

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

// memStore is an in-memory port.MeetingStore; methods the recording service does not use panic.
type memStore struct {
	port.MeetingStore
	mu       sync.Mutex
	meetings map[string]domain.Meeting
	chunks   []domain.Chunk
	created  int
}

func newMemStore() *memStore { return &memStore{meetings: map[string]domain.Meeting{}} }

func (s *memStore) CreateMeeting(_ context.Context, m domain.Meeting) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meetings[m.ID] = m
	s.created++
	return nil
}

func (s *memStore) GetMeeting(_ context.Context, id string) (domain.Meeting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.meetings[id]
	if !ok {
		return domain.Meeting{}, domain.ErrNotFound
	}
	return m, nil
}

func (s *memStore) ListMeetings(context.Context) ([]domain.Meeting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Meeting
	for _, m := range s.meetings {
		out = append(out, m)
	}
	return out, nil
}

func (s *memStore) UpdateState(_ context.Context, id string, st domain.MeetingState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.meetings[id]
	m.State = st
	s.meetings[id] = m
	return nil
}

func (s *memStore) SetEndedAt(_ context.Context, id string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.meetings[id]
	m.EndedAt = &t
	s.meetings[id] = m
	return nil
}

func (s *memStore) AddChunk(_ context.Context, c domain.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.meetings[c.MeetingID]; !ok {
		return errors.New("foreign key: no meeting")
	}
	s.chunks = append(s.chunks, c)
	return nil
}

func (s *memStore) meeting(id string) domain.Meeting {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.meetings[id]
}

func (s *memStore) chunkCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.chunks)
}

type emitted struct {
	name    string
	payload map[string]any
}

type recEmitter struct {
	mu     sync.Mutex
	events []emitted
}

func (e *recEmitter) Emit(name string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, _ := payload.(map[string]any)
	e.events = append(e.events, emitted{name, m})
}

func (e *recEmitter) named(name string) []emitted {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []emitted
	for _, ev := range e.events {
		if ev.name == name {
			out = append(out, ev)
		}
	}
	return out
}

// zeroInspector calls a chunk silent when every sample byte is zero.
type zeroInspector struct{}

func (zeroInspector) Inspect(path string) (port.WavInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return port.WavInfo{}, err
	}
	for _, c := range b[44:] {
		if c != 0 {
			return port.WavInfo{}, nil
		}
	}
	return port.WavInfo{Silent: true}, nil
}

type fakeRecording struct {
	exited  chan port.RecorderExit
	stopped int
	mu      sync.Mutex
}

func (r *fakeRecording) Stop() error {
	r.mu.Lock()
	r.stopped++
	r.mu.Unlock()
	r.exited <- port.RecorderExit{}
	return nil
}
func (r *fakeRecording) Exited() <-chan port.RecorderExit { return r.exited }

type fakeRecorder struct {
	err      error
	onEvent  func(port.HelperEvent)
	rec      *fakeRecording
	outDir   string
	earlyEvt *port.HelperEvent // emitted from a goroutine right after "started"
	entered  chan struct{}     // closed when Start is called, if set
	gate     chan struct{}     // Start waits for it to close before printing "started", if set
}

func (f *fakeRecorder) Start(outDir string, onEvent func(port.HelperEvent)) (port.Recording, error) {
	f.outDir = outDir
	if f.entered != nil {
		close(f.entered)
	}
	if f.gate != nil {
		<-f.gate
	}
	if f.err != nil {
		return nil, f.err
	}
	f.onEvent = onEvent
	f.rec = &fakeRecording{exited: make(chan port.RecorderExit, 1)}
	onEvent(port.HelperEvent{Kind: "started", T0Ms: 1})
	if f.earlyEvt != nil {
		go onEvent(*f.earlyEvt)
	}
	return f.rec, nil
}

type fixture struct {
	t     *testing.T
	store *memStore
	rec   *fakeRecorder
	emit  *recEmitter
	svc   *RecordingService
	root  string
	now   time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	f := &fixture{t: t, store: newMemStore(), rec: &fakeRecorder{}, emit: &recEmitter{}, root: t.TempDir(),
		now: time.Date(2026, 10, 23, 14, 5, 0, 0, time.Local)}
	f.svc = NewRecordingService(f.store, f.rec, f.emit, func(id string) string { return filepath.Join(f.root, id) }, zeroInspector{})
	f.svc.now = func() time.Time { return f.now }
	n := 0
	f.svc.newID = func() string { n++; return "m" + string(rune('0'+n)) }
	return f
}

func writeWav(t *testing.T, path string, amp int16) {
	t.Helper()
	n := 1600
	data := make([]byte, n*2)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(data[2*i:], uint16(amp))
	}
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 16000)
	binary.LittleEndian.PutUint32(h[28:], 32000)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	if err := os.WriteFile(path, append(h, data...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) chunk(track domain.Track, idx int, amp int16) {
	f.t.Helper()
	p := filepath.Join(f.rec.outDir, string(track)+"-"+string(rune('0'+idx))+".wav")
	writeWav(f.t, p, amp)
	f.rec.onEvent(port.HelperEvent{Kind: "chunk", Track: track, Index: idx, Path: p, StartMs: int64(idx-1) * 30000, EndMs: int64(idx) * 30000})
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met")
}

func TestStartCreatesMeetingOnlyAfterStarted(t *testing.T) {
	f := newFixture(t)
	m, err := f.svc.Start(context.Background(), "Weekly", domain.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	got := f.store.meeting(m.ID)
	if got.State != domain.StateRecording || got.Title != "Weekly" || got.Language != domain.LangEN || got.EndedAt != nil {
		t.Fatalf("meeting = %+v", got)
	}
	if !got.StartedAt.Equal(f.now) || got.StartedAt.Location() != time.UTC {
		t.Fatalf("started at = %v", got.StartedAt)
	}
	if f.rec.outDir != filepath.Join(f.root, m.ID) {
		t.Fatalf("outDir = %s", f.rec.outDir)
	}
	st := f.emit.named("recording:state")
	if len(st) != 1 || st[0].payload["meetingId"] != m.ID || st[0].payload["state"] != "recording" {
		t.Fatalf("state events = %+v", st)
	}
}

func TestStartFailureCreatesNoRowAndRemovesEmptyFolder(t *testing.T) {
	f := newFixture(t)
	f.rec.err = errors.New("Microphone access is off for Notted.")
	_, err := f.svc.Start(context.Background(), "x", domain.LangEN)
	if err == nil || err.Error() != "Microphone access is off for Notted." {
		t.Fatalf("err = %v", err)
	}
	if f.store.created != 0 {
		t.Fatal("meeting row created")
	}
	if _, statErr := os.Stat(f.rec.outDir); !os.IsNotExist(statErr) {
		t.Fatalf("folder still there: %v", statErr)
	}
	// and a new start is allowed afterwards
	f.rec.err = nil
	if _, err := f.svc.Start(context.Background(), "x", domain.LangEN); err != nil {
		t.Fatal(err)
	}
}

func TestSecondStartRefused(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Start(context.Background(), "a", domain.LangEN); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.Start(context.Background(), "b", domain.LangEN)
	if err == nil || err.Error() != "A recording is already running. Stop it first." {
		t.Fatalf("err = %v", err)
	}
}

func TestBadLanguage(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Start(context.Background(), "a", domain.Language("fr"))
	if err == nil || err.Error() != `Unknown language "fr". Use pt-BR or en.` {
		t.Fatalf("err = %v", err)
	}
	if f.rec.rec != nil {
		t.Fatal("helper started")
	}
}

func TestDefaultTitle(t *testing.T) {
	f := newFixture(t)
	m, err := f.svc.Start(context.Background(), "   ", domain.LangPTBR)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Meeting 2026-10-23 14:05" {
		t.Fatalf("title = %q", m.Title)
	}
}

func TestChunkStoresRowEmitsEventAndCallsListener(t *testing.T) {
	f := newFixture(t)
	var heard []domain.Chunk
	f.svc.SetChunkListener(func(c domain.Chunk) { heard = append(heard, c) })
	m, _ := f.svc.Start(context.Background(), "a", domain.LangEN)
	f.chunk(domain.TrackMic, 1, 8000)
	if f.store.chunkCount() != 1 {
		t.Fatal("no chunk row")
	}
	c := f.store.chunks[0]
	if c.MeetingID != m.ID || c.Track != domain.TrackMic || c.Index != 1 || c.EndMs != 30000 || c.Transcribed {
		t.Fatalf("chunk = %+v", c)
	}
	ev := f.emit.named("recording:chunk")
	if len(ev) != 1 || ev[0].payload["meetingId"] != m.ID || ev[0].payload["track"] != "mic" || ev[0].payload["index"] != 1 {
		t.Fatalf("chunk events = %+v", ev)
	}
	if len(heard) != 1 || heard[0].Index != 1 {
		t.Fatalf("listener = %+v", heard)
	}
}

func TestChunkArrivingBeforeStartReturnsIsStillRecorded(t *testing.T) {
	f := newFixture(t)
	p := filepath.Join(t.TempDir(), "mic-1.wav")
	writeWav(t, p, 8000)
	f.rec.earlyEvt = &port.HelperEvent{Kind: "chunk", Track: domain.TrackMic, Index: 1, Path: p, EndMs: 100}
	if _, err := f.svc.Start(context.Background(), "a", domain.LangEN); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return f.store.chunkCount() == 1 })
}

func TestSilentSystemAudioWarnsOnceAfterTwoChunks(t *testing.T) {
	f := newFixture(t)
	m, _ := f.svc.Start(context.Background(), "a", domain.LangEN)
	warnings := func() int {
		n := 0
		for _, e := range f.emit.named("recording:state") {
			if w, ok := e.payload["warning"]; ok {
				n++
				if e.payload["state"] != "recording" || e.payload["meetingId"] != m.ID ||
					w != "No system audio was heard in the last minute. If the other side was talking, check that Notted is allowed in System Settings > Privacy & Security > Screen & System Audio Recording." {
					t.Fatalf("warning payload = %+v", e.payload)
				}
			}
		}
		return n
	}
	f.chunk(domain.TrackOthers, 1, 0)
	if warnings() != 0 {
		t.Fatal("warned after one")
	}
	f.chunk(domain.TrackOthers, 2, 0)
	if warnings() != 1 {
		t.Fatalf("warnings = %d", warnings())
	}
	f.chunk(domain.TrackOthers, 3, 0)
	if warnings() != 1 {
		t.Fatalf("warnings = %d, want still 1", warnings())
	}
}

func TestAudibleChunkResetsSilentRun(t *testing.T) {
	f := newFixture(t)
	f.svc.Start(context.Background(), "a", domain.LangEN)
	f.chunk(domain.TrackOthers, 1, 0)
	f.chunk(domain.TrackOthers, 2, 8000)
	f.chunk(domain.TrackOthers, 3, 0)
	for _, e := range f.emit.named("recording:state") {
		if _, ok := e.payload["warning"]; ok {
			t.Fatal("warned although the silent chunks were not consecutive")
		}
	}
}

func TestStopMovesToTranscribing(t *testing.T) {
	f := newFixture(t)
	m, _ := f.svc.Start(context.Background(), "a", domain.LangEN)
	f.chunk(domain.TrackMic, 1, 8000)
	if err := f.svc.Stop(context.Background(), m.ID); err != nil {
		t.Fatal(err)
	}
	got := f.store.meeting(m.ID)
	if got.State != domain.StateTranscribing || got.EndedAt == nil || !got.EndedAt.Equal(f.now) {
		t.Fatalf("meeting = %+v", got)
	}
	if f.rec.rec.stopped != 1 {
		t.Fatal("helper not stopped")
	}
	st := f.emit.named("recording:state")
	last := st[len(st)-1]
	if last.payload["state"] != "transcribing" || last.payload["meetingId"] != m.ID {
		t.Fatalf("last state event = %+v", last)
	}
	// slot is free again
	if _, err := f.svc.Start(context.Background(), "b", domain.LangEN); err != nil {
		t.Fatal(err)
	}
}

func TestStopUnknownMeeting(t *testing.T) {
	f := newFixture(t)
	err := f.svc.Stop(context.Background(), "nope")
	if err == nil || err.Error() != "Meeting not found." || !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestStopOnStaleRecordingMeetingChangesStateWithoutHelper(t *testing.T) {
	f := newFixture(t)
	f.store.meetings["old"] = domain.Meeting{ID: "old", State: domain.StateRecording}
	if err := f.svc.Stop(context.Background(), "old"); err != nil {
		t.Fatal(err)
	}
	if got := f.store.meeting("old"); got.State != domain.StateTranscribing || got.EndedAt == nil {
		t.Fatalf("meeting = %+v", got)
	}
}

func TestStopOnOtherStateIsNoOp(t *testing.T) {
	f := newFixture(t)
	f.store.meetings["r"] = domain.Meeting{ID: "r", State: domain.StateReady}
	if err := f.svc.Stop(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	if got := f.store.meeting("r"); got.State != domain.StateReady || got.EndedAt != nil {
		t.Fatalf("meeting = %+v", got)
	}
	if len(f.emit.named("recording:state")) != 0 {
		t.Fatal("event emitted")
	}
}

func TestUnexpectedExitKeepsChunksAndReportsError(t *testing.T) {
	f := newFixture(t)
	m, _ := f.svc.Start(context.Background(), "a", domain.LangEN)
	f.chunk(domain.TrackMic, 1, 8000)
	msg := "The audio recorder stopped unexpectedly. The audio recorded so far is kept and will be transcribed. Start a new recording to continue."
	f.rec.rec.exited <- port.RecorderExit{Code: -1, Message: msg}
	eventually(t, func() bool { return f.store.meeting(m.ID).State == domain.StateTranscribing })
	got := f.store.meeting(m.ID)
	if got.EndedAt == nil || f.store.chunkCount() != 1 {
		t.Fatalf("meeting = %+v chunks = %d", got, f.store.chunkCount())
	}
	var found bool
	for _, e := range f.emit.named("recording:state") {
		if e.payload["state"] == "transcribing" && e.payload["error"] == msg && e.payload["meetingId"] == m.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("no error event: %+v", f.emit.named("recording:state"))
	}
	if _, err := f.svc.Start(context.Background(), "b", domain.LangEN); err != nil {
		t.Fatalf("slot not cleared: %v", err)
	}
}

func TestRecoverOnStartup(t *testing.T) {
	f := newFixture(t)
	f.store.meetings["a"] = domain.Meeting{ID: "a", State: domain.StateRecording}
	f.store.meetings["b"] = domain.Meeting{ID: "b", State: domain.StateReady}
	if err := f.svc.RecoverOnStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.store.meeting("a"); got.State != domain.StateTranscribing || got.EndedAt == nil || !got.EndedAt.Equal(f.now) {
		t.Fatalf("a = %+v", got)
	}
	if got := f.store.meeting("b"); got.State != domain.StateReady || got.EndedAt != nil {
		t.Fatalf("b = %+v", got)
	}
}

func TestStopAllStopsActiveRecording(t *testing.T) {
	f := newFixture(t)
	m, _ := f.svc.Start(context.Background(), "a", domain.LangEN)
	f.svc.StopAll()
	if f.store.meeting(m.ID).State != domain.StateTranscribing || f.rec.rec.stopped != 1 {
		t.Fatal("not stopped")
	}
	f.svc.StopAll() // no active recording: harmless
}

func TestStopAllDuringSlowStartIsRaceFree(t *testing.T) {
	for i := 0; i < 20; i++ {
		f := newFixture(t)
		f.rec.entered = make(chan struct{})
		f.rec.gate = make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, err := f.svc.Start(context.Background(), "a", domain.LangEN)
			done <- err
		}()
		<-f.rec.entered // the helper is waiting for a permission prompt
		f.svc.StopAll() // the person quits the app meanwhile: nothing to stop yet
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); f.svc.StopAll() }()
		close(f.rec.gate) // "started" arrives while another quit is in flight
		wg.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		f.svc.StopAll()
	}
}

func TestStoppedListenerCalledOnStopExitAndRecovery(t *testing.T) {
	f := newFixture(t)
	var mu sync.Mutex
	var got []string
	f.svc.SetStoppedListener(func(id string) { mu.Lock(); got = append(got, id); mu.Unlock() })
	ids := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), got...) }

	m1, _ := f.svc.Start(context.Background(), "a", domain.LangEN)
	if err := f.svc.Stop(context.Background(), m1.ID); err != nil {
		t.Fatal(err)
	}
	if g := ids(); len(g) != 1 || g[0] != m1.ID {
		t.Fatalf("after stop = %v", g)
	}
	m2, _ := f.svc.Start(context.Background(), "b", domain.LangEN)
	f.rec.rec.exited <- port.RecorderExit{Code: -1, Message: "gone"}
	eventually(t, func() bool { return len(ids()) == 2 })
	if g := ids(); g[1] != m2.ID {
		t.Fatalf("after exit = %v", g)
	}
	f.store.meetings["old"] = domain.Meeting{ID: "old", State: domain.StateRecording}
	if err := f.svc.RecoverOnStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if g := ids(); len(g) != 3 || g[2] != "old" {
		t.Fatalf("after recovery = %v", g)
	}
}

func TestRecordingReports(t *testing.T) {
	f := newFixture(t)
	if f.svc.Recording() {
		t.Fatal("recording before Start")
	}
	m, err := f.svc.Start(context.Background(), "a", domain.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	if !f.svc.Recording() {
		t.Fatal("not recording after Start")
	}
	if err := f.svc.Stop(context.Background(), m.ID); err != nil {
		t.Fatal(err)
	}
	if f.svc.Recording() {
		t.Fatal("recording after Stop")
	}
}
