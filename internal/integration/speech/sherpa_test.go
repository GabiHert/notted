package speech

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GabiHert/notted/internal/domain"
)

func TestReadyForWithNoFiles(t *testing.T) {
	e := New(Config{ModelsDir: t.TempDir()})
	ok, msg := e.ReadyFor(domain.LangEN)
	if ok || msg != "speech models are not installed" {
		t.Errorf("got %v %q", ok, msg)
	}
}

func TestReadyForListsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "sherpa-onnx-whisper-small.en")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "small.en-tokens.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e := New(Config{ModelsDir: dir})
	ok, msg := e.ReadyFor(domain.LangEN)
	if ok || !strings.HasPrefix(msg, "missing speech model files: ") ||
		!strings.Contains(msg, "small.en-encoder.int8.onnx") || !strings.Contains(msg, "small.en-decoder.int8.onnx") ||
		strings.Contains(msg, "tokens") {
		t.Errorf("got %v %q", ok, msg)
	}
}

func TestLanguageMapping(t *testing.T) {
	if sttDirName(domain.LangPTBR) != "sherpa-onnx-whisper-turbo" || sttFilePrefix(domain.LangPTBR) != "turbo" || whisperLang(domain.LangPTBR) != "pt" {
		t.Error("pt-BR mapping wrong")
	}
	if sttDirName(domain.LangEN) != "sherpa-onnx-whisper-small.en" || sttFilePrefix(domain.LangEN) != "small.en" || whisperLang(domain.LangEN) != "en" {
		t.Error("en mapping wrong")
	}
	if sttDirName(domain.Language("PT-br")) != "sherpa-onnx-whisper-turbo" || sttFilePrefix(domain.Language("PT-br")) != "turbo" {
		t.Error("pt-BR must match in any case")
	}
}

func TestReadyForPortugueseIgnoresLegacySmallFiles(t *testing.T) {
	dir := t.TempDir()
	seedFiles(t, dir, "sherpa-onnx-whisper-small", "small-encoder.int8.onnx", "small-decoder.int8.onnx", "small-tokens.txt")
	ok, msg := New(Config{ModelsDir: dir}).ReadyFor(domain.LangPTBR)
	if ok || msg != "speech models are not installed" {
		t.Errorf("got %v %q", ok, msg)
	}
}

func TestReadyForPortugueseNamesMissingTurboFiles(t *testing.T) {
	dir := t.TempDir()
	seedFiles(t, dir, "sherpa-onnx-whisper-turbo", "turbo-tokens.txt")
	ok, msg := New(Config{ModelsDir: dir}).ReadyFor(domain.LangPTBR)
	if ok || !strings.HasPrefix(msg, "missing speech model files: ") ||
		!strings.Contains(msg, "turbo-encoder.int8.onnx") || !strings.Contains(msg, "turbo-decoder.int8.onnx") ||
		strings.Contains(msg, "tokens") {
		t.Errorf("got %v %q", ok, msg)
	}
}

func TestReadyForPortugueseWithTurboFiles(t *testing.T) {
	dir := t.TempDir()
	seedFiles(t, dir, "sherpa-onnx-whisper-turbo", "turbo-encoder.int8.onnx", "turbo-decoder.int8.onnx", "turbo-tokens.txt")
	ok, msg := New(Config{ModelsDir: dir}).ReadyFor(domain.LangPTBR)
	if !ok || msg != "" {
		t.Errorf("got %v %q", ok, msg)
	}
}

func TestDetectorReadyNeedsModelFile(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(dir)
	if ok, _ := d.Ready(); ok {
		t.Fatal("ready without the model file")
	}
	if err := os.MkdirAll(filepath.Join(dir, "silero_vad"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "silero_vad", "silero_vad.onnx"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, msg := d.Ready(); !ok {
		t.Errorf("not ready with the file: %q", msg)
	}
}
