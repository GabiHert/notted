package wails

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/application/service"
	"github.com/GabiHert/notted/internal/domain"
)

type stubStore struct {
	port.MeetingStore
	list []domain.Meeting
}

func (s stubStore) ListMeetings(context.Context) ([]domain.Meeting, error)     { return s.list, nil }
func (s stubStore) ListChunks(context.Context, string) ([]domain.Chunk, error) { return nil, nil }
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

type noActive struct{}

func (noActive) ActiveMeetingID() string { return "" }

type noopStop struct{}

func (noopStop) Forget(string)     {}
func (noopStop) Cancel(string)     {}
func (noopStop) Stop(string) error { return nil }

func TestDeleteMeetingBinding(t *testing.T) {
	st := stubStore{list: []domain.Meeting{{ID: "rec", State: domain.StateRecording}}}
	a := New(&Emitter{}, Deps{
		Delete: service.NewDeleteService(service.DeleteDeps{Store: st, Recorder: noActive{}, Transcription: noopStop{}, Summary: noopStop{}, Session: noopStop{},
			AudioDir: func(id string) string { return t.TempDir() + "/" + id }}),
		Meetings: st,
	})
	if _, err := a.DeleteMeeting("zz"); err == nil || err.Error() != "Meeting not found." {
		t.Fatalf("delete missing err = %v", err)
	}
	if _, err := a.GetDeletePlan("zz"); err == nil || err.Error() != "Meeting not found." {
		t.Fatalf("plan missing err = %v", err)
	}
	if p, err := a.GetDeletePlan("rec"); err != nil || p.AudioFiles != 0 {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	if _, err := a.DeleteMeeting("rec"); err == nil || err.Error() != "Stop the recording before you delete this meeting." {
		t.Fatalf("delete while recording err = %v", err)
	}
}

type listEmitter struct{ got []string }

func (e *listEmitter) Emit(name string, _ any) { e.got = append(e.got, name) }

type okStore struct{ stubStore }

func (okStore) DeleteMeeting(context.Context, string) error { return nil }

func TestDeleteMeetingEmitsMeetingDeletedOnlyOnSuccess(t *testing.T) {
	st := okStore{stubStore{list: []domain.Meeting{{ID: "done", State: domain.StateReady}, {ID: "rec", State: domain.StateRecording}}}}
	ev := &listEmitter{}
	a := New(&Emitter{}, Deps{
		Delete: service.NewDeleteService(service.DeleteDeps{Store: st, Recorder: noActive{}, Transcription: noopStop{}, Summary: noopStop{}, Session: noopStop{},
			AudioDir: func(id string) string { return t.TempDir() + "/" + id }}),
		Events: ev,
	})
	if _, err := a.DeleteMeeting("rec"); err == nil {
		t.Fatal("a recording meeting must not be deleted")
	}
	if len(ev.got) != 0 {
		t.Fatalf("events after a refused delete = %v", ev.got)
	}
	if r, err := a.DeleteMeeting("done"); err != nil || r.Warning != "" {
		t.Fatalf("result = %+v, %v", r, err)
	}
	if len(ev.got) != 1 || ev.got[0] != "meeting:deleted" {
		t.Fatalf("events = %v", ev.got)
	}
}

func TestDeleteMeetingResolvesWithAWarningWhenOnlyTheAudioIsLeft(t *testing.T) {
	st := okStore{stubStore{list: []domain.Meeting{{ID: "done", State: domain.StateReady}}}}
	audio := t.TempDir()
	dir := filepath.Join(audio, "done")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mic.wav"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { // the file inside cannot be unlinked
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	ev := &listEmitter{}
	a := New(&Emitter{}, Deps{
		Delete: service.NewDeleteService(service.DeleteDeps{Store: st, Recorder: noActive{}, Transcription: noopStop{}, Summary: noopStop{}, Session: noopStop{},
			AudioDir: func(id string) string { return filepath.Join(audio, id) }}),
		Events: ev,
	})
	r, err := a.DeleteMeeting("done")
	if err != nil {
		t.Fatalf("the binding must resolve, got %v", err)
	}
	if !strings.HasPrefix(r.Warning, "The meeting was deleted, but its audio folder could not be removed: ") {
		t.Fatalf("warning = %q", r.Warning)
	}
	if len(ev.got) != 1 || ev.got[0] != "meeting:deleted" {
		t.Fatalf("events = %v", ev.got)
	}
}
