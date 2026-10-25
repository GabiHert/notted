package port

import (
	"context"

	"github.com/GabiHert/notted/internal/domain"
)

// SpeechSpan is one stretch of detected speech.
type SpeechSpan struct {
	StartSample int       // offset in the input, in samples
	Samples     []float32 // 16 kHz mono, [-1, 1]
}

// SpeechDetector finds the speech in a chunk of audio.
type SpeechDetector interface {
	Ready() (bool, string)
	Detect(samples []float32, sampleRate int) ([]SpeechSpan, error)
}

// SpeechEngine turns speech samples into text.
type SpeechEngine interface {
	Ready(lang domain.Language) (bool, string)
	Transcribe(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error)
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

// ModelManager reports and installs the speech models.
type ModelManager interface {
	Status(lang domain.Language) ModelStatus
	Download(ctx context.Context, lang domain.Language, onProgress func(ModelProgress)) error
}
