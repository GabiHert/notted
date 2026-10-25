package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/GabiHert/notted/internal/application/port"
)

type fakeRun struct {
	exited  chan port.RecorderExit
	mu      sync.Mutex
	stopped int
}

func (r *fakeRun) Stop() error {
	r.mu.Lock()
	r.stopped++
	r.mu.Unlock()
	r.exited <- port.RecorderExit{}
	return nil
}
func (r *fakeRun) Exited() <-chan port.RecorderExit { return r.exited }
func (r *fakeRun) stops() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

type fakeTester struct {
	err     error
	seconds []int
	onEvent func(port.HelperEvent)
	run     *fakeRun
}

func (f *fakeTester) Start(seconds int, onEvent func(port.HelperEvent)) (port.Recording, error) {
	f.seconds = append(f.seconds, seconds)
	if f.err != nil {
		return nil, f.err
	}
	f.onEvent = onEvent
	f.run = &fakeRun{exited: make(chan port.RecorderExit, 1)}
	return f.run, nil
}

type permsFake struct {
	set port.PermissionSet
	err error
}

func (p permsFake) Status(context.Context) (port.PermissionSet, error) { return p.set, p.err }
func (p permsFake) Ask(context.Context) (port.PermissionSet, error)    { return p.set, p.err }

type micFixture struct {
	t         *testing.T
	tester    *fakeTester
	emit      *recEmitter
	svc       *MicTestService
	recording bool
}

func newMicFixture(t *testing.T, mic port.PermissionState) *micFixture {
	t.Helper()
	f := &micFixture{t: t, tester: &fakeTester{}, emit: &recEmitter{}}
	perms := permsFake{set: port.PermissionSet{Microphone: mic, Screen: port.PermissionAllowed}}
	f.svc = NewMicTestService(f.tester, perms, func() bool { return f.recording }, f.emit)
	return f
}

func (f *micFixture) device(name string) {
	f.tester.onEvent(port.HelperEvent{Kind: "device", Device: name})
}
func (f *micFixture) level(rms, peak float64) {
	f.tester.onEvent(port.HelperEvent{Kind: "level", RMSdB: rms, PeakDB: peak})
}
func (f *micFixture) exit(x port.RecorderExit) { f.tester.run.exited <- x }

func (f *micFixture) done() map[string]any {
	f.t.Helper()
	eventually(f.t, func() bool { return len(f.emit.named("mictest:done")) == 1 })
	return f.emit.named("mictest:done")[0].payload
}

const (
	wantSilent = "No sound from Studio Mic. If your MacBook's lid is closed, its built-in microphone is off: open the lid, or choose another input in System Settings > Sound > Input."
	wantQuiet  = "Very quiet. Speak closer to Studio Mic, or choose another input in System Settings > Sound > Input."
	wantWorks  = "The microphone works."
)

func TestMicTestListens10SecondsAndStreamsLevelsWithTheDevice(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.tester.seconds) != 1 || f.tester.seconds[0] != 10 {
		t.Fatalf("seconds = %v", f.tester.seconds)
	}
	f.device("Studio Mic")
	f.level(-30.5, -12)
	lv := f.emit.named("mictest:level")
	if len(lv) != 1 || lv[0].payload["rmsDb"] != -30.5 || lv[0].payload["peakDb"] != -12.0 || lv[0].payload["device"] != "Studio Mic" {
		t.Fatalf("level = %+v", lv)
	}
	f.exit(port.RecorderExit{})
	f.done()
}

func TestMicTestVerdicts(t *testing.T) {
	cases := []struct {
		name    string
		levels  [][2]float64
		verdict string
		message string
	}{
		{"digital silence", [][2]float64{{-120, -120}, {-120, -120}}, "silent", wantSilent},
		{"very quiet", [][2]float64{{-90, -80}, {-70, -41.5}}, "quiet", wantQuiet},
		{"works", [][2]float64{{-90, -80}, {-30, -12}}, "ok", wantWorks},
		{"exactly at the quiet limit works", [][2]float64{{-50, -40}}, "ok", wantWorks},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newMicFixture(t, port.PermissionAllowed)
			if err := f.svc.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			f.device("Studio Mic")
			for _, l := range c.levels {
				f.level(l[0], l[1])
			}
			f.exit(port.RecorderExit{})
			got := f.done()
			if got["verdict"] != c.verdict || got["message"] != c.message || got["device"] != "Studio Mic" || got["stopped"] != false {
				t.Fatalf("done = %+v", got)
			}
		})
	}
}

