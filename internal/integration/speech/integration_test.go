//go:build integration

package speech

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
	"github.com/GabiHert/phonon/internal/integration/audio/wavinfo"
)

// Run with: go test -tags integration ./internal/integration/speech -run Integration -v
// PHONON_TEST_MODELS is the models folder (default ~/.cache/phonon-test-models); the models
// are downloaded there when missing. PHONON_TEST_WAVS adds files to transcribe through the
// worker, as lang=path pairs separated by commas (en=/x/a.wav,pt-BR=/y/b.wav). PHONON_TEST_WORKER_BIN
// runs that worker from a built Phonon binary (Phonon transcribe-worker) instead of this test.

func modelsCache(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("PHONON_TEST_MODELS"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".cache", "phonon-test-models")
}

func readWAV(t *testing.T, p string) ([]float32, int) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	samples, rate, err := wavinfo.DecodeWAV(b)
	if err != nil {
		t.Fatal(err)
	}
	return samples, rate
}

func joined(segs []port.SpeechSegment) string {
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = s.Text
	}
	return strings.Join(parts, " ")
}

// TestHelperRealWorker is the real transcribe worker the integration test starts.
func TestHelperRealWorker(t *testing.T) {
	dir := os.Getenv("PHONON_TEST_REAL_WORKER")
	if dir == "" {
		return
	}
	e := New(Config{ModelsDir: dir})
	defer e.Unload()
	if err := ServeWorker(os.Stdin, os.Stdout, e); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestIntegration(t *testing.T) {
	t.Setenv("PHONON_MODEL_MANIFEST", "")
	dir := modelsCache(t)
	mgr := NewModelManager(dir)
	if st := mgr.Status(domain.LangEN); !st.Ready {
		t.Logf("downloading %d bytes into %s", st.DownloadBytes, dir)
		if err := mgr.Download(t.Context(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
			t.Fatal(err)
		}
	}
	e := New(Config{ModelsDir: dir})
	defer e.Unload()

	t.Run("silence has no speech", func(t *testing.T) {
		segs, err := e.TranscribeSamples(t.Context(), make([]float32, 5*16000), 16000, domain.LangEN)
		if err != nil || len(segs) != 0 {
			t.Fatalf("segments=%+v err=%v", segs, err)
		}
	})

	for _, c := range []struct {
		file string
		lang domain.Language
		want []string
	}{
		{"en-sample.wav", domain.LangEN, []string{"report", "friday"}},
		{"pt-sample.wav", domain.LangPTBR, []string{"sexta"}},
	} {
		t.Run(c.file, func(t *testing.T) {
			samples, rate := readWAV(t, filepath.Join("testdata", c.file))
			segs, err := e.TranscribeSamples(t.Context(), samples, rate, c.lang)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ToLower(joined(segs))
			t.Logf("transcript: %q", text)
			for _, w := range c.want {
				if !strings.Contains(text, w) {
					t.Fatalf("transcript %q misses %q", text, w)
				}
			}
			for _, s := range segs {
				if s.StartMs < 0 || s.EndMs < s.StartMs || s.EndMs > int64(len(samples))*1000/int64(rate)+1000 {
					t.Errorf("segment times out of the audio: %+v", s)
				}
			}
		})
	}

	t.Run("worker", func(t *testing.T) {
		p := &ProcessEngine{Files: Files{ModelsDir: dir}, Command: func() *exec.Cmd {
			if bin := os.Getenv("PHONON_TEST_WORKER_BIN"); bin != "" {
				return exec.Command(bin, "transcribe-worker", "--models", dir)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestHelperRealWorker$")
			cmd.Env = append(os.Environ(), "PHONON_TEST_REAL_WORKER="+dir)
			return cmd
		}}
		defer p.Unload()
		files := []string{"en=" + filepath.Join("testdata", "en-sample.wav")}
		if v := os.Getenv("PHONON_TEST_WAVS"); v != "" {
			files = append(files, strings.Split(v, ",")...)
		}
		for _, f := range files {
			lang, path, _ := strings.Cut(f, "=")
			samples, rate := readWAV(t, path)
			begin := time.Now()
			segs, err := p.Transcribe(t.Context(), samples, rate, domain.Language(lang))
			if err != nil {
				t.Fatal(err)
			}
			took := time.Since(begin)
			audio := time.Duration(len(samples)) * time.Second / time.Duration(rate)
			t.Logf("%s (%s, %s audio): %s, RTF %.3f, worker footprint %.2f GB", filepath.Base(path), lang, audio.Round(time.Millisecond),
				took.Round(time.Millisecond), took.Seconds()/audio.Seconds(), float64(p.Footprint())/(1<<30))
			for _, s := range segs {
				t.Logf("  [%6d-%6d] %s", s.StartMs, s.EndMs, s.Text)
			}
		}
	})
}
