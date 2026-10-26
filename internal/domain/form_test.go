package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const validAll = `{"title":"Pick","pieces":[
{"kind":"text","id":"intro","markdown":"# Hello"},
{"kind":"html","id":"view","html":"<p>x</p>"},
{"kind":"choice","id":"c1","question":"Which?","options":[{"label":"A","recommended":true},{"label":"B","description":"bee"}]},
{"kind":"multi","id":"m1","question":"Many?","options":[{"label":"X"},{"label":"Y"},{"label":"Z"}]},
{"kind":"input","id":"i1","question":"Name?","placeholder":"p","multiline":true,"required":true},
{"kind":"confirm","id":"ok","question":"Sure?"}
]}`

func TestParseFormValidAndRoundTrip(t *testing.T) {
	f, err := ParseForm([]byte(validAll))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Pieces) != 6 || f.Title != "Pick" {
		t.Fatalf("bad form %+v", f)
	}
	c := f.Pieces[5]
	if c.YesLabel != "Yes" || c.NoLabel != "No" {
		t.Fatalf("defaults not filled: %+v", c)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := ParseForm(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, f2) {
		t.Fatalf("round trip differs:\n%+v\n%+v", f, f2)
	}
}

func TestParseFormKeepsGivenConfirmLabels(t *testing.T) {
	f, err := ParseForm([]byte(`{"title":"t","pieces":[{"kind":"confirm","id":"a","question":"q","yes_label":"Go","no_label":"Stop"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Pieces[0].YesLabel != "Go" || f.Pieces[0].NoLabel != "Stop" {
		t.Fatalf("%+v", f.Pieces[0])
	}
}

func opts(n int) string {
	var parts []string
	for i := 0; i < n; i++ {
		parts = append(parts, `{"label":"o`+strings.Repeat("x", i)+`"}`)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func form1(piece string) string {
	return `{"title":"t","pieces":[` + piece + `]}`
}

func TestParseFormRefusals(t *testing.T) {
	manyPieces := make([]string, 21)
	for i := range manyPieces {
		manyPieces[i] = `{"kind":"text","id":"p` + strings.Repeat("a", i) + `","markdown":"m"}`
	}
	cases := []struct {
		name string
		raw  string
		want string // full reason, or prefix when prefix is true
		pref bool
	}{
		{"not json", `nope`, "the form is not valid JSON: ", true},
		{"array", `[]`, "the form is not valid JSON: ", true},
		{"unknown field", `{"title":"t","pieces":[],"x":1}`, "the form is not valid JSON: ", true},
		{"bad type", `{"title":5,"pieces":[]}`, "the form is not valid JSON: ", true},
		{"trailing", `{"title":"t","pieces":[{"kind":"text","id":"a","markdown":"m"}]} {}`, "the form is not valid JSON: ", true},
		{"empty title", `{"title":"  ","pieces":[{"kind":"text","id":"a","markdown":"m"}]}`, "title is required", false},
		{"long title", `{"title":"` + strings.Repeat("é", 201) + `","pieces":[{"kind":"text","id":"a","markdown":"m"}]}`, "title is longer than 200 characters", false},
		{"no pieces", `{"title":"t","pieces":[]}`, "a form needs 1 to 20 pieces", false},
		{"missing pieces", `{"title":"t"}`, "a form needs 1 to 20 pieces", false},
		{"21 pieces", `{"title":"t","pieces":[` + strings.Join(manyPieces, ",") + `]}`, "a form needs 1 to 20 pieces", false},
		{"bad id upper", form1(`{"kind":"text","id":"Abc","markdown":"m"}`), "piece 1: id must be 1 to 40 lowercase letters, digits, dashes or underscores, starting with a letter or digit", false},
		{"bad id empty", form1(`{"kind":"text","markdown":"m"}`), "piece 1: id must be 1 to 40 lowercase letters, digits, dashes or underscores, starting with a letter or digit", false},
		{"bad id dash start", form1(`{"kind":"text","id":"-a","markdown":"m"}`), "piece 1: id must be 1 to 40 lowercase letters, digits, dashes or underscores, starting with a letter or digit", false},
		{"bad id long", form1(`{"kind":"text","id":"` + strings.Repeat("a", 41) + `","markdown":"m"}`), "piece 1: id must be 1 to 40 lowercase letters, digits, dashes or underscores, starting with a letter or digit", false},
		{"dup id", `{"title":"t","pieces":[{"kind":"text","id":"a","markdown":"m"},{"kind":"text","id":"a","markdown":"m"}]}`, `piece "a": the id is used twice`, false},
		{"unknown kind", form1(`{"kind":"slider","id":"a"}`), `piece "a": unknown kind "slider"; use text, html, choice, multi, input or confirm`, false},
		{"text with html", form1(`{"kind":"text","id":"a","markdown":"m","html":"h"}`), `piece "a": html is not allowed for kind text`, false},
		{"text with question", form1(`{"kind":"text","id":"a","markdown":"m","question":"q"}`), `piece "a": question is not allowed for kind text`, false},
		{"html with markdown", form1(`{"kind":"html","id":"a","html":"h","markdown":"m"}`), `piece "a": markdown is not allowed for kind html`, false},
		{"choice with placeholder", form1(`{"kind":"choice","id":"a","question":"q","options":` + opts(2) + `,"placeholder":"p"}`), `piece "a": placeholder is not allowed for kind choice`, false},
		{"multi with multiline", form1(`{"kind":"multi","id":"a","question":"q","options":` + opts(2) + `,"multiline":true}`), `piece "a": multiline is not allowed for kind multi`, false},
		{"input with options", form1(`{"kind":"input","id":"a","question":"q","options":` + opts(2) + `}`), `piece "a": options is not allowed for kind input`, false},
		{"choice with required", form1(`{"kind":"choice","id":"a","question":"q","options":` + opts(2) + `,"required":true}`), `piece "a": required is not allowed for kind choice`, false},
		{"input with yes_label", form1(`{"kind":"input","id":"a","question":"q","yes_label":"y"}`), `piece "a": yes_label is not allowed for kind input`, false},
		{"choice with no_label", form1(`{"kind":"choice","id":"a","question":"q","options":` + opts(2) + `,"no_label":"n"}`), `piece "a": no_label is not allowed for kind choice`, false},
		{"confirm with options", form1(`{"kind":"confirm","id":"a","question":"q","options":` + opts(2) + `}`), `piece "a": options is not allowed for kind confirm`, false},
		{"markdown empty", form1(`{"kind":"text","id":"a"}`), `piece "a": markdown is required`, false},
		{"markdown long", form1(`{"kind":"text","id":"a","markdown":"` + strings.Repeat("é", 50001) + `"}`), `piece "a": markdown is longer than 50000 characters`, false},
		{"html empty", form1(`{"kind":"html","id":"a"}`), `piece "a": html is required`, false},
		{"html big", form1(`{"kind":"html","id":"a","html":"` + strings.Repeat("a", 200001) + `"}`), `piece "a": html is larger than 200000 bytes`, false},
		{"question empty", form1(`{"kind":"input","id":"a","question":" "}`), `piece "a": question is required`, false},
		{"question long", form1(`{"kind":"confirm","id":"a","question":"` + strings.Repeat("a", 501) + `"}`), `piece "a": question is longer than 500 characters`, false},
		{"choice 1 option", form1(`{"kind":"choice","id":"a","question":"q","options":` + opts(1) + `}`), `piece "a": choice needs 2 to 10 options`, false},
		{"choice 11 options", form1(`{"kind":"choice","id":"a","question":"q","options":` + opts(11) + `}`), `piece "a": choice needs 2 to 10 options`, false},
		{"choice no options", form1(`{"kind":"choice","id":"a","question":"q"}`), `piece "a": choice needs 2 to 10 options`, false},
		{"multi 0 options", form1(`{"kind":"multi","id":"a","question":"q","options":[]}`), `piece "a": multi needs 1 to 50 options`, false},
		{"multi 51 options", form1(`{"kind":"multi","id":"a","question":"q","options":` + opts(51) + `}`), `piece "a": multi needs 1 to 50 options`, false},
		{"option no label", form1(`{"kind":"choice","id":"a","question":"q","options":[{"label":"A"},{"label":" "}]}`), `piece "a": option 2 needs a label`, false},
		{"option long label", form1(`{"kind":"choice","id":"a","question":"q","options":[{"label":"A"},{"label":"` + strings.Repeat("b", 121) + `"}]}`), `piece "a": option label "` + strings.Repeat("b", 121) + `" is longer than 120 characters`, false},
		{"option dup label", form1(`{"kind":"choice","id":"a","question":"q","options":[{"label":"A"},{"label":"A"}]}`), `piece "a": option label "A" is used twice`, false},
		{"option long desc", form1(`{"kind":"choice","id":"a","question":"q","options":[{"label":"A"},{"label":"B","description":"` + strings.Repeat("d", 1001) + `"}]}`), `piece "a": option "B" has a description longer than 1000 characters`, false},
		{"multi recommended", form1(`{"kind":"multi","id":"a","question":"q","options":[{"label":"A","recommended":false}]}`), `piece "a": recommended is not allowed for kind multi`, false},
		{"two recommended", form1(`{"kind":"choice","id":"a","question":"q","options":[{"label":"A","recommended":true},{"label":"B","recommended":true}]}`), `piece "a": only one option can be recommended`, false},
		{"placeholder long", form1(`{"kind":"input","id":"a","question":"q","placeholder":"` + strings.Repeat("p", 201) + `"}`), `piece "a": placeholder is longer than 200 characters`, false},
		{"yes_label long", form1(`{"kind":"confirm","id":"a","question":"q","yes_label":"` + strings.Repeat("y", 41) + `"}`), `piece "a": yes_label is longer than 40 characters`, false},
		{"no_label long", form1(`{"kind":"confirm","id":"a","question":"q","no_label":"` + strings.Repeat("n", 41) + `"}`), `piece "a": no_label is longer than 40 characters`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseForm([]byte(tc.raw))
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrInvalidForm) {
				t.Fatalf("not ErrInvalidForm: %v", err)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "invalid form: ") {
				t.Fatalf("bad prefix: %q", msg)
			}
			reason := strings.TrimPrefix(msg, "invalid form: ")
			if tc.pref {
				if !strings.HasPrefix(reason, tc.want) {
					t.Fatalf("got %q want prefix %q", reason, tc.want)
				}
			} else if reason != tc.want {
				t.Fatalf("got %q want %q", reason, tc.want)
			}
		})
	}
}

func TestParseFormTooBig(t *testing.T) {
	raw := []byte(`{"title":"` + strings.Repeat("a", 1048576) + `"}`)
	_, err := ParseForm(raw)
	if err == nil || err.Error() != "invalid form: the form is larger than 1 MB" {
		t.Fatalf("got %v", err)
	}
}

func TestParseFormBoundariesAccepted(t *testing.T) {
	raw := `{"title":"` + strings.Repeat("é", 200) + `","pieces":[` +
		`{"kind":"text","id":"` + strings.Repeat("a", 40) + `","markdown":"` + strings.Repeat("é", 50000) + `"},` +
		`{"kind":"html","id":"h","html":"` + strings.Repeat("a", 200000) + `"},` +
		`{"kind":"choice","id":"c","question":"q","options":` + opts(10) + `},` +
		`{"kind":"multi","id":"m","question":"q","options":` + opts(50) + `}]}`
	if _, err := ParseForm([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestAnswerable(t *testing.T) {
	for k, want := range map[PieceKind]bool{PieceText: false, PieceHTML: false, PieceChoice: true, PieceMulti: true, PieceInput: true, PieceConfirm: true} {
		if got := (Piece{Kind: k}).Answerable(); got != want {
			t.Errorf("%s: %v", k, got)
		}
	}
}

func answerForm(t *testing.T) Form {
	t.Helper()
	f, err := ParseForm([]byte(validAll))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestNewAnswerNormalization(t *testing.T) {
	f := answerForm(t)
	a, err := NewAnswer(f, map[string]any{
		"c1": "B",
		"m1": []any{"Z", "X"},
		"i1": "  hi ",
		"ok": "yes",
	}, "note")
	if err != nil {
		t.Fatal(err)
	}
	want := FormAnswer{Status: StatusAnswered, Note: "note", Answers: map[string]any{
		"c1": "B", "m1": []string{"X", "Z"}, "i1": "  hi ", "ok": "yes",
	}}
	if !reflect.DeepEqual(a, want) {
		t.Fatalf("got %#v", a)
	}
	got := EncodeAnswer(a)
	if got != `{"status":"answered","answers":{"c1":"B","i1":"  hi ","m1":["X","Z"],"ok":"yes"},"note":"note"}` {
		t.Fatal(got)
	}
}

func TestNewAnswerStringSliceAndDefaults(t *testing.T) {
	f := answerForm(t)
	a, err := NewAnswer(f, map[string]any{"c1": "A", "m1": []string{"Y"}, "i1": "x", "ok": "no"}, "")
	if err != nil || !reflect.DeepEqual(a.Answers["m1"], []string{"Y"}) || a.Answers["ok"] != "no" {
		t.Fatalf("%v %#v", err, a)
	}
	// missing multi -> [], missing optional input -> ""
	f2, _ := ParseForm([]byte(form1(`{"kind":"multi","id":"m","question":"q","options":` + opts(2) + `}`)))
	a, err = NewAnswer(f2, nil, "")
	if err != nil || !reflect.DeepEqual(a.Answers["m"], []string{}) {
		t.Fatalf("%v %#v", err, a)
	}
	f3, _ := ParseForm([]byte(form1(`{"kind":"input","id":"i","question":"q"}`)))
	a, err = NewAnswer(f3, map[string]any{}, "")
	if err != nil || a.Answers["i"] != "" {
		t.Fatalf("%v %#v", err, a)
	}
	if EncodeAnswer(a) != `{"status":"answered","answers":{"i":""},"note":""}` {
		t.Fatal(EncodeAnswer(a))
	}
}

func TestNewAnswerErrors(t *testing.T) {
	f := answerForm(t)
	good := func() map[string]any {
		return map[string]any{"c1": "A", "m1": []string{"X"}, "i1": "t", "ok": "yes"}
	}
	with := func(k string, v any) map[string]any {
		m := good()
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name string
		ans  map[string]any
		note string
		want string
	}{
		{"unknown id", with("zzz", "x"), "", "The form has no question with the id zzz."},
		{"text id", with("intro", "x"), "", "The form has no question with the id intro."},
		{"choice wrong type", with("c1", 5), "", `The answer to "Which?" has the wrong type.`},
		{"choice slice type", with("c1", []string{"A"}), "", `The answer to "Which?" has the wrong type.`},
		{"multi wrong type", with("m1", "X"), "", `The answer to "Many?" has the wrong type.`},
		{"multi any non-string", with("m1", []any{1}), "", `The answer to "Many?" has the wrong type.`},
		{"input wrong type", with("i1", []string{"a"}), "", `The answer to "Name?" has the wrong type.`},
		{"confirm wrong type", with("ok", true), "", `The answer to "Sure?" has the wrong type.`},
		{"choice missing", with("c1", nil), "", `Choose an option for "Which?".`},
		{"choice empty", with("c1", ""), "", `Choose an option for "Which?".`},
		{"choice not option", with("c1", "Q"), "", `The answer to "Which?" is not one of its options.`},
		{"multi not option", with("m1", []string{"Q"}), "", `The answer to "Many?" is not one of its options.`},
		{"multi twice", with("m1", []string{"X", "X"}), "", `The answer to "Many?" lists an option twice.`},
		{"confirm missing", with("ok", nil), "", `Answer yes or no to "Sure?".`},
		{"confirm bad", with("ok", "maybe"), "", `The answer to "Sure?" must be yes or no.`},
		{"input required empty", with("i1", "  "), "", `Fill in "Name?".`},
		{"input required missing", with("i1", nil), "", `Fill in "Name?".`},
		{"note long", good(), strings.Repeat("é", 5001), "The note is longer than 5000 characters."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAnswer(f, tc.ans, tc.note)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("got %v want %q", err, tc.want)
			}
		})
	}
	if _, err := NewAnswer(f, good(), strings.Repeat("é", 5000)); err != nil {
		t.Fatal(err)
	}
}

func TestDismissedAndEncode(t *testing.T) {
	d := Dismissed("later")
	if d.Status != StatusDismissed || d.Answers == nil || len(d.Answers) != 0 || d.Note != "later" {
		t.Fatalf("%#v", d)
	}
	if got := EncodeAnswer(Dismissed("")); got != `{"status":"dismissed","answers":{},"note":""}` {
		t.Fatal(got)
	}
	if got := EncodeAnswer(FormAnswer{Status: StatusDismissed}); got != `{"status":"dismissed","answers":{},"note":""}` {
		t.Fatal(got)
	}
	if got := EncodeAnswer(FormAnswer{Status: StatusAnswered, Answers: map[string]any{"a": []string{}}}); !strings.Contains(got, `"a":[]`) {
		t.Fatal(got)
	}
}
