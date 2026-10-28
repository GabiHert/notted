package wails

// ExportDTO is where an export landed, relative to the archive checkout.
type ExportDTO struct {
	Folder string   `json:"folder"`
	Files  []string `json:"files"`
}

// CommitDTO is the short sha of the saved commit and whether it was pushed.
type CommitDTO struct {
	Sha    string `json:"sha"`
	Pushed bool   `json:"pushed"`
}

// ListProjects returns the project keys found in the archive checkout.
func (a *App) ListProjects() ([]string, error) {
	return a.archive.ListProjects(a.context())
}

// SetMeetingProject ties a meeting to a project key from ListProjects.
func (a *App) SetMeetingProject(meetingID string, key string) error {
	return a.archive.SetMeetingProject(a.context(), meetingID, key)
}

// ExportMeeting writes the meeting's four files into the archive checkout.
func (a *App) ExportMeeting(meetingID string) (ExportDTO, error) {
	r, err := a.archive.ExportMeeting(a.context(), meetingID)
	if err != nil {
		return ExportDTO{}, err
	}
	return ExportDTO{Folder: r.Folder, Files: r.Files}, nil
}

// CommitAndPushMeeting commits only the meeting folder and pushes. Call it only after the person said yes.
func (a *App) CommitAndPushMeeting(meetingID string) (CommitDTO, error) {
	r, err := a.archive.CommitAndPushMeeting(a.context(), meetingID)
	if err != nil {
		return CommitDTO{}, err
	}
	return CommitDTO{Sha: r.SHA, Pushed: r.Pushed}, nil
}

// CommitProjectDeletion commits and pushes the removal of a deleted project's tracked files. Call it only after the person said yes.
func (a *App) CommitProjectDeletion(key string, paths []string) error {
	return userFacing(a.archive.CommitProjectDeletion(a.context(), key, paths))
}
