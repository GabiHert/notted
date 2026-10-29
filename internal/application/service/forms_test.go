package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

const (
	testMeeting  = "11111111-1111-4111-8111-111111111111"
	testMeeting2 = "22222222-2222-4222-8222-222222222222"
	msgNoToken   = "Notted did not recognize this session. Start it again from Notted."
)

type formStore struct {
	port.MeetingStore
	meetings map[string]domain.Meeting
	err      error
}

func (s *formStore) GetMeeting(_ context.Context, id string) (domain.Meeting, error) {
	if s.err != nil {
		return domain.Meeting{}, s.err
	}
	m, ok := s.meetings[id]
	if !ok {
		return domain.Meeting{}, domain.ErrNotFound
	}
	return m, nil
}

type formEvent struct {
	name    string
	payload any
}

type formEmitter struct {
	mu     sync.Mutex
	events []formEvent
}

func (e *formEmitter) Emit(name string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, formEvent{name, payload})
}

func (e *formEmitter) count(name string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, ev := range e.events {
		if ev.name == name {
			n++
		}
	}
	return n
}

type fakeNotifier struct {
	mu      sync.Mutex
	sent    []port.Notification
	removed []string
	err     error
}

func (n *fakeNotifier) Notify(p port.Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, p)
	return n.err
}

func (n *fakeNotifier) Remove(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.removed = append(n.removed, id)
}

func (n *fakeNotifier) snapshot() ([]port.Notification, []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]port.Notification(nil), n.sent...), append([]string(nil), n.removed...)
}

type formRig struct {
	svc      *FormService
	emit     *formEmitter
	notify   *fakeNotifier
	store    *formStore
	visible  bool
	visMu    sync.Mutex
	sessions string
}

func (r *formRig) setVisible(v bool) {
	r.visMu.Lock()
	r.visible = v
	r.visMu.Unlock()
}

func newFormRig(t *testing.T) *formRig {
	t.Helper()
	r := &formRig{emit: &formEmitter{}, notify: &fakeNotifier{}, visible: true, sessions: filepath.Join(t.TempDir(), "sessions")}
	r.store = &formStore{meetings: map[string]domain.Meeting{
		testMeeting:  {ID: testMeeting, Title: "Weekly sync"},
		testMeeting2: {ID: testMeeting2, Title: "Retro"},
	}}
	tokN, idN := 0, 0
	r.svc = NewFormService(FormDeps{
		Meetings: r.store, Emit: r.emit, Notify: r.notify,
		Visible: func() bool {
			r.visMu.Lock()
			defer r.visMu.Unlock()
			return r.visible
		},
		SessionsDir: r.sessions,
		Executable:  "/Applications/Notted.app/Contents/MacOS/Notted",
		Socket:      "/tmp/ask.sock",
		NewToken:    func() string { tokN++; return fmt.Sprintf("tok%d", tokN) },
		NewID:       func() string { idN++; return fmt.Sprintf("f%d", idN) },
	})
	return r
}

func (r *formRig) token(t *testing.T, id string) string {
	t.Helper()
	if _, err := r.svc.SessionConfig(id); err != nil {
		t.Fatal(err)
	}
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	return r.svc.tokens[id]
}

func askForm(title string) domain.Form {
	return domain.Form{Title: title, Pieces: []domain.Piece{
		{Kind: domain.PieceChoice, ID: "c", Question: "Which?", Options: []domain.Option{{Label: "A"}, {Label: "B"}}},
	}}
}

type handleResult struct {
	ans domain.FormAnswer
	err error
}

