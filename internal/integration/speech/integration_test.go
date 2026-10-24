//go:build integration

package speech

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/domain"
	"github.com/bhlabz/notted/internal/integration/audio/wavinfo"
)

func modelsCache(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("NOTTED_TEST_MODELS"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".cache", "notted-test-models")
}

func transcribeFixture(t *testing.T, e *Engine, d *Detector, name string, lang domain.Language) string {
	t.Helper()
	wav, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	samples, rate, err := wavinfo.DecodeWAV(wav)
	if err != nil {
		t.Fatal(err)
	}
	spans, err := d.Detect(samples, rate)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) < 1 {
		t.Fatalf("%s: no speech detected", name)
	}
	var parts []string
	for _, s := range spans {
		text, err := e.TranscribeSamples(t.Context(), s.Samples, rate, lang)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, text)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func TestIntegration(t *testing.T) {
	t.Setenv("NOTTED_MODEL_MANIFEST", "")
	dir := modelsCache(t)
	e := New(Config{ModelsDir: dir})
	defer e.Unload()
	d := NewDetector(dir)
	mgr := NewModelManager(dir, e, d)

	start := time.Now()
	t.Logf("models cache: %s; to download: %d bytes", dir, mgr.Status(domain.LangEN).DownloadBytes)
	var last string
	err := mgr.Download(t.Context(), domain.LangEN, func(p port.ModelProgress) {
		if p.Phase != "download" && p.Phase != last {
			t.Logf("phase %s %s", p.Phase, p.Name)
		}
		last = p.Phase
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("download step took %s", time.Since(start))
	if st := mgr.Status(domain.LangEN); !st.Ready {
		t.Fatalf("not ready after download: %+v", st)
	}

	t.Run("silence has no speech", func(t *testing.T) {
		spans, err := d.Detect(make([]float32, 2*16000), 16000)
		if err != nil || len(spans) != 0 {
			t.Fatalf("spans=%d err=%v", len(spans), err)
		}
	})

	t.Run("english sample", func(t *testing.T) {
		text := transcribeFixture(t, e, d, "en-sample.wav", domain.LangEN)
		t.Logf("transcript: %q", text)
		if !strings.Contains(text, "report") || !strings.Contains(text, "friday") {
			t.Fatalf("transcript %q misses report/friday", text)
		}
	})

	t.Run("portuguese sample", func(t *testing.T) {
		if ok, _ := e.ReadyFor(domain.LangPTBR); !ok {
			t.Skip("multilingual model not in the cache; not downloading 640 MB here")
		}
		text := transcribeFixture(t, e, d, "pt-sample.wav", domain.LangPTBR)
		t.Logf("transcript: %q", text)
		if !strings.Contains(text, "sexta") {
			t.Fatalf("transcript %q misses sexta", text)
		}
	})
}
