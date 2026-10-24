package wails

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bhlabz/notted/internal/application/service"
	"github.com/bhlabz/notted/internal/integration/git"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func fakeDialog(t *testing.T, path string, err error) *wruntime.OpenDialogOptions {
	t.Helper()
	got := &wruntime.OpenDialogOptions{}
	old := openDirectoryDialog
	openDirectoryDialog = func(_ context.Context, o wruntime.OpenDialogOptions) (string, error) {
		*got = o
		return path, err
	}
	t.Cleanup(func() { openDirectoryDialog = old })
	return got
}

func TestChooseFolderStartDir(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ start, want string }{
		{dir, dir}, {filepath.Join(dir, "missing"), ""}, {"rel/dir", ""}, {file, ""}, {"", ""},
	} {
		opts := fakeDialog(t, "/picked", nil)
		a := &App{emitter: &Emitter{}}
		got, err := a.ChooseFolder("Pick", c.start)
		if err != nil || got != "/picked" {
			t.Fatalf("%q %v", got, err)
		}
		if opts.DefaultDirectory != c.want || opts.Title != "Pick" || !opts.CanCreateDirectories {
			t.Fatalf("start %q: opts %+v", c.start, *opts)
		}
	}
}

func TestChooseFolderCancelAndError(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	fakeDialog(t, "", nil)
	got, err := (&App{emitter: &Emitter{}}).ChooseFolder("t", "")
	if got != "" || err != nil {
		t.Fatalf("%q %v", got, err)
	}
	fakeDialog(t, "", errors.New("boom"))
	_, err = (&App{emitter: &Emitter{}}).ChooseFolder("t", "")
	if err == nil || err.Error() != "The folder picker failed: boom" {
		t.Fatalf("%v", err)
	}
}

func TestInspectAndPrepareArchiveBindings(t *testing.T) {
	t.Setenv("NOTTED_HOME", t.TempDir())
	a := &App{emitter: &Emitter{}, setup: service.NewSetupService(nil, git.New(os.Environ))}
	dir := t.TempDir()
	c, err := a.InspectArchive(dir)
	if err != nil || c.Path != dir || c.Ready || len(c.Missing) == 0 || c.InRepo {
		t.Fatalf("%+v %v", c, err)
	}
	if err := a.PrepareArchive(dir); err != nil {
		t.Fatal(err)
	}
	if c, err = a.InspectArchive(dir); err != nil || !c.Ready || c.Missing == nil {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := a.InspectArchive("rel"); err == nil || err.Error() != "That folder does not exist: rel" {
		t.Fatalf("%v", err)
	}
}
