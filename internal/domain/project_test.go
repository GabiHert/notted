package domain

import "testing"

func basePlan() ProjectDeletePlan {
	return ProjectDeletePlan{Key: "acme", Files: []string{"a", "b"}, Folders: []string{"d/"}, LocalMeetings: 2, UnsavedMeetings: 1}
}

func TestProjectDeletePlanSame(t *testing.T) {
	if !basePlan().Same(basePlan()) {
		t.Fatal("equal plans differ")
	}
	nilPlan := ProjectDeletePlan{Key: "acme"}
	empty := ProjectDeletePlan{Key: "acme", Files: []string{}, Folders: []string{}}
	if !nilPlan.Same(empty) {
		t.Fatal("nil and empty slices differ")
	}
	rec := basePlan()
	rec.Recording = true
	if !basePlan().Same(rec) {
		t.Fatal("Recording must not be compared")
	}
	cases := map[string]func(p *ProjectDeletePlan){
		"key":      func(p *ProjectDeletePlan) { p.Key = "globex" },
		"files":    func(p *ProjectDeletePlan) { p.Files = []string{"b", "a"} },
		"folders":  func(p *ProjectDeletePlan) { p.Folders = nil },
		"local":    func(p *ProjectDeletePlan) { p.LocalMeetings = 3 },
		"unsaved":  func(p *ProjectDeletePlan) { p.UnsavedMeetings = 0 },
		"fewfiles": func(p *ProjectDeletePlan) { p.Files = []string{"a"} },
	}
	for name, mod := range cases {
		q := basePlan()
		mod(&q)
		if basePlan().Same(q) {
			t.Fatalf("%s: plans compare equal", name)
		}
	}
}
