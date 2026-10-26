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
	"github.com/GabiHert/notted/internal/infra/store"
)

type fakePrompts struct{ err error }

func (f fakePrompts) Prompt(_ context.Context, _, _, project string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "PROMPT " + project, nil
}

type fakeRunner struct {
	mu    sync.Mutex
	runs  []port.ClaudeRun
	reply []func() (string, error) // per call index
	block chan struct{}            // when set, the first call waits on it
}

func (f *fakeRunner) Run(_ context.Context, r port.ClaudeRun) (string, error) {
	f.mu.Lock()
	i := len(f.runs)
	f.runs = append(f.runs, r)
	block := f.block
	f.mu.Unlock()
	if block != nil && i == 0 {
		<-block
	}
	if i < len(f.reply) {
		return f.reply[i]()
	}
	return "", errors.New("unexpected run")
}

func ok(s string) func() (string, error) { return func() (string, error) { return s, nil } }
func fail(s string) func() (string, error) {
	return func() (string, error) { return "", errors.New(s) }
}

type fakeExporter struct {
	mu    sync.Mutex
	calls int
	emit  *recEmitter
}

func (f *fakeExporter) ExportMeeting(_ context.Context, id string) (ExportResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	f.emit.Emit("export:ready", map[string]any{"meetingId": id})
	return ExportResult{}, nil
}

type sumFixture struct {
	st   *store.Store
	em   *recEmitter
	run  *fakeRunner
	exp  *fakeExporter
	svc  *SummaryService
	ctx  context.Context
	root string
}

func newSumFixture(t *testing.T) *sumFixture {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &sumFixture{st: st, em: &recEmitter{}, run: &fakeRunner{}, ctx: context.Background(), root: t.TempDir()}
	f.exp = &fakeExporter{emit: f.em}
	if err := st.SaveSettings(f.ctx, domain.Settings{Profile: "ana", CheckoutPath: f.root,
		ProjectRoots: map[string]string{"acme": "/work/acme"}}); err != nil {
		t.Fatal(err)
	}
	f.svc = NewSummaryService(st, st, fakePrompts{}, f.run, f.exp, f.em)
	n := 0
	f.svc.newID = func() string { n++; return "00000000-0000-4000-8000-00000000000" + string(rune('0'+n)) }
	return f
}

func (f *sumFixture) meeting(t *testing.T, id string, state domain.MeetingState, project string, segs bool) {
	t.Helper()
	m := domain.Meeting{ID: id, Title: "Weekly sync", StartedAt: time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC),
		Language: domain.LangPTBR, State: state}
	if err := f.st.CreateMeeting(f.ctx, m); err != nil {
		t.Fatal(err)
	}
	if project != "" {
		if err := f.st.SetProjectKey(f.ctx, id, project); err != nil {
			t.Fatal(err)
		}
	}
	if !segs {
		return
	}
	for _, s := range []domain.Segment{
		{MeetingID: id, Track: domain.TrackMic, StartMs: 1_000, EndMs: 2_000, Text: "hello  there"},
		{MeetingID: id, Track: domain.TrackOthers, StartMs: 65_900, EndMs: 70_000, Text: "second"},
		{MeetingID: id, Track: domain.TrackMic, StartMs: 3_725_000, EndMs: 3_726_000, Text: "third"},
	} {
		if _, err := f.st.AddSegment(f.ctx, s); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *sumFixture) states() []string {
	var out []string
	for _, e := range f.em.named("summary:state") {
		s := e.payload["pass"].(string) + "/" + e.payload["state"].(string)
		out = append(out, s)
		if m, ok := e.payload["message"]; ok {
			out[len(out)-1] += ":" + m.(string)
		}
	}
	return out
}

