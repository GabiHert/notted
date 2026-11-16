package speech

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GabiHert/phonon/internal/domain"
)

func names(as []ModelArtifact) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name
	}
	return out
}

func TestEmbeddedManifestListsTheWhisperModelAndTheVAD(t *testing.T) {
	t.Setenv("PHONON_MODEL_MANIFEST", "")
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if m.Schema != 3 || m.Version != "whisper.cpp large-v3-turbo q5_0 + silero vad v5.1.2" {
		t.Errorf("schema %d version %q", m.Schema, m.Version)
	}
	want := []ModelArtifact{{
		Name:   "whisper large-v3-turbo",
		URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin",
		SHA256: "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2",
		Size:   574041195,
		Dir:    WhisperDirName,
		File:   WhisperFileName,
	}, {
		Name:   "silero vad",
		URL:    "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin",
		SHA256: "29940d98d42b91fbd05ce489f3ecf7c72f0a42f027e4875919a28fb4c04ea2cf",
		Size:   885098,
		Dir:    VADDirName,
		File:   VADFileName,
	}}
	if len(m.Models) != len(want) {
		t.Fatalf("want %d artifacts, got %+v", len(want), m.Models)
	}
	for i := range want {
		if m.Models[i] != want[i] {
			t.Errorf("artifact %d: %+v", i, m.Models[i])
		}
	}
}

func TestBothLanguagesNeedTheSameTwoFiles(t *testing.T) {
	t.Setenv("PHONON_MODEL_MANIFEST", "")
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []domain.Language{domain.LangEN, domain.LangPTBR} {
		got := names(m.forLanguage(lang))
		if len(got) != 2 || got[0] != "whisper large-v3-turbo" || got[1] != "silero vad" {
			t.Errorf("%s: %v", lang, got)
		}
	}
}

func TestNeededForSelectsByLanguage(t *testing.T) {
	m := DownloadManifest{Models: []ModelArtifact{{Name: "a", Lang: "en"}, {Name: "b", Lang: "pt-BR"}, {Name: "c"}}}
	if got := names(m.forLanguage(domain.LangEN)); len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("en: %v", got)
	}
	if got := names(m.forLanguage(domain.LangPTBR)); len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("pt-BR: %v", got)
	}
}

func TestManifestOverrideReplacesEmbedded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	body := `{"schema":2,"version":"x","models":[{"name":"only","url":"/x","sha256":"","size":1,"dir":"d"}]}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PHONON_MODEL_MANIFEST", p)
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Models) != 1 || m.Models[0].Name != "only" {
		t.Errorf("override ignored: %+v", m.Models)
	}
}
