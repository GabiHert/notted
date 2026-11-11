package wails

import (
	"errors"

	"github.com/GabiHert/phonon/internal/update"
)

// UpdateRelease is the newest release, as the Updates card shows it.
type UpdateRelease struct {
	Tag   string `json:"tag"`
	Name  string `json:"name"`
	Notes string `json:"notes"`
	Date  int64  `json:"date"` // milliseconds since the epoch
	URL   string `json:"url"`
}

// UpdateStatusDTO is the Updates card's state. The field names are the ones the card from the
// release-updater skill reads.
type UpdateStatusDTO struct {
	Version      string         `json:"version"`
	Repo         string         `json:"repo"`
	Latest       *UpdateRelease `json:"latest"`
	UpToDate     bool           `json:"upToDate"`
	HasToken     bool           `json:"hasToken"`
	State        string         `json:"state"`
	Message      string         `json:"message"`
	Lines        []string       `json:"lines"`
	Seq          int            `json:"seq"`
	InstalledApp string         `json:"installedApp"`
}

var errUpdateWhileRecording = errors.New("Stop the recording first. Phonon quits and reopens to finish the update.")

func toUpdateStatus(s update.Status) UpdateStatusDTO {
	d := UpdateStatusDTO{Version: s.Version, Repo: s.Repo, UpToDate: s.UpToDate, HasToken: s.HasToken, State: s.State,
		Message: s.Message, Lines: s.Lines, Seq: s.Seq, InstalledApp: s.InstalledApp}
	if d.Lines == nil {
		d.Lines = []string{}
	}
	if r := s.Latest; r != nil {
		d.Latest = &UpdateRelease{Tag: r.Tag, Name: r.Name, Notes: r.Notes, Date: r.Date.UnixMilli(), URL: r.URL}
	}
	return d
}

// GetUpdateStatus returns the running version, the newest release found and any update in progress.
func (a *App) GetUpdateStatus() UpdateStatusDTO {
	return toUpdateStatus(a.update.Status())
}

// CheckForUpdate asks GitHub for the newest release; it downloads nothing.
func (a *App) CheckForUpdate() (UpdateStatusDTO, error) {
	s, err := a.update.Check(a.context())
	return toUpdateStatus(s), err
}

// StartUpdate downloads the newest release, checks its checksum and signature, then quits and
// swaps the app; the old version comes back if the new one does not start. It refuses while a
// recording runs, because Phonon quits to finish the update. Output arrives as update:output
// events and states as update:state events.
func (a *App) StartUpdate() error {
	if a.recording != nil && a.recording.Recording() {
		return errUpdateWhileRecording
	}
	return a.update.Start()
}

// SetUpdateToken saves a GitHub token in the keychain for a private repo; "" removes it.
func (a *App) SetUpdateToken(token string) (UpdateStatusDTO, error) {
	err := a.update.SetToken(a.context(), token)
	return toUpdateStatus(a.update.Status()), err
}
