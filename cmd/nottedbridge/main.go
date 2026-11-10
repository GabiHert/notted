// Command nottedbridge is the executable of the Notted.app that make build places next to
// Phonon.app. Phonon was called Notted, and a Notted install updates itself by pulling its source
// clone, running make build and handing build/bin/Notted.app to its own installer, which waits
// for a process running <installed>/Contents/MacOS/Notted before it deletes the previous version.
// That code is already on people's Macs and cannot change, so the bridge plays the part: it runs
// as Notted, waits for the installer to finish, puts the Phonon.app it carries beside itself,
// moves itself to the Trash and opens Phonon, which then copies Notted's data on first launch.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	logf := openLog()
	if logf != nil {
		defer logf.Close()
		log.SetOutput(logf)
	}
	exe, err := os.Executable()
	if err != nil {
		log.Printf("bridge: %v", err)
		os.Exit(1)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	self, ok := bundleOf(exe)
	if !ok {
		log.Printf("bridge: %s is not inside an app bundle", exe)
		os.Exit(1)
	}
	b := Bridge{
		Self:          self,
		InstallerWait: 3 * time.Minute,
		Poll:          250 * time.Millisecond,
		Copy:          dittoCopy,
		Trash:         moveToTrash,
		Launch:        launchAfterExit,
		Alert:         alert,
	}
	if err := b.Run(); err != nil {
		log.Printf("bridge: %v", err)
		os.Exit(1)
	}
}

// Bridge installs the Phonon.app inside Self next to Self.
type Bridge struct {
	Self          string        // the running Notted.app
	InstallerWait time.Duration // how long Notted's installer may take to finish
	Poll          time.Duration
	Copy          func(src, dst string) error
	Trash         func(path string) error
	Launch        func(app string) error // opens app once this process has exited
	Alert         func(msg string)
}

// Embedded is the Phonon.app this bridge carries.
func (b Bridge) Embedded() string { return filepath.Join(b.Self, "Contents", "Helpers", "Phonon.app") }

// Target is where Phonon.app goes: beside the bridge, normally /Applications/Phonon.app.
func (b Bridge) Target() string { return filepath.Join(filepath.Dir(b.Self), "Phonon.app") }

// Run waits for Notted's installer, installs Phonon unless one is already there, moves the bridge
// to the Trash and opens Phonon. When it cannot install Phonon it keeps the bridge and says so,
// so opening Notted again retries and the person always has an app to open.
func (b Bridge) Run() error {
	if !b.waitForInstaller() {
		log.Printf("bridge: %s was moved away, so Notted's installer put the previous version back; nothing to do", b.Self)
		return nil
	}
	target := b.Target()
	if exists(target) {
		log.Printf("bridge: %s already exists; keeping it", target)
	} else if err := b.install(target); err != nil {
		b.Alert(fmt.Sprintf("Notted is now called Phonon, but Phonon could not be installed to %s: %v\n\nYour meetings are safe. Open Notted again to retry.", target, err))
		return err
	}
	if err := b.Trash(b.Self); err != nil {
		log.Printf("bridge: could not move %s to the Trash: %v", b.Self, err)
	} else {
		log.Printf("bridge: moved %s to the Trash", b.Self)
	}
	log.Printf("bridge: opening %s", target)
	return b.Launch(target)
}

// waitForInstaller waits until Notted's installer has removed <Self>.old, which it does only once
// this process has kept running. It reports false when the installer rolled back instead: then
// this bundle is no longer at Self. After InstallerWait it carries on, since Phonon is needed
// either way.
func (b Bridge) waitForInstaller() bool {
	deadline := time.Now().Add(b.InstallerWait)
	for {
		if !exists(b.Self) {
			return false
		}
		if !exists(b.Self+".old") || !time.Now().Before(deadline) {
			return true
		}
		time.Sleep(b.Poll)
	}
}

// install copies the embedded Phonon.app to a staging path and renames it into place, so target
// either does not exist or is a complete app.
func (b Bridge) install(target string) error {
	src := b.Embedded()
	if !exists(filepath.Join(src, "Contents", "MacOS", "Phonon")) {
		return fmt.Errorf("this Notted.app does not carry Phonon.app")
	}
	staged := target + ".new"
	if err := os.RemoveAll(staged); err != nil {
		return err
	}
	log.Printf("bridge: copying %s to %s", src, staged)
	if err := b.Copy(src, staged); err != nil {
		_ = os.RemoveAll(staged)
		return fmt.Errorf("copy failed: %w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.RemoveAll(staged)
		return err
	}
	log.Printf("bridge: installed %s", target)
	return nil
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// bundleOf returns the .app bundle that holds the executable exe.
func bundleOf(exe string) (string, bool) {
	i := strings.LastIndex(exe, ".app/Contents/MacOS/")
	if i < 0 {
		return "", false
	}
	return exe[:i+len(".app")], true
}

func dittoCopy(src, dst string) error {
	out, err := exec.Command("/usr/bin/ditto", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// moveToTrash renames path into ~/.Trash, under a new name when the Trash already has one.
func moveToTrash(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Rename(path, trashName(dir, filepath.Base(path), time.Now()))
}

// trashName is dir/base, or dir/<name> <time>.app when that is taken.
func trashName(dir, base string, now time.Time) string {
	p := filepath.Join(dir, base)
	if !exists(p) {
		return p
	}
	ext := filepath.Ext(base)
	return filepath.Join(dir, strings.TrimSuffix(base, ext)+" "+now.Format("15.04.05")+ext)
}

// launchAfterExit opens app from a detached shell once this process is gone: Phonon does not copy
// Notted's data while a process named Notted runs, and this one is named Notted.
// PHONON_BRIDGE_OPEN replaces /usr/bin/open in tests.
func launchAfterExit(app string) error {
	open := os.Getenv("PHONON_BRIDGE_OPEN")
	if open == "" {
		open = "/usr/bin/open"
	}
	cmd := exec.Command("/bin/sh", "-c", `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; exec "$2" "$3"`,
		"sh", strconv.Itoa(os.Getpid()), open, app)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func alert(msg string) {
	script := `on run argv
display alert "Phonon" message (item 1 of argv) as warning
end run`
	_ = exec.Command("/usr/bin/osascript", "-e", script, msg).Run()
}

// openLog appends to ~/Library/Logs/Phonon/notted-bridge.log.
func openLog() *os.File {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(home, "Library", "Logs", "Phonon")
	if os.MkdirAll(dir, 0o700) != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, "notted-bridge.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil
	}
	return f
}
