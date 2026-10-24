package install

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// IdentityName is the code-signing identity that scripts/create-signing-identity.sh creates.
const IdentityName = "Notted Local Signing"

// Signing checks the person's keychain for IdentityName.
type Signing struct {
	// Security is the security tool's path; "" means /usr/bin/security.
	Security string
}

// Present reports whether `security find-identity -v -p codesigning` lists IdentityName.
func (s Signing) Present(ctx context.Context) bool {
	tool := s.Security
	if tool == "" {
		tool = "/usr/bin/security"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "find-identity", "-v", "-p", "codesigning")
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "\""+IdentityName+"\"")
}
