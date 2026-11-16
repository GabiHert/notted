package speech

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
)

// fakeModels builds a manifest of tiny raw files named like the real ones, so Files sees them
// as installed once downloaded.
func fakeModels(t *testing.T) (modelsDir string, mgr *ModelManager) {
	t.Helper()
	src := t.TempDir()
	model := filepath.Join(src, WhisperFileName)
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	vad := filepath.Join(src, VADFileName)
	if err := os.WriteFile(vad, []byte("vad"), 0o600); err != nil {
		t.Fatal(err)
	}
	useManifest(t, &DownloadManifest{Schema: 3, Models: []ModelArtifact{
		{Name: "whisper large-v3-turbo", URL: model, SHA256: fileSum(t, model), Size: 2000, Dir: WhisperDirName, File: WhisperFileName},
		{Name: "silero vad", URL: vad, SHA256: fileSum(t, vad), Size: 24, Dir: VADDirName, File: VADFileName},
	}})
	modelsDir = t.TempDir()
	return modelsDir, NewModelManager(modelsDir)
}

// useManifest makes m the active manifest for the rest of the test.
func useManifest(t *testing.T, m *DownloadManifest) {
	t.Helper()
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PHONON_MODEL_MANIFEST", p)
}

// seedFiles writes small files under modelsDir/dir.
func seedFiles(t *testing.T, modelsDir, dir string, files ...string) {
	t.Helper()
	d := filepath.Join(modelsDir, dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(d, f), []byte("0123456789"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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
	for _, lang := range []domain.Language{domain.LangEN, domain.LangPTBR} {
		t.Run(string(lang), func(t *testing.T) {
			_, mgr := fakeModels(t)
			st := mgr.Status(lang)
			if st.Ready || st.DownloadBytes != 2024 || st.Missing != "speech models are not installed" {
				t.Fatalf("before: %+v", st)
			}
			var got []port.ModelProgress
			if err := mgr.Download(t.Context(), lang, func(p port.ModelProgress) { got = append(got, p) }); err != nil {
				t.Fatal(err)
			}
			st = mgr.Status(lang)
			if !st.Ready || st.DownloadBytes != 0 || st.Missing != "" || st.Downloading {
				t.Fatalf("after: %+v", st)
			}
			want := "download verify extract download verify extract done"
			if g := strings.Join(phases(got), " "); g != want {
				t.Errorf("phases: %q, want %q", g, want)
			}
			last := got[len(got)-1]
			if last.Phase != "done" || last.Name != "" || last.Language != lang {
				t.Errorf("last: %+v", last)
			}
		})
	}
}

func TestOneDownloadServesBothLanguages(t *testing.T) {
	_, mgr := fakeModels(t)
	if err := mgr.Download(t.Context(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	if st := mgr.Status(domain.LangPTBR); !st.Ready || st.DownloadBytes != 0 {
		t.Fatalf("pt-BR after an English download: %+v", st)
	}
}

func TestStatusNamesAMissingFile(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	seedFiles(t, modelsDir, WhisperDirName, WhisperFileName)
	st := mgr.Status(domain.LangPTBR)
	if st.Ready || st.DownloadBytes != 24 || !strings.Contains(st.Missing, filepath.Join(VADDirName, VADFileName)) {
		t.Fatalf("vad missing: %+v", st)
	}
}

func TestStatusIgnoresTheOldSherpaModels(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	seedFiles(t, modelsDir, "sherpa-onnx-whisper-turbo", "turbo-encoder.int8.onnx", "turbo-decoder.int8.onnx", "turbo-tokens.txt")
	seedFiles(t, modelsDir, "silero_vad", "silero_vad.onnx")
	if st := mgr.Status(domain.LangPTBR); st.Ready {
		t.Fatalf("ready with only the old models: %+v", st)
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
	if err := mgr.RemoveUnused("sherpa-onnx-whisper-turbo"); !errors.Is(err, ErrDownloadRunning) {
		t.Errorf("remove during a download: %v", err)
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
	if err := mgr.Download(t.Context(), domain.LangPTBR, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	if n != 1 || lang != domain.LangPTBR {
		t.Errorf("onReady calls=%d lang=%q", n, lang)
	}
}

func TestDownloadErrorEmitsErrorPhase(t *testing.T) {
	_, mgr := fakeModels(t)
	m, _ := loadManifest()
	m.Models[0].SHA256 = strings.Repeat("0", 64)
	useManifest(t, m)
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

func TestDownloadKeepsTheOldModels(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	old := []string{"sherpa-onnx-whisper-small", "sherpa-onnx-whisper-turbo", "silero_vad"}
	for _, d := range old {
		seedFiles(t, modelsDir, d, "x.onnx")
	}
	if err := mgr.Download(t.Context(), domain.LangPTBR, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	for _, d := range old {
		if _, err := os.Stat(filepath.Join(modelsDir, d, "x.onnx")); err != nil {
			t.Errorf("%s removed by a download: %v", d, err)
		}
	}
}

func TestStatusUsesThePinnedSizes(t *testing.T) {
	t.Setenv("PHONON_MODEL_MANIFEST", "")
	dir := t.TempDir()
	mgr := NewModelManager(dir)
	if b := mgr.Status(domain.LangPTBR).DownloadBytes; b != 574041195+885098 {
		t.Errorf("nothing installed: %d", b)
	}
	seedFiles(t, dir, VADDirName, VADFileName)
	if b := mgr.Status(domain.LangEN).DownloadBytes; b != 574041195 {
		t.Errorf("vad installed: %d", b)
	}
}

func TestStatusSurfacesManifestError(t *testing.T) {
	_, mgr := fakeModels(t)
	t.Setenv("PHONON_MODEL_MANIFEST", filepath.Join(t.TempDir(), "missing.json"))
	st := mgr.Status(domain.LangEN)
	if st.Ready || !strings.Contains(st.Missing, "model manifest") {
		t.Fatalf("manifest error not surfaced: %+v", st)
	}
}

func TestUnusedListsWhatTheManifestDoesNotInstall(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	if got, err := mgr.Unused(); err != nil || len(got) != 0 {
		t.Fatalf("empty folder: %+v %v", got, err)
	}
	if err := mgr.Download(context.Background(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	seedFiles(t, modelsDir, "sherpa-onnx-whisper-turbo", "a", "b")
	seedFiles(t, modelsDir, "silero_vad", "silero_vad.onnx")
	seedFiles(t, modelsDir, ".downloads", "partial")
	if err := os.WriteFile(filepath.Join(modelsDir, "stray.bin"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(modelsDir, "elsewhere")); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.Unused()
	if err != nil {
		t.Fatal(err)
	}
	want := []port.UnusedModel{{Name: "sherpa-onnx-whisper-turbo", Bytes: 20}, {Name: "silero_vad", Bytes: 10}, {Name: "stray.bin", Bytes: 5}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestUnusedWithNoModelsFolder(t *testing.T) {
	t.Setenv("PHONON_MODEL_MANIFEST", "")
	mgr := NewModelManager(filepath.Join(t.TempDir(), "none"))
	if got, err := mgr.Unused(); err != nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestRemoveUnusedDeletesOnlyThatEntry(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	if err := mgr.Download(context.Background(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	seedFiles(t, modelsDir, "sherpa-onnx-whisper-small", "a")
	seedFiles(t, modelsDir, "sherpa-onnx-whisper-turbo", "b")
	if err := mgr.RemoveUnused("sherpa-onnx-whisper-small"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(modelsDir, "sherpa-onnx-whisper-small")); !os.IsNotExist(err) {
		t.Errorf("still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(modelsDir, "sherpa-onnx-whisper-turbo", "b")); err != nil {
		t.Errorf("other folder touched: %v", err)
	}
	if st := mgr.Status(domain.LangEN); !st.Ready {
		t.Errorf("the current model is gone: %+v", st)
	}
}

func TestRemoveUnusedRefusesWhatItMustNotDelete(t *testing.T) {
	modelsDir, mgr := fakeModels(t)
	if err := mgr.Download(context.Background(), domain.LangEN, func(port.ModelProgress) {}); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	seedFiles(t, outside, "victim", "keep")
	if err := os.Symlink(filepath.Join(outside, "victim"), filepath.Join(modelsDir, "link")); err != nil {
		t.Fatal(err)
	}
	seedFiles(t, modelsDir, ".downloads", "partial")
	for _, name := range []string{WhisperDirName, VADDirName, "", ".", "..", "../victim", "a/b", ".downloads", "link", "missing"} {
		if err := mgr.RemoveUnused(name); err == nil {
			t.Errorf("RemoveUnused(%q) succeeded", name)
		}
	}
	if err := mgr.RemoveUnused(WhisperDirName); !errors.Is(err, ErrModelInUse) {
		t.Errorf("in-use error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "victim", "keep")); err != nil {
		t.Errorf("followed a link: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(modelsDir, "link")); err != nil {
		t.Errorf("link removed: %v", err)
	}
	if st := mgr.Status(domain.LangEN); !st.Ready {
		t.Errorf("the current model is gone: %+v", st)
	}
}
