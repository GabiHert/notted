package wails

import (
	"context"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/application/service"
	"github.com/GabiHert/notted/internal/domain"
)

const formsTestMeeting = "11111111-1111-4111-8111-111111111111"

type formsStore struct{ port.MeetingStore }

func (formsStore) GetMeeting(_ context.Context, id string) (domain.Meeting, error) {
	return domain.Meeting{ID: id, Title: "Weekly"}, nil
}

func newFormsApp(t *testing.T) (*App, *service.FormService, string) {
	t.Helper()
	forms := service.NewFormService(service.FormDeps{Meetings: formsStore{}, Emit: nopEmitter{}, SessionsDir: t.TempDir(), Executable: "/x", NewToken: func() string { return "tok" }})
	if _, err := forms.SessionConfig(formsTestMeeting); err != nil {
		t.Fatal(err)
	}
	return New(&Emitter{}, Deps{Forms: forms, Visibility: &Visibility{}}), forms, "tok"
}

func askAsync(forms *service.FormService, tok string, f domain.Form) <-chan domain.FormAnswer {
	ch := make(chan domain.FormAnswer, 1)
	go func() {
		a, _ := forms.Handle(context.Background(), port.FormRequest{MeetingID: formsTestMeeting, Token: tok, Kind: domain.FormAsk, Form: f})
		ch <- a
	}()
	return ch
}

func waitOpenForms(t *testing.T, a *App, n int) []OpenFormDTO {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := a.ListOpenForms(); len(got) == n {
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("no %d open forms", n)
	return nil
}

func TestListOpenFormsEmptyIsNotNil(t *testing.T) {
	a, _, _ := newFormsApp(t)
	if got := a.ListOpenForms(); got == nil || len(got) != 0 {
		t.Fatalf("%#v", got)
	}
}

func TestOpenFormDTOMapping(t *testing.T) {
	a, forms, tok := newFormsApp(t)
	form := domain.Form{Title: "Pick", Pieces: []domain.Piece{
		{Kind: domain.PieceText, ID: "intro", Markdown: "# Hi"},
		{Kind: domain.PieceChoice, ID: "c", Question: "Which?", Options: []domain.Option{{Label: "A", Description: "a", Recommended: true}, {Label: "B"}}},
		{Kind: domain.PieceConfirm, ID: "ok", Question: "Sure?", YesLabel: "Go", NoLabel: "Stop"},
		{Kind: domain.PieceInput, ID: "i", Question: "Name?", Placeholder: "p", Multiline: true, Required: true},
	}}
	askAsync(forms, tok, form)
	got := waitOpenForms(t, a, 1)[0]
	if got.MeetingID != formsTestMeeting || got.Kind != "ask" || got.Title != "Pick" || got.Queued != 0 || got.ID == "" || len(got.Pieces) != 4 {
		t.Fatalf("%+v", got)
	}
	if p := got.Pieces[0]; p.Kind != "text" || p.Markdown != "# Hi" || p.Options == nil || len(p.Options) != 0 {
		t.Fatalf("%+v", p)
	}
	if o := got.Pieces[1].Options; len(o) != 2 || o[0] != (FormOptionDTO{Label: "A", Description: "a", Recommended: true}) {
		t.Fatalf("%+v", o)
	}
	if p := got.Pieces[2]; p.YesLabel != "Go" || p.NoLabel != "Stop" || p.Question != "Sure?" {
		t.Fatalf("%+v", p)
	}
	if p := got.Pieces[3]; !p.Multiline || !p.Required || p.Placeholder != "p" {
		t.Fatalf("%+v", p)
	}
	forms.CloseAll()
}

func TestAnswerFormPassesListsThrough(t *testing.T) {
	a, forms, tok := newFormsApp(t)
	form := domain.Form{Title: "t", Pieces: []domain.Piece{
		{Kind: domain.PieceMulti, ID: "m", Question: "q", Options: []domain.Option{{Label: "X"}, {Label: "Y"}}},
	}}
	ch := askAsync(forms, tok, form)
	f := waitOpenForms(t, a, 1)[0]
	if err := a.AnswerForm(f.ID, map[string]interface{}{"m": []interface{}{"Y", "X"}}, "n"); err != nil {
		t.Fatal(err)
	}
	ans := <-ch
	got, _ := ans.Answers["m"].([]string)
	if ans.Status != domain.StatusAnswered || len(got) != 2 || got[0] != "X" || got[1] != "Y" {
		t.Fatalf("%+v", ans)
	}
}

func TestAnswerFormAndDismissFormErrors(t *testing.T) {
	a, forms, tok := newFormsApp(t)
	if err := a.AnswerForm("nope", nil, ""); err == nil || err.Error() != "This form is no longer open." {
		t.Fatalf("%v", err)
	}
	ch := askAsync(forms, tok, domain.Form{Title: "t", Pieces: []domain.Piece{{Kind: domain.PieceText, ID: "a", Markdown: "m"}}})
	f := waitOpenForms(t, a, 1)[0]
	if err := a.DismissForm(f.ID, "later"); err != nil {
		t.Fatal(err)
	}
	if ans := <-ch; ans.Status != domain.StatusDismissed || ans.Note != "later" {
		t.Fatalf("%+v", ans)
	}
	if err := a.DismissForm(f.ID, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetWindowVisibleFlipsVisibility(t *testing.T) {
	v := &Visibility{}
	a := New(&Emitter{}, Deps{Visibility: v})
	if !v.Visible() {
		t.Fatal("zero value must be visible")
	}
	a.SetWindowVisible(false)
	if v.Visible() {
		t.Fatal("still visible")
	}
	a.SetWindowVisible(true)
	if !v.Visible() {
		t.Fatal("still hidden")
	}
}
