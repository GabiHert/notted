// Package audio supervises the audio helper process.
package audio

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/domain"
)

const (
	msgMicDenied    = "Microphone access is off for Notted. Turn it on in System Settings > Privacy & Security > Microphone, then try again."
	msgScreenDenied = "Screen & System Audio Recording access is off for Notted. Turn it on in System Settings > Privacy & Security > Screen & System Audio Recording, then try again."
	msgNoDevice     = "No microphone or display is available. Connect one and try again."
	msgUnexpected   = "The audio recorder stopped unexpectedly. The audio recorded so far is kept and will be transcribed. Start a new recording to continue."
	msgStartTimeout = "The audio recorder did not start within 2 minutes. Try again."
)

var _ port.AudioRecorder = (*Supervisor)(nil)

// Supervisor runs the helper binary and never restarts it.
type Supervisor struct {
	helperPath   string
	startTimeout time.Duration
	stopTimeout  time.Duration
}

// NewSupervisor builds a supervisor for the helper at helperPath.
func NewSupervisor(helperPath string) *Supervisor {
	return &Supervisor{helperPath: helperPath, startTimeout: 120 * time.Second, stopTimeout: 10 * time.Second}
}

type wireEvent struct {
	Event   string `json:"event"`
	T0Ms    int64  `json:"t0_ms"`
	Track   string `json:"track"`
	Index   int    `json:"index"`
	Path    string `json:"path"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type running struct {
	cmd         *exec.Cmd
	stopTimeout time.Duration
	exited      chan port.RecorderExit
	done        chan struct{}
}

// Start launches the helper and returns once it printed `started`.
func (s *Supervisor) Start(outDir string, onEvent func(port.HelperEvent)) (port.Recording, error) {
	cmd := exec.Command(s.helperPath, "record", "--out", outDir, "--chunk-seconds", "30")
	stdin, err := cmd.StdinPipe() // held open and never written: closing it would stop the helper
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "audiocap: launch failed: %v\n", err)
		return nil, errors.New(msgUnexpected)
	}

	r := &running{cmd: cmd, stopTimeout: s.stopTimeout, exited: make(chan port.RecorderExit, 1), done: make(chan struct{})}
	started := make(chan struct{})
	var startedOnce sync.Once
	var tail errTail
	go tail.collect(stderr)

	go func() {
		defer func() { _ = stdin.Close() }()
		lastErr := "unknown error"
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
		for sc.Scan() {
			var w wireEvent
			if err := json.Unmarshal(sc.Bytes(), &w); err != nil || w.Event == "" {
				fmt.Fprintf(os.Stderr, "audiocap: ignored line: %q\n", sc.Text())
				continue
			}
			if w.Event == "error" && w.Message != "" {
				lastErr = w.Message
			}
			onEvent(port.HelperEvent{
				Kind: w.Event, T0Ms: w.T0Ms, Track: domain.Track(w.Track), Index: w.Index, Path: w.Path,
				StartMs: w.StartMs, EndMs: w.EndMs, Code: w.Code, Message: w.Message,
			})
			if w.Event == "started" {
				startedOnce.Do(func() { close(started) })
			}
		}
		waitErr := cmd.Wait()
		code := 0
		if waitErr != nil {
			var ee *exec.ExitError
			if errors.As(waitErr, &ee) {
				code = ee.ExitCode() // -1 when killed by a signal
			} else {
				code = -1
			}
		}
		if lines := tail.lines(); len(lines) > 0 && code != 0 {
			fmt.Fprintf(os.Stderr, "audiocap stderr (last %d lines): %v\n", len(lines), lines)
		}
		r.exited <- port.RecorderExit{Code: code, Message: exitMessage(code, lastErr)}
		close(r.done)
	}()

	timer := time.NewTimer(s.startTimeout)
	defer timer.Stop()
	select {
	case <-started:
		return r, nil
	case <-r.done:
		x := <-r.exited
		return nil, errors.New(x.Message)
	case <-timer.C:
		_ = r.Stop()
		return nil, errors.New(msgStartTimeout)
	}
}

func exitMessage(code int, lastErr string) string {
	switch code {
	case 0:
		return ""
	case 2:
		return msgMicDenied
	case 3:
		return msgScreenDenied
	case 4:
		return msgNoDevice
	case 1:
		return "The audio recorder stopped: " + lastErr + ". The audio recorded so far is kept."
	default:
		return msgUnexpected
	}
}

// Stop sends SIGINT, then SIGKILL when the helper has not exited in time.
func (r *running) Stop() error {
	select {
	case <-r.done:
		return nil
	default:
	}
	_ = r.cmd.Process.Signal(os.Interrupt)
	select {
	case <-r.done:
	case <-time.After(r.stopTimeout):
		_ = r.cmd.Process.Kill()
		<-r.done
	}
	return nil
}

// Exited receives exactly once, after every event was delivered.
func (r *running) Exited() <-chan port.RecorderExit { return r.exited }

// errTail keeps the last 50 stderr lines for the log.
type errTail struct {
	mu  sync.Mutex
	buf []string
}

func (t *errTail) collect(rd io.Reader) {
	sc := bufio.NewScanner(rd)
	for sc.Scan() {
		t.mu.Lock()
		t.buf = append(t.buf, sc.Text())
		if len(t.buf) > 50 {
			t.buf = t.buf[len(t.buf)-50:]
		}
		t.mu.Unlock()
	}
}

func (t *errTail) lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.buf...)
}
