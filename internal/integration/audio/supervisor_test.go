package audio

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

const (
	msgMic    = "Microphone access is off for Notted. Turn it on in System Settings > Privacy & Security > Microphone, then try again."
	msgScreen = "Screen & System Audio Recording access is off for Notted. Turn it on in System Settings > Privacy & Security > Screen & System Audio Recording, then try again."
	msgDevice = "No microphone or display is available. Connect one and try again."
	msgOther  = "The audio recorder stopped unexpectedly. The audio recorded so far is kept and will be transcribed. Start a new recording to continue."
)

type collector struct {
	mu     sync.Mutex
	events []port.HelperEvent
}

func (c *collector) add(e port.HelperEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *collector) snapshot() []port.HelperEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]port.HelperEvent(nil), c.events...)
}

func setup(t *testing.T, mode string) (*Supervisor, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	args := filepath.Join(dir, "args")
	t.Setenv("FAKE_AUDIOCAP_MODE", mode)
	t.Setenv("FAKE_AUDIOCAP_COUNT", count)
	t.Setenv("FAKE_AUDIOCAP_ARGS", args)
	s := NewSupervisor(filepath.Join("testdata", "fake-audiocap.sh"))
	return s, filepath.Join(dir, "out"), count, args
}

func startCount(t *testing.T, count string) int {
	t.Helper()
	b, err := os.ReadFile(count)
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

func waitExit(t *testing.T, r port.Recording) port.RecorderExit {
	t.Helper()
	select {
	case x := <-r.Exited():
		return x
	case <-time.After(10 * time.Second):
		t.Fatal("no exit")
		return port.RecorderExit{}
	}
}

func TestStartPassesExactArgvAndParsesEvents(t *testing.T) {
	s, out, _, args := setup(t, "normal")
	c := &collector{}
	rec, err := s.Start(out, c.add)
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}
	x := waitExit(t, rec)
	if x.Code != 0 || x.Message != "" {
		t.Fatalf("exit = %+v", x)
	}
	b, _ := os.ReadFile(args)
	if got, want := strings.TrimSpace(string(b)), "record --out "+out+" --chunk-seconds 30"; got != want {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	ev := c.snapshot()
	var kinds []string
	for _, e := range ev {
		kinds = append(kinds, e.Kind)
	}
	if got, want := strings.Join(kinds, ","), "started,chunk,chunk,chunk,chunk,stopped"; got != want {
		t.Fatalf("kinds = %s, want %s", got, want)
	}
	if ev[0].T0Ms != 1000 {
		t.Fatalf("t0 = %d", ev[0].T0Ms)
	}
	ch := ev[1]
	if ch.Track != domain.TrackMic || ch.Index != 1 || ch.StartMs != 0 || ch.EndMs != 100 ||
		ch.Path != filepath.Join(out, "mic-000001.wav") {
		t.Fatalf("chunk = %+v", ch)
	}
	if ev[2].Track != domain.TrackOthers {
		t.Fatalf("second chunk = %+v", ev[2])
	}
	// chunk 2 events arrive on stop
	if ev[3].Index != 2 || ev[4].Index != 2 {
		t.Fatalf("stop chunks = %+v %+v", ev[3], ev[4])
	}
}

func TestStartFailsWithMappedMessage(t *testing.T) {
	cases := map[string]string{
		"mic-denied":    msgMic,
		"screen-denied": msgScreen,
		"no-device":     msgDevice,
	}
	for mode, want := range cases {
		t.Run(mode, func(t *testing.T) {
			s, out, count, _ := setup(t, mode)
			_, err := s.Start(out, func(port.HelperEvent) {})
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
			if n := startCount(t, count); n != 1 {
				t.Fatalf("starts = %d, want 1 (never restart)", n)
			}
		})
	}
}

func TestCrashReportsCodeOneWithLastErrorMessage(t *testing.T) {
	s, out, count, _ := setup(t, "crash")
	c := &collector{}
	rec, err := s.Start(out, c.add)
	if err != nil {
		t.Fatal(err)
	}
	x := waitExit(t, rec)
	want := "The audio recorder stopped: capture_failed: stream stopped. The audio recorded so far is kept."
	if x.Code != 1 || x.Message != want {
		t.Fatalf("exit = %+v", x)
	}
	var chunks int
	for _, e := range c.snapshot() {
		if e.Kind == "chunk" {
			chunks++
		}
	}
	if chunks != 1 {
		t.Fatalf("chunks delivered before exit = %d", chunks)
	}
	time.Sleep(100 * time.Millisecond)
	if n := startCount(t, count); n != 1 {
		t.Fatalf("starts = %d", n)
	}
}

func TestKilledReportsUnexpectedMessage(t *testing.T) {
	s, out, count, _ := setup(t, "killed")
	rec, err := s.Start(out, func(port.HelperEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	x := waitExit(t, rec)
	if x.Code != -1 || x.Message != msgOther {
		t.Fatalf("exit = %+v", x)
	}
	if n := startCount(t, count); n != 1 {
		t.Fatalf("starts = %d", n)
	}
}

func TestStopFallsBackToKill(t *testing.T) {
	s, out, _, _ := setup(t, "ignore-int")
	s.stopTimeout = 200 * time.Millisecond
	rec, err := s.Start(out, func(port.HelperEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	begin := time.Now()
	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(begin); d < 200*time.Millisecond || d > 5*time.Second {
		t.Fatalf("stop took %v", d)
	}
	if x := waitExit(t, rec); x.Code != -1 {
		t.Fatalf("exit = %+v", x)
	}
}

func TestStartTimeoutStopsHelper(t *testing.T) {
	s, out, _, _ := setup(t, "silent-hang")
	s.startTimeout = 300 * time.Millisecond
	s.stopTimeout = 200 * time.Millisecond
	_, err := s.Start(out, func(port.HelperEvent) {})
	if err == nil || err.Error() != "The audio recorder did not start within 2 minutes. Try again." {
		t.Fatalf("err = %v", err)
	}
}
