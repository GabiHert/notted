package port

import (
	"context"
	"errors"
)

// Git runs one git command in a repository.
type Git interface {
	Run(ctx context.Context, repo string, args ...string) (combinedOutput string, err error)
}

// ExitCode returns 0 for a nil error, the exit code a Git.Run error carries, or -1 when it has none.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var e interface{ ExitCode() int }
	if errors.As(err, &e) {
		return e.ExitCode()
	}
	return -1
}
