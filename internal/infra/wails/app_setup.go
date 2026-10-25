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

// ArchiveCheckDTO says what a folder still needs to be a meetings archive.
type ArchiveCheckDTO struct {
	Path     string   `json:"path"`
	Missing  []string `json:"missing"`
	InRepo   bool     `json:"inRepo"`
	Nested   bool     `json:"nested"`
	RepoRoot string   `json:"repoRoot"`
	Ready    bool     `json:"ready"`
}

// InspectArchive reports what the folder lacks to be a meetings archive. It changes nothing.
func (a *App) InspectArchive(path string) (ArchiveCheckDTO, error) {
	c, err := a.setup.InspectArchive(a.context(), path)
	if err != nil {
		return ArchiveCheckDTO{}, err
	}
	return ArchiveCheckDTO{Path: c.Path, Missing: c.Missing, InRepo: c.InRepo, Nested: c.Nested, RepoRoot: c.RepoRoot, Ready: c.Ready}, nil
}

// PrepareArchive adds the missing archive files to the folder and runs git init when it is in no repository.
func (a *App) PrepareArchive(path string) error {
	return a.setup.PrepareArchive(a.context(), path)
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

// ListProfileFiles returns every file and folder that deleting the profile would remove.
func (a *App) ListProfileFiles(name string) ([]string, error) {
	return a.setup.ListProfileFiles(a.context(), name)
}

// DeleteProfile deletes exactly the files ListProfileFiles returned; it never commits.
func (a *App) DeleteProfile(name string, files []string) error {
	return a.setup.DeleteProfile(a.context(), name, files)
}

// ListMyProjectKeys returns the projects the saved profile has its own file for.
func (a *App) ListMyProjectKeys() ([]string, error) {
	return a.setup.ListMyProjectKeys(a.context())
}

// CreateMyProjectFile creates the saved profile's file for a project and returns its path.
func (a *App) CreateMyProjectFile(key string) (string, error) {
	return a.setup.CreateMyProjectFile(a.context(), key)
}

// GetSetupPrompt returns the text the person pastes into Claude Code to finish the setup.
func (a *App) GetSetupPrompt() (string, error) {
	return a.setup.GetSetupPrompt(a.context())
}
