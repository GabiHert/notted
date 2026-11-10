package system

import "os/exec"

// Alert shows a blocking critical alert titled Phonon with msg. It is for errors before the
// window exists.
func Alert(msg string) {
	_ = exec.Command("/usr/bin/osascript",
		"-e", "on run argv", "-e", `display alert "Phonon" message (item 1 of argv) as critical`, "-e", "end run",
		msg).Run()
}
