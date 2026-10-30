package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GabiHert/notted/internal/domain"
)

type failSettings struct{}

func (failSettings) GetSettings(context.Context) (domain.Settings, error) {
	return domain.Settings{}, errors.New("settings unreadable")
}
func (failSettings) SaveSettings(context.Context, domain.Settings) error { return nil }

func mustContain(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(text, p) {
			t.Errorf("prompt lacks %q", p)
		}
	}
}

func mustNotContain(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(text, p) {
			t.Errorf("prompt has %q", p)
		}
	}
}

func seedSetupState(t *testing.T, f *setupFx) {
	t.Helper()
	put(t, filepath.Join(f.root, "instructions", "projects", "acme.md"), "x")
	put(t, filepath.Join(f.root, "instructions", "projects", "globex.md"), "x")
	put(t, filepath.Join(f.root, "instructions", "people", "ana.md"), "x")
	put(t, filepath.Join(f.root, "instructions", "people", "ana.local.md"), "x")
	put(t, filepath.Join(f.root, "instructions", "people", "ana", "acme.md"), "x")
}

func TestSetupPromptWithArchiveAndProfile(t *testing.T) {
	f := newSetupFx(t, "ana")
	seedSetupState(t, f)
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, got,
		f.root,
		"instructions/README.md",
		"prompts/base.md",
		"bin/meeting",
		"instructions/projects/",
		"instructions/people/",
		"meetings/",
		"ana",
		"acme",
		"globex",
		"instructions/people/ana.md",
		"instructions/people/ana.local.md",
		"one question at a time",
		"only under",
		"never commit or push without",
		"never edit prompts/base.md",
		"show each file",
		"Settings",
		"Prompt preview",
		"root folder",
		"microphone",
		"transcribe",
		"one Claude Code session per meeting",
	)
	// globex has no file of this person yet; acme does.
	mustContain(t, got, "Projects without a file of mine: globex")
	mustNotContain(t, got, "Projects without a file of mine: acme", "not chosen yet", "Choose or clone the archive")
}

func TestSetupPromptListsNoMissingWhenComplete(t *testing.T) {
	f := newSetupFx(t, "ana")
	seedSetupState(t, f)
	put(t, filepath.Join(f.root, "instructions", "people", "ana", "globex.md"), "x")
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, got, "Projects without a file of mine: none")
}

func TestSetupPromptWithoutProfile(t *testing.T) {
	f := newSetupFx(t, "")
	seedSetupState(t, f)
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, got, f.root, "Profile: not chosen yet", "acme", "globex", "one question at a time", "instructions/README.md")
	mustNotContain(t, got, "Projects without a file of mine")
}

func TestSetupPromptWithoutArchive(t *testing.T) {
	f := newSetupFx(t, "ana")
	f.set.s.CheckoutPath = ""
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, got,
		"Archive folder: not chosen yet",
		"Choose or clone the archive",
		"Settings",
		"Notted sets the folder up",
		"microphone",
	)
	mustNotContain(t, got,
		"instructions/README.md",
		"one question at a time",
		"only under",
		"never commit or push without",
		"prompts/base.md",
		"Prompt preview",
	)
}

func TestSetupPromptArchiveReadErrorIsALine(t *testing.T) {
	f := newSetupFx(t, "ana")
	seedSetupState(t, f)
	outside := t.TempDir()
	proj := filepath.Join(f.root, "instructions", "projects")
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, proj); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatalf("a read error must not fail the prompt: %v", err)
	}
	mustContain(t, got, "Notted could not read the archive's projects", "one question at a time")
	mustNotContain(t, got, "Projects without a file of mine")
}

func TestSetupPromptArchiveNotReadyIsALine(t *testing.T) {
	f := newSetupFx(t, "ana")
	if err := os.Remove(filepath.Join(f.root, "bin", "meeting")); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, got, "bin/meeting is missing", f.root)
}

