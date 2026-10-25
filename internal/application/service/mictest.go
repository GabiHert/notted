package service

import (
	"context"
	"errors"
	"sync"

	"github.com/GabiHert/notted/internal/application/port"
)

const (
	micTestSeconds       = 10
	digitalSilenceDB     = -120.0 // the helper's level for a peak of exactly 0
	quietPeakBelowDB     = -40.0
	msgMicTestRecording  = "Stop the recording to test the microphone."
	msgMicTestRunning    = "The microphone test is already running."
	msgMicTestNotAllowed = "Microphone access is off for Notted. Allow it in the Permissions card (Ask, or Open System Settings), then test again."
	msgMicWorks          = "The microphone works."
	// msgLidClosed ends both silent-microphone messages: the mic test's and the recording warning's.
	msgLidClosed = "If your MacBook's lid is closed, its built-in microphone is off: open the lid, or choose another input in System Settings > Sound > Input."
)

// The verdicts a finished test reports.
const (
	verdictSilent = "silent"
	verdictQuiet  = "quiet"
	verdictOK     = "ok"
)

// MicTestService listens to the default input for a few seconds and says whether it works.
type MicTestService struct {
	tester    port.MicTester
	perms     port.PermissionChecker
	recording func() bool
	emit      port.EventEmitter
	mu        sync.Mutex
	busy      bool
	stopping  bool
	run       port.Recording
}

// NewMicTestService builds the service; recording reports whether a recording runs.
func NewMicTestService(t port.MicTester, p port.PermissionChecker, recording func() bool, e port.EventEmitter) *MicTestService {
	return &MicTestService{tester: t, perms: p, recording: recording, emit: e}
}

// Start begins a test. It returns once the helper runs; the levels and the result arrive as events.
func (s *MicTestService) Start(ctx context.Context) error {
	if s.recording() {
		return errors.New(msgMicTestRecording)
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return errors.New(msgMicTestRunning)
	}
	s.busy = true
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		s.busy, s.stopping = false, false
		s.mu.Unlock()
	}

	set, err := s.perms.Status(ctx)
	if err != nil {
		release()
		return err
	}
	if set.Microphone != port.PermissionAllowed {
		release()
		return errors.New(msgMicTestNotAllowed)
	}
	// A Stop that came while the permission check ran: the test ends without launching the helper.
	s.mu.Lock()
	stopped := s.stopping
	s.mu.Unlock()
	if stopped {
		release()
		s.emit.Emit("mictest:done", map[string]any{"device": "", "verdict": "", "message": "", "stopped": true})
		return nil
	}
	var heard micHeard
	run, err := s.tester.Start(micTestSeconds, func(ev port.HelperEvent) { s.handle(&heard, ev) })
	if err != nil {
		release()
		return err
	}
	s.mu.Lock()
	s.run = run
	stopNow := s.stopping // a Stop that came while the helper launched
	s.mu.Unlock()
	go s.watch(run, &heard)
	if stopNow {
		return run.Stop()
	}
	return nil
}

// Stop ends the test early; with none running it does nothing. A test that is still starting
// (checking the permission or launching the helper) notices the request and ends as stopped.
func (s *MicTestService) Stop() error {
	s.mu.Lock()
	run := s.run
	if s.busy {
		s.stopping = true
	}
	s.mu.Unlock()
	if run == nil {
		return nil
	}
	return run.Stop()
}

// micHeard is what one test has heard so far; the helper's events arrive on one goroutine.
type micHeard struct {
	device string
	peak   float64
	levels int
}

func (s *MicTestService) handle(h *micHeard, ev port.HelperEvent) {
	switch ev.Kind {
	case "device":
		h.device = ev.Device
	case "level":
		if h.levels == 0 || ev.PeakDB > h.peak {
			h.peak = ev.PeakDB
		}
		h.levels++
		s.emit.Emit("mictest:level", map[string]any{"device": h.device, "rmsDb": ev.RMSdB, "peakDb": ev.PeakDB})
	}
}

// watch reports how the test ended. Every event was delivered before the exit.
func (s *MicTestService) watch(run port.Recording, h *micHeard) {
	x := <-run.Exited()
	s.mu.Lock()
	stopped := s.stopping
	s.run, s.stopping, s.busy = nil, false, false
	s.mu.Unlock()
	switch {
	case stopped:
		s.emit.Emit("mictest:done", map[string]any{"device": h.device, "verdict": "", "message": "", "stopped": true})
	case x.Code != 0:
		s.emit.Emit("mictest:error", map[string]any{"message": x.Message})
	default:
		verdict, message := micVerdict(h)
		s.emit.Emit("mictest:done", map[string]any{"device": h.device, "verdict": verdict, "message": message, "stopped": false})
	}
}

func micVerdict(h *micHeard) (verdict, message string) {
	name := h.device
	if name == "" {
		name = "the microphone"
	}
	switch {
	case h.levels == 0 || h.peak <= digitalSilenceDB:
		return verdictSilent, "No sound from " + name + ". " + msgLidClosed
	case h.peak < quietPeakBelowDB:
		return verdictQuiet, "Very quiet. Speak closer to " + name + ", or choose another input in System Settings > Sound > Input."
	}
	return verdictOK, msgMicWorks
}
