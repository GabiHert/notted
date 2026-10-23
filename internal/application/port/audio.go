package port

import "github.com/bhlabz/notted/internal/domain"

// HelperEvent is one parsed line of the audio helper's stdout.
type HelperEvent struct {
	Kind    string // "started" | "chunk" | "error" | "stopped" | "permission"
	T0Ms    int64
	Track   domain.Track
	Index   int
	Path    string
	StartMs int64
	EndMs   int64
	Code    string
	Message string
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

// WavInfo is what the recording service needs to know about a closed chunk.
type WavInfo struct {
	Silent bool // the level is below the silence threshold
}

// AudioInspector reads a closed WAV chunk.
type AudioInspector interface {
	Inspect(path string) (WavInfo, error)
}
