package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bhlabz/notted/internal/domain"
)

type memSettings struct{ s domain.Settings }

func (m *memSettings) GetSettings(context.Context) (domain.Settings, error) {
	out := m.s
	out.ProjectRoots = map[string]string{}
	for k, v := range m.s.ProjectRoots {
		out.ProjectRoots[k] = v
	}
	return out, nil
}

func (m *memSettings) SaveSettings(_ context.Context, s domain.Settings) error {
	m.s = s
	return nil
}

// layerPrompts mimics the archive's command: the base line plus the optional person-project layer.
type layerPrompts struct{}

func (layerPrompts) Prompt(_ context.Context, checkout, profile, project string) (string, error) {
	out := "PROMPT project=" + project + " profile=" + profile
	if b, err := os.ReadFile(filepath.Join(checkout, "instructions", "people", profile, project+".md")); err == nil {
		out += "\n" + string(b)
	}
	return out, nil
}

type insFixture struct {
	t       *testing.T
	root    string
	outside string
	set     *memSettings
	svc     *InstructionsService
	ctx     context.Context
}

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
}

func newInsFixture(t *testing.T) *insFixture {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "archive")
	outside := filepath.Join(base, "outside")
	put(t, filepath.Join(root, "bin", "meeting"), "#!/bin/sh\n")
	put(t, filepath.Join(root, "bin", "other.sh"), "echo hi\n")
	put(t, filepath.Join(root, "prompts", "base.md"), "BASE")
	put(t, filepath.Join(root, "instructions", "README.md"), "README")
	put(t, filepath.Join(root, "instructions", "projects", "acme.md"), "ACME")
	put(t, filepath.Join(root, "instructions", "people", "ana.md"), "ANA")
	put(t, filepath.Join(root, "instructions", "people", "ana", "acme.md"), "ANA-ACME")
	put(t, filepath.Join(root, "instructions", "people", "ana.local.md"), "LOCAL")
	put(t, filepath.Join(base, "secret.md"), "SECRET")
	put(t, filepath.Join(outside, "secret.md"), "SECRET")
	if err := os.Symlink(filepath.Join(base, "secret.md"), filepath.Join(root, "instructions", "projects", "evil.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "instructions", "people", "out")); err != nil {
		t.Fatal(err)
	}
	set := &memSettings{s: domain.Settings{Profile: "ana", CheckoutPath: root, ProjectRoots: map[string]string{}}}
	arch := NewArchiveService(nil, set, nil, nil)
	return &insFixture{t: t, root: root, outside: base, set: set, ctx: context.Background(),
		svc: NewInstructionsService(set, layerPrompts{}, arch.ListProjects)}
}

func (f *insFixture) text(rel string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, rel))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

const (
	msgOutside  = "That path is outside the meetings archive folder."
	msgPatterns = "Only prompts/base.md and Markdown files under instructions/ can be opened here."
)

