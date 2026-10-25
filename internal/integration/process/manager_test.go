package process

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/integration/claude"
)

type ev struct {
	name    string
	payload any
}

type rec struct {
	mu     sync.Mutex
	events []ev
}

func (r *rec) Emit(name string, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev{name, payload})
}

func (r *rec) data(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	for _, e := range r.events {
		if e.name == "terminal:data:"+key {
			b.WriteString(e.payload.(string))
		}
	}
	return b.String()
}

func (r *rec) exits(key string) []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []map[string]any
	for _, e := range r.events {
		if e.name == "terminal:exit:"+key {
			out = append(out, e.payload.(map[string]any))
		}
	}
	return out
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitData(t *testing.T, r *rec, key, want string) {
	t.Helper()
	waitFor(t, "output "+want, func() bool { return strings.Contains(r.data(key), want) })
}

func stub(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("testdata/stub-claude-tty.sh")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func newMgr(t *testing.T) (*Manager, *rec) {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	r := &rec{}
	m := NewManager(r)
	t.Cleanup(m.StopAll)
	return m, r
}

func baseEnv() []string {
	return append(os.Environ(), "TERM=dumb")
}

func TestStartArgvCwdTtyTerm(t *testing.T) {
	m, r := newMgr(t)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	if err := m.Start("m1", dir, []string{stub(t), "--resume", "abc", "--add-dir", "/tmp/root"}, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "TTY:yes TERM:xterm-256color")
	out := r.data("m1")
	t.Logf("stub saw: %q", out)
	for _, w := range []string{"ARGV:--resume abc --add-dir /tmp/root", "CWD:" + dir} {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in %q", w, out)
		}
	}
	m2, r2 := newMgr(t)
	if err := m2.Start("m2", dir, []string{stub(t), "--resume", "abc"}, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitData(t, r2, "m2", "TTY:yes")
	if o := r2.data("m2"); !strings.Contains(o, "ARGV:--resume abc\r\n") {
		t.Fatalf("argv without root: %q", o)
	}
}

func TestCleanEnvCredentialsGone(t *testing.T) {
	m, r := newMgr(t)
	env := claude.CleanEnv(append(os.Environ(), "ANTHROPIC_API_KEY=dummy1", "ANTHROPIC_AUTH_TOKEN=dummy2"))
	if err := m.Start("m1", t.TempDir(), []string{stub(t), "--resume", "abc"}, env); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "KEY:none TOKEN:none")
}

func TestWriteResizeUTF8(t *testing.T) {
	m, r := newMgr(t)
	if err := m.Start("m1", t.TempDir(), []string{stub(t), "--resume", "abc"}, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "TTY:")
	if err := m.Write("m1", "hello\n"); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "ECHO:hello")
	if err := m.Resize("m1", 100, 40); err != nil {
		t.Fatal(err)
	}
	if err := m.Write("m1", "size\n"); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "40 100")
	for i := 0; i < 20; i++ {
		_ = m.Write("m1", "utf8\n")
	}
	waitFor(t, "20 utf8 lines", func() bool { return strings.Count(r.data("m1"), "ação ✓") >= 20 })
}

func TestExitCodeNoRespawn(t *testing.T) {
	m, r := newMgr(t)
	if err := m.Start("m1", t.TempDir(), []string{stub(t), "--resume", "abc"}, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "TTY:")
	_ = m.Write("m1", "exit\n")
	waitFor(t, "exit event", func() bool { return len(r.exits("m1")) == 1 })
	if code := r.exits("m1")[0]["code"]; code != 7 {
		t.Fatalf("code = %v", code)
	}
	if m.Running("m1") {
		t.Fatal("still running")
	}
	time.Sleep(300 * time.Millisecond)
	if n := strings.Count(r.data("m1"), "ARGV:"); n != 1 {
		t.Fatalf("started %d times", n)
	}
	if err := m.Write("m1", "x\n"); err == nil {
		t.Fatal("write to a stopped session succeeded")
	}
}