func TestMicTestWithoutADeviceNameSaysTheMicrophone(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	_ = f.svc.Start(context.Background())
	f.level(-120, -120)
	f.exit(port.RecorderExit{})
	got := f.done()
	if got["message"] != "No sound from the microphone. If your MacBook's lid is closed, its built-in microphone is off: open the lid, or choose another input in System Settings > Sound > Input." {
		t.Fatalf("message = %v", got["message"])
	}
}

func TestMicTestThatHeardNothingAtAllIsSilent(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	_ = f.svc.Start(context.Background())
	f.device("Studio Mic")
	f.exit(port.RecorderExit{})
	if got := f.done(); got["verdict"] != "silent" {
		t.Fatalf("done = %+v", got)
	}
}

func TestMicTestRefusedWhileRecording(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	f.recording = true
	err := f.svc.Start(context.Background())
	if err == nil || err.Error() != "Stop the recording to test the microphone." {
		t.Fatalf("err = %v", err)
	}
	if len(f.tester.seconds) != 0 {
		t.Fatal("the helper must not run during a recording")
	}
}

func TestMicTestRefusedWhenTheMicrophoneIsNotAllowed(t *testing.T) {
	for _, st := range []port.PermissionState{port.PermissionDenied, port.PermissionNotAsked} {
		t.Run(string(st), func(t *testing.T) {
			f := newMicFixture(t, st)
			err := f.svc.Start(context.Background())
			if err == nil || err.Error() != msgMicTestNotAllowed {
				t.Fatalf("err = %v", err)
			}
			if len(f.tester.seconds) != 0 {
				t.Fatal("the helper must not run without the permission")
			}
		})
	}
}

func TestMicTestPermissionReadFailureIsReturned(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	f.svc = NewMicTestService(f.tester, permsFake{err: errors.New("helper missing")}, func() bool { return false }, f.emit)
	if err := f.svc.Start(context.Background()); err == nil || err.Error() != "helper missing" {
		t.Fatalf("err = %v", err)
	}
	// the failed start must not leave the service busy
	f.svc = NewMicTestService(f.tester, permsFake{set: port.PermissionSet{Microphone: port.PermissionAllowed}}, func() bool { return false }, f.emit)
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMicTestSecondStartRefusedWhileOneRuns(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := f.svc.Start(context.Background())
	if err == nil || err.Error() != "The microphone test is already running." {
		t.Fatalf("err = %v", err)
	}
	if len(f.tester.seconds) != 1 {
		t.Fatalf("helper started %d times", len(f.tester.seconds))
	}
}

func TestMicTestCanStartAgainAfterItEnds(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	_ = f.svc.Start(context.Background())
	f.exit(port.RecorderExit{})
	f.done()
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(f.tester.seconds) != 2 {
		t.Fatalf("starts = %v", f.tester.seconds)
	}
}

func TestMicTestStartFailureIsReturnedAndFreesTheService(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	f.tester.err = errors.New("The microphone test stopped unexpectedly. Try again.")
	if err := f.svc.Start(context.Background()); err == nil || err.Error() != f.tester.err.Error() {
		t.Fatalf("err = %v", err)
	}
	f.tester.err = nil
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatalf("after a failed start: %v", err)
	}
}

func TestMicTestStopEndsTheRunWithoutAVerdict(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	_ = f.svc.Start(context.Background())
	f.device("Studio Mic")
	f.level(-120, -120)
	if err := f.svc.Stop(); err != nil {
		t.Fatal(err)
	}
	if f.tester.run.stops() != 1 {
		t.Fatalf("stops = %d", f.tester.run.stops())
	}
	got := f.done()
	if got["stopped"] != true || got["verdict"] != "" || got["message"] != "" || got["device"] != "Studio Mic" {
		t.Fatalf("done = %+v", got)
	}
	if len(f.emit.named("mictest:error")) != 0 {
		t.Fatal("a stop is not an error")
	}
}

