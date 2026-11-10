package infra

import (
	"os"

	"github.com/GabiHert/phonon/internal/infra/paths"
)

// isolateHome points the data folder at a new throwaway folder when on is true, and returns the
// function that removes it. wails build runs the app once to list the bound methods; that run
// must not open, create or move the real data folder.
func isolateHome(on bool) (func(), error) {
	if !on {
		return func() {}, nil
	}
	dir, err := os.MkdirTemp("", "phonon-bindings-")
	if err != nil {
		return nil, err
	}
	if err := os.Setenv(paths.HomeEnv, dir); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return func() { _ = os.RemoveAll(dir) }, nil
}
