package wails

// GenerateSummary checks the meeting, then Claude Code writes its summary and suggestions in the background.
// Progress arrives as summary:state events.
func (a *App) GenerateSummary(meetingID string) error {
	return userFacing(a.summary.Generate(a.context(), meetingID))
}