func (r *formRig) handleAsync(ctx context.Context, meeting, token, title string) <-chan handleResult {
	ch := make(chan handleResult, 1)
	go func() {
		a, err := r.svc.Handle(ctx, port.FormRequest{MeetingID: meeting, Token: token, Kind: domain.FormAsk, Form: askForm(title)})
		ch <- handleResult{a, err}
	}()
	return ch
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// lastChanged returns the meeting id of the most recent ask:changed event.
func (e *formEmitter) lastChanged() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := len(e.events) - 1; i >= 0; i-- {
		if e.events[i].name == "ask:changed" {
			p, _ := e.events[i].payload.(map[string]any)
			id, _ := p["meetingId"].(string)
			return id
		}
	}
	return ""
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (r *formRig) waitOpen(t *testing.T, n int) []OpenForm {
	t.Helper()
	waitFor(t, fmt.Sprintf("%d open forms", n), func() bool { return len(r.svc.Open()) == n })
	return r.svc.Open()
}

func waitQueued(t *testing.T, r *formRig, meeting string, queued int) {
	t.Helper()
	waitFor(t, "queue length", func() bool {
		for _, f := range r.svc.Open() {
			if f.MeetingID == meeting && f.Queued == queued {
				return true
			}
		}
		return false
	})
}

func recv(t *testing.T, ch <-chan handleResult) handleResult {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("Handle did not return")
		return handleResult{}
	}
}

func TestHandleRejectsUnknownAndWrongToken(t *testing.T) {
	r := newFormRig(t)
	_, err := r.svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: "x", Form: askForm("t")})
	if err == nil || err.Error() != msgNoToken {
		t.Fatalf("unknown: %v", err)
	}
	r.token(t, testMeeting)
	_, err = r.svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: "wrong", Form: askForm("t")})
	if err == nil || err.Error() != msgNoToken {
		t.Fatalf("wrong: %v", err)
	}
}

func TestHandleMeetingGone(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	delete(r.store.meetings, testMeeting)
	_, err := r.svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: tok, Form: askForm("t")})
	if err == nil || err.Error() != "This meeting no longer exists in Notted." {
		t.Fatalf("%v", err)
	}
	boom := errors.New("db down")
	r.store.err = boom
	_, err = r.svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: tok, Form: askForm("t")})
	if !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}

func TestHandleRechecksForm(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	_, err := r.svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: tok, Form: domain.Form{Title: "t"}})
	if !errors.Is(err, domain.ErrInvalidForm) {
		t.Fatalf("%v", err)
	}
}

func TestOneFormAnswered(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	ch := r.handleAsync(context.Background(), testMeeting, tok, "First")
	open := r.waitOpen(t, 1)
	if open[0].MeetingID != testMeeting || open[0].Kind != domain.FormAsk || open[0].Form.Title != "First" || open[0].Queued != 0 || open[0].ID != "f1" {
		t.Fatalf("%+v", open[0])
	}
	if err := r.svc.Answer("f1", map[string]any{"c": "B"}, "ok"); err != nil {
		t.Fatal(err)
	}
	got := recv(t, ch)
	if got.err != nil || got.ans.Status != domain.StatusAnswered || got.ans.Answers["c"] != "B" || got.ans.Note != "ok" {
		t.Fatalf("%+v", got)
	}
	if len(r.svc.Open()) != 0 {
		t.Fatal("form still open")
	}
	if r.emit.count("ask:changed") != 2 {
		t.Fatalf("events: %d", r.emit.count("ask:changed"))
	}
	r.emit.mu.Lock()
	defer r.emit.mu.Unlock()
	p, _ := r.emit.events[0].payload.(map[string]any)
	if r.emit.events[0].name != "ask:changed" || p["meetingId"] != testMeeting {
		t.Fatalf("%+v", r.emit.events[0])
	}
}

