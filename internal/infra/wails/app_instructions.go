package wails

import "github.com/GabiHert/notted/internal/domain"

// InstructionFileDTO is one instruction file of the archive.
type InstructionFileDTO struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	ReadOnly bool   `json:"readOnly"`
}

// SettingsDTO is the person's saved settings.
type SettingsDTO struct {
	Profile      string            `json:"profile"`
	CheckoutPath string            `json:"checkoutPath"`
	ProjectRoots map[string]string `json:"projectRoots"`
}

// ListInstructionFiles returns the instruction files of the archive folder.
func (a *App) ListInstructionFiles() ([]InstructionFileDTO, error) {
	files, err := a.instructions.List(a.context())
	if err != nil {
		return nil, err
	}
	out := make([]InstructionFileDTO, 0, len(files))
	for _, f := range files {
		out = append(out, InstructionFileDTO{Path: f.Path, Kind: f.Kind, ReadOnly: f.ReadOnly})
	}
	return out, nil
}

// ReadInstructionFile returns one instruction file's text.
func (a *App) ReadInstructionFile(path string) (string, error) {
	return a.instructions.Read(a.context(), path)
}

// WriteInstructionFile replaces one writable instruction file.
func (a *App) WriteInstructionFile(path string, text string) error {
	return a.instructions.Write(a.context(), path, text)
}

// GetSettings returns the profile, the archive folder and the project roots.
func (a *App) GetSettings() (SettingsDTO, error) {
	st, err := a.instructions.GetSettings(a.context())
	if err != nil {
		return SettingsDTO{}, err
	}
	return SettingsDTO{Profile: st.Profile, CheckoutPath: st.CheckoutPath, ProjectRoots: st.ProjectRoots}, nil
}

// SetSettings validates and saves the settings; empty values clear them.
func (a *App) SetSettings(s SettingsDTO) error {
	return a.instructions.SetSettings(a.context(), domain.Settings{Profile: s.Profile, CheckoutPath: s.CheckoutPath, ProjectRoots: s.ProjectRoots})
}

// PreviewPrompt returns the prompt the next summary for the project gets.
func (a *App) PreviewPrompt(projectKey string) (string, error) {
	return a.instructions.PreviewPrompt(a.context(), projectKey)
}
