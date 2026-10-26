package askipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

const goodForm = `{"title":"Go?","pieces":[{"kind":"confirm","id":"ok","question":"Proceed?"}]}`

type fakeHandler struct {
	mu     sync.Mutex
	calls  int
	last   port.FormRequest
	fn     func(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error)
	called chan struct{}
}

func (f *fakeHandler) Handle(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
	f.mu.Lock()
	f.calls++
	f.last = req
	f.mu.Unlock()
	if f.called != nil {
		select {
		case f.called <- struct{}{}:
		default:
		}
	}
	return f.fn(ctx, req)
}

func (f *fakeHandler) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func start(t *testing.T, h port.FormHandler) (*Server, string) {
	t.Helper()
	path := filepath.Join(shortDir(t), "s", "a.sock")
	srv, err := Listen(path, h)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv, path
}

func okHandler() *fakeHandler {
	return &fakeHandler{fn: func(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
		return domain.FormAnswer{Status: domain.StatusAnswered, Answers: map[string]any{"ok": "yes"}, Note: "n"}, nil
	}}
}

func parse(t *testing.T) domain.Form {
	t.Helper()
	f, err := domain.ParseForm([]byte(goodForm))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func rawExchange(t *testing.T, path, line string) string {
	t.Helper()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	go func() { _, _ = c.Write([]byte(line)) }()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReaderSize(c, 1<<20)
	s, _ := r.ReadString('\n')
	return s
}

func TestRoundTripAnswered(t *testing.T) {
	h := okHandler()
	_, path := start(t, h)
	c := Client{Socket: path, MeetingID: "m1", Token: "tok"}
	ans, err := c.Ask(context.Background(), domain.FormAsk, parse(t))
	if err != nil {
		t.Fatal(err)
	}
	if ans.Status != domain.StatusAnswered || ans.Answers["ok"] != "yes" || ans.Note != "n" {
		t.Fatalf("answer %+v", ans)
	}
	if h.last.MeetingID != "m1" || h.last.Token != "tok" || h.last.Kind != domain.FormAsk || h.last.Form.Title != "Go?" {
		t.Fatalf("request %+v", h.last)
	}
}

func TestNullAnswersBecomeEmptyMap(t *testing.T) {
	h := &fakeHandler{fn: func(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
		return domain.FormAnswer{Status: domain.StatusDismissed}, nil
	}}
	_, path := start(t, h)
	ans, err := Client{Socket: path}.Ask(context.Background(), domain.FormAsk, parse(t))
	if err != nil {
		t.Fatal(err)
	}
	if ans.Answers == nil {
		t.Fatal("Answers is nil")
	}
}

func TestHandlerErrorReachesClient(t *testing.T) {
	h := &fakeHandler{fn: func(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
		return domain.FormAnswer{}, errors.New("no such meeting")
	}}
	_, path := start(t, h)
	_, err := Client{Socket: path}.Ask(context.Background(), domain.FormAsk, parse(t))
	if err == nil || err.Error() != "no such meeting" {
		t.Fatalf("err %v", err)
	}
}

func TestClientCancelCancelsHandler(t *testing.T) {
	done := make(chan error, 1)
	h := &fakeHandler{called: make(chan struct{}, 1), fn: func(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
		<-ctx.Done()
		done <- ctx.Err()
		return domain.FormAnswer{}, ctx.Err()
	}}
	_, path := start(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := Client{Socket: path}.Ask(ctx, domain.FormAsk, parse(t))
		errc <- err
	}()
	<-h.called
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handler ctx err %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler context not cancelled within 2s")
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("client err %v", err)
	}
}