func TestSummaryChecksInOrder(t *testing.T) {
	f := newSumFixture(t)
	wantErr(t, f.svc.Generate(f.ctx, "nope"), "Meeting not found.")

	f.meeting(t, "t1", domain.StateTranscribing, "", true)
	wantErr(t, f.svc.Generate(f.ctx, "t1"), "The transcript is not finished yet. Wait until transcription completes.")
	f.meeting(t, "r1", domain.StateRecording, "acme", true)
	wantErr(t, f.svc.Generate(f.ctx, "r1"), "The transcript is not finished yet. Wait until transcription completes.")

	f.meeting(t, "nc", domain.StateReady, "", true)
	wantErr(t, f.svc.Generate(f.ctx, "nc"), "Choose a project for this meeting first.")

	f.meeting(t, "ok", domain.StateReady, "acme", true)
	f.meeting(t, "empty", domain.StateReady, "acme", false)

	set := func(s domain.Settings) {
		if err := f.st.SaveSettings(f.ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	set(domain.Settings{CheckoutPath: f.root, ProjectRoots: map[string]string{}})
	wantErr(t, f.svc.Generate(f.ctx, "ok"), "Set your profile in Settings > Instructions first.")
	set(domain.Settings{Profile: "ana", ProjectRoots: map[string]string{}})
	wantErr(t, f.svc.Generate(f.ctx, "ok"), "Set the meetings archive folder in Settings > Instructions first.")
	set(domain.Settings{Profile: "ana", CheckoutPath: f.root, ProjectRoots: map[string]string{}})
	wantErr(t, f.svc.Generate(f.ctx, "empty"), "The transcript is empty. Nothing to summarize.")
	if len(f.run.runs) != 0 {
		t.Fatalf("runner called by failed checks: %d", len(f.run.runs))
	}
}

func TestSummaryAlreadyRunning(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.run.block = make(chan struct{})
	f.run.reply = []func() (string, error){ok("# Summary: X"), ok("# Suggestions: X")}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	wantErr(t, f.svc.Generate(f.ctx, "m1"), "A summary is already being written for this meeting.")
	close(f.run.block)
	f.svc.Wait()
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatalf("after the run ended: %v", err)
	}
	f.svc.Wait()
}

const wantSummaryStdin = "Use the meetings skill's `process` format (its file is .claude/skills/meetings/SKILL.md) to write this meeting's summary.md, but do not write, stage or commit any file and do not run the save step. Reply with the content of summary.md only: start with the line \"# Summary: Weekly sync\" and add nothing before or after it.\n\n" +
	"Meeting: Weekly sync\nDate: 2026-03-10\nProject: acme\nLanguage: pt-BR\n" +
	"Transcript lines: (mic) is the person who recorded; (others) is everyone else.\n\n" +
	"[00:00:01] (mic) hello there\n[00:01:05] (others) second\n[01:02:05] (mic) third\n"

const wantSuggestStdin = "Use the meetings skill's `suggest` format (its file is .claude/skills/meetings/SKILL.md) to write this meeting's suggestions.md from the summary you just wrote and the transcript above, checking the existing-work sources the instructions name. Do not write any file. Every Decision is `undecided`. Reply with the content of suggestions.md only: start with the line \"# Suggestions: Weekly sync\" and add nothing before or after it."

func TestSummarySuccessFlow(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.run.reply = []func() (string, error){ok("Sure! Here it is:\n# Summary: X\nbody\n"), ok("noise\n# Suggestions: X\n- a\n")}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if len(f.run.runs) != 2 {
		t.Fatalf("runs = %d", len(f.run.runs))
	}
	a, b := f.run.runs[0], f.run.runs[1]
	sid := "00000000-0000-4000-8000-000000000001"
	if a.Resume || a.SessionID != sid || a.PromptText != "PROMPT acme" || a.ProjectRoot != "/work/acme" || a.Checkout != f.root {
		t.Fatalf("summary run = %+v", a)
	}
	if !b.Resume || b.SessionID != sid || b.PromptText != "" || b.ProjectRoot != "/work/acme" || b.Checkout != f.root {
		t.Fatalf("suggestions run = %+v", b)
	}
	if a.Stdin != wantSummaryStdin {
		t.Fatalf("summary stdin:\n%q\nwant\n%q", a.Stdin, wantSummaryStdin)
	}
	if b.Stdin != wantSuggestStdin {
		t.Fatalf("suggestions stdin:\n%q", b.Stdin)
	}
	m, _ := f.st.GetMeeting(f.ctx, "m1")
	if m.State != domain.StateSummarized || m.ClaudeSessionID != sid {
		t.Fatalf("meeting = %+v", m)
	}
	if !strings.HasPrefix(m.SummaryMD, "# Summary: X") || strings.Contains(m.SummaryMD, "Sure") {
		t.Fatalf("summary = %q", m.SummaryMD)
	}
	if !strings.HasPrefix(m.SuggestionsMD, "# Suggestions: X") || strings.Contains(m.SuggestionsMD, "noise") {
		t.Fatalf("suggestions = %q", m.SuggestionsMD)
	}
	if f.exp.calls != 1 {
		t.Fatalf("export calls = %d", f.exp.calls)
	}
	want := "summary/running summary/done suggestions/running suggestions/done"
	if got := strings.Join(f.states(), " "); got != want {
		t.Fatalf("states = %q", got)
	}
	if f.em.events[len(f.em.events)-1].name != "export:ready" {
		t.Fatalf("last event = %q", f.em.events[len(f.em.events)-1].name)
	}
	if f.em.events[0].payload["meetingId"] != "m1" {
		t.Fatalf("payload = %v", f.em.events[0].payload)
	}
}

func TestSummaryResumesStoredSession(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	if err := f.st.SetClaudeSessionID(f.ctx, "m1", "11111111-2222-3333-4444-555555555555"); err != nil {
		t.Fatal(err)
	}
	f.run.reply = []func() (string, error){ok("# Summary: X"), ok("# Suggestions: X")}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	a := f.run.runs[0]
	if !a.Resume || a.SessionID != "11111111-2222-3333-4444-555555555555" || a.PromptText != "PROMPT acme" {
		t.Fatalf("run = %+v", a)
	}
	if m, _ := f.st.GetMeeting(f.ctx, "m1"); m.State != domain.StateSummarized {
		t.Fatalf("state = %s", m.State)
	}
}

func TestSummaryPassFailureStoresNothing(t *testing.T) {
	msg := "Claude Code is not logged in. Run claude in your terminal and log in, then try again."
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.run.reply = []func() (string, error){fail(msg)}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	m, _ := f.st.GetMeeting(f.ctx, "m1")
	if m.ClaudeSessionID != "" || m.SummaryMD != "" || m.State != domain.StateReady || f.exp.calls != 0 {
		t.Fatalf("stored on failure: %+v calls %d", m, f.exp.calls)
	}
	if got := strings.Join(f.states(), "|"); got != "summary/running|summary/error:"+msg {
		t.Fatalf("states = %q", got)
	}
}

func TestSummaryPromptFailure(t *testing.T) {
	f := newSumFixture(t)
	f.svc.prompts = fakePrompts{err: errors.New("No project file instructions/projects/acme.md.")}
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if got := strings.Join(f.states(), "|"); got != "summary/error:No project file instructions/projects/acme.md." {
		t.Fatalf("states = %q", got)
	}
	if len(f.run.runs) != 0 {
		t.Fatal("runner called")
	}
}

func TestSummaryReplyWithoutHeader(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.run.reply = []func() (string, error){ok("I could not do it.")}
	_ = f.svc.Generate(f.ctx, "m1")
	f.svc.Wait()
	m, _ := f.st.GetMeeting(f.ctx, "m1")
	if m.ClaudeSessionID != "" || m.SummaryMD != "" || f.exp.calls != 0 {
		t.Fatalf("stored: %+v", m)
	}
	if got := strings.Join(f.states(), "|"); got != "summary/running|summary/error:Claude Code returned no summary. Nothing was saved." {
		t.Fatalf("states = %q", got)
	}
}

func TestSuggestionsFailureKeepsSummary(t *testing.T) {
	for name, second := range map[string]func() (string, error){
		"error":     fail("Claude Code did not finish within 15 minutes."),
		"no header": ok("nothing useful"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newSumFixture(t)
			f.meeting(t, "m1", domain.StateReady, "acme", true)
			f.run.reply = []func() (string, error){ok("# Summary: X"), second}
			_ = f.svc.Generate(f.ctx, "m1")
			f.svc.Wait()
			m, _ := f.st.GetMeeting(f.ctx, "m1")
			if m.ClaudeSessionID == "" || m.SummaryMD == "" || m.SuggestionsMD != "" || m.State != domain.StateReady || f.exp.calls != 0 {
				t.Fatalf("meeting = %+v calls %d", m, f.exp.calls)
			}
			st := f.states()
			last := st[len(st)-1]
			want := "suggestions/error:Claude Code did not finish within 15 minutes."
			if name == "no header" {
				want = "suggestions/error:Claude Code returned no suggestions. The summary is saved; generate again to retry."
			}
			if last != want {
				t.Fatalf("last = %q", last)
			}
		})
	}
}

