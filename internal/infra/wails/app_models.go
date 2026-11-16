package wails

import (
	"fmt"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
)

// ModelStatusDTO says whether the speech models for a language are installed.
type ModelStatusDTO struct {
	Language      string `json:"language"`
	Ready         bool   `json:"ready"`
	Missing       string `json:"missing"`
	DownloadBytes int64  `json:"downloadBytes"`
	Downloading   bool   `json:"downloading"`
}

// ModelProgressDTO is the payload of the models:progress event.
type ModelProgressDTO struct {
	Language   string `json:"language"`
	Name       string `json:"name"`
	Phase      string `json:"phase"` // download | verify | extract | done | error
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"bytesTotal"`
	Error      string `json:"error,omitempty"`
}

func parseModelLanguage(language string) (domain.Language, error) {
	switch domain.Language(language) {
	case domain.LangPTBR, domain.LangEN:
		return domain.Language(language), nil
	}
	return "", fmt.Errorf("Unknown language %q. Use pt-BR or en.", language)
}

// GetModelStatus reports whether the speech models for a language are installed.
func (a *App) GetModelStatus(language string) (ModelStatusDTO, error) {
	lang, err := parseModelLanguage(language)
	if err != nil {
		return ModelStatusDTO{}, err
	}
	st := a.models.Status(lang)
	return ModelStatusDTO{
		Language:      string(st.Language),
		Ready:         st.Ready,
		Missing:       st.Missing,
		DownloadBytes: st.DownloadBytes,
		Downloading:   st.Downloading,
	}, nil
}

// DownloadModels starts downloading the models for a language and returns at once;
// progress arrives as models:progress events.
func (a *App) DownloadModels(language string) error {
	lang, err := parseModelLanguage(language)
	if err != nil {
		return err
	}
	go func() {
		// A failed download is reported through the "error" progress event.
		_ = a.models.Download(a.context(), lang, func(p port.ModelProgress) {
			a.events.Emit("models:progress", ModelProgressDTO{
				Language:   string(p.Language),
				Name:       p.Name,
				Phase:      p.Phase,
				BytesDone:  p.Done,
				BytesTotal: p.Total,
				Error:      p.Err,
			})
		})
	}()
	return nil
}

// UnusedModelDTO is a file or folder in the models folder the current speech models do not use.
type UnusedModelDTO struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// GetUnusedModels lists what in the models folder the current speech models do not use, such as
// the models of earlier versions, with sizes.
func (a *App) GetUnusedModels() ([]UnusedModelDTO, error) {
	list, err := a.models.Unused()
	if err != nil {
		return nil, err
	}
	out := make([]UnusedModelDTO, 0, len(list))
	for _, m := range list {
		out = append(out, UnusedModelDTO{Name: m.Name, Bytes: m.Bytes})
	}
	return out, nil
}

// RemoveUnusedModel deletes one entry GetUnusedModels lists. It refuses anything the current
// models use or anything outside the models folder.
func (a *App) RemoveUnusedModel(name string) error {
	return a.models.RemoveUnused(name)
}