func TestSecondFormWaitsInFIFO(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	ch1 := r.handleAsync(context.Background(), testMeeting, tok, "One")
	r.waitOpen(t, 1)
	waitFor(t, "one event for the first form", func() bool { return r.emit.count("ask:changed") == 1 })
	ch2 := r.handleAsync(context.Background(), testMeeting, tok, "Two")
	waitQueued(t, r, testMeeting, 1)
	waitFor(t, "one event for the queued form", func() bool { return r.emit.count("ask:changed") == 2 })
	if r.emit.lastChanged() != testMeeting {
		t.Fatalf("wrong meeting id %q", r.emit.lastChanged())
	}
	open := r.svc.Open()
	if len(open) != 1 || open[0].Form.Title != "One" {
		t.Fatalf("%+v", open)
	}
	if err := r.svc.Dismiss("f1", "later"); err != nil {
		t.Fatal(err)
	}
	if n := r.emit.count("ask:changed"); n != 3 {
		t.Fatalf("closing the head must emit exactly one event, total %d", n)
	}
	if got := recv(t, ch1); got.ans.Status != domain.StatusDismissed || got.ans.Note != "later" {
		t.Fatalf("%+v", got)
	}
	waitQueued(t, r, testMeeting, 0)
	open = r.svc.Open()
	if open[0].Form.Title != "Two" || open[0].ID != "f2" {
		t.Fatalf("%+v", open)
	}
	if err := r.svc.Answer("f2", map[string]any{"c": "A"}, ""); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, ch2); got.ans.Status != domain.StatusAnswered {
		t.Fatalf("%+v", got)
	}
}

func TestTwoMeetingsEachHaveAnOpenForm(t *testing.T) {
	r := newFormRig(t)
	t1, t2 := r.token(t, testMeeting), r.token(t, testMeeting2)
	r.handleAsync(context.Background(), testMeeting, t1, "A")
	r.waitOpen(t, 1)
	r.handleAsync(context.Background(), testMeeting2, t2, "B")
	open := r.waitOpen(t, 2)
	if open[0].Form.Title != "A" || open[1].Form.Title != "B" {
		t.Fatalf("not oldest first: %+v", open)
	}
	r.svc.CloseAll()
}

func TestAnswerInvalidKeepsFormOpen(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	r.handleAsync(context.Background(), testMeeting, tok, "One")
	r.waitOpen(t, 1)
	_, want := domain.NewAnswer(askForm("One"), map[string]any{}, "")
	if want == nil {
		t.Fatal("expected the domain to refuse an empty answer")
	}
	err := r.svc.Answer("f1", map[string]any{}, "")
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("got %v want %v", err, want)
	}
	if len(r.svc.Open()) != 1 {
		t.Fatal("form closed")
	}
	r.svc.CloseAll()
}

func TestAnswerAndDismissUnknownID(t *testing.T) {
	r := newFormRig(t)
	for _, err := range []error{r.svc.Answer("nope", nil, ""), r.svc.Dismiss("nope", "")} {
		if err == nil || err.Error() != "This form is no longer open." {
			t.Fatalf("%v", err)
		}
	}
}

func TestAnswerQueuedFormIsNotOpen(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	r.handleAsync(context.Background(), testMeeting, tok, "One")
	r.waitOpen(t, 1)
	r.handleAsync(context.Background(), testMeeting, tok, "Two")
	waitQueued(t, r, testMeeting, 1)
	if err := r.svc.Dismiss("f2", ""); err == nil || err.Error() != "This form is no longer open." {
		t.Fatalf("%v", err)
	}
	r.svc.CloseAll()
}

func TestDismissNoteLimit(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	ch := r.handleAsync(context.Background(), testMeeting, tok, "One")
	r.waitOpen(t, 1)
	err := r.svc.Dismiss("f1", strings.Repeat("é", 5001))
	if err == nil || err.Error() != "The note is longer than 5000 characters." {
		t.Fatalf("%v", err)
	}
	if err := r.svc.Dismiss("f1", strings.Repeat("é", 5000)); err != nil {
		t.Fatal(err)
	}
	recv(t, ch)
}

