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
	if len(pt) != 2 || pt[0] != "whisper small" || pt[1] != "silero vad" {
		t.Errorf("pt-BR: %v", pt)
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
