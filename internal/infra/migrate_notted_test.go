package infra

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GabiHert/phonon/internal/infra/migrate"
)

func nottedPlan(t *testing.T, running bool) migrate.Plan {
	t.Helper()
	root := t.TempDir()
	old := filepath.Join(root, ".notted")
	if err := os.MkdirAll(filepath.Join(old, "audio"), 0o700); err != nil {
		t.Fatal(err)
	}
	return migrate.Plan{OldHome: old, NewHome: filepath.Join(root, ".phonon"), Running: func() bool { return running }}
}

func TestMigrateNottedRefusesWithADialogWhileNottedRuns(t *testing.T) {
	p := nottedPlan(t, true)
	var shown []string
	err := migrateNotted(p, true, func(m string) { shown = append(shown, m) })
	if !errors.Is(err, migrate.ErrOldRunning) {
		t.Fatalf("err = %v", err)
	}
	if len(shown) != 1 || shown[0] != migrate.ErrOldRunning.Error() {
		t.Fatalf("dialogs %q", shown)
	}
	if _, err := os.Stat(p.NewHome); !os.IsNotExist(err) {
		t.Fatalf("Phonon's folder exists: %v", err)
	}
}

func TestMigrateNottedStopsWithADialogWhenTheCopyFails(t *testing.T) {
	p := nottedPlan(t, false)
	blocked := filepath.Join(filepath.Dir(p.OldHome), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.NewHome = filepath.Join(blocked, ".phonon")
	var shown []string
	err := migrateNotted(p, true, func(m string) { shown = append(shown, m) })
	if err == nil || len(shown) != 1 || !strings.HasPrefix(shown[0], "Phonon could not bring over Notted's data, so it did not start: ") {
		t.Fatalf("err %v dialogs %q", err, shown)
	}
}

func TestMigrateNottedCopiesQuietly(t *testing.T) {
	p := nottedPlan(t, false)
	if err := migrateNotted(p, true, func(m string) { t.Fatalf("dialog %q", m) }); err != nil {
		t.Fatal(err)
	}
	for _, home := range []string{p.NewHome, p.OldHome} {
		if _, err := os.Stat(filepath.Join(home, "audio")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrateNottedOffDoesNothing(t *testing.T) {
	p := nottedPlan(t, true)
	if err := migrateNotted(p, false, func(m string) { t.Fatalf("dialog %q", m) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.OldHome); err != nil {
		t.Fatal(err)
	}
}