func TestRequesterCancelOnHeadOpensNext(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	ctx, cancel := context.WithCancel(context.Background())
	ch1 := r.handleAsync(ctx, testMeeting, tok, "One")
	r.waitOpen(t, 1)
	r.handleAsync(context.Background(), testMeeting, tok, "Two")
	waitQueued(t, r, testMeeting, 1)
	cancel()
	if got := recv(t, ch1); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("%+v", got)
	}
	waitQueued(t, r, testMeeting, 0)
	if open := r.svc.Open(); open[0].Form.Title != "Two" {
		t.Fatalf("%+v", open)
	}
	_, removed := r.notify.snapshot()
	if !containsString(removed, "ask-f1") {
		t.Fatalf("removed %v, want ask-f1", removed)
	}
	r.svc.CloseAll()
}

func TestRequesterCancelOnQueuedRemovesOnlyIt(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	r.handleAsync(context.Background(), testMeeting, tok, "One")
	r.waitOpen(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	ch2 := r.handleAsync(ctx, testMeeting, tok, "Two")
	waitQueued(t, r, testMeeting, 1)
	waitFor(t, "events settled", func() bool { return r.emit.count("ask:changed") == 2 })
	cancel()
	recv(t, ch2)
	waitQueued(t, r, testMeeting, 0)
	if n := r.emit.count("ask:changed"); n != 3 || r.emit.lastChanged() != testMeeting {
		t.Fatalf("cancelling a queued form must emit exactly one event for its meeting, total %d", n)
	}
	if open := r.svc.Open(); len(open) != 1 || open[0].Form.Title != "One" {
		t.Fatalf("%+v", open)
	}
	r.svc.CloseAll()
}

func TestRequesterGoneAfterAnswerReopensNothing(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)
	ctx, cancel := context.WithCancel(context.Background())
	ch := r.handleAsync(ctx, testMeeting, tok, "One")
	r.waitOpen(t, 1)
	r.handleAsync(context.Background(), testMeeting, tok, "Two")
	waitQueued(t, r, testMeeting, 1)
	if err := r.svc.Dismiss("f1", ""); err != nil {
		t.Fatal(err)
	}
	cancel()
	recv(t, ch)
	waitQueued(t, r, testMeeting, 0)
	if open := r.svc.Open(); len(open) != 1 || open[0].ID != "f2" {
		t.Fatalf("%+v", open)
	}
	r.svc.CloseAll()
}

