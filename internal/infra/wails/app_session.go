package wails

// StartSession resumes the meeting's Claude Code session in a terminal. Output arrives as
// terminal:data:<meetingID> events and the end as terminal:exit:<meetingID>.
func (a *App) StartSession(meetingID string) error {
	return userFacing(a.session.Start(a.context(), meetingID))
}

// WriteSession sends keystrokes to the meeting's running session.
func (a *App) WriteSession(meetingID string, data string) error {
	return userFacing(a.session.Write(meetingID, data))
}

// ResizeSession sets the terminal size of the meeting's running session.
func (a *App) ResizeSession(meetingID string, cols int, rows int) error {
	return userFacing(a.session.Resize(meetingID, cols, rows))
}

// StopSession ends the meeting's session process; the Claude Code session stays resumable.
func (a *App) StopSession(meetingID string) error {
	return userFacing(a.session.Stop(meetingID))
}
