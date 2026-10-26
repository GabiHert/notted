package wails

// ReviewStateDTO says whether a review runs for a meeting.
type ReviewStateDTO struct {
	Running bool `json:"running"`
}

// ReviewSuggestions runs the review turn for a meeting. Progress arrives as review:state events.
func (a *App) ReviewSuggestions(meetingID string) error {
	return userFacing(a.review.Start(a.context(), meetingID))
}

// GetReviewState says whether a review runs for a meeting.
func (a *App) GetReviewState(meetingID string) ReviewStateDTO {
	return ReviewStateDTO{Running: a.review.Running(meetingID)}
}

// GetReviewActWithoutAsking says whether review turns run without permission prompts (off by default).
func (a *App) GetReviewActWithoutAsking() (bool, error) {
	on, err := a.review.ActWithoutAsking(a.context())
	return on, userFacing(err)
}

// SetReviewActWithoutAsking saves the review switch; the next review reads it.
func (a *App) SetReviewActWithoutAsking(on bool) error {
	return userFacing(a.review.SetActWithoutAsking(a.context(), on))
}
