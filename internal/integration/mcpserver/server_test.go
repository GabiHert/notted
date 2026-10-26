package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GabiHert/notted/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeAsker struct {
	kind   domain.FormKind
	form   domain.Form
	calls  int
	answer domain.FormAnswer
	err    error
	cancel context.CancelFunc
}

func (f *fakeAsker) Ask(ctx context.Context, kind domain.FormKind, form domain.Form) (domain.FormAnswer, error) {
	f.calls++
	f.kind, f.form = kind, form
	if f.cancel != nil {
		f.cancel()
		<-ctx.Done()
	}
	return f.answer, f.err
}

func session(t *testing.T, a *fakeAsker) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := New(a).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, ctx
}

func text(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if len(r.Content) != 1 {
		t.Fatalf("content %v", r.Content)
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type %T", r.Content[0])
	}
	return tc.Text
}

var goodArgs = map[string]any{
	"title":  "Go?",
	"pieces": []any{map[string]any{"kind": "confirm", "id": "ok", "question": "Proceed?"}},
}

func TestListTools(t *testing.T) {
	cs, ctx := session(t, &fakeAsker{})
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 2 {
		t.Fatalf("tools %d", len(res.Tools))
	}
	got := map[string]string{}
	for _, tl := range res.Tools {
		got[tl.Name] = tl.Description
	}
	if got["ask"] != askDescription || got["permission"] != permissionDescription {
		t.Fatalf("descriptions %v", got)
	}
	if !strings.HasPrefix(askDescription, "Show the person a form in the Notted app") ||
		permissionDescription != "Notted's permission prompt for Claude Code. Claude Code calls it; do not call it yourself." {
		t.Fatal("description constants drifted")
	}
	for _, s := range []string{askSchema, permissionSchema} {
		var v map[string]any
		if err := json.Unmarshal([]byte(s), &v); err != nil || v["type"] != "object" {
			t.Fatalf("schema %v %v", v, err)
		}
	}
}

func TestAskAnswered(t *testing.T) {
	a := &fakeAsker{answer: domain.FormAnswer{Status: domain.StatusAnswered, Answers: map[string]any{"ok": "yes"}}}
	cs, ctx := session(t, a)
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ask", Arguments: goodArgs})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("error %s", text(t, r))
	}
	if text(t, r) != domain.EncodeAnswer(a.answer) {
		t.Fatalf("text %s", text(t, r))
	}
	if a.kind != domain.FormAsk || a.form.Title != "Go?" || a.calls != 1 {
		t.Fatalf("asker %+v", a)
	}
}

func TestAskInvalidForm(t *testing.T) {
	a := &fakeAsker{}
	cs, ctx := session(t, a)
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ask", Arguments: map[string]any{"title": "x", "pieces": []any{}}})
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.HasPrefix(text(t, r), "invalid form:") {
		t.Fatalf("result %v %v", r.IsError, r.Content)
	}
	if a.calls != 0 {
		t.Fatal("asked")
	}
}

func TestAskAppNotRunning(t *testing.T) {
	a := &fakeAsker{err: fmt.Errorf("%w: dial", domain.ErrAppNotRunning)}
	cs, ctx := session(t, a)
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ask", Arguments: goodArgs})
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || text(t, r) != "Notted is not running. Open Notted, then ask again." {
		t.Fatalf("result %v %v", r.IsError, r.Content)
	}
}

func TestAskOtherError(t *testing.T) {
	a := &fakeAsker{err: errors.New("boom")}
	cs, ctx := session(t, a)
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ask", Arguments: goodArgs})
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || text(t, r) != "boom" {
		t.Fatalf("result %v %v", r.IsError, r.Content)
	}
}

func TestAskContextCancelledIsProtocolError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &fakeAsker{err: context.Canceled}
	a.cancel = cancel
	h := askHandler(a)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "ask"}}
	req.Params.Arguments, _ = json.Marshal(goodArgs)
	r, err := h(ctx, req)
	if r != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("r=%v err=%v", r, err)
	}
}

var permArgs = map[string]any{
	"tool_name":   "Bash",
	"input":       map[string]any{"command": "git status"},
	"tool_use_id": "tu_1",
	"extra":       "ignored",
}

func TestPermissionAllow(t *testing.T) {
	a := &fakeAsker{answer: domain.FormAnswer{Status: domain.StatusAnswered, Answers: map[string]any{"allow": "yes"}}}
	cs, ctx := session(t, a)
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "permission", Arguments: permArgs})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("error %s", text(t, r))
	}
	if a.kind != domain.FormPermission {
		t.Fatalf("kind %v", a.kind)
	}
	want := `{"behavior":"allow","updatedInput":{"command":"git status"}}`
	if text(t, r) != want {
		t.Fatalf("text %s want %s", text(t, r), want)
	}
}

func TestPermissionUnreadable(t *testing.T) {
	a := &fakeAsker{}
	cs, ctx := session(t, a)
	for _, args := range []map[string]any{
		{"input": map[string]any{}},
		{"tool_name": 5},
	} {
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "permission", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if r.IsError || text(t, r) != domain.PermissionDeny("Notted could not read this permission request.") {
			t.Fatalf("result %v %s", r.IsError, text(t, r))
		}
	}
	if a.calls != 0 {
		t.Fatal("asked")
	}
}

func TestPermissionAppNotRunning(t *testing.T) {
	a := &fakeAsker{err: fmt.Errorf("%w: x", domain.ErrAppNotRunning)}
	cs, ctx := session(t, a)
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "permission", Arguments: permArgs})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError || text(t, r) != domain.PermissionDeny("Notted is not running, so nobody could answer this request.") {
		t.Fatalf("result %v %s", r.IsError, text(t, r))
	}
}

func TestPermissionOtherError(t *testing.T) {
	a := &fakeAsker{err: errors.New("boom")}
	cs, ctx := session(t, a)
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "permission", Arguments: permArgs})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError || text(t, r) != domain.PermissionDeny("Notted could not show this request: boom") {
		t.Fatalf("result %v %s", r.IsError, text(t, r))
	}
}

func TestPermissionContextCancelledIsProtocolError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &fakeAsker{err: context.Canceled}
	a.cancel = cancel
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "permission"}}
	req.Params.Arguments, _ = json.Marshal(permArgs)
	r, err := permissionHandler(a)(ctx, req)
	if r != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("r=%v err=%v", r, err)
	}
}
