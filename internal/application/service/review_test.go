package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

type fakeRunning struct {
	mu sync.Mutex
	m  map[string]bool
}

func (f *fakeRunning) set(id string, v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[string]bool{}
	}
	f.m[id] = v
}
func (f *fakeRunning) Running(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[id]
}

type revStore struct {
	port.MeetingStore
	mu          sync.Mutex
	meeting     domain.Meeting
	getErr      error
	suggestions []string
	setErr      error
}

func (s *revStore) GetMeeting(_ context.Context, id string) (domain.Meeting, error) {
	if s.getErr != nil {
		return domain.Meeting{}, s.getErr
	}
	return s.meeting, nil
}
func (s *revStore) SetSuggestions(_ context.Context, id, md string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suggestions = append(s.suggestions, md)
	return s.setErr
}

type revSettings struct {
	port.SettingsStore
	st domain.Settings
}

func (r revSettings) GetSettings(context.Context) (domain.Settings, error) { return r.st, nil }

type revRunner struct {
	mu    sync.Mutex
	runs  []port.ClaudeRun
	out   string
	err   error
	block chan struct{}
}

func (r *revRunner) Run(ctx context.Context, run port.ClaudeRun) (string, error) {
	r.mu.Lock()
	r.runs = append(r.runs, run)
	block := r.block
	r.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return r.out, r.err
}

type fakeReviewMode struct {
	mu     sync.Mutex
	on     bool
	getErr error
	setErr error
	sets   []bool
}

func (f *fakeReviewMode) GetReviewActWithoutAsking(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on, f.getErr
}

func (f *fakeReviewMode) SetReviewActWithoutAsking(_ context.Context, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, on)
	return f.setErr
}

func (f *fakeReviewMode) flip(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on = on
}

type revFixture struct {
	store    *revStore
	runner   *revRunner
	em       *recEmitter
	term     *fakeRunning
	sum      *fakeRunning
	svc      *ReviewService
	checkout string
	files    map[string]string
	cfgErr   error
}

func newRevFixture(t *testing.T) *revFixture {
	t.Helper()
	f := &revFixture{
		store:    &revStore{meeting: domain.Meeting{ID: "m1", Title: "Weekly sync", ProjectKey: "acme", ClaudeSessionID: "sid-1", SuggestionsMD: "# Suggestions: Weekly sync\n", Folder: "2026/weekly"}},
		runner:   &revRunner{out: "  done reply  \n"},
		em:       &recEmitter{},
		term:     &fakeRunning{},
		sum:      &fakeRunning{},
		checkout: t.TempDir(),
		files:    map[string]string{},
	}
	f.svc = NewReviewService(ReviewDeps{
		Store:    f.store,
		Settings: revSettings{st: domain.Settings{CheckoutPath: f.checkout, ProjectRoots: map[string]string{"acme": "/work/acme"}}},
		Runner:   f.runner,
		Terminal: f.term,
		Summary:  f.sum,
		MCPConfig: func(id string) (string, error) {
			if f.cfgErr != nil {
				return "", f.cfgErr
			}
			return "/s/" + id + "/mcp.json", nil
		},
		Emit: f.em,
		ReadFile: func(p string) ([]byte, error) {
			if c, ok := f.files[p]; ok {
				return []byte(c), nil
			}
			return nil, errors.New("not found")
		},
	})
	return f
}

func (f *revFixture) states() []string {
	var out []string
	for _, e := range f.em.named("review:state") {
		s := e.payload["state"].(string)
		if m, ok := e.payload["message"]; ok {
			s += ":" + m.(string)
		}
		out = append(out, s)
	}
	return out
}

