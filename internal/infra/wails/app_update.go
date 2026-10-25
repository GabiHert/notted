package wails

// UpdateStatusDTO is the update card's state.
type UpdateStatusDTO struct {
	SourcePath   string   `json:"sourcePath"`
	InstalledApp string   `json:"installedApp"`
	Signing      bool     `json:"signing"`
	State        string   `json:"state"`
	Message      string   `json:"message"`
	Lines        []string `json:"lines"`
	Seq          int      `json:"seq"`
}

// GetUpdateStatus returns the source folder, the installed app, the signing check and the current update.
func (a *App) GetUpdateStatus() (UpdateStatusDTO, error) {
	st, err := a.update.GetStatus(a.context())
	if err != nil {
		return UpdateStatusDTO{}, err
	}
	lines := st.Lines
	if lines == nil {
		lines = []string{}
	}
	return UpdateStatusDTO{SourcePath: st.SourcePath, InstalledApp: st.InstalledApp, Signing: st.Signing, State: st.State, Message: st.Message, Lines: lines, Seq: st.Seq}, nil
}

// SetUpdateSource checks and saves the Notted source folder; it runs nothing.
func (a *App) SetUpdateSource(path string) error {
	return a.update.SetSource(a.context(), path)
}

// StartUpdate pulls and builds the source folder, then replaces this app. Output arrives as
// update:output events ({seq, line}) and states as update:state events ({state, message}).
func (a *App) StartUpdate() error {
	return a.update.Start(a.context())
}
