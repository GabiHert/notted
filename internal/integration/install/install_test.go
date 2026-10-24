package install

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func tempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// fakeApp builds a bundle whose executable is a re-signed copy of /bin/sleep.
func fakeApp(t *testing.T, dir, marker string) {
	t.Helper()
	exe := filepath.Join(dir, "Contents", "MacOS", "Notted")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, src, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "-f", "-s", "-", exe).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v: %s", err, out)
	}
	writeFile(t, filepath.Join(dir, "Contents", "marker"), marker, 0o644)
}

type rig struct {
	root, apps, dest, newApp, fakeOpen, openLog, pidLog string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	root := tempDir(t)
	r := &rig{
		root:    root,
		apps:    filepath.Join(root, "apps with space"),
		openLog: filepath.Join(root, "open.log"),
		pidLog:  filepath.Join(root, "pids.log"),
	}
	r.dest = filepath.Join(r.apps, "Notted.app")
	r.newApp = filepath.Join(root, "src", "build", "bin", "Notted.app")
	fakeApp(t, r.dest, "old")
	fakeApp(t, r.newApp, "new")
	r.fakeOpen = filepath.Join(root, "bin", "open")
	writeFile(t, r.fakeOpen, `#!/bin/bash
echo "$1" >> "$OPEN_LOG"
if [ "${FAKE_OPEN_LAUNCH:-1}" != "0" ]; then
  "$1/Contents/MacOS/Notted" 30 >/dev/null 2>&1 &
  echo $! >> "$PID_LOG"
fi
`, 0o755)
	t.Cleanup(func() {
		raw, err := os.ReadFile(r.pidLog)
		if err != nil {
			return
		}
		for _, f := range strings.Fields(string(raw)) {
			if p, err := strconv.Atoi(f); err == nil {
				_ = syscall.Kill(p, syscall.SIGKILL)
			}
		}
	})
	return r
}

func (r *rig) env() []string {
	return []string{"NOTTED_OPEN=" + r.fakeOpen, "NOTTED_LAUNCH_SETTLE=0", "OPEN_LOG=" + r.openLog, "PID_LOG=" + r.pidLog}
}

func (r *rig) marker(t *testing.T, app string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(app, "Contents", "marker"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (r *rig) opens(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(r.openLog)
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func (r *rig) assertNoLeftovers(t *testing.T, newToo, oldToo bool) {
	t.Helper()
	if newToo {
		if _, err := os.Stat(r.dest + ".new"); !os.IsNotExist(err) {
			t.Errorf("%s.new exists", r.dest)
		}
	}
	if oldToo {
		if _, err := os.Stat(r.dest + ".old"); !os.IsNotExist(err) {
			t.Errorf("%s.old exists", r.dest)
		}
	}
}

// exitingPid starts a sleep that is killed after delay (and reaped), or stays alive until cleanup.
func exitingPid(t *testing.T, delay time.Duration) int {
	t.Helper()
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
	if delay > 0 {
		go func() { time.Sleep(delay); _ = cmd.Process.Kill() }()
	}
	return cmd.Process.Pid
}

func runScript(t *testing.T, env []string, pid int, newApp, dest string) (string, int, string) {
	t.Helper()
	dir := filepath.Join(tempDir(t), "notted-install-x")
	path := filepath.Join(dir, "install.sh")
	writeFile(t, path, string(script), 0o700)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/bash", path, strconv.Itoa(pid), newApp, dest)
	cmd.Env = append([]string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}, env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	return out.String(), code, dir
}

func assertGone(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("%s still exists", dir)
	}
}

func assertOrdered(t *testing.T, out string, lines ...string) {
	t.Helper()
	rest := out
	for _, l := range lines {
		i := strings.Index(rest, l)
		if i < 0 {
			t.Fatalf("missing or out of order: %q in\n%s", l, out)
		}
		rest = rest[i+len(l):]
	}
}

func TestInstallReplacesTheApp(t *testing.T) {
	r := newRig(t)
	pid := exitingPid(t, 300*time.Millisecond)
	out, code, dir := runScript(t, r.env(), pid, r.newApp, r.dest)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if m := r.marker(t, r.dest); m != "new" {
		t.Errorf("marker = %q", m)
	}
	r.assertNoLeftovers(t, true, true)
	if got := r.opens(t); len(got) != 1 || got[0] != r.dest {
		t.Errorf("opens = %q", got)
	}
	assertOrdered(t, out,
		"install: waiting for Notted (pid "+strconv.Itoa(pid)+") to quit",
		"install: copying "+r.newApp+" to "+r.dest+".new",
		"install: moving "+r.dest+" to "+r.dest+".old",
		"install: moving "+r.dest+".new to "+r.dest,
		"install: opening "+r.dest,
		"install: done; removed "+r.dest+".old")
	assertGone(t, dir)
}

func TestInstallQuitTimeout(t *testing.T) {
	r := newRig(t)
	pid := exitingPid(t, 0)
	out, code, dir := runScript(t, append(r.env(), "NOTTED_QUIT_WAIT=1"), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "install: Notted did not quit within 1 seconds; nothing changed") {
		t.Errorf("output:\n%s", out)
	}
	if m := r.marker(t, r.dest); m != "old" {
		t.Errorf("marker = %q", m)
	}
	r.assertNoLeftovers(t, true, true)
	if got := r.opens(t); len(got) != 0 {
		t.Errorf("opens = %q", got)
	}
	assertGone(t, dir)
}

func TestInstallLeftoverOld(t *testing.T) {
	r := newRig(t)
	writeFile(t, filepath.Join(r.dest+".old", "keep.txt"), "keep", 0o644)
	pid := exitingPid(t, 100*time.Millisecond)
	out, code, _ := runScript(t, r.env(), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "install: "+r.dest+".old is left from an earlier update; nothing changed") {
		t.Errorf("output:\n%s", out)
	}
	if m := r.marker(t, r.dest); m != "old" {
		t.Errorf("marker = %q", m)
	}
	if b, err := os.ReadFile(filepath.Join(r.dest+".old", "keep.txt")); err != nil || string(b) != "keep" {
		t.Errorf("leftover changed: %v %q", err, b)
	}
	r.assertNoLeftovers(t, true, false)
	if got := r.opens(t); len(got) != 1 || got[0] != r.dest {
		t.Errorf("opens = %q", got)
	}
}

func TestInstallCopyFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	r := newRig(t)
	if err := os.Chmod(r.apps, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(r.apps, 0o755) })
	pid := exitingPid(t, 100*time.Millisecond)
	out, code, _ := runScript(t, r.env(), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "install: copy failed; nothing changed") {
		t.Errorf("output:\n%s", out)
	}
	if m := r.marker(t, r.dest); m != "old" {
		t.Errorf("marker = %q", m)
	}
	r.assertNoLeftovers(t, true, false)
	if got := r.opens(t); len(got) != 1 || got[0] != r.dest {
		t.Errorf("opens = %q", got)
	}
}