func TestReviewRefusalsInOrder(t *testing.T) {
	f := newRevFixture(t)
	ctx := context.Background()

	f.store.getErr = domain.ErrNotFound
	wantErr(t, f.svc.Start(ctx, "m1"), "Meeting not found.")
	f.store.getErr = nil

	f.store.meeting.SuggestionsMD = ""
	wantErr(t, f.svc.Start(ctx, "m1"), "This meeting has no suggestions yet. Generate the summary first.")
	f.store.meeting.SuggestionsMD = "# Suggestions: x"
	f.store.meeting.ClaudeSessionID = ""
	wantErr(t, f.svc.Start(ctx, "m1"), "This meeting has no suggestions yet. Generate the summary first.")
	f.store.meeting.ClaudeSessionID = "sid-1"

	f.store.meeting.Folder = ""
	wantErr(t, f.svc.Start(ctx, "m1"), "This meeting's files are not in the archive folder yet. Generate the summary again to write them.")
	f.store.meeting.Folder = "2026/weekly"

	f.svc.settings = revSettings{st: domain.Settings{}}
	wantErr(t, f.svc.Start(ctx, "m1"), "Set the meetings archive folder in Settings > Instructions first.")
	f.svc.settings = revSettings{st: domain.Settings{CheckoutPath: f.checkout}}

	f.term.set("m1", true)
	f.sum.set("m1", true)
	wantErr(t, f.svc.Start(ctx, "m1"), "Close the Claude Code session on this page first. The review and the session use the same conversation.")
	f.term.set("m1", false)
	wantErr(t, f.svc.Start(ctx, "m1"), "A summary is being written for this meeting. Wait until it finishes.")
	f.sum.set("m1", false)

	f.cfgErr = errors.New("no space")
	wantErr(t, f.svc.Start(ctx, "m1"), "Notted could not prepare the session's tools: no space")
	f.cfgErr = nil
	if len(f.runner.runs) != 0 || len(f.em.events) != 0 {
		t.Fatal("a refused start ran or emitted")
	}
	if f.svc.Running("m1") {
		t.Fatal("a refused start registered a run")
	}
}

func TestReviewAlreadyActive(t *testing.T) {
	f := newRevFixture(t)
	f.runner.block = make(chan struct{})
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	if !f.svc.Running("m1") {
		t.Fatal("not running")
	}
	wantErr(t, f.svc.Start(context.Background(), "m1"), "Suggestions are already being reviewed for this meeting.")
	close(f.runner.block)
	f.svc.Wait()
	if f.svc.Running("m1") {
		t.Fatal("still running")
	}
}

func TestReviewRunsTheTurnAndReloads(t *testing.T) {
	f := newRevFixture(t)
	path := filepath.Join(f.checkout, "2026", "weekly", "suggestions.md")
	f.files[path] = "noise\n# Suggestions: Weekly sync\n- a: accepted\n"
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()

	if len(f.runner.runs) != 1 {
		t.Fatalf("runs = %d", len(f.runner.runs))
	}
	want := port.ClaudeRun{Checkout: f.checkout, SessionID: "sid-1", Resume: true, ProjectRoot: "/work/acme",
		MCPConfig: "/s/m1/mcp.json", Timeout: 24 * time.Hour, Stdin: reviewStdin(f.store.meeting)}
	if f.runner.runs[0] != want {
		t.Fatalf("run = %+v", f.runner.runs[0])
	}
	if f.runner.runs[0].PromptText != "" {
		t.Fatal("the review must not pass a prompt")
	}
	if got := f.states(); len(got) != 2 || got[0] != "running" || got[1] != "done:done reply" {
		t.Fatalf("states = %v", got)
	}
	if f.em.named("review:state")[0].payload["meetingId"] != "m1" {
		t.Fatal("meetingId missing")
	}
	if len(f.store.suggestions) != 1 || f.store.suggestions[0] != "# Suggestions: Weekly sync\n- a: accepted" {
		t.Fatalf("suggestions = %q", f.store.suggestions)
	}
}

const wantReviewStdin = "Review this meeting's suggestions with me through Notted's ask tool (mcp__notted__ask). I am not reading a terminal, so ask me nothing outside ask.\n\n" +
	"Meeting: Weekly sync\nMeeting folder: 2026/weekly\n\n" +
	"1. Read 2026/weekly/suggestions.md. Take every suggestion whose Decision is undecided. If there is none, reply with the single line \"No suggestions are waiting for a decision.\" and stop.\n" +
	"2. Call ask once. Title: \"Review suggestions: Weekly sync\". One piece: kind multi, id \"accept\", question \"Which suggestions do you accept? The ones you leave unchecked are rejected.\", and one option per undecided suggestion, in order, with label \"<id>: <task title>\" and description \"<verdict>. <why>\".\n" +
	"3. If the status is dismissed, change nothing and reply with the single line \"The review was dismissed. Nothing changed.\" Then stop.\n" +
	"4. Otherwise run the meetings skill's accept command (.claude/skills/meetings/SKILL.md) for 2026/weekly: the checked suggestions are accepted and the unchecked ones are rejected. Read the form's note as my comment on the review.\n" +
	"5. Ask every yes-or-no question the skill or my instructions need through ask: one form with a text piece that shows what you will do (the paths, the file content, the diff) and one confirm piece. Only a confirm answered yes is a yes. A no or a dismissed form is a no.\n" +
	"6. Finish with one line per suggestion: its id, accepted or rejected, and its outcome."

