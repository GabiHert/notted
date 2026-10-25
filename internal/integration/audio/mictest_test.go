package audio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
)

func testerRig(t *testing.T, mode string) (*MicTester, string) {
	t.Helper()
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_AUDIOCAP_MODE", mode)
	t.Setenv("FAKE_AUDIOCAP_ARGS", args)
	return NewMicTester(filepath.Join("testdata", "fake-audiocap.sh")), args
}

func TestMicTestRunsLevelWithTheSecondsAndStreamsItsEvents(t *testing.T) {
	m, args := testerRig(t, "level-ok")
	c := &collector{}
	run, err := m.Start(10, c.add)
	if err != nil {
		t.Fatal(err)
	}
	if x := waitExit(t, run); x.Code != 0 || x.Message != "" {
		t.Fatalf("exit = %+v", x)
	}
	if b, _ := os.ReadFile(args); strings.TrimSpace(string(b)) != "level --seconds 10" {
		t.Fatalf("argv = %q", b)
	}
	ev := c.snapshot()
	var kinds []string
	for _, e := range ev {
		kinds = append(kinds, e.Kind)
	}
	if got, want := strings.Join(kinds, ","), "device,level,level,level,stopped"; got != want {
		t.Fatalf("kinds = %s, want %s", got, want)
	}
	if ev[0].Device != "Fake Mic" {
		t.Fatalf("device = %+v", ev[0])
	}
	if ev[1].RMSdB != -35.5 || ev[1].PeakDB != -12.25 {
		t.Fatalf("level = %+v", ev[1])
	}
}

func TestMicTestSkipsLinesThatAreNotEvents(t *testing.T) {
	m, _ := testerRig(t, "level-garbage")
	c := &collector{}
	run, err := m.Start(5, c.add)
	if err != nil {
		t.Fatal(err)
	}
	waitExit(t, run)
	if ev := c.snapshot(); len(ev) != 3 || ev[0].Kind != "device" {
		t.Fatalf("events = %+v", ev)
	}
}

func TestMicTestStopEndsARunningHelperEarly(t *testing.T) {
	m, _ := testerRig(t, "level-run")
	c := &collector{}
	run, err := m.Start(60, c.add)
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return len(c.snapshot()) >= 2 })
	if err := run.Stop(); err != nil {
		t.Fatal(err)
	}
	if x := waitExit(t, run); x.Code != 0 {
		t.Fatalf("exit = %+v", x)
	}
	ev := c.snapshot()
	if ev[len(ev)-1].Kind != "stopped" {
		t.Fatalf("last event = %+v", ev[len(ev)-1])
	}
}

func TestMicTestKillsAHelperThatIgnoresStop(t *testing.T) {
	m, _ := testerRig(t, "level-hang")
	m.stopTimeout = 200 * time.Millisecond
	run, err := m.Start(60, (&collector{}).add)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_ = run.Stop()
	x := waitExit(t, run)
	if x.Code == 0 || time.Since(start) > 5*time.Second {
		t.Fatalf("exit = %+v after %v", x, time.Since(start))
	}
}

func TestMicTestStopsAHelperThatOutlivesItsSeconds(t *testing.T) {
	m, _ := testerRig(t, "level-run")
	m.grace = 100 * time.Millisecond
	run, err := m.Start(0, (&collector{}).add) // 0 seconds plus the grace
	if err != nil {
		t.Fatal(err)
	}
	if x := waitExit(t, run); x.Code != 0 {
		t.Fatalf("exit = %+v", x)
	}
}

func TestMicTestMapsTheExitCodes(t *testing.T) {
	cases := map[string]string{
		"level-denied":    msgMic,
		"level-no-device": msgMicNoDevice,
		"level-crash":     msgMicTestFailed,
		"level-killed":    msgMicTestFailed,
	}
	for mode, want := range cases {
		t.Run(mode, func(t *testing.T) {
			m, _ := testerRig(t, mode)
			run, err := m.Start(10, (&collector{}).add)
			if err != nil {
				t.Fatal(err)
			}
			if x := waitExit(t, run); x.Code == 0 || x.Message != want {
				t.Fatalf("exit = %+v, want message %q", x, want)
			}
		})
	}
}

func TestMicTestMissingHelperIsUserFacing(t *testing.T) {
	m := NewMicTester(filepath.Join(t.TempDir(), "nope"))
	if _, err := m.Start(10, (&collector{}).add); err == nil || err.Error() != msgMicTestFailed {
		t.Fatalf("err = %v", err)
	}
}

func TestStartedEventCarriesTheMicDevice(t *testing.T) {
	s, out, _, _ := setup(t, "normal")
	c := &collector{}
	rec, err := s.Start(out, c.add)
	if err != nil {
		t.Fatal(err)
	}
	_ = rec.Stop()
	waitExit(t, rec)
	if ev := c.snapshot(); ev[0].Kind != "started" || ev[0].Device != "Fake Mic" {
		t.Fatalf("started = %+v", ev[0])
	}
}

var _ port.MicTester = (*MicTester)(nil)

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
