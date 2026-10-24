package wails

import (
	"fmt"
	"os"
	"path/filepath"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// openDirectoryDialog is the Wails folder picker; tests replace it.
var openDirectoryDialog = wruntime.OpenDirectoryDialog

// ChooseFolder opens the native folder picker and returns the chosen path, or "" when the person cancels.
func (a *App) ChooseFolder(title string, startDir string) (string, error) {
	opts := wruntime.OpenDialogOptions{Title: title, CanCreateDirectories: true, ResolvesAliases: true}
	if filepath.IsAbs(startDir) {
		if fi, err := os.Stat(startDir); err == nil && fi.IsDir() {
			opts.DefaultDirectory = startDir
		}
	}
	p, err := openDirectoryDialog(a.context(), opts)
	if err != nil {
		return "", fmt.Errorf("The folder picker failed: %w", err)
	}
	return p, nil
}

// CloneArchive clones the meetings archive into parentDir and returns the clone's path.
func (a *App) CloneArchive(address string, parentDir string) (string, error) {
	return a.setup.CloneArchive(a.context(), address, parentDir)
}

// ListProfiles returns the people in the saved archive folder.
func (a *App) ListProfiles() ([]string, error) { return a.setup.ListProfiles(a.context()) }

// CreateProfile creates the person's file and returns its archive-relative path.
func (a *App) CreateProfile(name string) (string, error) {
	return a.setup.CreateProfile(a.context(), name)
}

// CreateProject creates a project's files and returns the paths created.
func (a *App) CreateProject(key string) ([]string, error) {
	return a.setup.CreateProject(a.context(), key)
}
