package service

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

const (
	msgNoArchive = "Set the meetings archive folder in Settings > Instructions first."
	msgNoProfile = "Set your profile in Settings > Instructions first."
)

var brunoList = []string{
	"instructions/people/bruno.local.md",
	"instructions/people/bruno.md",
	"instructions/people/bruno/",
	"instructions/people/bruno/acme.md",
	"instructions/people/bruno/sub/",
	"instructions/people/bruno/sub/notes.txt",
}

func seedBruno(t *testing.T, f *setupFx) string {
	t.Helper()
	ppl := filepath.Join(f.root, "instructions", "people")
	for _, n := range []string{"ana.md", "bruno.md", "bruno.local.md", "bruno/acme.md", "bruno/sub/notes.txt", "bruno-lima.md"} {
		put(t, filepath.Join(ppl, n), "x-"+n)
	}
	return ppl
}

// snapshot maps every path under dir to its kind and, for files, its bytes.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		switch {
		case d.Type()&os.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			out[rel] = "link:" + l
		case d.IsDir():
			out[rel] = "dir"
		case d.Type().IsRegular():
			b, _ := os.ReadFile(p)
			out[rel] = "file:" + string(b)
		default:
			out[rel] = "special"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestListProfileFiles(t *testing.T) {
	f := newSetupFx(t, "ana")
	f.set.s.CheckoutPath = ""
	_, err := f.svc.ListProfileFiles(f.ctx, "bruno")
	wantErr(t, err, msgNoArchive)
	f.set.s.CheckoutPath = f.root

	_, err = f.svc.ListProfileFiles(f.ctx, " ")
	wantErr(t, err, "Choose a profile to delete.")
	_, err = f.svc.ListProfileFiles(f.ctx, "Bruno")
	wantErr(t, err, `Profile must use lowercase letters, digits and dashes, like "ana" or "ana-lima".`)
	_, err = f.svc.ListProfileFiles(f.ctx, "ana")
	wantErr(t, err, "You cannot delete the profile in use. Choose another profile first.")
	_, err = f.svc.ListProfileFiles(f.ctx, "zed")
	wantErr(t, err, `There is no profile "zed" in the archive.`)

	seedBruno(t, f)
	got, err := f.svc.ListProfileFiles(f.ctx, "bruno")
	if err != nil || !reflect.DeepEqual(got, brunoList) {
		t.Fatalf("got %v %v", got, err)
	}
	got, err = f.svc.ListProfileFiles(f.ctx, " bruno ")
	if err != nil || !reflect.DeepEqual(got, brunoList) {
		t.Fatalf("trimmed got %v %v", got, err)
	}

	g := newSetupFx(t, "ana")
	put(t, filepath.Join(g.root, "instructions", "people", "bruno", "acme.md"), "x")
	got, err = g.svc.ListProfileFiles(g.ctx, "bruno")
	want := []string{"instructions/people/bruno/", "instructions/people/bruno/acme.md"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("folder only got %v %v", got, err)
	}
}

func TestListProfileFilesRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, f *setupFx, outside string)
		want  string
	}{
		{"file link", func(t *testing.T, f *setupFx, out string) {
			put(t, filepath.Join(out, "t.md"), "T")
			mustLink(t, filepath.Join(out, "t.md"), filepath.Join(f.root, "instructions", "people", "bruno.md"))
		}, msgOutside},
		{"link inside folder", func(t *testing.T, f *setupFx, out string) {
			put(t, filepath.Join(out, "t.md"), "T")
			put(t, filepath.Join(f.root, "instructions", "people", "bruno", "ok.md"), "x")
			mustLink(t, filepath.Join(out, "t.md"), filepath.Join(f.root, "instructions", "people", "bruno", "evil.md"))
		}, msgOutside},
		{"folder link", func(t *testing.T, f *setupFx, out string) {
			put(t, filepath.Join(out, "t.md"), "T")
			mustLink(t, out, filepath.Join(f.root, "instructions", "people", "bruno"))
		}, msgOutside},
		{"people link", func(t *testing.T, f *setupFx, out string) {
			put(t, filepath.Join(out, "bruno.md"), "T")
			if err := os.RemoveAll(filepath.Join(f.root, "instructions", "people")); err != nil {
				t.Fatal(err)
			}
			mustLink(t, out, filepath.Join(f.root, "instructions", "people"))
		}, msgOutside},
		{"file is folder", func(t *testing.T, f *setupFx, out string) {
			if err := os.MkdirAll(filepath.Join(f.root, "instructions", "people", "bruno.md"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "Notted cannot delete instructions/people/bruno.md: it is not a regular file."},
		{"folder is file", func(t *testing.T, f *setupFx, out string) {
			put(t, filepath.Join(f.root, "instructions", "people", "bruno"), "x")
		}, "Notted cannot delete instructions/people/bruno: it is not a folder."},
		{"fifo", func(t *testing.T, f *setupFx, out string) {
			put(t, filepath.Join(f.root, "instructions", "people", "bruno", "ok.md"), "x")
			if err := syscall.Mkfifo(filepath.Join(f.root, "instructions", "people", "bruno", "pipe"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "Notted cannot delete instructions/people/bruno/pipe: it is not a regular file."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newSetupFx(t, "ana")
			out := t.TempDir()
			c.setup(t, f, out)
			beforeOut, beforeRoot := snapshot(t, out), snapshot(t, f.root)
			_, err := f.svc.ListProfileFiles(f.ctx, "bruno")
			wantErr(t, err, c.want)
			err = f.svc.DeleteProfile(f.ctx, "bruno", []string{"instructions/people/bruno.md"})
			wantErr(t, err, c.want)
			if !reflect.DeepEqual(beforeOut, snapshot(t, out)) || !reflect.DeepEqual(beforeRoot, snapshot(t, f.root)) {
				t.Fatal("something changed")
			}
		})
	}
}

func mustLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProfile(t *testing.T) {
	f := newSetupFx(t, "ana")
	ppl := seedBruno(t, f)
	list, err := f.svc.ListProfileFiles(f.ctx, "bruno")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteProfile(f.ctx, "bruno", list); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"bruno.md", "bruno.local.md", "bruno"} {
		if _, err := os.Lstat(filepath.Join(ppl, n)); !os.IsNotExist(err) {
			t.Fatalf("%s still there: %v", n, err)
		}
	}
	for p, want := range map[string]string{
		filepath.Join(ppl, "ana.md"):                       "x-ana.md",
		filepath.Join(ppl, "bruno-lima.md"):                "x-bruno-lima.md",
		filepath.Join(f.root, "prompts", "base.md"):        "BASE",
		filepath.Join(f.root, "instructions", "README.md"): "R",
	} {
		if read(t, p) != want {
			t.Fatalf("%s changed", p)
		}
	}
	if len(f.git.calls) != 0 {
		t.Fatalf("git ran: %v", f.git.calls)
	}
}

func TestDeleteProfileRefusals(t *testing.T) {
	f := newSetupFx(t, "ana")
	ppl := seedBruno(t, f)
	wantErr(t, f.svc.DeleteProfile(f.ctx, "ana", []string{"instructions/people/ana.md"}),
		"You cannot delete the profile in use. Choose another profile first.")
	if read(t, filepath.Join(ppl, "ana.md")) != "x-ana.md" {
		t.Fatal("ana.md changed")
	}

	list, _ := f.svc.ListProfileFiles(f.ctx, "bruno")
	put(t, filepath.Join(ppl, "bruno", "new.md"), "n")
	changed := `The files of profile "bruno" changed. Review the list again before deleting.`
	wantErr(t, f.svc.DeleteProfile(f.ctx, "bruno", list), changed)
	wantErr(t, f.svc.DeleteProfile(f.ctx, "bruno", nil), changed)
	for _, n := range []string{"bruno.md", "bruno.local.md", "bruno/acme.md", "bruno/sub/notes.txt", "bruno/new.md", "bruno-lima.md", "ana.md"} {
		if _, err := os.Lstat(filepath.Join(ppl, n)); err != nil {
			t.Fatalf("%s gone: %v", n, err)
		}
	}

	g := newSetupFx(t, "ana")
	gp := seedBruno(t, g)
	list, _ = g.svc.ListProfileFiles(g.ctx, "bruno")
	out := t.TempDir()
	put(t, filepath.Join(out, "t.md"), "T")
	mustLink(t, filepath.Join(out, "t.md"), filepath.Join(gp, "bruno", "evil.md"))
	beforeOut, beforeRoot := snapshot(t, out), snapshot(t, g.root)
	wantErr(t, g.svc.DeleteProfile(g.ctx, "bruno", list), msgOutside)
	if !reflect.DeepEqual(beforeOut, snapshot(t, out)) || !reflect.DeepEqual(beforeRoot, snapshot(t, g.root)) {
		t.Fatal("something changed")
	}
}

func TestDeleteProfilePartialFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	f := newSetupFx(t, "ana")
	ppl := filepath.Join(f.root, "instructions", "people")
	put(t, filepath.Join(ppl, "bruno.md"), "x")
	put(t, filepath.Join(ppl, "bruno", "acme.md"), "x")
	list, err := f.svc.ListProfileFiles(f.ctx, "bruno")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(ppl, "bruno"), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(ppl, "bruno"), 0o755) })
	err = f.svc.DeleteProfile(f.ctx, "bruno", list)
	if err == nil || !strings.HasPrefix(err.Error(), "Notted could not delete instructions/people/bruno/acme.md: ") ||
		!strings.HasSuffix(err.Error(), ". The files listed before it are already deleted.") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Lstat(filepath.Join(ppl, "bruno.md")); !os.IsNotExist(err) {
		t.Fatal("bruno.md should be gone")
	}
	if _, err := os.Lstat(filepath.Join(ppl, "bruno", "acme.md")); err != nil {
		t.Fatal("acme.md should remain")
	}
}

