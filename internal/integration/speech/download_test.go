package speech

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// buildTarGz writes a .tar.gz holding the given name->content entries and returns its path.
func buildTarGz(t *testing.T, dir string, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(dir, "fake.tar.gz")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func fileSum(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sum(b)
}

func noProgress(string, int64, int64) {}

func TestDownloadExtractsArchiveWithRightChecksum(t *testing.T) {
	src, models := t.TempDir(), t.TempDir()
	p := buildTarGz(t, src, map[string]string{"fake-dir/a.onnx": "weights"})
	a := ModelArtifact{Name: "fake", URL: p, SHA256: fileSum(t, p), Dir: "fake-dir"}
	if err := downloadAndExtract(context.Background(), models, a, noProgress); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(models, "fake-dir", "a.onnx"))
	if err != nil || string(got) != "weights" {
		t.Fatalf("not installed: %v %q", err, got)
	}
}

func TestDownloadWrongChecksumInstallsNothing(t *testing.T) {
	src, models := t.TempDir(), t.TempDir()
	p := buildTarGz(t, src, map[string]string{"fake-dir/a.onnx": "weights"})
	a := ModelArtifact{Name: "fake model", URL: p, SHA256: strings.Repeat("0", 64), Dir: "fake-dir"}
	err := downloadAndExtract(context.Background(), models, a, noProgress)
	if err == nil || !strings.Contains(err.Error(), "fake model") {
		t.Fatalf("want an error naming the artifact, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(models, "fake-dir")); !os.IsNotExist(statErr) {
		t.Error("model folder installed despite bad checksum")
	}
	left, _ := os.ReadDir(filepath.Join(models, ".downloads"))
	if len(left) != 0 {
		t.Errorf("temp download left behind: %v", left)
	}
}

func TestDownloadRawFileGoesToDirSlashFile(t *testing.T) {
	src, models := t.TempDir(), t.TempDir()
	p := filepath.Join(src, "silero_vad.onnx")
	if err := os.WriteFile(p, []byte("vad"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := ModelArtifact{Name: "silero vad", URL: p, SHA256: fileSum(t, p), Dir: "silero_vad", File: "silero_vad.onnx"}
	if err := downloadAndExtract(context.Background(), models, a, noProgress); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(models, "silero_vad", "silero_vad.onnx"))
	if err != nil || string(got) != "vad" {
		t.Fatalf("raw file not installed: %v %q", err, got)
	}
}

func TestDownloadRefusesPathTraversal(t *testing.T) {
	src, models := t.TempDir(), t.TempDir()
	p := buildTarGz(t, src, map[string]string{"../escape": "bad"})
	a := ModelArtifact{Name: "evil", URL: p, SHA256: fileSum(t, p), Dir: "evil"}
	err := downloadAndExtract(context.Background(), models, a, noProgress)
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("want unsafe path error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(models), "escape")); !os.IsNotExist(statErr) {
		t.Error("file escaped the models folder")
	}
}

func TestDownloadCancelStopsPromptlyAndCleansUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000000")
		for i := 0; i < 1000; i++ {
			if _, err := w.Write(make([]byte, 1024)); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}))
	defer srv.Close()
	models := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := ModelArtifact{Name: "slow", URL: srv.URL, SHA256: strings.Repeat("0", 64), Dir: "slow"}
	var cancelled time.Time
	err := downloadAndExtract(ctx, models, a, func(phase string, done, _ int64) {
		if phase == "download" && done > 0 && cancelled.IsZero() {
			cancelled = time.Now()
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if d := time.Since(cancelled); d > time.Second {
		t.Errorf("cancel took %s", d)
	}
	if _, statErr := os.Stat(filepath.Join(models, "slow")); !os.IsNotExist(statErr) {
		t.Error("model folder exists after cancel")
	}
	left, _ := os.ReadDir(filepath.Join(models, ".downloads"))
	if len(left) != 0 {
		t.Errorf("scratch left behind: %v", left)
	}
}
