package port

import (
	"context"

	"github.com/GabiHert/phonon/internal/domain"
)

// SpeechSegment is one piece of recognized speech, times in ms from the start of the audio given.
type SpeechSegment struct {
	StartMs int64
	EndMs   int64
	Text    string
}

// SpeechEngine turns audio into timed text; it finds the speech in it itself.
type SpeechEngine interface {
	Ready(lang domain.Language) (bool, string)
	Transcribe(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) ([]SpeechSegment, error)
}

// ModelProgress is one step of a model download.
type ModelProgress struct {
	Language domain.Language
	Name     string // artifact name, "" on the final "done"
	Phase    string // "download" | "verify" | "extract" | "done" | "error"
	Done     int64
	Total    int64
	Err      string
}

// ModelStatus says whether the models for a language are installed.
type ModelStatus struct {
	Language      domain.Language
	Ready         bool
	Missing       string // what is missing, "" when ready
	DownloadBytes int64  // sum of sizes still to download for this language
	Downloading   bool
}

// UnusedModel is a file or folder in the models folder that the current models do not use.
type UnusedModel struct {
	Name  string // its name in the models folder
	Bytes int64
}

// ModelManager reports and installs the speech models, and removes ones no longer used.
type ModelManager interface {
	Status(lang domain.Language) ModelStatus
	Download(ctx context.Context, lang domain.Language, onProgress func(ModelProgress)) error
	Unused() ([]UnusedModel, error)
	RemoveUnused(name string) error
}
