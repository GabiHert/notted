package speech

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

// fakeModels builds a manifest of tiny archives named like the real ones, so the
// engine and the detector see them as installed.
func fakeModels(t *testing.T) (modelsDir string, mgr *ModelManager) {
	t.Helper()
	src := t.TempDir()
	en := buildTarGz(t, src, map[string]string{
		"sherpa-onnx-whisper-small.en/small.en-encoder.int8.onnx": "e",
		"sherpa-onnx-whisper-small.en/small.en-decoder.int8.onnx": "d",
		"sherpa-onnx-whisper-small.en/small.en-tokens.txt":        "t",
	})
	vad := filepath.Join(src, "silero_vad.onnx")
	if err := os.WriteFile(vad, []byte("vad"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := DownloadManifest{Schema: 2, Models: []ModelArtifact{
		{Name: "whisper small.en", URL: en, SHA256: fileSum(t, en), Size: 1000, Dir: "sherpa-onnx-whisper-small.en", Lang: "en"},
		{Name: "silero vad", URL: vad, SHA256: fileSum(t, vad), Size: 24, Dir: "silero_vad", File: "silero_vad.onnx"},
	}}
	body, _ := json.Marshal(m)
	mp := filepath.Join(src, "manifest.json")
	if err := os.WriteFile(mp, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOTTED_MODEL_MANIFEST", mp)
	modelsDir = t.TempDir()
	return modelsDir, NewModelManager(modelsDir, New(Config{ModelsDir: modelsDir}), NewDetector(modelsDir))
}

func phases(ps []port.ModelProgress) []string {
	var out []string
	for _, p := range ps {
		if len(out) == 0 || out[len(out)-1] != p.Phase {
			out = append(out, p.Phase)
		}
	}
	return out
}

func TestStatusBeforeAndAfterDownload(t *testing.T) {
	_, mgr := fakeModels(t)
	st := mgr.Status(domain.LangEN)
	if st.Ready || st.DownloadBytes != 1024 || st.Missing == "" {
		t.Fatalf("before: %+v", st)
	}
	var got []port.ModelProgress
	if err := mgr.Download(t.Context(), domain.LangEN, func(p port.ModelProgress) { got = append(got, p) }); err != nil {
		t.Fatal(err)
	}
	st = mgr.Status(domain.LangEN)
	if !st.Ready || st.DownloadBytes != 0 || st.Missing != "" || st.Downloading {
		t.Fatalf("after: %+v", st)
	}
	want := "download verify extract download verify extract done"
	if g := strings.Join(phases(got), " "); g != want {
		t.Errorf("phases: %q, want %q", g, want)
	}
	last := got[len(got)-1]
	if last.Phase != "done" || last.Name != "" || last.Language != domain.LangEN {
		t.Errorf("last: %+v", last)
	}
}

func TestSecondDownloadSkipsInstalled(t *testing.T) {
	_, mgr := fakeModels(t)
	if err := mgr.Download(t.Context(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	var got []port.ModelProgress
	if err := mgr.Download(t.Context(), domain.LangEN, func(p port.ModelProgress) { got = append(got, p) }); err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if p.Phase == "download" {
			t.Errorf("re-downloaded: %+v", p)
		}
	}
}

func TestConcurrentDownloadIsRefused(t *testing.T) {
	_, mgr := fakeModels(t)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	var once atomic.Bool
	go func() {
		done <- mgr.Download(t.Context(), domain.LangEN, func(p port.ModelProgress) {
			if p.Phase == "download" && once.CompareAndSwap(false, true) {
				close(started)
				<-release
			}
		})
	}()
	<-started
	if !mgr.Status(domain.LangEN).Downloading {
		t.Error("Downloading not set while running")
	}
	err := mgr.Download(t.Context(), domain.LangEN, func(port.ModelProgress) {})
	if err == nil || err.Error() != "A model download is already running." {
		t.Errorf("second call: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestOnReadyCalledOnce(t *testing.T) {
	_, mgr := fakeModels(t)
	var n int
	var lang domain.Language
	mgr.SetOnReady(func(l domain.Language) { n++; lang = l })
	if err := mgr.Download(t.Context(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	if n != 1 || lang != domain.LangEN {
		t.Errorf("onReady calls=%d lang=%q", n, lang)
	}
}

func TestDownloadErrorEmitsErrorPhase(t *testing.T) {
	_, mgr := fakeModels(t)
	m, _ := loadManifest()
	m.Models[0].SHA256 = strings.Repeat("0", 64)
	body, _ := json.Marshal(m)
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOTTED_MODEL_MANIFEST", bad)
	var got []port.ModelProgress
	err := mgr.Download(t.Context(), domain.LangEN, func(p port.ModelProgress) { got = append(got, p) })
	if err == nil {
		t.Fatal("want error")
	}
	last := got[len(got)-1]
	if last.Phase != "error" || last.Err == "" {
		t.Errorf("last: %+v", last)
	}
	if mgr.Status(domain.LangEN).Downloading {
		t.Error("still Downloading after failure")
	}
}

func TestStatusSurfacesManifestError(t *testing.T) {
	_, mgr := fakeModels(t)
	t.Setenv("NOTTED_MODEL_MANIFEST", filepath.Join(t.TempDir(), "missing.json"))
	st := mgr.Status(domain.LangEN)
	if st.Ready || !strings.Contains(st.Missing, "model manifest") {
		t.Fatalf("manifest error not surfaced: %+v", st)
	}
}