func TestMicTestStopWithNothingRunningIsNoOp(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	if err := f.svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestMicTestHelperFailureEmitsTheErrorAndNoVerdict(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	_ = f.svc.Start(context.Background())
	f.device("Studio Mic")
	f.exit(port.RecorderExit{Code: 2, Message: "Microphone access is off for Notted."})
	eventually(t, func() bool { return len(f.emit.named("mictest:error")) == 1 })
	if got := f.emit.named("mictest:error")[0].payload["message"]; got != "Microphone access is off for Notted." {
		t.Fatalf("message = %v", got)
	}
	if len(f.emit.named("mictest:done")) != 0 {
		t.Fatal("a failed run must not report a verdict")
	}
	if err := f.svc.Start(context.Background()); err != nil {
		t.Fatalf("start after a failure: %v", err)
	}
}

func TestMicTestHelperKilledAfterAStopStillEndsAsStopped(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	_ = f.svc.Start(context.Background())
	f.svc.mu.Lock()
	f.svc.stopping = true
	f.svc.mu.Unlock()
	f.exit(port.RecorderExit{Code: -1, Message: "stopped unexpectedly"})
	if got := f.done(); got["stopped"] != true {
		t.Fatalf("done = %+v", got)
	}
}

// gatedPerms blocks Status until release is closed.
type gatedPerms struct {
	entered chan struct{}
	release chan struct{}
}

func (g gatedPerms) Status(context.Context) (port.PermissionSet, error) {
	close(g.entered)
	<-g.release
	return port.PermissionSet{Microphone: port.PermissionAllowed, Screen: port.PermissionAllowed}, nil
}
func (g gatedPerms) Ask(context.Context) (port.PermissionSet, error) { return g.Status(nil) }

func TestMicTestStopWhileThePermissionCheckRunsNeverLaunchesTheHelper(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	g := gatedPerms{entered: make(chan struct{}), release: make(chan struct{})}
	f.svc = NewMicTestService(f.tester, g, func() bool { return false }, f.emit)
	errc := make(chan error, 1)
	go func() { errc <- f.svc.Start(context.Background()) }()
	<-g.entered
	if err := f.svc.Stop(); err != nil {
		t.Fatal(err)
	}
	close(g.release)
	if err := <-errc; err != nil {
		t.Fatalf("start = %v", err)
	}
	if got := f.done(); got["stopped"] != true || got["verdict"] != "" {
		t.Fatalf("done = %+v", got)
	}
	if len(f.tester.seconds) != 0 {
		t.Fatalf("helper launched %d times after a stop", len(f.tester.seconds))
	}
	f.svc.mu.Lock()
	busy := f.svc.busy
	f.svc.mu.Unlock()
	if busy {
		t.Fatal("the service stayed busy after the skipped launch")
	}
}

func TestMicTestStopWhileTheHelperLaunchesStopsTheRunOnce(t *testing.T) {
	f := newMicFixture(t, port.PermissionAllowed)
	entered, release := make(chan struct{}), make(chan struct{})
	f.svc.tester = &gatedTester{fakeTester: f.tester, entered: entered, release: release}
	errc := make(chan error, 1)
	go func() { errc <- f.svc.Start(context.Background()) }()
	<-entered
	if err := f.svc.Stop(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errc; err != nil {
		t.Fatalf("start = %v", err)
	}
	if got := f.done(); got["stopped"] != true {
		t.Fatalf("done = %+v", got)
	}
	if f.tester.run.stops() != 1 {
		t.Fatalf("stops = %d, want 1", f.tester.run.stops())
	}
}

// gatedTester blocks Start until release is closed.
type gatedTester struct {
	*fakeTester
	entered chan struct{}
	release chan struct{}
}

func (g *gatedTester) Start(seconds int, on func(port.HelperEvent)) (port.Recording, error) {
	close(g.entered)
	<-g.release
	return g.fakeTester.Start(seconds, on)
}