func TestCloseAll(t *testing.T) {
	r := newFormRig(t)
	t1, t2 := r.token(t, testMeeting), r.token(t, testMeeting2)
	r.setVisible(false)
	c1 := r.handleAsync(context.Background(), testMeeting, t1, "One")
	r.waitOpen(t, 1)
	c2 := r.handleAsync(context.Background(), testMeeting, t1, "Two")
	waitQueued(t, r, testMeeting, 1)
	c3 := r.handleAsync(context.Background(), testMeeting2, t2, "Three")
	r.waitOpen(t, 2)
	before := r.emit.count("ask:changed")
	r.svc.CloseAll()
	for _, c := range []<-chan handleResult{c1, c2, c3} {
		if got := recv(t, c); got.err != nil || got.ans.Status != domain.StatusDismissed {
			t.Fatalf("%+v", got)
		}
	}
	if len(r.svc.Open()) != 0 {
		t.Fatal("still open")
	}
	if d := r.emit.count("ask:changed") - before; d != 2 {
		t.Fatalf("want one event per meeting, got %d", d)
	}
	_, removed := r.notify.snapshot()
	if len(removed) != 2 {
		t.Fatalf("removed %v", removed)
	}
	got, err := r.svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: t1, Form: askForm("late")})
	if err != nil || got.Status != domain.StatusDismissed {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestNotificationOnlyWhenHiddenAndRemovedOnEveryClose(t *testing.T) {
	r := newFormRig(t)
	tok := r.token(t, testMeeting)

	// visible: nothing sent
	r.handleAsync(context.Background(), testMeeting, tok, "Vis")
	r.waitOpen(t, 1)
	if sent, _ := r.notify.snapshot(); len(sent) != 0 {
		t.Fatalf("sent while visible: %v", sent)
	}
	if err := r.svc.Dismiss("f1", ""); err != nil {
		t.Fatal(err)
	}

	// hidden: one per opening, with the exact fields
	r.setVisible(false)
	ch := r.handleAsync(context.Background(), testMeeting, tok, "Hidden")
	r.waitOpen(t, 1)
	sent, _ := r.notify.snapshot()
	want := port.Notification{ID: "ask-f2", Title: "Waiting for you", Subtitle: "Weekly sync", Body: "Hidden", MeetingID: testMeeting}
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("%+v", sent)
	}
	// a queued form does not notify until it opens
	ch2 := r.handleAsync(context.Background(), testMeeting, tok, "Next")
	waitQueued(t, r, testMeeting, 1)
	if sent, _ := r.notify.snapshot(); len(sent) != 1 {
		t.Fatalf("queued form notified: %v", sent)
	}
	if err := r.svc.Answer("f2", map[string]any{"c": "A"}, ""); err != nil {
		t.Fatal(err)
	}
	recv(t, ch)
	waitQueued(t, r, testMeeting, 0)
	sent, removed := r.notify.snapshot()
	if len(sent) != 2 || sent[1].ID != "ask-f3" || len(removed) == 0 || removed[len(removed)-1] != "ask-f2" {
		t.Fatalf("%+v %v", sent, removed)
	}
	// dismiss removes
	if err := r.svc.Dismiss("f3", ""); err != nil {
		t.Fatal(err)
	}
	recv(t, ch2)
	if _, removed := r.notify.snapshot(); removed[len(removed)-1] != "ask-f3" {
		t.Fatalf("%v", removed)
	}
	// requester gone removes
	ctx, cancel := context.WithCancel(context.Background())
	ch3 := r.handleAsync(ctx, testMeeting, tok, "Gone")
	r.waitOpen(t, 1)
	cancel()
	recv(t, ch3)
	if _, removed := r.notify.snapshot(); removed[len(removed)-1] != "ask-f4" {
		t.Fatalf("%v", removed)
	}
}

func TestNotifyErrorIsNotFatal(t *testing.T) {
	r := newFormRig(t)
	r.notify.err = errors.New("denied")
	r.setVisible(false)
	tok := r.token(t, testMeeting)
	r.handleAsync(context.Background(), testMeeting, tok, "One")
	r.waitOpen(t, 1)
	r.svc.CloseAll()
}

func TestNilNotifierAndVisible(t *testing.T) {
	st := &formStore{meetings: map[string]domain.Meeting{testMeeting: {ID: testMeeting, Title: "m"}}}
	svc := NewFormService(FormDeps{Meetings: st, Emit: &formEmitter{}, SessionsDir: t.TempDir(), Executable: "/x"})
	if _, err := svc.SessionConfig(testMeeting); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	tok := svc.tokens[testMeeting]
	svc.mu.Unlock()
	if len(tok) != 64 {
		t.Fatalf("default token %q", tok)
	}
	ch := make(chan error, 1)
	go func() {
		_, err := svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: tok, Form: askForm("t")})
		ch <- err
	}()
	waitFor(t, "open", func() bool { return len(svc.Open()) == 1 })
	id := svc.Open()[0].ID
	if len(id) != 36 {
		t.Fatalf("default id %q", id)
	}
	if err := svc.Dismiss(id, ""); err != nil {
		t.Fatal(err)
	}
	if err := <-ch; err != nil {
		t.Fatal(err)
	}
}