func TestListMyProjectKeys(t *testing.T) {
	f := newSetupFx(t, "ana")
	f.set.s.CheckoutPath = ""
	_, err := f.svc.ListMyProjectKeys(f.ctx)
	wantErr(t, err, msgNoArchive)
	f.set.s.CheckoutPath = f.root
	f.set.s.Profile = ""
	_, err = f.svc.ListMyProjectKeys(f.ctx)
	wantErr(t, err, msgNoProfile)
	f.set.s.Profile = "ana"

	got, err := f.svc.ListMyProjectKeys(f.ctx)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v %v", got, err)
	}
	dir := filepath.Join(f.root, "instructions", "people", "ana")
	for _, n := range []string{"acme.md", "globex.md", "notes.txt", "Bad.md", "acme.local.md"} {
		put(t, filepath.Join(dir, n), "x")
	}
	if err := os.MkdirAll(filepath.Join(dir, "x.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(f.root, "secret.md"), "s")
	mustLink(t, filepath.Join(f.root, "secret.md"), filepath.Join(dir, "evil.md"))
	got, err = f.svc.ListMyProjectKeys(f.ctx)
	if err != nil || !reflect.DeepEqual(got, []string{"acme", "globex"}) {
		t.Fatalf("got %v %v", got, err)
	}

	g := newSetupFx(t, "ana")
	mustLink(t, t.TempDir(), filepath.Join(g.root, "instructions", "people", "ana"))
	_, err = g.svc.ListMyProjectKeys(g.ctx)
	wantErr(t, err, msgOutside)
}

func TestCreateMyProjectFile(t *testing.T) {
	f := newSetupFx(t, "ana")
	f.set.s.CheckoutPath = ""
	_, err := f.svc.CreateMyProjectFile(f.ctx, "acme")
	wantErr(t, err, msgNoArchive)
	f.set.s.CheckoutPath = f.root
	f.set.s.Profile = ""
	_, err = f.svc.CreateMyProjectFile(f.ctx, "acme")
	wantErr(t, err, msgNoProfile)
	f.set.s.Profile = "ana"
	_, err = f.svc.CreateMyProjectFile(f.ctx, " ")
	wantErr(t, err, "Enter a key for the new project.")
	_, err = f.svc.CreateMyProjectFile(f.ctx, "Acme Web")
	wantErr(t, err, `Project key must use lowercase letters, digits and dashes, like "acme" or "acme-web".`)
	_, err = f.svc.CreateMyProjectFile(f.ctx, "acme")
	wantErr(t, err, `There is no project "acme" in the archive. Create it with New project.`)

	put(t, filepath.Join(f.root, "instructions", "projects", "acme.md"), "P")
	got, err := f.svc.CreateMyProjectFile(f.ctx, "acme")
	if err != nil || got != "instructions/people/ana/acme.md" {
		t.Fatalf("%q %v", got, err)
	}
	p := filepath.Join(f.root, got)
	if read(t, p) != "# ana — acme\n\n## Project root\n\n## Tools\n" {
		t.Fatalf("template %q", read(t, p))
	}
	put(t, p, "mine")
	_, err = f.svc.CreateMyProjectFile(f.ctx, "acme")
	wantErr(t, err, "instructions/people/ana/acme.md already exists.")
	if read(t, p) != "mine" {
		t.Fatal("overwritten")
	}

	g := newSetupFx(t, "ana")
	put(t, filepath.Join(g.root, "instructions", "projects", "acme.md"), "P")
	out := t.TempDir()
	mustLink(t, out, filepath.Join(g.root, "instructions", "people", "ana"))
	_, err = g.svc.CreateMyProjectFile(g.ctx, "acme")
	wantErr(t, err, msgOutside)
	if es, _ := os.ReadDir(out); len(es) != 0 {
		t.Fatalf("outside changed: %v", es)
	}
}
