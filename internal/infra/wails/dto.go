package wails

import (
	"time"

	"github.com/GabiHert/notted/internal/domain"
)

// MeetingDTO is a meeting as the frontend sees it.
type MeetingDTO struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	StartedAt     string `json:"startedAt"` // RFC 3339
	EndedAt       string `json:"endedAt"`   // "" while recording
	Language      string `json:"language"`  // "pt-BR" | "en"
	ProjectKey    string `json:"projectKey"`
	State         string `json:"state"`
	HasSession    bool   `json:"hasSession"`
	Folder        string `json:"folder"`
	SummaryMD     string `json:"summaryMd"`
	SuggestionsMD string `json:"suggestionsMd"`
}

func toMeetingDTO(m domain.Meeting) MeetingDTO {
	d := MeetingDTO{
		ID:            m.ID,
		Title:         m.Title,
		StartedAt:     m.StartedAt.UTC().Format(time.RFC3339),
		Language:      string(m.Language),
		ProjectKey:    m.ProjectKey,
		State:         string(m.State),
		HasSession:    m.ClaudeSessionID != "",
		Folder:        m.Folder,
		SummaryMD:     m.SummaryMD,
		SuggestionsMD: m.SuggestionsMD,
	}
	if m.EndedAt != nil {
		d.EndedAt = m.EndedAt.UTC().Format(time.RFC3339)
	}
	return d
}

// DeletePlanDTO is what deleting a meeting removes, for the confirm.
type DeletePlanDTO struct {
	AudioFiles    int    `json:"audioFiles"`
	ArchiveFolder string `json:"archiveFolder"` // "" when the meeting was never saved to the archive
}

// DeleteResultDTO is the outcome of a delete that went through; Warning is "" when nothing is left behind.
type DeleteResultDTO struct {
	Warning string `json:"warning"`
}
