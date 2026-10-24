package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
)

var _ port.LineStreamer = (*Streamer)(nil)

const maxLine = 64 * 1024

// readGrace is how long after ctx ends Stream waits for output before it stops reading, so a
// descendant that left the process group cannot hold Stream open.
const readGrace = time.Second

var escapeCodes = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)|\x1b[@-Z\\\\-_]")

// Streamer runs one command and hands each output line to a callback as it arrives.
type Streamer struct {
	// EnvFn returns the base environment: the login shell's, without API credentials.
	EnvFn func() []string
}

// Stream runs argv in dir with EnvFn's environment plus extraEnv and passes each cleaned output line to onLine.
func (s *Streamer) Stream(ctx context.Context, dir string, argv []string, extraEnv []string, onLine func(line string)) error {
	if len(argv) == 0 {
		return errors.New("no command to run")
	}
	env := append(append([]string{}, s.EnvFn()...), extraEnv...)
	path := argv[0]
	if !strings.Contains(path, "/") {
		p, ok := lookPathIn(env, path)
		if !ok {
			return fmt.Errorf("Notted cannot find %s on your login shell's PATH.", argv[0])
		}
		path = p
	}

	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			select {
			case <-done:
			case <-time.After(readGrace):
				_ = out.Close()
			}
		}
	}()
	readLines(out, onLine)
	close(done)
	return cmd.Wait()
}

// readLines splits r on newlines, cleans each line and passes it on; a line over maxLine bytes goes in pieces.
func readLines(r io.Reader, onLine func(string)) {
	br := bufio.NewReaderSize(r, maxLine)
	afterFull := false
	for {
		chunk, err := br.ReadSlice('\n')
		hasNL := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		piece := chunk
		if hasNL {
			piece = chunk[:len(chunk)-1]
		}
		// A "\n" right after a full piece only ends that piece; it is not an empty line.
		skip := afterFull && hasNL && len(piece) == 0
		afterFull = errors.Is(err, bufio.ErrBufferFull)
		if !skip && (hasNL || len(piece) > 0) {
			onLine(cleanLine(string(piece)))
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return
		}
	}
}

func cleanLine(s string) string {
	s = strings.TrimSuffix(s, "\r")
	if i := strings.LastIndex(s, "\r"); i >= 0 {
		s = s[i+1:]
	}
	s = escapeCodes.ReplaceAllString(s, "")
	return strings.ToValidUTF8(s, "\uFFFD")
}

func lookPathIn(env []string, name string) (string, bool) {
	var path string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			path = v
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, true
		}
	}
	return "", false
}