type failingExporter struct{}

func (failingExporter) ExportMeeting(context.Context, string) (ExportResult, error) {
	return ExportResult{}, errors.New("disk full")
}

func TestExportFailureIsReported(t *testing.T) {
	f := newSumFixture(t)
	f.svc.export = failingExporter{}
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.run.reply = []func() (string, error){ok("# Summary: X"), ok("# Suggestions: X")}
	_ = f.svc.Generate(f.ctx, "m1")
	f.svc.Wait()
	st := f.states()
	if st[len(st)-1] != "suggestions/error:The summary is saved in Notted, but writing the files failed: disk full" {
		t.Fatalf("states = %v", st)
	}
	if m, _ := f.st.GetMeeting(f.ctx, "m1"); m.State != domain.StateSummarized {
		t.Fatalf("state = %s", m.State)
	}
}

// hookStore runs a hook on the first ListSegments call, which sits between Generate's
// first read of the meeting and its run lock.
type hookStore struct {
	*store.Store
	mu   sync.Mutex
	n    int
	hook func()
}

func (h *hookStore) ListSegments(ctx context.Context, id string) ([]domain.Segment, error) {
	h.mu.Lock()
	h.n++
	first := h.n == 1
	h.mu.Unlock()
	segs, err := h.Store.ListSegments(ctx, id)
	if first && h.hook != nil {
		h.hook()
	}
	return segs, err
}

