package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakes struct {
	trashed, launched []string
	alerts            []string
	copyErr           error
}

func newBridge(t *testing.T, f *fakes) (Bridge, string) {
	t.Helper()
	apps := t.TempDir()
	self := filepath.Join(apps, "Notted.app")
	mkApp(t, filepath.Join(self, "Contents", "Helpers", "Phonon.app"), "Phonon")
	mkApp(t, self, "Notted")
	return Bridge{
		Self:          self,
		InstallerWait: 2 * time.Second,
		Poll:          10 * time.Millisecond,
		Copy: func(src, dst string) error {
			if f.copyErr != nil {
				return f.copyErr
			}
			return dittoCopy(src, dst)
		},
		Trash:  func(p string) error { f.trashed = append(f.trashed, p); return nil },
		Launch: func(app string) error { f.launched = append(f.launched, app); return nil },
		Alert:  func(m string) { f.alerts = append(f.alerts, m) },
	}, apps
}

func mkApp(t *testing.T, app, exe string) {
	t.Helper()
	dir := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, exe), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRunInstallsPhononTrashesBridgeAndOpensPhonon(t *testing.T) {
	f := &fakes{}
	b, apps := newBridge(t, f)
	if err := b.Run(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(apps, "Phonon.app")
	if !exists(filepath.Join(target, "Contents", "MacOS", "Phonon")) {
		t.Fatalf("Phonon.app not installed")
	}
	if exists(target + ".new") {
		t.Fatalf("staging copy left behind")
	}
	if len(f.trashed) != 1 || f.trashed[0] != b.Self {
		t.Fatalf("trashed = %v", f.trashed)
	}
	if len(f.launched) != 1 || f.launched[0] != target {
		t.Fatalf("launched = %v", f.launched)
	}
}

func TestRunWaitsForNottedInstallerToRemoveOld(t *testing.T) {
	f := &fakes{}
	b, _ := newBridge(t, f)
	old := b.Self + ".old"
	mkApp(t, old, "Notted")
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.RemoveAll(old)
	}()
	start := time.Now()
	if err := b.Run(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatalf("did not wait for the installer")
	}
	if len(f.launched) != 1 {
		t.Fatalf("launched = %v", f.launched)
	}
}

func TestRunDoesNothingWhenInstallerRolledBack(t *testing.T) {
	f := &fakes{}
	b, apps := newBridge(t, f)
	mkApp(t, b.Self+".old", "Notted")
	go func() {
		time.Sleep(50 * time.Millisecond)
		// The installer moves the bridge away and the previous Notted back.
		_ = os.Rename(b.Self, b.Self+".new")
	}()
	if err := b.Run(); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(apps, "Phonon.app")) || len(f.launched) != 0 || len(f.trashed) != 0 {
		t.Fatalf("acted after a rollback: launched %v trashed %v", f.launched, f.trashed)
	}
}

func TestRunKeepsAnExistingPhonon(t *testing.T) {
	f := &fakes{}
	b, apps := newBridge(t, f)
	target := filepath.Join(apps, "Phonon.app")
	mkApp(t, target, "Phonon")
	marker := filepath.Join(target, "mine")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.Run(); err != nil {
		t.Fatal(err)
	}
	if !exists(marker) {
		t.Fatalf("existing Phonon.app was replaced")
	}
	if len(f.launched) != 1 || f.launched[0] != target {
		t.Fatalf("launched = %v", f.launched)
	}
}

func TestRunKeepsBridgeAndAlertsWhenCopyFails(t *testing.T) {
	f := &fakes{copyErr: errors.New("disk full")}
	b, apps := newBridge(t, f)
	if err := b.Run(); err == nil {
		t.Fatal("want an error")
	}
	if exists(filepath.Join(apps, "Phonon.app")) || exists(filepath.Join(apps, "Phonon.app.new")) {
		t.Fatalf("partial Phonon.app left")
	}
	if !exists(b.Self) || len(f.trashed) != 0 || len(f.launched) != 0 {
		t.Fatalf("bridge not kept: trashed %v launched %v", f.trashed, f.launched)
	}
	if len(f.alerts) != 1 || !strings.Contains(f.alerts[0], "Open Notted again") {
		t.Fatalf("alerts = %v", f.alerts)
	}
}

func TestRunRefusesABridgeWithoutPhonon(t *testing.T) {
	f := &fakes{}
	b, _ := newBridge(t, f)
	if err := os.RemoveAll(b.Embedded()); err != nil {
		t.Fatal(err)
	}
	if err := b.Run(); err == nil || len(f.alerts) != 1 || exists(b.Target()) {
		t.Fatalf("err %v alerts %v", err, f.alerts)
	}
}

func TestTrashNameAvoidsCollisions(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 10, 18, 4, 5, 0, time.UTC)
	if got := trashName(dir, "Notted.app", now); got != filepath.Join(dir, "Notted.app") {
		t.Fatalf("got %s", got)
	}
	mkApp(t, filepath.Join(dir, "Notted.app"), "Notted")
	if got := trashName(dir, "Notted.app", now); got != filepath.Join(dir, "Notted 18.04.05.app") {
		t.Fatalf("got %s", got)
	}
}

func TestBundleOf(t *testing.T) {
	if b, ok := bundleOf("/Applications/Notted.app/Contents/MacOS/Notted"); !ok || b != "/Applications/Notted.app" {
		t.Fatalf("got %q %v", b, ok)
	}
	if _, ok := bundleOf("/usr/local/bin/notted"); ok {
		t.Fatal("want false")
	}
}