func TestSetupPromptSettingsError(t *testing.T) {
	svc := NewSetupService(failSettings{}, &fakeGit{})
	if _, err := svc.GetSetupPrompt(context.Background()); err == nil || err.Error() != "settings unreadable" {
		t.Fatalf("%v", err)
	}
}

func TestSetupPromptHoldsNoURL(t *testing.T) {
	f := newSetupFx(t, "ana")
	seedSetupState(t, f)
	set, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.set.s.CheckoutPath = ""
	unset, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{set, unset} {
		low := strings.ToLower(text)
		for _, w := range []string{"http://", "https://"} {
			if strings.Contains(low, w) {
				t.Errorf("prompt holds %q", w)
			}
		}
	}
}

func TestSetupPromptProfileFolderReadErrorIsALine(t *testing.T) {
	f := newSetupFx(t, "ana")
	seedSetupState(t, f)
	ana := filepath.Join(f.root, "instructions", "people", "ana")
	if err := os.RemoveAll(ana); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), ana); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatalf("a read error must not fail the prompt: %v", err)
	}
	mustContain(t, got, "Notted could not read my per-project files")
	mustNotContain(t, got, "Projects without a file of mine")
}

func TestSetupPromptNoProfileStartsWithAName(t *testing.T) {
	f := newSetupFx(t, "")
	seedSetupState(t, f)
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, got,
		"Profile: not chosen yet",
		"start by asking me for a short profile name",
		"lowercase letters, digits and dashes, starting with a letter or digit",
		`"ana" or "ana-lima"`,
		"My person file will be instructions/people/<name>.md",
		"show it to me in full as rule 6 says, and write it only after my yes",
		"choose the profile I just named",
		"one question at a time",
	)
	mustNotContain(t, got, "- choose my profile;")
	// The chosen-profile variant does not ask for a name.
	g := newSetupFx(t, "ana")
	seedSetupState(t, g)
	set, err := g.svc.GetSetupPrompt(g.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, set, "start by asking me for a short profile name", "choose the profile I just named")
	mustContain(t, set, "- choose my profile;")
}

func TestSetupPromptFolderFailingTheCheckStopsAtSettings(t *testing.T) {
	f := newSetupFx(t, "ana")
	seedSetupState(t, f)
	if err := os.Remove(filepath.Join(f.root, "bin", "meeting")); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetSetupPrompt(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, got,
		f.root,
		"is not set up as a meetings archive yet",
		"bin/meeting is missing",
		"choose this folder again in Notted's Settings",
		"Notted sets it up",
		"Then stop.",
	)
	mustNotContain(t, got,
		"instructions/README.md",
		"one question at a time",
		"only under",
		"never commit or push without",
		"Prompt preview",
		"Projects in the archive",
	)
}

const sharedProjectRule = "A project file is shared by every profile that works on the project, so everything in it must be true and readable for all of them: the project's name, aliases, what it is about, and the issue tracker and GitHub repositories the whole team can read. Never write a local path, a folder on my machine or anything only I can open into a project file; put those in my per-project file. Before you write a project file, check each line against this rule."

const sharedExistingWork = "where its existing work lives: what the whole team can read (an issue tracker, GitHub repositories), which goes in the project file, and any folders only I can read, which go in my per-project file"

func TestSetupPromptProjectFileIsSharedAcrossProfiles(t *testing.T) {
	for _, profile := range []string{"ana", ""} {
		f := newSetupFx(t, profile)
		seedSetupState(t, f)
		got, err := f.svc.GetSetupPrompt(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		mustContain(t, got,
			"Use the headings the README gives.\n4. "+sharedProjectRule+"\n5. You must never edit prompts/base.md",
			sharedExistingWork,
			"\n6. Before you write a file, show each file to me in full",
			"\n7. You must never commit or push without my yes.",
			"\n8. When we are done, tell me to open Notted's Settings and:",
		)
		mustNotContain(t, got, "(task files, notes, folders)", "as rule 5 says")
	}
	g := newSetupFx(t, "ana")
	g.set.s.CheckoutPath = ""
	unset, err := g.svc.GetSetupPrompt(g.ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, unset, sharedProjectRule)
}
