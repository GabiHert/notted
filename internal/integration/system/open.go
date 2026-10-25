// Package system calls the operating system's own tools.
package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/GabiHert/notted/internal/application/port"
)

var _ port.URLOpener = Opener{}

// systemOpen is macOS's own `open`, by absolute path so a look-alike earlier on PATH never runs.
const systemOpen = "/usr/bin/open"

// Opener opens URLs with the macOS `open` command; Command overrides it in tests.
type Opener struct {
	Command string
}

// Open hands url to the system.
func (o Opener) Open(url string) error {
	command := o.Command
	if command == "" {
		command = systemOpen
	}
	if out, err := exec.Command(command, url).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v: %s\n", url, err, out)
		return errors.New("Could not open System Settings. Open it yourself, then go to Privacy & Security.")
	}
	return nil
}
