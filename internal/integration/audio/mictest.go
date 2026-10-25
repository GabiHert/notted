package audio

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
)

const (
	msgMicNoDevice   = "No microphone input device is available. Connect one, or choose one in System Settings > Sound > Input, then try again."
	msgMicTestFailed = "The microphone test stopped unexpectedly. Try again."
)

var _ port.MicTester = (*MicTester)(nil)

// MicTester runs the helper's `level` command, which listens to the default input and writes no files.
type MicTester struct {
	helperPath  string
	stopTimeout time.Duration
	// grace is how long past its own seconds a helper may run before it is stopped.
	grace time.Duration
}

// NewMicTester builds a tester for the helper at helperPath.
func NewMicTester(helperPath string) *MicTester {
	return &MicTester{helperPath: helperPath, stopTimeout: 3 * time.Second, grace: 10 * time.Second}
}

// Start launches `audiocap level --seconds N` and returns once the process runs.
func (m *MicTester) Start(seconds int, onEvent func(port.HelperEvent)) (port.Recording, error) {
	cmd := exec.Command(m.helperPath, "level", "--seconds", strconv.Itoa(seconds))
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
		fmt.Fprintf(os.Stderr, "audiocap: level launch failed: %v\n", err)
		return nil, errors.New(msgMicTestFailed)
	}
	r := &running{cmd: cmd, stopTimeout: m.stopTimeout, exited: make(chan port.RecorderExit, 1), done: make(chan struct{})}
	var tail errTail
	go tail.collect(stderr)
	// The helper ends itself after its seconds; this only catches one that hangs.
	watchdog := time.AfterFunc(time.Duration(seconds)*time.Second+m.grace, func() { _ = r.Stop() })

	go func() {
		defer watchdog.Stop()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
		for sc.Scan() {
			var w wireEvent
			if err := json.Unmarshal(sc.Bytes(), &w); err != nil || w.Event == "" {
				fmt.Fprintf(os.Stderr, "audiocap level: ignored line: %q\n", sc.Text())
				continue
			}
			onEvent(port.HelperEvent{Kind: w.Event, Code: w.Code, Message: w.Message, Device: w.Device, RMSdB: w.RMSdB, PeakDB: w.PeakDB})
		}
		code := exitCode(cmd.Wait())
		if lines := tail.lines(); len(lines) > 0 && code != 0 {
			fmt.Fprintf(os.Stderr, "audiocap level stderr (last %d lines): %v\n", len(lines), lines)
		}
		r.exited <- port.RecorderExit{Code: code, Message: micTestExitMessage(code)}
		close(r.done)
	}()
	return r, nil
}

func micTestExitMessage(code int) string {
	switch code {
	case 0:
		return ""
	case 2:
		return msgMicDenied
	case 4:
		return msgMicNoDevice
	default:
		return msgMicTestFailed
	}
}

// exitCode is the process exit code; -1 when a signal killed it or Wait failed some other way.
func exitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		return ee.ExitCode()
	}
	return -1
}
