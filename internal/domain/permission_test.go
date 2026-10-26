package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPermissionFormBasics(t *testing.T) {
	f, err := PermissionForm("Bash", json.RawMessage(`{"command":"ls"}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Claude Code wants to use Bash" || len(f.Pieces) != 2 {
		t.Fatalf("%+v", f)
	}
	p0, p1 := f.Pieces[0], f.Pieces[1]
	if p0.Kind != PieceText || p0.ID != "request" {
		t.Fatalf("%+v", p0)
	}
	wantMD := "```json\n{\n  \"command\": \"ls\"\n}\n```"
	if p0.Markdown != wantMD {
		t.Fatalf("got %q", p0.Markdown)
	}
	if p1.Kind != PieceConfirm || p1.ID != "allow" || p1.Question != "Allow this?" || p1.YesLabel != "Allow" || p1.NoLabel != "Deny" {
		t.Fatalf("%+v", p1)
	}
	raw, _ := json.Marshal(f)
	if _, err := ParseForm(raw); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionFormToolNameCut(t *testing.T) {
	name := strings.Repeat("é", 80)
	f, err := PermissionForm(name, json.RawMessage(`{}`))
	if err != nil || f.Title != "Claude Code wants to use "+name {
		t.Fatalf("%v %q", err, f.Title)
	}
	f, err = PermissionForm(strings.Repeat("é", 81), json.RawMessage(`{}`))
	if err != nil || f.Title != "Claude Code wants to use "+strings.Repeat("é", 79)+"…" {
		t.Fatalf("%v %q", err, f.Title)
	}
}

func TestPermissionFormFenceGrows(t *testing.T) {
	in := json.RawMessage("{\"c\":\"a ```` b ``` c\"}")
	f, err := PermissionForm("Write", in)
	if err != nil {
		t.Fatal(err)
	}
	md := f.Pieces[0].Markdown
	if !strings.HasPrefix(md, "`````json\n") || !strings.HasSuffix(md, "\n`````") {
		t.Fatalf("fence: %q", md)
	}
	raw, _ := json.Marshal(f)
	if _, err := ParseForm(raw); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionFormCut(t *testing.T) {
	in := json.RawMessage(`{"a":"` + strings.Repeat("é", 45000) + `"}`)
	f, err := PermissionForm("Write", in)
	if err != nil {
		t.Fatal(err)
	}
	var indented strings.Builder
	indented.WriteString("{\n  \"a\": \"" + strings.Repeat("é", 45000) + "\"\n}")
	total := len([]rune(indented.String()))
	kept := string([]rune(indented.String())[:40000])
	want := "```json\n" + kept + "\n… (cut; " + itoa(total) + " characters in all)\n```"
	if f.Pieces[0].Markdown != want {
		t.Fatalf("cut mismatch: len %d", len(f.Pieces[0].Markdown))
	}
	raw, _ := json.Marshal(f)
	if _, err := ParseForm(raw); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestPermissionFormBad(t *testing.T) {
	cases := []struct {
		tool  string
		input string
	}{
		{"", `{}`}, {"  ", `{}`}, {"Bash", `[]`}, {"Bash", `"x"`}, {"Bash", `nope`}, {"Bash", ``}, {"Bash", `null`},
	}
	for _, tc := range cases {
		_, err := PermissionForm(tc.tool, json.RawMessage(tc.input))
		if !errors.Is(err, ErrBadPermissionRequest) {
			t.Errorf("%q %q: %v", tc.tool, tc.input, err)
		}
	}
}

func TestPermissionResult(t *testing.T) {
	in := json.RawMessage(`{"command":"ls","n":1}`)
	allow := FormAnswer{Status: StatusAnswered, Answers: map[string]any{"allow": "yes"}}
	if got := PermissionResult(in, allow); got != `{"behavior":"allow","updatedInput":{"command":"ls","n":1}}` {
		t.Fatal(got)
	}
	deny := FormAnswer{Status: StatusAnswered, Answers: map[string]any{"allow": "no"}}
	if got := PermissionResult(in, deny); got != `{"behavior":"deny","message":"The person denied this in Notted."}` {
		t.Fatal(got)
	}
	deny.Note = "  too risky "
	if got := PermissionResult(in, deny); got != `{"behavior":"deny","message":"The person denied this in Notted. Their note: `+"  too risky "+`"}` {
		t.Fatal(got)
	}
	deny.Note = "   "
	if got := PermissionResult(in, deny); got != `{"behavior":"deny","message":"The person denied this in Notted."}` {
		t.Fatal(got)
	}
	dis := Dismissed("x")
	if got := PermissionResult(in, dis); got != `{"behavior":"deny","message":"The person dismissed this request in Notted (Not now). Do not try it again unless they ask."}` {
		t.Fatal(got)
	}
}

func TestPermissionDeny(t *testing.T) {
	if got := PermissionDeny("no <way>"); got != "{\"behavior\":\"deny\",\"message\":\"no \\u003cway\\u003e\"}" {
		t.Fatal(got)
	}
}