func TestStartTwiceOneProcess(t *testing.T) {
	m, r := newMgr(t)
	argv := []string{stub(t), "--resume", "abc"}
	if err := m.Start("m1", t.TempDir(), argv, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "TTY:")
	if err := m.Start("m1", t.TempDir(), argv, baseEnv()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if n := strings.Count(r.data("m1"), "ARGV:"); n != 1 {
		t.Fatalf("ARGV lines = %d", n)
	}
}

func TestStopAndStopAll(t *testing.T) {
	m, r := newMgr(t)
	for _, k := range []string{"a", "b", "c"} {
		if err := m.Start(k, t.TempDir(), []string{stub(t), "--resume", "abc"}, baseEnv()); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []string{"a", "b", "c"} {
		waitData(t, r, k, "TTY:")
	}
	if err := m.Stop("a"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a exit", func() bool { return len(r.exits("a")) == 1 })
	m.StopAll()
	waitFor(t, "b,c exit", func() bool { return len(r.exits("b")) == 1 && len(r.exits("c")) == 1 })
	if m.Running("b") || m.Running("c") {
		t.Fatal("still running")
	}
	if err := m.Stop("zzz"); err != nil {
		t.Fatalf("stop of unknown key: %v", err)
	}
}

func TestStopThenStartRunsFresh(t *testing.T) {
	m, r := newMgr(t)
	argv := []string{stub(t), "--resume", "abc"}
	if err := m.Start("m1", t.TempDir(), argv, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "TTY:")
	if err := m.Stop("m1"); err != nil {
		t.Fatal(err)
	}
	// The stopped instance's exit is emitted before Stop returns, so it can never follow a new Start.
	if n := len(r.exits("m1")); n != 1 {
		t.Fatalf("exits right after Stop = %d, want 1", n)
	}
	if err := m.Start("m1", t.TempDir(), argv, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "second ARGV", func() bool { return strings.Count(r.data("m1"), "ARGV:") == 2 })
	time.Sleep(300 * time.Millisecond)
	if n := len(r.exits("m1")); n != 1 {
		t.Fatalf("stale exit after the new Start: exits = %d, want 1", n)
	}
	if !m.Running("m1") {
		t.Fatal("fresh session is not running")
	}
	if err := m.Write("m1", "again\n"); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "ECHO:again")
	_ = m.Write("m1", "exit\n")
	waitFor(t, "new session's exit", func() bool { return len(r.exits("m1")) == 2 })
	if code := r.exits("m1")[1]["code"]; code != 7 {
		t.Fatalf("new session's exit code = %v, want 7", code)
	}
}

// TestStopStartStressExitsMatchInstances checks, over many Stop→Start cycles where a natural exit
// races Stop, that every instance gets exactly one exit and that no instance's output or exit
// lands inside another instance's stream.
func TestStopStartStressExitsMatchInstances(t *testing.T) {
	m, r := newMgr(t)
	const cycles = 40
	for i := 0; i < cycles; i++ {
		script := fmt.Sprintf("for n in 1 2 3 4 5 6; do echo I%d; sleep 0.01; done; exit 3", i)
		if err := m.Start("k", t.TempDir(), []string{"/bin/bash", "-c", script}, baseEnv()); err != nil {
			t.Fatal(err)
		}
		tok := fmt.Sprintf("I%d\r\n", i)
		waitFor(t, "instance output", func() bool { return strings.Contains(r.data("k"), tok) })
		time.Sleep(time.Duration(i%8) * 10 * time.Millisecond)
		if err := m.Stop("k"); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	r.mu.Lock()
	defer r.mu.Unlock()
	tokRe := regexp.MustCompile(`I(\d+)`)
	inst, exits := 0, 0
	var seg strings.Builder
	for _, e := range r.events {
		switch e.name {
		case "terminal:data:k":
			seg.WriteString(e.payload.(string))
		case "terminal:exit:k":
			for _, mm := range tokRe.FindAllStringSubmatch(seg.String(), -1) {
				if mm[1] != strconv.Itoa(inst) {
					t.Fatalf("instance %d's segment holds output of instance %s", inst, mm[1])
				}
			}
			seg.Reset()
			inst++
			exits++
		}
	}
	if exits != cycles {
		t.Fatalf("exits = %d, want %d (one per instance)", exits, cycles)
	}
	if seg.Len() != 0 {
		t.Fatalf("output after the last exit: %q", seg.String())
	}
}

func TestInvalidByteDoesNotStallOutput(t *testing.T) {
	m, r := newMgr(t)
	argv := []string{"/bin/bash", "-c", `printf 'one \377 two\n'; sleep 0.2; printf 'after\n'; cat`}
	if err := m.Start("m1", t.TempDir(), argv, baseEnv()); err != nil {
		t.Fatal(err)
	}
	waitData(t, r, "m1", "after")
	if !strings.Contains(r.data("m1"), "two") {
		t.Fatalf("output after the invalid byte lost: %q", r.data("m1"))
	}
}

func TestSplitUTF8CarryBounded(t *testing.T) {
	emit, carry := splitUTF8([]byte("ok \xff\xfe more text"))
	if string(emit) != "ok \xff\xfe more text" || len(carry) != 0 {
		t.Fatalf("invalid bytes mid-stream: emit=%q carry=%q", emit, carry)
	}
	emit, carry = splitUTF8([]byte("a\xe2\x9c"))
	if string(emit) != "a" || len(carry) != 2 {
		t.Fatalf("incomplete tail: emit=%q carry=%q", emit, carry)
	}
	big := append([]byte{0xff}, make([]byte, 100000)...)
	if _, carry = splitUTF8(big); len(carry) > 3 {
		t.Fatalf("carry grew to %d", len(carry))
	}
}