func TestInstallSecondRenameFails(t *testing.T) {
	r := newRig(t)
	fakeMv := filepath.Join(r.root, "bin", "mv")
	writeFile(t, fakeMv, "#!/bin/bash\ncase \"$1\" in *.new) exit 1 ;; esac\nexec /bin/mv \"$@\"\n", 0o755)
	pid := exitingPid(t, 100*time.Millisecond)
	out, code, _ := runScript(t, append(r.env(), "NOTTED_MV="+fakeMv), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "install: rename failed; restored "+r.dest) {
		t.Errorf("output:\n%s", out)
	}
	if m := r.marker(t, r.dest); m != "old" {
		t.Errorf("marker = %q", m)
	}
	r.assertNoLeftovers(t, true, true)
	if got := r.opens(t); len(got) != 1 || got[0] != r.dest {
		t.Errorf("opens = %q", got)
	}
}

func TestInstallRestoreDoesNotNestWhenTheTargetExists(t *testing.T) {
	r := newRig(t)
	fakeMv := filepath.Join(r.root, "bin", "mv")
	// Refuses to move anything to <dest>.new, which is the rollback of the new app.
	writeFile(t, fakeMv, "#!/bin/bash\ncase \"$2\" in *.new) exit 1 ;; esac\nexec /bin/mv \"$@\"\n", 0o755)
	pid := exitingPid(t, 100*time.Millisecond)
	out, code, _ := runScript(t, append(r.env(), "NOTTED_MV="+fakeMv, "FAKE_OPEN_LAUNCH=0", "NOTTED_LAUNCH_WAIT=1"), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	want := "install: could not restore the previous version; it is at " + r.dest + ".old. Move it back to " + r.dest + " in Finder, then open it."
	if !strings.Contains(out, want) {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "restored the previous version") && !strings.Contains(out, "could not restore") {
		t.Errorf("claims a restore:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(r.dest, "Notted.app.old")); !os.IsNotExist(err) {
		t.Error("the old app was moved into the new bundle")
	}
	if m := r.marker(t, r.dest); m != "new" {
		t.Errorf("dest marker = %q", m)
	}
	if m := r.marker(t, r.dest+".old"); m != "old" {
		t.Errorf("old marker = %q", m)
	}
	if got := r.opens(t); len(got) != 1 {
		t.Errorf("opens = %q, want only the first open", got)
	}
}

func TestInstallIgnoresAnOldProcessThatWasReopened(t *testing.T) {
	r := newRig(t)
	old := exec.Command(filepath.Join(r.dest, "Contents", "MacOS", "Notted"), "30")
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan struct{})
	go func() { _ = old.Wait(); close(reaped) }()
	t.Cleanup(func() { _ = old.Process.Kill(); <-reaped })
	pid := exitingPid(t, 100*time.Millisecond)
	out, code, _ := runScript(t, append(r.env(), "FAKE_OPEN_LAUNCH=0", "NOTTED_LAUNCH_WAIT=1"), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "install: the new Notted did not start within 1 seconds; restored the previous version") {
		t.Errorf("output:\n%s", out)
	}
	if m := r.marker(t, r.dest); m != "old" {
		t.Errorf("marker = %q", m)
	}
	r.assertNoLeftovers(t, true, true)
}

func TestInstallFirstRenameFails(t *testing.T) {
	r := newRig(t)
	fakeMv := filepath.Join(r.root, "bin", "mv")
	writeFile(t, fakeMv, "#!/bin/bash\nexit 1\n", 0o755)
	pid := exitingPid(t, 100*time.Millisecond)
	out, code, _ := runScript(t, append(r.env(), "NOTTED_MV="+fakeMv), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "install: rename failed; nothing changed") {
		t.Errorf("output:\n%s", out)
	}
	if m := r.marker(t, r.dest); m != "old" {
		t.Errorf("marker = %q", m)
	}
	r.assertNoLeftovers(t, true, true)
}

func TestInstallNewAppDoesNotStart(t *testing.T) {
	r := newRig(t)
	pid := exitingPid(t, 100*time.Millisecond)
	out, code, _ := runScript(t, append(r.env(), "FAKE_OPEN_LAUNCH=0", "NOTTED_LAUNCH_WAIT=1"), pid, r.newApp, r.dest)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "install: the new Notted did not start within 1 seconds; restored the previous version") {
		t.Errorf("output:\n%s", out)
	}
	if m := r.marker(t, r.dest); m != "old" {
		t.Errorf("marker = %q", m)
	}
	r.assertNoLeftovers(t, true, true)
	if got := r.opens(t); len(got) != 2 || got[0] != r.dest || got[1] != r.dest {
		t.Errorf("opens = %q", got)
	}
}