func TestReviewStdinExactText(t *testing.T) {
	got := reviewStdin(domain.Meeting{Title: "Weekly sync", Folder: "2026/weekly"})
	if got != wantReviewStdin {
		t.Fatalf("stdin differs:\n%s", got)
	}
}

func TestReviewReplyIsCutToLast4000Runes(t *testing.T) {
	f := newRevFixture(t)
	f.runner.out = strings.Repeat("a", 10) + strings.Repeat("é", 4000) + "\n"
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	msg := f.em.named("review:state")[1].payload["message"].(string)
	if msg != strings.Repeat("é", 4000) {
		t.Fatalf("message has %d runes", len([]rune(msg)))
	}
}

func TestReviewErrorPath(t *testing.T) {
	f := newRevFixture(t)
	f.runner.err = errors.New("Claude Code stopped")
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if got := f.states(); len(got) != 2 || got[1] != "error:Claude Code stopped" {
		t.Fatalf("states = %v", got)
	}
	if len(f.store.suggestions) != 0 {
		t.Fatal("suggestions reloaded after an error")
	}
}

func TestReviewReloadOnlyWithHeaderAndInsideArchive(t *testing.T) {
	t.Run("no header", func(t *testing.T) {
		f := newRevFixture(t)
		f.files[filepath.Join(f.checkout, "2026", "weekly", "suggestions.md")] = "no header here"
		_ = f.svc.Start(context.Background(), "m1")
		f.svc.Wait()
		if len(f.store.suggestions) != 0 || f.states()[1] != "done:done reply" {
			t.Fatalf("%v %v", f.store.suggestions, f.states())
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		f := newRevFixture(t)
		_ = f.svc.Start(context.Background(), "m1")
		f.svc.Wait()
		if len(f.store.suggestions) != 0 || len(f.states()) != 2 {
			t.Fatalf("%v %v", f.store.suggestions, f.states())
		}
	})
	t.Run("folder escapes the archive", func(t *testing.T) {
		f := newRevFixture(t)
		f.store.meeting.Folder = "../outside"
		read := 0
		f.svc.readFile = func(p string) ([]byte, error) { read++; return []byte("# Suggestions: x"), nil }
		_ = f.svc.Start(context.Background(), "m1")
		f.svc.Wait()
		if read != 0 || len(f.store.suggestions) != 0 {
			t.Fatalf("read %d, suggestions %v", read, f.store.suggestions)
		}
		if got := f.states(); len(got) != 2 || got[1] != "done:done reply" {
			t.Fatalf("states = %v", got)
		}
	})
	t.Run("store failure does not fail the review", func(t *testing.T) {
		f := newRevFixture(t)
		f.store.setErr = errors.New("db")
		f.files[filepath.Join(f.checkout, "2026", "weekly", "suggestions.md")] = "# Suggestions: x"
		_ = f.svc.Start(context.Background(), "m1")
		f.svc.Wait()
		if got := f.states(); got[1] != "done:done reply" {
			t.Fatalf("states = %v", got)
		}
	})
}

func TestReviewCancelEmitsNothing(t *testing.T) {
	f := newRevFixture(t)
	f.runner.block = make(chan struct{})
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Cancel("m1")
	f.svc.Cancel("m1") // no run: a no-op
	if f.svc.Running("m1") {
		t.Fatal("still running after Cancel")
	}
	if got := f.states(); len(got) != 1 || got[0] != "running" {
		t.Fatalf("states = %v", got)
	}
}

func TestReviewStopAll(t *testing.T) {
	f := newRevFixture(t)
	f.runner.block = make(chan struct{})
	_ = f.svc.Start(context.Background(), "m1")
	f.svc.StopAll()
	if f.svc.Running("m1") || len(f.states()) != 1 {
		t.Fatalf("running %v states %v", f.svc.Running("m1"), f.states())
	}
}

func TestBusyChecks(t *testing.T) {
	review, sum := &fakeRunning{}, &fakeRunning{}
	sess, summ := SessionBusyCheck(review, sum), SummaryBusyCheck(review)
	if sess("m1") != "" || summ("m1") != "" {
		t.Fatal("free meeting reported busy")
	}
	sum.set("m1", true)
	if got := sess("m1"); got != "A summary is being written for this meeting. Open the session when it finishes." {
		t.Fatal(got)
	}
	if summ("m1") != "" {
		t.Fatal("a summary must not refuse a summary here")
	}
	review.set("m1", true)
	if got := sess("m1"); got != "Suggestions are being reviewed for this meeting. Open the session when the review finishes." {
		t.Fatal(got)
	}
	if got := summ("m1"); got != "Suggestions are being reviewed for this meeting. Wait until the review finishes." {
		t.Fatal(got)
	}
	if sess("other") != "" {
		t.Fatal("other meeting busy")
	}
}

func TestReviewRunCarriesTheSwitch(t *testing.T) {
	for _, c := range []struct {
		name string
		mode *fakeReviewMode
		want bool
	}{{"on", &fakeReviewMode{on: true}, true}, {"off", &fakeReviewMode{}, false}, {"nil store", nil, false}} {
		t.Run(c.name, func(t *testing.T) {
			f := newRevFixture(t)
			if c.mode != nil {
				f.svc.mode = c.mode
			}
			if err := f.svc.Start(context.Background(), "m1"); err != nil {
				t.Fatal(err)
			}
			f.svc.Wait()
			want := port.ClaudeRun{Checkout: f.checkout, SessionID: "sid-1", Resume: true, ProjectRoot: "/work/acme",
				MCPConfig: "/s/m1/mcp.json", Timeout: 24 * time.Hour, Stdin: reviewStdin(f.store.meeting), ActWithoutAsking: c.want}
			if len(f.runner.runs) != 1 || f.runner.runs[0] != want {
				t.Fatalf("runs = %+v", f.runner.runs)
			}
		})
	}
}

func TestReviewRefusesWhenTheSettingCannotBeRead(t *testing.T) {
	f := newRevFixture(t)
	f.svc.mode = &fakeReviewMode{getErr: errors.New("boom")}
	wantErr(t, f.svc.Start(context.Background(), "m1"), "Notted could not read the review setting: boom")
	if len(f.runner.runs) != 0 || len(f.em.events) != 0 || f.svc.Running("m1") {
		t.Fatal("a refused start ran, emitted or registered")
	}
}

func TestReviewReadsTheSwitchOnEveryStart(t *testing.T) {
	f := newRevFixture(t)
	mode := &fakeReviewMode{}
	f.svc.mode = mode
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	mode.flip(true)
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if len(f.runner.runs) != 2 || f.runner.runs[0].ActWithoutAsking || !f.runner.runs[1].ActWithoutAsking {
		t.Fatalf("runs = %+v", f.runner.runs)
	}
}

func TestRunningReviewKeepsItsMode(t *testing.T) {
	f := newRevFixture(t)
	mode := &fakeReviewMode{on: true}
	f.svc.mode = mode
	f.runner.block = make(chan struct{})
	if err := f.svc.Start(context.Background(), "m1"); err != nil {
		t.Fatal(err)
	}
	mode.flip(false)
	close(f.runner.block)
	f.svc.Wait()
	if len(f.runner.runs) != 1 || !f.runner.runs[0].ActWithoutAsking {
		t.Fatalf("runs = %+v", f.runner.runs)
	}
}

func TestReviewSwitchGetterAndSetter(t *testing.T) {
	ctx := context.Background()
	f := newRevFixture(t)
	if on, err := f.svc.ActWithoutAsking(ctx); err != nil || on {
		t.Fatalf("nil store get = %v, %v", on, err)
	}
	wantErr(t, f.svc.SetActWithoutAsking(ctx, true), "Notted could not save the review setting: no store.")

	mode := &fakeReviewMode{on: true}
	f.svc.mode = mode
	if on, err := f.svc.ActWithoutAsking(ctx); err != nil || !on {
		t.Fatalf("get = %v, %v", on, err)
	}
	if err := f.svc.SetActWithoutAsking(ctx, false); err != nil || len(mode.sets) != 1 || mode.sets[0] {
		t.Fatalf("set = %v, %v", mode.sets, err)
	}
	mode.getErr = errors.New("boom")
	if _, err := f.svc.ActWithoutAsking(ctx); err == nil || err.Error() != "Notted could not read the review setting: boom" {
		t.Fatalf("get err = %v", err)
	}
	mode.setErr = errors.New("locked")
	wantErr(t, f.svc.SetActWithoutAsking(ctx, true), "Notted could not save the review setting: locked")
}