func TestSessionConfigContentAndModes(t *testing.T) {
	r := newFormRig(t)
	path, err := r.svc.SessionConfig(testMeeting)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(r.sessions, testMeeting, "mcp.json"); path != want {
		t.Fatalf("%q != %q", path, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	want := `{
  "mcpServers": {
    "notted": {
      "command": "/Applications/Notted.app/Contents/MacOS/Notted",
      "args": [
        "mcp",
        "--meeting",
        "` + testMeeting + `",
        "--socket",
        "/tmp/ask.sock",
        "--token",
        "tok1"
      ]
    }
  }
}
`
	if string(raw) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", raw, want)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Dir(path))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", di.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
	// same token on a second call, different per meeting
	if _, err := r.svc.SessionConfig(testMeeting); err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(path)
	if string(raw2) != want {
		t.Fatal("token changed")
	}
	p2, err := r.svc.SessionConfig(testMeeting2)
	if err != nil {
		t.Fatal(err)
	}
	raw3, _ := os.ReadFile(p2)
	if !strings.Contains(string(raw3), "tok2") {
		t.Fatalf("%s", raw3)
	}
}

func TestSessionConfigRefusals(t *testing.T) {
	r := newFormRig(t)
	if _, err := r.svc.SessionConfig("../x"); err == nil || err.Error() != "Meeting not found." {
		t.Fatalf("%v", err)
	}
	svc := NewFormService(FormDeps{Meetings: r.store, SessionsDir: t.TempDir()})
	if _, err := svc.SessionConfig(testMeeting); err == nil || err.Error() != "Notted could not find its own program file." {
		t.Fatalf("%v", err)
	}
}

func TestDismissMeetingAnswersEveryFormOfThatMeetingOnly(t *testing.T) {
	r := newFormRig(t)
	t1, t2 := r.token(t, testMeeting), r.token(t, testMeeting2)
	r.setVisible(false)
	c1 := r.handleAsync(context.Background(), testMeeting, t1, "One")
	r.waitOpen(t, 1)
	c2 := r.handleAsync(context.Background(), testMeeting, t1, "Two")
	waitQueued(t, r, testMeeting, 1)
	r.handleAsync(context.Background(), testMeeting2, t2, "Three")
	open := r.waitOpen(t, 2)
	openID := open[0].ID
	before := r.emit.count("ask:changed")
	r.svc.DismissMeeting(testMeeting)
	for _, c := range []<-chan handleResult{c1, c2} {
		if got := recv(t, c); got.err != nil || got.ans.Status != domain.StatusDismissed {
			t.Fatalf("%+v", got)
		}
	}
	left := r.svc.Open()
	if len(left) != 1 || left[0].MeetingID != testMeeting2 {
		t.Fatalf("open = %+v", left)
	}
	if d := r.emit.count("ask:changed") - before; d != 1 {
		t.Fatalf("want one event, got %d", d)
	}
	_, removed := r.notify.snapshot()
	if len(removed) != 1 || removed[0] != "ask-"+openID {
		t.Fatalf("removed %v", removed)
	}
	r.svc.CloseAll()
}

func TestDismissMeetingForgetsTheToken(t *testing.T) {
	r := newFormRig(t)
	t1, t2 := r.token(t, testMeeting), r.token(t, testMeeting2)
	r.svc.DismissMeeting(testMeeting)
	_, err := r.svc.Handle(context.Background(), port.FormRequest{MeetingID: testMeeting, Token: t1, Form: askForm("late")})
	if err == nil || err.Error() != msgNoToken {
		t.Fatalf("err = %v", err)
	}
	r.handleAsync(context.Background(), testMeeting2, t2, "Still works")
	r.waitOpen(t, 1)
	r.svc.CloseAll()
}

func TestDismissMeetingWithNothingIsANoOp(t *testing.T) {
	r := newFormRig(t)
	before := r.emit.count("ask:changed")
	r.svc.DismissMeeting(testMeeting)
	if d := r.emit.count("ask:changed") - before; d != 0 {
		t.Fatalf("events = %d", d)
	}
	if sent, removed := r.notify.snapshot(); len(sent) != 0 || len(removed) != 0 {
		t.Fatalf("notifier called: %v %v", sent, removed)
	}
}
