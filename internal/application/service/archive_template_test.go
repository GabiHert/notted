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