func TestSummaryRerunInTheGapResumesTheStoredSession(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	hs := &hookStore{Store: f.st}
	f.svc = NewSummaryService(hs, hs, fakePrompts{}, f.run, f.exp, f.em)
	n := 0
	f.svc.newID = func() string { n++; return "00000000-0000-4000-8000-00000000000" + string(rune('0'+n)) }
	f.run.reply = []func() (string, error){ok("# Summary: A"), ok("# Suggestions: A"), ok("# Summary: B"), ok("# Suggestions: B")}
	hs.hook = func() { // a whole run finishes after B read the meeting and before B takes the lock
		if err := f.svc.Generate(f.ctx, "m1"); err != nil {
			t.Error(err)
		}
		f.svc.Wait()
	}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if len(f.run.runs) != 4 {
		t.Fatalf("runs = %d", len(f.run.runs))
	}
	sid := "00000000-0000-4000-8000-000000000001"
	if f.run.runs[0].Resume || f.run.runs[0].SessionID != sid {
		t.Fatalf("first = %+v", f.run.runs[0])
	}
	for _, r := range f.run.runs[1:] {
		if !r.Resume || r.SessionID != sid {
			t.Fatalf("later run = %+v", r)
		}
	}
	for _, s := range f.states() {
		if strings.Contains(s, domain.ErrSessionIDAlreadySet.Error()) {
			t.Fatalf("state %q", s)
		}
	}
	if m, _ := f.st.GetMeeting(f.ctx, "m1"); m.ClaudeSessionID != sid || !strings.HasPrefix(m.SummaryMD, "# Summary: B") {
		t.Fatalf("meeting = %+v", m)
	}
}

