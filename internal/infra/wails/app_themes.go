package wails

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GabiHert/phonon/internal/infra/themes"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The Wails file pickers; tests replace them.
var (
	openFileDialog = wruntime.OpenFileDialog
	saveFileDialog = wruntime.SaveFileDialog
)

// maxThemeFile is the largest file Import reads; a colour theme is a few kilobytes.
const maxThemeFile = 4 << 20

// ThemeDTO is a colour theme. Mode is light, dark, or system for the default, which has no tokens
// and follows macOS. Tokens maps a token of tokens.css (without the --) to #rrggbb.
type ThemeDTO struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Mode    string            `json:"mode"`
	Builtin bool              `json:"builtin"`
	Tokens  map[string]string `json:"tokens"`
}

func toThemeDTO(t themes.Theme) ThemeDTO {
	tok := t.Tokens
	if tok == nil {
		tok = map[string]string{}
	}
	return ThemeDTO{ID: t.ID, Name: t.Name, Mode: t.Mode, Builtin: t.Builtin, Tokens: tok}
}

func (a *App) themeStore() (*themes.Store, error) {
	if a.themes == nil {
		return nil, errors.New("Themes are not available")
	}
	return a.themes, nil
}

// ListThemes returns the built-in themes, then the custom ones by name.
func (a *App) ListThemes() ([]ThemeDTO, error) {
	s, err := a.themeStore()
	if err != nil {
		return nil, err
	}
	list, err := s.List()
	if err != nil {
		return nil, fmt.Errorf("Could not list the themes: %w", err)
	}
	out := make([]ThemeDTO, 0, len(list))
	for _, t := range list {
		out = append(out, toThemeDTO(t))
	}
	return out, nil
}

// GetSelectedTheme returns the chosen theme; system when none is chosen or the chosen one is gone.
func (a *App) GetSelectedTheme() (ThemeDTO, error) {
	s, err := a.themeStore()
	if err != nil {
		return toThemeDTO(themes.System), err
	}
	return toThemeDTO(s.Selected()), nil
}

// SelectTheme makes a theme the chosen one and returns it.
func (a *App) SelectTheme(id string) (ThemeDTO, error) {
	s, err := a.themeStore()
	if err != nil {
		return ThemeDTO{}, err
	}
	if err := s.SetSelected(id); err != nil {
		return ThemeDTO{}, err
	}
	return toThemeDTO(s.Selected()), nil
}

// ImportVSCodeTheme asks for a VS Code colour theme file (.json or .jsonc; a theme exported from
// Phonon works too) and saves it as a custom theme. It returns the saved theme, or a theme with an
// empty id when the person cancels.
func (a *App) ImportVSCodeTheme() (ThemeDTO, error) {
	s, err := a.themeStore()
	if err != nil {
		return ThemeDTO{}, err
	}
	p, err := openFileDialog(a.context(), wruntime.OpenDialogOptions{
		Title:           "Import a VS Code theme",
		Filters:         []wruntime.FileFilter{{DisplayName: "VS Code theme (*.json, *.jsonc)", Pattern: "*.json;*.jsonc"}},
		ResolvesAliases: true,
	})
	if err != nil {
		return ThemeDTO{}, fmt.Errorf("The file picker failed: %w", err)
	}
	if p == "" {
		return ThemeDTO{}, nil
	}
	return a.importThemeFile(s, p)
}

func (a *App) importThemeFile(s *themes.Store, p string) (ThemeDTO, error) {
	st, err := os.Stat(p)
	if err != nil {
		return ThemeDTO{}, fmt.Errorf("Could not read %s: %w", filepath.Base(p), err)
	}
	if st.Size() > maxThemeFile {
		return ThemeDTO{}, fmt.Errorf("%s is larger than 4 MB, so it is not a colour theme.", filepath.Base(p))
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ThemeDTO{}, fmt.Errorf("Could not read %s: %w", filepath.Base(p), err)
	}
	t, err := themes.Parse(data, "")
	if err != nil {
		return ThemeDTO{}, err
	}
	saved, err := s.Save(t)
	if err != nil {
		return ThemeDTO{}, fmt.Errorf("Could not save the theme: %w", err)
	}
	return toThemeDTO(saved), nil
}

// DeleteTheme deletes a custom theme. When it was the chosen one, Phonon goes back to system.
func (a *App) DeleteTheme(id string) error {
	s, err := a.themeStore()
	if err != nil {
		return err
	}
	return s.Delete(id)
}

// ExportTheme asks where to save a theme and writes it there as JSON that Import reads back. It
// returns the path written, or "" when the person cancels.
func (a *App) ExportTheme(id string) (string, error) {
	s, err := a.themeStore()
	if err != nil {
		return "", err
	}
	raw, err := s.Export(id)
	if err != nil {
		return "", err
	}
	opts := wruntime.SaveDialogOptions{
		Title:                "Export theme",
		DefaultFilename:      id + ".phonon-theme.json",
		Filters:              []wruntime.FileFilter{{DisplayName: "Theme (*.json)", Pattern: "*.json"}},
		CanCreateDirectories: true,
	}
	if home, err := os.UserHomeDir(); err == nil {
		if fi, err := os.Stat(filepath.Join(home, "Downloads")); err == nil && fi.IsDir() {
			opts.DefaultDirectory = filepath.Join(home, "Downloads")
		}
	}
	p, err := saveFileDialog(a.context(), opts)
	if err != nil {
		return "", fmt.Errorf("The save dialog failed: %w", err)
	}
	if p == "" {
		return "", nil
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		return "", fmt.Errorf("Could not write %s: %w", filepath.Base(p), err)
	}
	return p, nil
}
