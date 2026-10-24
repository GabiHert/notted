package wails

import (
	"context"
	"errors"

	"github.com/GabiHert/notted/internal/domain"
)

func (a *App) context() context.Context {
	return a.emitter.Context()
}

// userFacing returns an error whose text is exactly the message the UI shows.
func userFacing(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrNotFound) {
		return errors.New("Meeting not found.")
	}
	return errors.New(err.Error())
}

// StartRecording starts recording a meeting in the given language ("pt-BR" or "en").
func (a *App) StartRecording(title string, language string) (MeetingDTO, error) {
	m, err := a.recording.Start(a.context(), title, domain.Language(language))
	if err != nil {
		return MeetingDTO{}, userFacing(err)
	}
	return toMeetingDTO(m), nil
}

// StopRecording stops the recording of a meeting and queues it for transcription.
func (a *App) StopRecording(meetingID string) error {
	return userFacing(a.recording.Stop(a.context(), meetingID))
}

// ListMeetings returns every meeting, newest first.
func (a *App) ListMeetings() ([]MeetingDTO, error) {
	ms, err := a.meetings.ListMeetings(a.context())
	if err != nil {
		return nil, userFacing(err)
	}
	out := make([]MeetingDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, toMeetingDTO(m))
	}
	return out, nil
}

// GetMeeting returns one meeting.
func (a *App) GetMeeting(id string) (MeetingDTO, error) {
	m, err := a.meetings.GetMeeting(a.context(), id)
	if err != nil {
		return MeetingDTO{}, userFacing(err)
	}
	return toMeetingDTO(m), nil
}
