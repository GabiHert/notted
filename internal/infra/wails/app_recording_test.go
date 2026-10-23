package wails

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/application/service"
	"github.com/bhlabz/notted/internal/domain"
)

type stubStore struct {
	port.MeetingStore
	list []domain.Meeting
}

func (s stubStore) ListMeetings(context.Context) ([]domain.Meeting, error) { return s.list, nil }
func (s stubStore) GetMeeting(_ context.Context, id string) (domain.Meeting, error) {
	for _, m := range s.list {
		if m.ID == id {
			return m, nil
		}
	}
	return domain.Meeting{}, domain.ErrNotFound
}

type failRecorder struct{}

func (failRecorder) Start(string, func(port.HelperEvent)) (port.Recording, error) {
	return nil, errors.New("No microphone or display is available. Connect one and try again.")
}

type nopEmitter struct{}

func (nopEmitter) Emit(string, any) {}

func TestDTOMapping(t *testing.T) {
	end := time.Date(2026, 10, 23, 15, 0, 0, 0, time.UTC)
	m := domain.Meeting{ID: "a", Title: "T", StartedAt: end.Add(-time.Hour), EndedAt: &end, Language: domain.LangPTBR,
		State: domain.StateReady, ClaudeSessionID: "s"}
	d := toMeetingDTO(m)
	if d.StartedAt != "2026-10-23T14:00:00Z" || d.EndedAt != "2026-10-23T15:00:00Z" || !d.HasSession || d.Language != "pt-BR" || d.State != "ready" {
		t.Fatalf("dto = %+v", d)
	}
	if toMeetingDTO(domain.Meeting{}).EndedAt != "" {
		t.Fatal("endedAt must be empty while recording")
	}
}

func TestBoundMethods(t *testing.T) {
	st := stubStore{list: []domain.Meeting{{ID: "b", State: domain.StateReady}, {ID: "a", State: domain.StateRecording}}}
	a := New(&Emitter{}, Deps{
		Recording: service.NewRecordingService(st, failRecorder{}, nopEmitter{}, func(id string) string { return t.TempDir() }, nil),
		Meetings:  st,
	})

	got, err := a.ListMeetings()
	if err != nil || len(got) != 2 || got[0].ID != "b" {
		t.Fatalf("list = %+v, %v", got, err)
	}
	one, err := a.GetMeeting("a")
	if err != nil || one.State != "recording" {
		t.Fatalf("get = %+v, %v", one, err)
	}
	if _, err := a.GetMeeting("zz"); err == nil || err.Error() != "Meeting not found." {
		t.Fatalf("get missing err = %v", err)
	}
	if err := a.StopRecording("zz"); err == nil || err.Error() != "Meeting not found." {
		t.Fatalf("stop missing err = %v", err)
	}
	if _, err := a.StartRecording("x", "en"); err == nil || err.Error() != "No microphone or display is available. Connect one and try again." {
		t.Fatalf("start err = %v", err)
	}
}
