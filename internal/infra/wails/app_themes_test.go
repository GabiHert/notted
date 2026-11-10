package wails

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GabiHert/phonon/internal/infra/themes"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func themeApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("PHONON_HOME", t.TempDir())
	return &App{emitter: &Emitter{}, themes: &themes.Store{Dir: filepath.Join(t.TempDir(), "themes")}}
}

func fakeOpenFile(t *testing.T, path string, err error) *wruntime.OpenDialogOptions {
	t.Helper()
	got := &wruntime.OpenDialogOptions{}
	old := openFileDialog
	openFileDialog = func(_ context.Context, o wruntime.OpenDialogOptions) (string, error) {
		*got = o
		return path, err
	}
	t.Cleanup(func() { openFileDialog = old })
	return got
}

func fakeSaveFile(t *testing.T, path string, err error) *wruntime.SaveDialogOptions {
	t.Helper()
	got := &wruntime.SaveDialogOptions{}
	old := saveFileDialog
	saveFileDialog = func(_ context.Context, o wruntime.SaveDialogOptions) (string, error) {
		*got = o
		return path, err
	}
	t.Cleanup(func() { saveFileDialog = old })
	return got
}

const vscodeFile = `{
	// a comment
	"name": "Test Night",
	"type": "dark",
	"colors": { "editor.background": "#101820", "editor.foreground": "#e0e6f0", "button.background": "#4fa3ff", },
}`

func TestThemesImportSelectExportDelete(t *testing.T) {
	a := themeApp(t)
	list, err := a.ListThemes()
	if err != nil || len(list) != 3 || list[0].ID != "system" || list[0].Tokens == nil {
		t.Fatalf("fresh list %+v %v", list, err)
	}
	sel, err := a.GetSelectedTheme()
	if err != nil || sel.ID != "system" || sel.Mode != "system" {
		t.Fatalf("default selection %+v %v", sel, err)
	}

	file := filepath.Join(t.TempDir(), "night.jsonc")
	if err := os.WriteFile(file, []byte(vscodeFile), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := fakeOpenFile(t, file, nil)
	th, err := a.ImportVSCodeTheme()
	if err != nil {
		t.Fatal(err)
	}
	if th.ID != "test-night" || th.Mode != "dark" || th.Tokens["bg"] != "#101820" || len(th.Tokens) != len(themes.TokenNames) {
		t.Fatalf("imported %+v", th)
	}
	if len(opts.Filters) != 1 || !strings.Contains(opts.Filters[0].Pattern, "*.jsonc") || !strings.Contains(opts.Filters[0].Pattern, "*.json") {
		t.Errorf("filters %+v", opts.Filters)
	}

	if got, err := a.SelectTheme(th.ID); err != nil || got.ID != th.ID {
		t.Fatalf("select %+v %v", got, err)
	}
	if got, _ := a.GetSelectedTheme(); got.ID != th.ID {
		t.Fatalf("selected %q", got.ID)
	}

	out := filepath.Join(t.TempDir(), "out.json")
	saveOpts := fakeSaveFile(t, out, nil)
	p, err := a.ExportTheme(th.ID)
	if err != nil || p != out {
		t.Fatalf("export %q %v", p, err)
	}
	if saveOpts.DefaultFilename != "test-night.phonon-theme.json" {
		t.Errorf("default filename %q", saveOpts.DefaultFilename)
	}
	// The export imports back as a second copy.
	fakeOpenFile(t, out, nil)
	again, err := a.ImportVSCodeTheme()
	if err != nil || again.ID != "test-night-2" || again.Tokens["accent"] != th.Tokens["accent"] {
		t.Fatalf("re-import %+v %v", again, err)
	}

	if err := a.DeleteTheme(th.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.GetSelectedTheme(); got.ID != "system" {
		t.Errorf("after deleting the chosen theme: %q", got.ID)
	}
	if err := a.DeleteTheme("oscilloscope-dark"); err == nil {
		t.Error("a built-in was deleted")
	}
}

func TestThemesImportErrorsAndCancel(t *testing.T) {
	a := themeApp(t)
	fakeOpenFile(t, "", nil)
	if th, err := a.ImportVSCodeTheme(); err != nil || th.ID != "" {
		t.Fatalf("cancel: %+v %v", th, err)
	}
	fakeOpenFile(t, "", errors.New("boom"))
	if _, err := a.ImportVSCodeTheme(); err == nil || err.Error() != "The file picker failed: boom" {
		t.Fatalf("picker error: %v", err)
	}
	pkg := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(pkg, []byte(`{"name":"x","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeOpenFile(t, pkg, nil)
	if _, err := a.ImportVSCodeTheme(); !errors.Is(err, themes.ErrNotTheme) {
		t.Fatalf("not a theme: %v", err)
	}
	if list, _ := a.ListThemes(); len(list) != 3 {
		t.Errorf("a failed import saved something: %d themes", len(list))
	}
	if _, err := a.SelectTheme("missing"); err == nil {
		t.Error("selecting an unknown theme must fail")
	}
}

func TestThemesExportCancelAndSystem(t *testing.T) {
	a := themeApp(t)
	fakeSaveFile(t, "", nil)
	if p, err := a.ExportTheme("oscilloscope-light"); err != nil || p != "" {
		t.Fatalf("cancel: %q %v", p, err)
	}
	if _, err := a.ExportTheme("system"); err == nil {
		t.Error("the system theme has nothing to export")
	}
}

func TestThemesWithoutStore(t *testing.T) {
	a := &App{emitter: &Emitter{}}
	if _, err := a.ListThemes(); err == nil {
		t.Error("want an error without a store")
	}
}