func waitFor(t *testing.T, d time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func installFolders(t *testing.T, tmp string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(tmp, "notted-install-*"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInstallerStart(t *testing.T) {
	r := newRig(t)
	tmp := filepath.Join(r.root, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-exited })
	pid := cmd.Process.Pid
	logPath := filepath.Join(r.root, "home", "update.log")
	start := time.Now()
	if err := (&Installer{Env: r.env()}).Start(pid, r.newApp, r.dest, logPath); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Start took %v", d)
	}
	readLog := func() string { b, _ := os.ReadFile(logPath); return string(b) }
	waitFor(t, 2*time.Second, "waiting line", func() bool {
		return strings.Contains(readLog(), "install: waiting for Notted (pid "+strconv.Itoa(pid)+") to quit")
	})
	_ = cmd.Process.Kill()
	waitFor(t, 15*time.Second, "done line", func() bool {
		return strings.Contains(readLog(), "install: done; removed "+r.dest+".old")
	})
	if m := r.marker(t, r.dest); m != "new" {
		t.Errorf("marker = %q", m)
	}
	waitFor(t, 2*time.Second, "temp folder removed", func() bool { return len(installFolders(t, tmp)) == 0 })
	st, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("log mode = %v", st.Mode().Perm())
	}
}

func TestInstallerStartBadLog(t *testing.T) {
	root := tempDir(t)
	tmp := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	file := filepath.Join(root, "afile")
	writeFile(t, file, "x", 0o644)
	err := (&Installer{}).Start(1, "/nope", "/nope2", filepath.Join(file, "sub", "update.log"))
	if err == nil || !strings.HasPrefix(err.Error(), "cannot write ") {
		t.Fatalf("err = %v", err)
	}
	if got := installFolders(t, tmp); len(got) != 0 {
		t.Errorf("temp folders: %q", got)
	}
}

func TestScriptIsEmbedded(t *testing.T) {
	if !bytes.HasPrefix(script, []byte("#!/bin/bash\n")) {
		t.Error("script does not start with the bash shebang")
	}
	if !bytes.Contains(script, []byte("ditto")) {
		t.Error("script does not use ditto")
	}
}
