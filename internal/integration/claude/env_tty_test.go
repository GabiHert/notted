package claude

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/creack/pty"
)

const ttyHelperEnv = "NOTTED_TEST_TTY_HELPER"

// An app started from a terminal has a controlling tty. The interactive shell must not
// stop on terminal access there and run into the timeout.
func TestInteractiveShellUnderATerminalReturnsQuickly(t *testing.T) {
	if os.Getenv("NOTTED_TEST_REAL_SHELL") != "1" {
		t.Skip("set NOTTED_TEST_REAL_SHELL=1 to run against the real zsh")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("no /bin/zsh")
	}
	home := t.TempDir()
	zdotdir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExecShellHelperUnderTTY$", "-test.v", "-test.count=1")
	cmd.Env = []string{
		ttyHelperEnv + "=1",
		"HOME=" + home,
		"ZDOTDIR=" + zdotdir,
		"NOTTED_HOME=" + t.TempDir(),
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"TERM=xterm",
	}
	// pty.Start makes the helper a session leader with the pty as its controlling tty.
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, _ = io.Copy(&out, ptmx)
	_ = ptmx.Close()
	_ = cmd.Wait()
	m := regexp.MustCompile(`(?m)elapsed_ms=(\d+) err=(.+?)\r?$`).FindStringSubmatch(out.String())
	if m == nil {
		t.Fatalf("helper output had no result:\n%s", out.String())
	}
	ms, _ := strconv.Atoi(m[1])
	if m[2] != "<nil>" || ms > 3000 {
		t.Fatalf("interactive zsh under a tty: %dms err=%s, want well under the 5s timeout", ms, m[2])
	}
	t.Logf("interactive zsh under a tty returned in %dms", ms)
}

func TestExecShellHelperUnderTTY(t *testing.T) {
	if os.Getenv(ttyHelperEnv) != "1" {
		t.Skip("helper for TestInteractiveShellUnderATerminalReturnsQuickly")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	_, err := execShell(ctx, "/bin/zsh", "-l", "-i", "-c", "printf ok")
	errText := "<nil>"
	if err != nil {
		errText = strconv.Quote(err.Error())
	}
	fmt.Printf("elapsed_ms=%d err=%s\n", time.Since(start).Milliseconds(), errText)
}
