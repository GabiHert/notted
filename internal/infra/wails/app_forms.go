package wails

import "github.com/GabiHert/notted/internal/application/service"

// FormOptionDTO is one option of a choice or multi piece.
type FormOptionDTO struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Recommended bool   `json:"recommended"`
}

// FormPieceDTO is one block of a form.
type FormPieceDTO struct {
	Kind        string          `json:"kind"`
	ID          string          `json:"id"`
	Markdown    string          `json:"markdown"`
	HTML        string          `json:"html"`
	Question    string          `json:"question"`
	Options     []FormOptionDTO `json:"options"` // never nil
	Placeholder string          `json:"placeholder"`
	Multiline   bool            `json:"multiline"`
	Required    bool            `json:"required"`
	YesLabel    string          `json:"yesLabel"`
	NoLabel     string          `json:"noLabel"`
}

// OpenFormDTO is the open form of one meeting.
type OpenFormDTO struct {
	ID        string         `json:"id"`
	MeetingID string         `json:"meetingId"`
	Kind      string         `json:"kind"` // "ask" | "permission"
	Title     string         `json:"title"`
	Pieces    []FormPieceDTO `json:"pieces"`
	Queued    int            `json:"queued"`
}

func toOpenFormDTO(f service.OpenForm) OpenFormDTO {
	pieces := make([]FormPieceDTO, 0, len(f.Form.Pieces))
	for _, p := range f.Form.Pieces {
		opts := make([]FormOptionDTO, 0, len(p.Options))
		for _, o := range p.Options {
			opts = append(opts, FormOptionDTO{Label: o.Label, Description: o.Description, Recommended: o.Recommended})
		}
		pieces = append(pieces, FormPieceDTO{
			Kind: string(p.Kind), ID: p.ID, Markdown: p.Markdown, HTML: p.HTML, Question: p.Question, Options: opts,
			Placeholder: p.Placeholder, Multiline: p.Multiline, Required: p.Required, YesLabel: p.YesLabel, NoLabel: p.NoLabel,
		})
	}
	return OpenFormDTO{ID: f.ID, MeetingID: f.MeetingID, Kind: string(f.Kind), Title: f.Form.Title, Pieces: pieces, Queued: f.Queued}
}

// ListOpenForms returns the open form of every meeting that has one (never nil).
func (a *App) ListOpenForms() []OpenFormDTO {
	out := []OpenFormDTO{}
	if a.forms == nil {
		return out
	}
	for _, f := range a.forms.Open() {
		out = append(out, toOpenFormDTO(f))
	}
	return out
}

// AnswerForm answers an open form. answers maps a piece id to a string or a list of strings.
func (a *App) AnswerForm(formID string, answers map[string]interface{}, note string) error {
	return userFacing(a.forms.Answer(formID, answers, note))
}

// DismissForm answers an open form with "Not now".
func (a *App) DismissForm(formID string, note string) error {
	return userFacing(a.forms.Dismiss(formID, note))
}

// SetWindowVisible records whether the window's page is visible.
func (a *App) SetWindowVisible(visible bool) {
	if a.visibility != nil {
		a.visibility.Set(visible)
	}
}