func TestServerSendsNothingWhenClientLeft(t *testing.T) {
	h := &fakeHandler{called: make(chan struct{}, 1), fn: func(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
		<-ctx.Done()
		return domain.FormAnswer{Status: domain.StatusAnswered}, nil
	}}
	_, path := start(t, h)
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"v":1,"token":"t","meeting_id":"m","kind":"ask","form":` + goodForm + "}\n"
	if _, err := c.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	<-h.called
	// half-close the write side: the server's 1-byte read returns EOF and cancels.
	_ = c.(*net.UnixConn).CloseWrite()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	b, _ := bufio.NewReader(c).ReadString('\n')
	if b != "" {
		t.Fatalf("server replied %q", b)
	}
	c.Close()
}

func TestDialMissingSocket(t *testing.T) {
	path := filepath.Join(shortDir(t), "none.sock")
	_, err := Client{Socket: path}.Ask(context.Background(), domain.FormAsk, parse(t))
	if !errors.Is(err, domain.ErrAppNotRunning) {
		t.Fatalf("err %v", err)
	}
}

func TestInvalidFormRefused(t *testing.T) {
	h := okHandler()
	_, path := start(t, h)
	got := rawExchange(t, path, `{"v":1,"token":"t","meeting_id":"m","kind":"ask","form":{"title":"","pieces":[]}}`+"\n")
	var rep struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(got), &rep); err != nil {
		t.Fatalf("reply %q: %v", got, err)
	}
	if rep.OK || !strings.HasPrefix(rep.Error, "invalid form:") {
		t.Fatalf("reply %+v", rep)
	}
	if h.count() != 0 {
		t.Fatal("handler called")
	}
}

func TestVersionAndKindRefused(t *testing.T) {
	h := okHandler()
	_, path := start(t, h)
	got := rawExchange(t, path, `{"v":2,"token":"t","meeting_id":"m","kind":"ask","form":`+goodForm+"}\n")
	if !strings.Contains(got, "unsupported request version") {
		t.Fatalf("got %q", got)
	}
	got = rawExchange(t, path, `{"v":1,"token":"t","meeting_id":"m","kind":"bogus","form":`+goodForm+"}\n")
	if !strings.Contains(got, "unknown request kind") {
		t.Fatalf("got %q", got)
	}
	got = rawExchange(t, path, "not json\n")
	if !strings.Contains(got, `"ok":false`) {
		t.Fatalf("got %q", got)
	}
	if h.count() != 0 {
		t.Fatal("handler called")
	}
}

func TestOversizeRequestRefused(t *testing.T) {
	_, path := start(t, okHandler())
	big := `{"v":1,"token":"` + strings.Repeat("a", 3<<20) + "\"}\n"
	got := rawExchange(t, path, big)
	if !strings.Contains(got, "the request is larger than 2 MB") {
		n := len(got)
		if n > 100 {
			n = 100
		}
		t.Fatalf("got %q", got[:n])
	}
}

func TestModes(t *testing.T) {
	_, path := start(t, okHandler())
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", di.Mode().Perm())
	}
	si, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if si.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v", si.Mode().Perm())
	}
}

func TestStaleFileReplaced(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, "a.sock")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := Listen(path, okHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if _, err := (Client{Socket: path}).Ask(context.Background(), domain.FormAsk, parse(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCloseCancelsInFlightAndRemovesSocket(t *testing.T) {
	done := make(chan struct{})
	h := &fakeHandler{called: make(chan struct{}, 1), fn: func(ctx context.Context, req port.FormRequest) (domain.FormAnswer, error) {
		<-ctx.Done()
		close(done)
		return domain.FormAnswer{}, ctx.Err()
	}}
	srv, path := start(t, h)
	go func() { _, _ = Client{Socket: path}.Ask(context.Background(), domain.FormAsk, parse(t)) }()
	<-h.called
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("handler still running after Close")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket still there: %v", err)
	}
}

func TestServerClosingEarly(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, "x.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		_, _ = bufio.NewReader(c).ReadString('\n')
		c.Close()
	}()
	_, err = Client{Socket: path}.Ask(context.Background(), domain.FormAsk, parse(t))
	if err == nil || err.Error() != "Notted stopped before the form was answered." {
		t.Fatalf("err %v", err)
	}
}

func TestPathTooLong(t *testing.T) {
	long := "/tmp/" + strings.Repeat("a", 100) + "/s.sock"
	_, err := Listen(long, okHandler())
	if err == nil || !strings.HasPrefix(err.Error(), "the socket path is longer than 103 bytes: ") {
		t.Fatalf("err %v", err)
	}
}

func TestListenFailsWhenDirIsAFile(t *testing.T) {
	dir := shortDir(t)
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(filepath.Join(f, "s.sock"), okHandler()); err == nil {
		t.Fatal("expected error")
	}
}

func TestSlowRequestTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("10 s deadline")
	}
	_, path := start(t, okHandler())
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(14 * time.Second))
	b, _ := bufio.NewReader(c).ReadString('\n')
	if !strings.Contains(b, `"ok":false`) {
		t.Fatalf("got %q", b)
	}
}

func TestHTMLHeavyFormArrivesUnchanged(t *testing.T) {
	chunk := strings.Repeat("<p>a&b</p>", 20000) // 200 KB, 6x if HTML-escaped
	raw := mustEncode(t, map[string]any{"title": "Big", "pieces": []any{
		map[string]any{"kind": "html", "id": "a", "html": chunk},
		map[string]any{"kind": "html", "id": "b", "html": chunk},
	}})
	form, err := domain.ParseForm(raw)
	if err != nil {
		t.Fatalf("test form invalid: %v", err)
	}
	h := okHandler()
	_, path := start(t, h)
	if _, err := (Client{Socket: path, MeetingID: "m", Token: "t"}).Ask(context.Background(), domain.FormAsk, form); err != nil {
		t.Fatal(err)
	}
	if len(h.last.Form.Pieces) != 2 || h.last.Form.Pieces[0].HTML != chunk || h.last.Form.Pieces[1].HTML != chunk {
		t.Fatal("form changed on the wire")
	}
}

func TestWireIsOneLine(t *testing.T) {
	if got := string(mustEncode(t, map[string]string{"a": "<b>"})); got != "{\"a\":\"<b>\"}\n" {
		t.Fatalf("got %q", got)
	}
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := encodeLine(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
