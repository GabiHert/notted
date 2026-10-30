package speech

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GabiHert/notted/internal/domain"
)

func names(as []ModelArtifact) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name
	}
	return out
}

func TestEmbeddedManifestListsThreeArtifacts(t *testing.T) {
	t.Setenv("NOTTED_MODEL_MANIFEST", "")
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Models) != 3 {
		t.Fatalf("want 3 artifacts, got %d", len(m.Models))
	}
	for _, a := range m.Models {
		if len(a.SHA256) != 64 || a.Size <= 0 || a.URL == "" || a.Dir == "" {
			t.Errorf("incomplete artifact %+v", a)
		}
	}
}

func TestNeededForSelectsByLanguage(t *testing.T) {
	t.Setenv("NOTTED_MODEL_MANIFEST", "")
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	en := names(m.forLanguage(domain.LangEN))
	if len(en) != 2 || en[0] != "whisper small.en" || en[1] != "silero vad" {
		t.Errorf("en: %v", en)
	}
	pt := names(m.forLanguage(domain.LangPTBR))
	if len(pt) != 2 || pt[0] != "whisper turbo" || pt[1] != "silero vad" {
		t.Errorf("pt-BR: %v", pt)
	}
}

func TestEmbeddedManifestPinsTurboForPortuguese(t *testing.T) {
	t.Setenv("NOTTED_MODEL_MANIFEST", "")
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "whisper turbo + small.en (int8) + silero vad" {
		t.Errorf("version: %q", m.Version)
	}
	want := ModelArtifact{
		Name:   "whisper turbo",
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-whisper-turbo.tar.bz2",
		SHA256: "b11acbbcd660b44a8e0df33724feb5aaa709cf65668f2823d59f656312544f22",
		Size:   563790207,
		Dir:    "sherpa-onnx-whisper-turbo",
		Lang:   "pt-BR",
	}
	if got := m.forLanguage(domain.LangPTBR)[0]; got != want {
		t.Errorf("pt-BR artifact: %+v", got)
	}
}

func TestEmbeddedManifestKeepsEnglishAndDetector(t *testing.T) {
	t.Setenv("NOTTED_MODEL_MANIFEST", "")
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	wantEN := ModelArtifact{
		Name:   "whisper small.en",
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-whisper-small.en.tar.bz2",
		SHA256: "0cdba2b8aaab69e04847f3427cc9709574112e67913a1a84b7fec3a8729faa9a",
		Size:   635693775,
		Dir:    "sherpa-onnx-whisper-small.en",
		Lang:   "en",
	}
	wantVAD := ModelArtifact{
		Name:   "silero vad",
		URL:    "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx",
		SHA256: "9e2449e1087496d8d4caba907f23e0bd3f78d91fa552479bb9c23ac09cbb1fd6",
		Size:   643854,
		Dir:    "silero_vad",
		File:   "silero_vad.onnx",
	}
	if m.Models[0] != wantEN {
		t.Errorf("english artifact: %+v", m.Models[0])
	}
	if m.Models[2] != wantVAD {
		t.Errorf("detector artifact: %+v", m.Models[2])
	}
}

func TestManifestOverrideReplacesEmbedded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	body := `{"schema":2,"version":"x","models":[{"name":"only","url":"/x","sha256":"","size":1,"dir":"d"}]}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOTTED_MODEL_MANIFEST", p)
	m, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Models) != 1 || m.Models[0].Name != "only" {
		t.Errorf("override ignored: %+v", m.Models)
	}
}
