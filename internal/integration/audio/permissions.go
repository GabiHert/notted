package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
)

const msgPermissionsUnavailable = "Notted could not read the permissions: the audio recorder did not answer. Try Check again, or reinstall Notted if this keeps happening."

var _ port.PermissionChecker = (*Permissions)(nil)

// Permissions reads and requests the recording permissions by running the helper's `status` and `check`.
type Permissions struct {
	helperPath    string
	statusTimeout time.Duration
	askTimeout    time.Duration
}

// NewPermissions builds a checker for the helper at helperPath.
func NewPermissions(helperPath string) *Permissions {
	return &Permissions{helperPath: helperPath, statusTimeout: 15 * time.Second, askTimeout: 2 * time.Minute}
}

// Status runs `audiocap status`, which never prompts.
func (p *Permissions) Status(ctx context.Context) (port.PermissionSet, error) {
	return p.run(ctx, p.statusTimeout, "status")
}

// Ask runs `audiocap check`, which lets macOS prompt for what is still open.
func (p *Permissions) Ask(ctx context.Context) (port.PermissionSet, error) {
	return p.run(ctx, p.askTimeout, "check")
}

// run keeps the answer even when the helper exits non-zero: `check` does that for a denied permission.
func (p *Permissions) run(ctx context.Context, timeout time.Duration, command string) (port.PermissionSet, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.helperPath, command)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	set, ok := parsePermissions(out)
	if ok {
		return set, nil
	}
	fmt.Fprintf(os.Stderr, "audiocap %s: no permission answer (err=%v, output=%q)\n", command, err, out)
	return port.PermissionSet{}, errors.New(msgPermissionsUnavailable)
}

type permissionLine struct {
	Event   string `json:"event"`
	Name    string `json:"name"`
	Granted bool   `json:"granted"`
	Status  string `json:"status"`
}

// parsePermissions reads the `permission` events of the helper's output; ok only when both are there.
func parsePermissions(out []byte) (port.PermissionSet, bool) {
	var set port.PermissionSet
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
	for sc.Scan() {
		var l permissionLine
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.Event != "permission" {
			continue
		}
		state, ok := permissionState(l)
		if !ok {
			continue
		}
		switch l.Name {
		case "microphone":
			set.Microphone = state
		case "screen":
			set.Screen = state
		}
	}
	return set, set.Microphone != "" && set.Screen != ""
}

func permissionState(l permissionLine) (port.PermissionState, bool) {
	switch port.PermissionState(l.Status) {
	case port.PermissionAllowed, port.PermissionNotAsked, port.PermissionDenied:
		return port.PermissionState(l.Status), true
	case "":
		if l.Granted {
			return port.PermissionAllowed, true
		}
		return port.PermissionDenied, true
	}
	return "", false
}
