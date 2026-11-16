// Package speech runs the offline speech engine: whisper.cpp large-v3-turbo (q5_0, on Metal)
// with whisper.cpp's built-in Silero VAD, plus the model manifest and downloader. The engine runs
// inside the transcribe worker process (see worker.go), so its memory goes back when that exits.
package speech

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
	"github.com/GabiHert/phonon/internal/integration/speech/whispercpp"
)

// Model files under the models folder; the manifest installs exactly these.
const (
	WhisperDirName  = "whisper-large-v3-turbo-q5_0"
	WhisperFileName = "ggml-large-v3-turbo-q5_0.bin"
	VADDirName      = "silero-vad-v5.1.2"
	VADFileName     = "ggml-silero-v5.1.2.bin"
)

const (
	defaultNumThreads = 4
	whisperSampleRate = whispercpp.SampleRate
)

// isPTBR reports whether lang is Brazilian Portuguese, in any case.
func isPTBR(lang domain.Language) bool {
	return strings.EqualFold(strings.TrimSpace(string(lang)), string(domain.LangPTBR))
}

// whisperLang is the Whisper language code forced for a meeting language.
func whisperLang(lang domain.Language) string {
	if isPTBR(lang) {
		return "pt"
	}
	return "en"
}

// Files reports whether the model files are on disk, without loading anything.
type Files struct {
	ModelsDir string
}

// required lists the files every language needs, relative to the models folder.
func required() []string {
	return []string{filepath.Join(WhisperDirName, WhisperFileName), filepath.Join(VADDirName, VADFileName)}
}

// ReadyFor reports whether the files to serve lang exist. Every language uses the same model.
func (f Files) ReadyFor(domain.Language) (bool, string) {
	var missing []string
	all := required()
	for _, rel := range all {
		if st, err := os.Stat(filepath.Join(f.ModelsDir, rel)); err != nil || !st.Mode().IsRegular() {
			missing = append(missing, rel)
		}
	}
	if len(missing) == len(all) {
		return false, "speech models are not installed"
	}
	if len(missing) > 0 {
		return false, "missing speech model files: " + strings.Join(missing, ", ")
	}
	return true, ""
}

// Config configures the engine.
type Config struct {
	// ModelsDir is the folder holding WhisperDirName and VADDirName.
	ModelsDir string
	// NumThreads is the CPU thread count (default 4); the model itself runs on Metal.
	NumThreads int
	// CPU runs the model without Metal (tests on machines without a GPU).
	CPU bool
}

// Engine is whisper.cpp loaded in this process. It loads on first use and keeps the model until
// Unload; the worker process exiting is what normally frees it.
type Engine struct {
	cfg   Config
	mu    sync.Mutex
	model *whispercpp.Model
}

// New creates an engine rooted at cfg.ModelsDir. Nothing loads until the first call.
func New(cfg Config) *Engine {
	if cfg.NumThreads <= 0 {
		cfg.NumThreads = defaultNumThreads
	}
	return &Engine{cfg: cfg}
}

// ReadyFor reports whether the model files for lang are on disk.
func (e *Engine) ReadyFor(lang domain.Language) (bool, string) {
	return Files{ModelsDir: e.cfg.ModelsDir}.ReadyFor(lang)
}

// TranscribeSamples transcribes 16 kHz mono samples in [-1, 1], with the VAD skipping silence,
// and returns the timed segments, times in ms from the start of samples.
func (e *Engine) TranscribeSamples(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) ([]port.SpeechSegment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, errors.New("empty audio")
	}
	if sampleRate != whisperSampleRate {
		return nil, fmt.Errorf("the speech engine needs %d Hz audio, got %d Hz", whisperSampleRate, sampleRate)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.model == nil {
		if ok, detail := e.ReadyFor(lang); !ok {
			return nil, fmt.Errorf("speech engine not ready: %s", detail)
		}
		m, err := whispercpp.Load(filepath.Join(e.cfg.ModelsDir, WhisperDirName, WhisperFileName), !e.cfg.CPU)
		if err != nil {
			return nil, err
		}
		e.model = m
	}
	segs, err := e.model.Transcribe(samples, whispercpp.Params{
		Language: whisperLang(lang),
		VADModel: filepath.Join(e.cfg.ModelsDir, VADDirName, VADFileName),
		Threads:  e.cfg.NumThreads,
	})
	if err != nil {
		return nil, err
	}
	out := make([]port.SpeechSegment, 0, len(segs))
	for _, s := range segs {
		text := strings.Join(strings.Fields(s.Text), " ")
		if text == "" {
			continue
		}
		out = append(out, port.SpeechSegment{StartMs: s.StartMs, EndMs: s.EndMs, Text: text})
	}
	return out, nil
}

// Unload frees the model. Idempotent.
func (e *Engine) Unload() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.model != nil {
		e.model.Close()
		e.model = nil
	}
}
