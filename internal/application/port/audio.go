package port

import (
	"context"

	"github.com/GabiHert/notted/internal/domain"
)

// HelperEvent is one parsed line of the audio helper's stdout.
type HelperEvent struct {
	Kind    string // "started" | "chunk" | "error" | "stopped" | "permission" | "device" | "level"
	T0Ms    int64
	Track   domain.Track
	Index   int
	Path    string
	StartMs int64
	EndMs   int64
	Code    string
	Message string
	Device  string  // the input device's name; on "started" and "device"
	RMSdB   float64 // "level": the last 100 ms, in dBFS; -120 for digital silence
	PeakDB  float64 // "level": the loudest sample of the last 100 ms, in dBFS; -120 for digital silence
}

// RecorderExit is how the helper process ended.
type RecorderExit struct {
	Code    int    // process exit code; -1 when killed by a signal or it crashed
	Message string // the user-facing message; "" for a clean stop
}

// Recording is one running helper.
type Recording interface {
	Stop() error                 // SIGINT, wait for exit up to 10 s, then SIGKILL
	Exited() <-chan RecorderExit // receives exactly once
}

// AudioRecorder launches the audio helper.
type AudioRecorder interface {
	// Start launches the helper and returns once it printed `started`
	// (or fails with the mapped message if it exits first, or after 120 s).
	Start(outDir string, onEvent func(HelperEvent)) (Recording, error)
}

// MicTester runs the helper's input level meter, which records nothing.
type MicTester interface {
	// Start launches `level` for the given number of seconds and returns once the process runs.
	// Events arrive on onEvent; the run's Exited channel reports how the process ended.
	Start(seconds int, onEvent func(HelperEvent)) (Recording, error)
}

// WavInfo is what the recording service needs to know about a closed chunk.
type WavInfo struct {
	Silent         bool // the level is below the silence threshold
	DigitalSilence bool // every sample is exactly 0
}

// AudioInspector reads a closed WAV chunk.
type AudioInspector interface {
	Inspect(path string) (WavInfo, error)
}

// PermissionState is one permission's state as the helper reports it.
type PermissionState string

const (
	PermissionAllowed  PermissionState = "allowed"
	PermissionNotAsked PermissionState = "not_asked"
	PermissionDenied   PermissionState = "denied"
)

// PermissionSet is the state of the two permissions recording needs.
type PermissionSet struct {
	Microphone PermissionState
	Screen     PermissionState // never PermissionNotAsked in practice: macOS cannot tell the two apart
}

// PermissionChecker reads and requests the recording permissions through the audio helper.
type PermissionChecker interface {
	// Status reads both permissions and never prompts.
	Status(ctx context.Context) (PermissionSet, error)
	// Ask lets macOS prompt for what is still open, then reads both permissions.
	Ask(ctx context.Context) (PermissionSet, error)
}

// URLOpener opens a URL with the system's default handler.
type URLOpener interface {
	Open(url string) error
}
