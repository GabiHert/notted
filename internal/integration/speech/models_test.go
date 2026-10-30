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
	pt := buildTarGz(t, t.TempDir(), map[string]string{ // its own folder: buildTarGz always writes fake.tar.gz
		"sherpa-onnx-whisper-turbo/turbo-encoder.int8.onnx": "e",
		"sherpa-onnx-whisper-turbo/turbo-decoder.int8.onnx": "d",
		"sherpa-onnx-whisper-turbo/turbo-tokens.txt":        "t",
	})
	vad := filepath.Join(src, "silero_vad.onnx")
	if err := os.WriteFile(vad, []byte("vad"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := DownloadManifest{Schema: 2, Models: []ModelArtifact{
		{Name: "whisper small.en", URL: en, SHA256: fileSum(t, en), Size: 1000, Dir: "sherpa-onnx-whisper-small.en", Lang: "en"},
		{Name: "whisper turbo", URL: pt, SHA256: fileSum(t, pt), Size: 2000, Dir: "sherpa-onnx-whisper-turbo", Lang: "pt-BR"},
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

// seedFiles writes empty files under modelsDir/dir.
func seedFiles(t *testing.T, modelsDir, dir string, files ...string) {
	t.Helper()
	d := filepath.Join(modelsDir, dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(d, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// seedLegacySmall creates the old Portuguese model folder and returns its path.
func seedLegacySmall(t *testing.T, modelsDir string) string {
	t.Helper()
	seedFiles(t, modelsDir, "sherpa-onnx-whisper-small", "small-encoder.int8.onnx", "small-decoder.int8.onnx", "small-tokens.txt")
	return filepath.Join(modelsDir, "sherpa-onnx-whisper-small")
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

func TestStatusWithOnlyLegacyModelIsNotReady(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	seedLegacySmall(t, modelsDir)
	seedFiles(t, modelsDir, "silero_vad", "silero_vad.onnx")
	st := mgr.Status(domain.LangPTBR)
	if st.Ready || st.DownloadBytes != 2000 || st.Missing != "speech models are not installed" {
		t.Fatalf("legacy only: %+v", st)
	}
}

func TestPortugueseDownloadInstallsTurboAndCallsOnReady(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	var n int
	var lang domain.Language
	mgr.SetOnReady(func(l domain.Language) { n++; lang = l })
	var got []port.ModelProgress
	if err := mgr.Download(t.Context(), domain.LangPTBR, func(p port.ModelProgress) { got = append(got, p) }); err != nil {
		t.Fatal(err)
	}
	st := mgr.Status(domain.LangPTBR)
	if !st.Ready || st.DownloadBytes != 0 || st.Missing != "" {
		t.Fatalf("after: %+v", st)
	}
	for _, f := range []string{"turbo-encoder.int8.onnx", "turbo-decoder.int8.onnx", "turbo-tokens.txt"} {
		if _, err := os.Stat(filepath.Join(modelsDir, "sherpa-onnx-whisper-turbo", f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if n != 1 || lang != domain.LangPTBR {
		t.Errorf("onReady calls=%d lang=%q", n, lang)
	}
	want := "download verify extract download verify extract done"
	if g := strings.Join(phases(got), " "); g != want {
		t.Errorf("phases: %q, want %q", g, want)
	}
}

func TestEnglishIsUnchangedByThePortugueseArtifact(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	if b := mgr.Status(domain.LangEN).DownloadBytes; b != 1024 {
		t.Fatalf("en bytes: %d", b)
	}
	if err := mgr.Download(t.Context(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	if !mgr.Status(domain.LangEN).Ready {
		t.Error("en not ready")
	}
	if _, err := os.Stat(filepath.Join(modelsDir, "sherpa-onnx-whisper-turbo")); !os.IsNotExist(err) {
		t.Errorf("turbo folder exists after an English download: %v", err)
	}
	if b := mgr.Status(domain.LangPTBR).DownloadBytes; b != 2000 {
		t.Errorf("pt-BR bytes: %d", b)
	}
}

func TestStatusUsesThePinnedTurboSize(t *testing.T) {
	t.Setenv("NOTTED_MODEL_MANIFEST", "")
	dir := t.TempDir()
	mgr := NewModelManager(dir, New(Config{ModelsDir: dir}), NewDetector(dir))
	if b := mgr.Status(domain.LangPTBR).DownloadBytes; b != 564434061 {
		t.Errorf("nothing installed: %d", b)
	}
	seedFiles(t, dir, "silero_vad", "silero_vad.onnx")
	if b := mgr.Status(domain.LangPTBR).DownloadBytes; b != 563790207 {
		t.Errorf("detector installed: %d", b)
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
