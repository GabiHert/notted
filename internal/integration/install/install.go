// Package install puts a newly built Notted.app in place of the installed one.
package install

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/bhlabz/notted/internal/application/port"
)

//go:embed install.sh
var script []byte

var _ port.AppInstaller = (*Installer)(nil)

// Installer starts install.sh detached, so it outlives Notted.
type Installer struct {
	// Env is added to the script's environment; tests set the NOTTED_* overrides here.
	Env []string
}

// Start writes the script to a new temp folder and starts it in its own session; it does not wait.
func (i *Installer) Start(pid int, newApp, installedApp, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("cannot write %s: %w", logPath, err)
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", logPath, err)
	}
	defer logf.Close()

	dir, err := os.MkdirTemp("", "notted-install-")
	if err != nil {
		return err
	}
	scriptPath := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(scriptPath, script, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}

	home, _ := os.UserHomeDir()
	cmd := exec.Command("/bin/bash", scriptPath, strconv.Itoa(pid), newApp, installedApp)
	cmd.Env = append([]string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + home}, i.Env...)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
