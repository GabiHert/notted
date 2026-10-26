package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

// ErrAudioLeft marks a delete whose rows are gone but whose audio folder could not be removed.
var ErrAudioLeft = errors.New("audio folder left behind")

const msgStopFirst = "Stop the recording before you delete this meeting."

type activeMeeting interface{ ActiveMeetingID() string }
type transcriptionForgetter interface{ Forget(meetingID string) }
type summaryCanceller interface{ Cancel(meetingID string) }
type sessionStopper interface{ Stop(meetingID string) error }

// DeleteDeps are the collaborators of DeleteService.
type DeleteDeps struct {
	Store         port.MeetingStore
	Recorder      activeMeeting
	Transcription transcriptionForgetter
	Summary       summaryCanceller
	Review        summaryCanceller
	Session       sessionStopper
	AudioDir      func(meetingID string) string
}

// DeleteService removes a meeting from Notted: its rows and its audio. It never touches the archive.
type DeleteService struct {
	store         port.MeetingStore
	recorder      activeMeeting
	transcription transcriptionForgetter
	summary       summaryCanceller
	review        summaryCanceller
	session       sessionStopper
	audioDir      func(meetingID string) string
}

// NewDeleteService builds the delete service.
func NewDeleteService(d DeleteDeps) *DeleteService {
	return &DeleteService{store: d.Store, recorder: d.Recorder, transcription: d.Transcription, summary: d.Summary, review: d.Review, session: d.Session, audioDir: d.AudioDir}
}

// Delete removes meetingID. It refuses while the meeting records. Otherwise it stops the meeting's
// transcription, summary and Claude Code session, deletes its rows in one transaction, then removes
// its audio folder. A meeting saved to the archive keeps its folder there.
func (s *DeleteService) Delete(ctx context.Context, meetingID string) error {
	if !domain.ValidMeetingID(meetingID) {
		return &userError{msg: msgMeetingNotFound, err: domain.ErrNotFound}
	}
	m, err := s.store.GetMeeting(ctx, meetingID)
	if errors.Is(err, domain.ErrNotFound) {
		return &userError{msg: msgMeetingNotFound, err: domain.ErrNotFound}
	}
	if err != nil {
		return err
	}
	if m.State == domain.StateRecording || s.recorder.ActiveMeetingID() == meetingID {
		return errors.New(msgStopFirst)
	}
	dir, err := s.audioPath(meetingID)
	if err != nil {
		return err
	}

	s.transcription.Forget(meetingID)
	s.summary.Cancel(meetingID)
	if s.review != nil {
		s.review.Cancel(meetingID)
	}
	if err := s.session.Stop(meetingID); err != nil {
		log.Printf("delete: stop session %s: %v", meetingID, err)
	}

	if err := s.store.DeleteMeeting(ctx, meetingID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return &userError{msg: msgMeetingNotFound, err: domain.ErrNotFound}
		}
		return fmt.Errorf("The meeting was not deleted: %w", err)
	}
	if err := removeAudio(dir, meetingID); err != nil {
		return &userError{msg: "The meeting was deleted, but its audio folder could not be removed: " + err.Error(), err: ErrAudioLeft}
	}
	return nil
}

// audioPath returns the meeting's audio folder after checking it is exactly <audio folder>/<id>.
func (s *DeleteService) audioPath(meetingID string) (string, error) {
	dir := filepath.Clean(s.audioDir(meetingID))
	if filepath.Base(dir) != meetingID {
		return "", fmt.Errorf("The meeting was not deleted: its audio folder %q is not named after the meeting.", dir)
	}
	return dir, nil
}

// removeAudio deletes <parent>/<id> through a root opened on the parent, so the removal cannot leave it:
// a symlink named <id> is unlinked, never followed. A missing folder is not an error.
func removeAudio(dir, meetingID string) error {
	root, err := os.OpenRoot(filepath.Dir(dir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer root.Close()
	if err := root.RemoveAll(meetingID); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// DeletePlan is what Delete would remove, for the confirm.
type DeletePlan struct {
	AudioFiles    int    // chunk files in the meeting's audio folder
	ArchiveFolder string // the meeting's folder in the archive, "" when never saved; Delete leaves it alone
}

// Plan reports what deleting meetingID would remove. It changes nothing.
func (s *DeleteService) Plan(ctx context.Context, meetingID string) (DeletePlan, error) {
	if !domain.ValidMeetingID(meetingID) {
		return DeletePlan{}, &userError{msg: msgMeetingNotFound, err: domain.ErrNotFound}
	}
	m, err := s.store.GetMeeting(ctx, meetingID)
	if errors.Is(err, domain.ErrNotFound) {
		return DeletePlan{}, &userError{msg: msgMeetingNotFound, err: domain.ErrNotFound}
	}
	if err != nil {
		return DeletePlan{}, err
	}
	chunks, err := s.store.ListChunks(ctx, meetingID)
	if err != nil {
		return DeletePlan{}, err
	}
	return DeletePlan{AudioFiles: len(chunks), ArchiveFolder: m.Folder}, nil
}
