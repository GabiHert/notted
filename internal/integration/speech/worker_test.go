package speech

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/phonon/internal/domain"
)

// fakeTranscriber echoes what it got. A first sample of -1 makes the worker die, -2 an error reply.
type fakeTranscriber struct{}

func (fakeTranscriber) TranscribeSamples(_ context.Context, s []float32, rate int, lang domain.Language) (string, error) {
	switch s[0] {
	case -1:
		os.Exit(3)
	case -2:
		return "", errors.New("model failed")
	}
	return fmt.Sprintf("%s %d %d %.2f %.2f", lang, rate, len(s), s[0], s[len(s)-1]), nil
}

// TestHelperTranscribeWorker is the worker process the tests start; it does nothing in a normal run.
func TestHelperTranscribeWorker(t *testing.T) {
	if os.Getenv("PHONON_TEST_WORKER") != "1" {
		return
	}
	// The fake reports a footprint of 1000 bytes per call served, so a test can cross a limit.
	calls := uint64(0)
	mem := func() uint64 { calls++; return calls * 1000 }
	if err := serveWorker(os.Stdin, os.Stdout, fakeTranscriber{}, mem); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

type filesReady struct{}

func (filesReady) ReadyFor(domain.Language) (bool, string) { return true, "" }

func newTestProcessEngine(idle time.Duration) (*ProcessEngine, *int) {
	starts := 0
	p := &ProcessEngine{
		Files:      filesReady{},
		IdleUnload: idle,
		Command: func() *exec.Cmd {
			starts++
			cmd := exec.Command(os.Args[0], "-test.run=^TestHelperTranscribeWorker$")
			cmd.Env = append(os.Environ(), "PHONON_TEST_WORKER=1")
			return cmd
		},
	}
	return p, &starts
}

func ramp(n int) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(i) / float32(n)
	}
	return s
}

func TestProcessEngineSendsSamplesToAWorkerStartedOnFirstUse(t *testing.T) {
	p, starts := newTestProcessEngine(time.Minute)
	defer p.Unload()
	if p.Running() {
		t.Fatal("a worker runs before the first call")
	}
	// 20 s at 16 kHz, the longest span the detector cuts.
	got, err := p.Transcribe(context.Background(), ramp(320000), 16000, domain.LangPTBR)
	if err != nil {
		t.Fatal(err)
	}
	if want := "pt-BR 16000 320000 0.00 1.00"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := p.Transcribe(context.Background(), ramp(10), 16000, domain.LangEN); err != nil {
		t.Fatal(err)
	}
	if *starts != 1 || !p.Running() {
		t.Fatalf("starts = %d running = %v, want one worker kept between calls", *starts, p.Running())
	}
}

func TestProcessEngineUnloadStopsTheWorkerAndTheNextCallStartsOne(t *testing.T) {
	p, starts := newTestProcessEngine(time.Minute)
	defer p.Unload()
	if _, err := p.Transcribe(context.Background(), ramp(10), 16000, domain.LangEN); err != nil {
		t.Fatal(err)
	}
	pid := p.cmd.Process.Pid
	p.Unload()
	if p.Running() {
		t.Fatal("still running after Unload")
	}
	if err := exec.Command("kill", "-0", fmt.Sprint(pid)).Run(); err == nil {
		t.Fatalf("worker %d is still alive after Unload", pid)
	}
	if _, err := p.Transcribe(context.Background(), ramp(10), 16000, domain.LangEN); err != nil {
		t.Fatal(err)
	}
	if *starts != 2 {
		t.Fatalf("starts = %d, want 2", *starts)
	}
}

func TestProcessEngineStopsAnIdleWorker(t *testing.T) {
	p, _ := newTestProcessEngine(100 * time.Millisecond)
	defer p.Unload()
	if _, err := p.Transcribe(context.Background(), ramp(10), 16000, domain.LangEN); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for p.Running() {
		if time.Now().After(deadline) {
			t.Fatal("the idle worker was not stopped")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestProcessEngineReportsAModelErrorAndKeepsTheWorker(t *testing.T) {
	p, starts := newTestProcessEngine(time.Minute)
	defer p.Unload()
	if _, err := p.Transcribe(context.Background(), []float32{-2, 0}, 16000, domain.LangEN); err == nil || err.Error() != "model failed" {
		t.Fatalf("err = %v, want model failed", err)
	}
	if _, err := p.Transcribe(context.Background(), ramp(10), 16000, domain.LangEN); err != nil {
		t.Fatal(err)
	}
	if *starts != 1 {
		t.Fatalf("starts = %d, want the same worker", *starts)
	}
}

func TestProcessEngineRecoversFromAWorkerThatDies(t *testing.T) {
	p, starts := newTestProcessEngine(time.Minute)
	defer p.Unload()
	_, err := p.Transcribe(context.Background(), []float32{-1, 0}, 16000, domain.LangEN)
	if err == nil || !strings.Contains(err.Error(), "transcribe worker stopped") {
		t.Fatalf("err = %v, want the worker stopped", err)
	}
	got, err := p.Transcribe(context.Background(), []float32{0.5}, 16000, domain.LangEN)
	if err != nil || got != "en 16000 1 0.50 0.50" {
		t.Fatalf("after a crash: %q, %v", got, err)
	}
	if *starts != 2 {
		t.Fatalf("starts = %d, want a new worker", *starts)
	}
}

func TestProcessEngineRejectsEmptyAudioAndACancelledContext(t *testing.T) {
	p, starts := newTestProcessEngine(time.Minute)
	defer p.Unload()
	if _, err := p.Transcribe(context.Background(), nil, 16000, domain.LangEN); err == nil {
		t.Fatal("empty audio was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Transcribe(ctx, ramp(10), 16000, domain.LangEN); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	if *starts != 0 {
		t.Fatalf("starts = %d, want none", *starts)
	}
}

func TestProcessEngineReplacesAWorkerThatGrewPastTheLimit(t *testing.T) {
	p, starts := newTestProcessEngine(time.Minute)
	p.MaxFootprint = 2500 // the fake grows 1000 bytes per call
	defer p.Unload()
	for i := 0; i < 3; i++ {
		if _, err := p.Transcribe(context.Background(), ramp(10), 16000, domain.LangEN); err != nil {
			t.Fatal(err)
		}
	}
	if p.Running() {
		t.Fatal("the worker past its limit is still running")
	}
	if _, err := p.Transcribe(context.Background(), ramp(10), 16000, domain.LangEN); err != nil {
		t.Fatal(err)
	}
	if *starts != 2 {
		t.Fatalf("starts = %d, want a fresh worker after the third call", *starts)
	}
}

func TestFootprintIsThisProcesssMemory(t *testing.T) {
	if got := footprint(); got < 1<<20 {
		t.Fatalf("footprint = %d bytes, want at least 1 MB", got)
	}
}
