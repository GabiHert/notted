package process

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
)

func newStreamer(t *testing.T, extra ...string) *Streamer {
	t.Helper()
	home := t.TempDir()
	return &Streamer{EnvFn: func() []string {
		return append([]string{"PATH=/usr/bin:/bin", "HOME=" + home}, extra...)
	}}
}

func streamSh(t *testing.T, s *Streamer, script string) ([]string, error) {
	t.Helper()
	var lines []string
	err := s.Stream(context.Background(), t.TempDir(), []string{"/bin/sh", "-c", script}, nil, func(l string) { lines = append(lines, l) })
	return lines, err
}

func TestStreamMergesStdoutAndStderrInOrder(t *testing.T) {
	lines, err := streamSh(t, newStreamer(t), `printf 'one\n'; printf 'two\n' >&2; printf 'three\n'`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one", "two", "three"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
}

func TestStreamCleansLines(t *testing.T) {
	long := strings.Repeat("x", 70000)
	cases := []struct {
		name, script string
		want         []string
	}{
		{"carriage return overwrite", `printf 'a\rb\r\n'`, []string{"b"}},
		{"escape codes", `printf '\033[32mok\033[0m\n'`, []string{"ok"}},
		{"no trailing newline", `printf 'tail'`, []string{"tail"}},
		{"invalid utf8", `printf '\377x\n'`, []string{"�x"}},
		{"exactly 64 KiB", `head -c 65536 /dev/zero | tr '\0' x; echo`, []string{long[:65536]}},
		{"long line", `head -c 70000 /dev/zero | tr '\0' x; echo`, []string{long[:65536], long[65536:]}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines, err := streamSh(t, newStreamer(t), c.script)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(lines, c.want) {
				t.Fatalf("lines = %d items (first len %d), want %d items", len(lines), len(firstOr(lines)), len(c.want))
			}
		})
	}
}

func firstOr(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[0]
}

func TestStreamReturnsExitError(t *testing.T) {
	_, err := streamSh(t, newStreamer(t), `exit 3`)
	if got := port.ExitCode(err); got != 3 {
		t.Fatalf("ExitCode = %d (err %v), want 3", got, err)
	}
}

func TestStreamExtraEnvWinsAndDirIsUsed(t *testing.T) {
	s := newStreamer(t, "FOO=a")
	var lines []string
	if err := s.Stream(context.Background(), t.TempDir(), []string{"/bin/sh", "-c", "echo $FOO"}, []string{"FOO=b"}, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, []string{"b"}) {
		t.Fatalf("lines = %q", lines)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lines = nil
	if err := s.Stream(context.Background(), dir, []string{"/bin/sh", "-c", "pwd"}, nil, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, []string{dir}) {
		t.Fatalf("pwd = %q, want %q", lines, dir)
	}
}

func TestStreamRefusesMissingCommand(t *testing.T) {
	called := 0
	err := newStreamer(t).Stream(context.Background(), t.TempDir(), []string{"nosuch-tool"}, nil, func(string) { called++ })
	if err == nil || err.Error() != "Notted cannot find nosuch-tool on your login shell's PATH." {
		t.Fatalf("err = %v", err)
	}
	if called != 0 {
		t.Fatalf("onLine called %d times", called)
	}
}

func TestStreamNoCommand(t *testing.T) {
	err := newStreamer(t).Stream(context.Background(), t.TempDir(), nil, nil, func(string) {})
	if err == nil || err.Error() != "no command to run" {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamTimeoutKillsTheGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := newStreamer(t).Stream(ctx, t.TempDir(), []string{"/bin/sh", "-c", "sleep 30 & echo $! > '" + pidFile + "'; sleep 30"}, nil, func(string) {})
	if err == nil {
		t.Fatal("err = nil, want an error")
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("took %v", d)
	}
	raw, rerr := os.ReadFile(pidFile)
	if rerr != nil {
		t.Fatal(rerr)
	}
	pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if perr != nil {
		t.Fatal(perr)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("child %d survived the group kill", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStreamReturnsWhenADescendantLeftTheSession(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "far.pid")
	t.Cleanup(func() {
		if raw, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	script := `perl -MPOSIX -e 'POSIX::setsid(); open F, ">", $ARGV[0]; print F $$; close F; sleep 30' '` + pidFile + `' & sleep 30`
	start := time.Now()
	err := newStreamer(t).Stream(ctx, dir, []string{"/bin/sh", "-c", script}, nil, func(string) {})
	if err == nil {
		t.Fatal("err = nil, want an error")
	}
	if d := time.Since(start); d > 500*time.Millisecond+readGrace+time.Second {
		t.Fatalf("Stream took %v", d)
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines: %d before, %d after", before, runtime.NumGoroutine())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
