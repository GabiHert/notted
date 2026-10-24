package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/domain"
)

// InstructionFile is one instruction file of the archive.
type InstructionFile struct {
	Path     string
	Kind     string
	ReadOnly bool
}

// InstructionsService reads and writes the archive's instruction files and keeps the person's settings.
type InstructionsService struct {
	settings port.SettingsStore
	prompts  port.PromptSource
	projects func(ctx context.Context) ([]string, error)
}

// NewInstructionsService builds the instructions service; projects lists the archive's project keys.
func NewInstructionsService(settings port.SettingsStore, prompts port.PromptSource,
	projects func(ctx context.Context) ([]string, error)) *InstructionsService {
	return &InstructionsService{settings: settings, prompts: prompts, projects: projects}
}

var errNoArchive = errors.New("Set the meetings archive folder in Settings > Instructions first.")

func (s *InstructionsService) checkout(ctx context.Context) (string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	if st.CheckoutPath == "" {
		return "", errNoArchive
	}
	return st.CheckoutPath, nil
}

// List returns the existing instruction files, ordered by kind and then path.
func (s *InstructionsService) List(ctx context.Context) ([]InstructionFile, error) {
	checkout, err := s.checkout(ctx)
	if err != nil {
		return nil, err
	}
	return listInstructions(checkout)
}

// Read returns one instruction file's text.
func (s *InstructionsService) Read(ctx context.Context, path string) (string, error) {
	checkout, err := s.checkout(ctx)
	if err != nil {
		return "", err
	}
	return readInstruction(checkout, path)
}

// Write replaces one writable instruction file.
func (s *InstructionsService) Write(ctx context.Context, path, text string) error {
	checkout, err := s.checkout(ctx)
	if err != nil {
		return err
	}
	return writeInstruction(checkout, path, text)
}

// GetSettings returns the saved settings.
func (s *InstructionsService) GetSettings(ctx context.Context) (domain.Settings, error) {
	st, err := s.settings.GetSettings(ctx)
	if st.ProjectRoots == nil {
		st.ProjectRoots = map[string]string{}
	}
	return st, err
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return filepath.IsAbs(p) && err == nil && fi.IsDir()
}

// SetSettings validates and saves the settings; empty values clear a setting.
func (s *InstructionsService) SetSettings(ctx context.Context, st domain.Settings) error {
	if st.Profile != "" && !keyRe.MatchString(st.Profile) {
		return errProfileFormat
	}
	if st.CheckoutPath != "" {
		if err := checkArchive(st.CheckoutPath); err != nil {
			return err
		}
	}
	roots := map[string]string{}
	for key, p := range st.ProjectRoots {
		if p == "" {
			continue
		}
		if !isDir(p) {
			return fmt.Errorf("The project root for %s does not exist: %s", key, p)
		}
		roots[key] = p
	}
	st.ProjectRoots = roots
	return s.settings.SaveSettings(ctx, st)
}

// PreviewPrompt returns the prompt the next summary of the project's meeting gets: the same call a summary makes.
func (s *InstructionsService) PreviewPrompt(ctx context.Context, projectKey string) (string, error) {
	st, err := s.settings.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	if st.CheckoutPath == "" {
		return "", errNoArchive
	}
	if st.Profile == "" {
		return "", errProfileUnset
	}
	keys, err := s.projects(ctx)
	if err != nil {
		return "", err
	}
	known := false
	for _, k := range keys {
		known = known || k == projectKey
	}
	if !known {
		return "", fmt.Errorf("Unknown project %q. Add instructions/projects/%s.md to the archive first.", projectKey, projectKey)
	}
	return s.prompts.Prompt(ctx, st.CheckoutPath, st.Profile, projectKey)
}