func TestInstructionsListKindsAndOrder(t *testing.T) {
	f := newInsFixture(t)
	got, err := f.svc.List(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []InstructionFile{
		{"prompts/base.md", "base", true},
		{"instructions/README.md", "readme", true},
		{"instructions/projects/acme.md", "project", false},
		{"instructions/people/ana.md", "person", false},
		{"instructions/people/ana.local.md", "local", false},
		{"instructions/people/ana/acme.md", "person-project", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestInstructionsUnsetArchiveFolder(t *testing.T) {
	f := newInsFixture(t)
	f.set.s.CheckoutPath = ""
	_, err := f.svc.List(f.ctx)
	wantErr(t, err, "Set the meetings archive folder in Settings > Instructions first.")
	_, err = f.svc.Read(f.ctx, "prompts/base.md")
	wantErr(t, err, "Set the meetings archive folder in Settings > Instructions first.")
	wantErr(t, f.svc.Write(f.ctx, "instructions/projects/acme.md", "x"), "Set the meetings archive folder in Settings > Instructions first.")
}

func TestInstructionsRead(t *testing.T) {
	f := newInsFixture(t)
	got, err := f.svc.Read(f.ctx, "instructions/projects/acme.md")
	if err != nil || got != "ACME" {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := f.svc.Read(f.ctx, "prompts/base.md"); err != nil || got != "BASE" {
		t.Fatalf("base: %q, %v", got, err)
	}
	for _, p := range []string{"../secret.md", "/etc/hosts", `..\secret.md`, "C:/x.md",
		"instructions/projects/evil.md", "instructions/people/out/secret.md",
		"instructions/../../secret.md"} {
		_, err := f.svc.Read(f.ctx, p)
		if err == nil || err.Error() != msgOutside {
			t.Errorf("Read(%q) error = %v, want outside message", p, err)
		}
	}
	for _, p := range []string{"bin/other.sh", "instructions/projects/acme.txt", "instructions", "instructions/projects/deep/er/x.md", "README.md", "instructions/.md"} {
		_, err := f.svc.Read(f.ctx, p)
		if err == nil || err.Error() != msgPatterns {
			t.Errorf("Read(%q) error = %v, want patterns message", p, err)
		}
	}
}

func TestInstructionsReadTooLarge(t *testing.T) {
	f := newInsFixture(t)
	put(t, filepath.Join(f.root, "instructions", "projects", "big.md"), strings.Repeat("a", 2<<20+1))
	_, err := f.svc.Read(f.ctx, "instructions/projects/big.md")
	wantErr(t, err, "That file is too large to open here (over 2 MB).")
	put(t, filepath.Join(f.root, "instructions", "projects", "edge.md"), strings.Repeat("a", 2<<20))
	if _, err := f.svc.Read(f.ctx, "instructions/projects/edge.md"); err != nil {
		t.Fatal(err)
	}
}

func TestInstructionsWriteReadOnly(t *testing.T) {
	f := newInsFixture(t)
	wantErr(t, f.svc.Write(f.ctx, "prompts/base.md", "x"), "prompts/base.md is read-only in Notted. Change it through the archive repository.")
	wantErr(t, f.svc.Write(f.ctx, "instructions/README.md", "x"), "instructions/README.md is read-only in Notted.")
	if f.text("prompts/base.md") != "BASE" || f.text("instructions/README.md") != "README" {
		t.Fatal("a read-only file changed")
	}
}

func TestInstructionsWriteOutsidePatterns(t *testing.T) {
	f := newInsFixture(t)
	for _, p := range []string{"bin/other.sh", "instructions/projects/x.txt", "prompts/other.md", "instructions/a/b/c.md"} {
		err := f.svc.Write(f.ctx, p, "x")
		if err == nil || err.Error() != msgPatterns {
			t.Errorf("Write(%q) error = %v, want patterns message", p, err)
		}
	}
	if f.text("bin/other.sh") != "echo hi\n" {
		t.Fatal("bin/other.sh changed")
	}
}

func TestInstructionsWriteSucceedsAtomically(t *testing.T) {
	f := newInsFixture(t)
	if err := f.svc.Write(f.ctx, "instructions/people/ana/acme.md", "x"); err != nil {
		t.Fatal(err)
	}
	if f.text("instructions/people/ana/acme.md") != "x" {
		t.Fatal("not written")
	}
	if err := f.svc.Write(f.ctx, "instructions/projects/globex.md", "NEW"); err != nil {
		t.Fatal(err)
	}
	if f.text("instructions/projects/globex.md") != "NEW" {
		t.Fatal("new file not written")
	}
	entries, _ := os.ReadDir(filepath.Join(f.root, "instructions", "projects"))
	for _, e := range entries {
		if strings.Contains(e.Name(), "tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	if err := f.svc.Write(f.ctx, "instructions/people/nodir/acme.md", "x"); err == nil {
		t.Fatal("write into a missing parent folder must fail")
	}
}

func TestInstructionsWriteNeverLeavesTheFolder(t *testing.T) {
	f := newInsFixture(t)
	for _, p := range []string{"../secret.md", "/tmp/x.md", "instructions/projects/evil.md", "instructions/people/out/secret.md", "instructions/people/out/new.md"} {
		err := f.svc.Write(f.ctx, p, "PWNED")
		if err == nil || err.Error() != msgOutside {
			t.Errorf("Write(%q) error = %v, want outside message", p, err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(f.outside, "secret.md"))
	b2, _ := os.ReadFile(filepath.Join(f.outside, "outside", "secret.md"))
	if string(b) != "SECRET" || string(b2) != "SECRET" {
		t.Fatal("an outside file changed")
	}
	if _, err := os.Stat(filepath.Join(f.outside, "outside", "new.md")); err == nil {
		t.Fatal("a file was created outside")
	}
	if fi, err := os.Lstat(filepath.Join(f.root, "instructions", "projects", "evil.md")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the link was replaced")
	}
}

func TestInstructionsSettingsRoundTripAndClear(t *testing.T) {
	f := newInsFixture(t)
	proj := t.TempDir()
	want := domain.Settings{Profile: "ana-lima", CheckoutPath: f.root, ProjectRoots: map[string]string{"acme": proj, "globex": ""}}
	if err := f.svc.SetSettings(f.ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetSettings(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != "ana-lima" || got.CheckoutPath != f.root || !reflect.DeepEqual(got.ProjectRoots, map[string]string{"acme": proj}) {
		t.Fatalf("got %+v", got)
	}
	if err := f.svc.SetSettings(f.ctx, domain.Settings{}); err != nil {
		t.Fatal(err)
	}
	got, _ = f.svc.GetSettings(f.ctx)
	if got.Profile != "" || got.CheckoutPath != "" || len(got.ProjectRoots) != 0 || got.ProjectRoots == nil {
		t.Fatalf("not cleared: %+v", got)
	}
}

func TestInstructionsSettingsValidation(t *testing.T) {
	f := newInsFixture(t)
	missing := filepath.Join(t.TempDir(), "nope")
	notArchive := t.TempDir()
	cases := []struct {
		name string
		st   domain.Settings
		want string
	}{
		{"profile caps", domain.Settings{Profile: "Ana"}, `Profile must use lowercase letters, digits and dashes, like "ana" or "ana-lima".`},
		{"profile space", domain.Settings{Profile: "ana lima"}, `Profile must use lowercase letters, digits and dashes, like "ana" or "ana-lima".`},
		{"profile dash first", domain.Settings{Profile: "-ana"}, `Profile must use lowercase letters, digits and dashes, like "ana" or "ana-lima".`},
		{"folder missing", domain.Settings{CheckoutPath: missing}, "That folder does not exist: " + missing},
		{"folder relative", domain.Settings{CheckoutPath: "rel/dir"}, "That folder does not exist: rel/dir"},
		{"folder not archive", domain.Settings{CheckoutPath: notArchive}, "That folder is not a meetings archive checkout: bin/meeting is missing."},
		{"root missing", domain.Settings{ProjectRoots: map[string]string{"acme": missing}}, "The project root for acme does not exist: " + missing},
		{"root relative", domain.Settings{ProjectRoots: map[string]string{"acme": "rel"}}, "The project root for acme does not exist: rel"},
	}
	for _, c := range cases {
		before := f.set.s
		wantErr(t, f.svc.SetSettings(f.ctx, c.st), c.want)
		if !reflect.DeepEqual(before, f.set.s) {
			t.Errorf("%s: settings changed on error", c.name)
		}
	}
}

func TestInstructionsPreview(t *testing.T) {
	f := newInsFixture(t)
	got, err := f.svc.PreviewPrompt(f.ctx, "acme")
	if err != nil || !strings.HasPrefix(got, "PROMPT project=acme profile=ana") {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := f.svc.Write(f.ctx, "instructions/people/ana/acme.md", "FRESH TEXT"); err != nil {
		t.Fatal(err)
	}
	got, _ = f.svc.PreviewPrompt(f.ctx, "acme")
	if !strings.Contains(got, "FRESH TEXT") {
		t.Fatalf("preview missed the new text: %q", got)
	}
	_, err = f.svc.PreviewPrompt(f.ctx, "initech")
	wantErr(t, err, `Unknown project "initech". Add instructions/projects/initech.md to the archive first.`)
	f.set.s.Profile = ""
	_, err = f.svc.PreviewPrompt(f.ctx, "acme")
	wantErr(t, err, "Set your profile in Settings > Instructions first.")
	f.set.s.CheckoutPath = ""
	_, err = f.svc.PreviewPrompt(f.ctx, "acme")
	wantErr(t, err, "Set the meetings archive folder in Settings > Instructions first.")
}

func TestInstructionsRefuseLinksInsideTheFolder(t *testing.T) {
	f := newInsFixture(t)
	put(t, filepath.Join(f.root, ".git", "x.md"), "GITDATA")
	links := map[string]string{
		"instructions/people/p":          "../../prompts",
		"instructions/people/g":          "../../.git",
		"instructions/projects/alias.md": "acme.md",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(f.root, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"instructions/people/p/base.md", "instructions/people/g/x.md", "instructions/projects/alias.md"} {
		got, err := f.svc.Read(f.ctx, p)
		if err == nil || err.Error() != msgOutside || got != "" {
			t.Errorf("Read(%q) = %q, %v; want outside message", p, got, err)
		}
		err = f.svc.Write(f.ctx, p, "PWNED")
		if err == nil || err.Error() != msgOutside {
			t.Errorf("Write(%q) error = %v, want outside message", p, err)
		}
	}
	if f.text("prompts/base.md") != "BASE" || f.text(".git/x.md") != "GITDATA" || f.text("instructions/projects/acme.md") != "ACME" {
		t.Fatal("a file reached through a link changed")
	}
	if _, err := os.Stat(filepath.Join(f.root, "instructions", "people", "g", "new.md")); err == nil {
		t.Fatal("a file was created through a link")
	}
	if err := f.svc.Write(f.ctx, "instructions/people/g/new.md", "x"); err == nil {
		t.Fatal("a new file through a directory link must be refused")
	}
}

func TestInstructionsWorkWhenTheFolderItselfIsALink(t *testing.T) {
	f := newInsFixture(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(f.root, alias); err != nil {
		t.Fatal(err)
	}
	f.set.s.CheckoutPath = alias
	if got, err := f.svc.Read(f.ctx, "instructions/projects/acme.md"); err != nil || got != "ACME" {
		t.Fatalf("got %q, %v", got, err)
	}
	if err := f.svc.Write(f.ctx, "instructions/projects/acme.md", "Z"); err != nil {
		t.Fatal(err)
	}
}
