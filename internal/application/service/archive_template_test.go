package service

import (
	"strings"
	"testing"
)

func TestTemplateGuideExplainsDeletingAProject(t *testing.T) {
	b, err := archiveTemplate.ReadFile("archive_template/instructions/README.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"\n## Deleting a project\n",
		"project(<key>): delete",
		"instructions/people/<other>/<key>.md",
		"gone for good",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the template guide lacks %q", want)
		}
	}
	del, lim := strings.Index(s, "\n## Deleting a project\n"), strings.Index(s, "\n## Limits\n")
	if del < 0 || lim < 0 || del > lim {
		t.Errorf("## Deleting a project must come before ## Limits (at %d and %d)", del, lim)
	}
}

// squashSpace joins every run of white space into one space, so a check survives rewrapping.
func squashSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestTemplateGuideSeparatesSharedAndPrivateSources(t *testing.T) {
	file := func(p string) string {
		t.Helper()
		b, err := archiveTemplate.ReadFile("archive_template/" + p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	guide := file("instructions/README.md")
	for _, want := range []string{
		"\n- `## Existing work`: where this project's existing tasks live, naming only what every teammate can read: the issue tracker (team and identifier), GitHub repositories (`owner/repo` and the folder in it), and free text. One bullet per source, with the identifier it uses. May say \"None\"; then every suggestion is `new`. This file is shared with the whole team, so never name a local path or anything only you can open; put those in your per-project file.\n",
		"\n## Existing work\n- GitHub `acme/dashboard`: one issue per task, identified by `#<n>`.\n",
		"\n- `## Tools`: tools the person uses for this project.\n- `## Existing work`: sources only you can read, as paths relative to your `## Project root`, with the identifier each uses. Claude reads them together with the project file's.\n- `## On accept`:",
		"\n## Tools\nTask files under `tasks/`.\n\n## Existing work\n- `tasks/*.md`: one file per task, identified by its file name.\n\n## On accept\n",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("the template guide lacks %q", want)
		}
	}
	if strings.Contains(guide, "relative to the person's project root") {
		t.Error("the template guide still puts local paths in the project file")
	}
	person, private := strings.Index(guide, "\n## Per-project person file\n"), strings.Index(guide, "- `tasks/*.md`")
	if person < 0 || private < person {
		t.Errorf("the tasks/*.md example must sit in the per-project person file (at %d and %d)", person, private)
	}
	base := squashSpace(file("prompts/base.md"))
	if want := "The project file names only sources the whole team can read; the person's per-project file adds the sources only that person can read, as paths under their project root."; !strings.Contains(base, want) {
		t.Errorf("the base prompt lacks %q", want)
	}
	skill := squashSpace(file(".claude/skills/meetings/SKILL.md"))
	if want := "(and the `## Existing work` section of the per-project person file, if it has one)"; !strings.Contains(skill, want) {
		t.Errorf("the skill lacks %q", want)
	}
	if strings.Contains(skill, "if it adds any") {
		t.Error("the skill keeps the old suggest step 3 wording")
	}
}