// ctxRunner blocks until the run's context ends, then returns its error.
type ctxRunner struct{ started chan struct{} }

func (r ctxRunner) Run(ctx context.Context, _ port.ClaudeRun) (string, error) {
	close(r.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func TestCancelStopsTheMeetingsRunAndWaitsForIt(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	r := ctxRunner{started: make(chan struct{})}
	f.svc.runner = r
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	<-r.started
	f.svc.Cancel("m1")
	// Cancel returns only once the run ended, so nothing writes to the meeting afterwards.
	f.svc.mu.Lock()
	n := len(f.svc.active)
	f.svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("runs still active after Cancel: %d", n)
	}
	for _, s := range f.states() {
		if strings.Contains(s, "error") {
			t.Fatalf("a cancelled run reported an error: %v", f.states())
		}
	}
	f.svc.Wait()
}

func TestCancelWithoutARunIsANoOp(t *testing.T) {
	f := newSumFixture(t)
	f.svc.Cancel("none")
}

func TestCancelLeavesAnotherMeetingsRunAlone(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.run.block = make(chan struct{})
	f.run.reply = []func() (string, error){ok("# Summary: X"), ok("# Suggestions: X")}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Cancel("other")
	close(f.run.block)
	f.svc.Wait()
	if got := f.states(); len(got) == 0 || got[len(got)-1] != "suggestions/done" {
		t.Fatalf("the run was disturbed: %v", got)
	}
}

func TestSummaryPassesCarryMCPConfig(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.svc.SetMCPConfig(func(id string) (string, error) { return "/s/" + id + "/mcp.json", nil })
	f.run.reply = []func() (string, error){ok("# Summary: X"), ok("# Suggestions: X")}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	if len(f.run.runs) != 2 {
		t.Fatalf("runs = %d", len(f.run.runs))
	}
	for i, r := range f.run.runs {
		if r.MCPConfig != "/s/m1/mcp.json" {
			t.Fatalf("run %d MCPConfig = %q", i, r.MCPConfig)
		}
		if r.Timeout != 0 {
			t.Fatalf("run %d Timeout = %v", i, r.Timeout)
		}
	}
}

func TestSummaryConfigErrorEmitsError(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	f.svc.SetMCPConfig(func(string) (string, error) { return "", errors.New("no space") })
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	f.svc.Wait()
	got := f.states()
	if len(got) != 1 || got[0] != "summary/error:Notted could not prepare the session's tools: no space" {
		t.Fatalf("states = %v", got)
	}
	if len(f.run.runs) != 0 {
		t.Fatal("the runner was called")
	}
}

func TestSummaryRunningAndBusy(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	if f.svc.Running("m1") {
		t.Fatal("running before Generate")
	}
	f.svc.SetBusyCheck(func(id string) string { return "wait " + id })
	wantErr(t, f.svc.Generate(f.ctx, "m1"), "wait m1")
	f.svc.SetBusyCheck(func(string) string { return "" })
	f.run.block = make(chan struct{})
	f.run.reply = []func() (string, error){ok("# Summary: X"), ok("# Suggestions: X")}
	if err := f.svc.Generate(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if !f.svc.Running("m1") || f.svc.Running("other") {
		t.Fatal("Running is wrong while a run is active")
	}
	close(f.run.block)
	f.svc.Wait()
	if f.svc.Running("m1") {
		t.Fatal("still running")
	}
}

func TestSummaryBusyMessageWhileReviewRuns(t *testing.T) {
	f := newSumFixture(t)
	f.meeting(t, "m1", domain.StateReady, "acme", true)
	review := &fakeRunning{}
	f.svc.SetBusyCheck(SummaryBusyCheck(review))
	review.set("m1", true)
	wantErr(t, f.svc.Generate(f.ctx, "m1"), "Suggestions are being reviewed for this meeting. Wait until the review finishes.")
}
