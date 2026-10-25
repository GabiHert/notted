package service

import (
	"context"
	"errors"
	"testing"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

type fakeItem struct{ states []bool }

func (f *fakeItem) SetRecording(r bool) { f.states = append(f.states, r) }

type fakeMenuRecorder struct {
	startErr, stopErr error
	gotTitle          string
	gotLang           domain.Language
	starts, stops     int
	stoppedID, active string
	listener          func(bool)
}

func (f *fakeMenuRecorder) Start(_ context.Context, title string, lang domain.Language) (domain.Meeting, error) {
	f.starts++
	f.gotTitle, f.gotLang = title, lang
	if f.startErr != nil {
		return domain.Meeting{}, f.startErr
	}
	return domain.Meeting{ID: "m1"}, nil
}

func (f *fakeMenuRecorder) Stop(_ context.Context, id string) error {
	f.stops++
	f.stoppedID = id
	return f.stopErr
}

func (f *fakeMenuRecorder) ActiveMeetingID() string        { return f.active }
func (f *fakeMenuRecorder) SetStateListener(fn func(bool)) { f.listener = fn }

type listStore struct {
	port.MeetingStore
	list []domain.Meeting
	err  error
}

func (s listStore) ListMeetings(context.Context) ([]domain.Meeting, error) { return s.list, s.err }

type menuRig struct {
	svc   *MenuBarService
	rec   *fakeMenuRecorder
	item  *fakeItem
	emit  *recEmitter
	shown int
}

func newMenuRig(store port.MeetingStore) *menuRig {
	r := &menuRig{rec: &fakeMenuRecorder{}, item: &fakeItem{}, emit: &recEmitter{}}
	r.svc = NewMenuBarService(MenuBarDeps{
		Recorder:   r.rec,
		Meetings:   store,
		Item:       r.item,
		ShowWindow: func() { r.shown++ },
		Emit:       r.emit,
		Context:    context.Background,
	})
	return r
}

func TestMenuBarStartUsesEmptyTitleAndNewestMeetingLanguage(t *testing.T) {
	r := newMenuRig(listStore{list: []domain.Meeting{{ID: "b", Language: domain.LangEN}, {ID: "a", Language: domain.LangPTBR}}})
	r.svc.Start()
	if r.rec.starts != 1 || r.rec.gotTitle != "" || r.rec.gotLang != domain.LangEN {
		t.Fatalf("start = %d %q %q", r.rec.starts, r.rec.gotTitle, r.rec.gotLang)
	}
	if r.shown != 0 || len(r.emit.named("menubar:error")) != 0 {
		t.Fatal("a good start must not show the window or emit an error")
	}
}

func TestMenuBarStartDefaultsToPortugueseWithoutMeetings(t *testing.T) {
	r := newMenuRig(listStore{})
	r.svc.Start()
	if r.rec.gotLang != domain.LangPTBR {
		t.Fatalf("lang = %q", r.rec.gotLang)
	}
}

func TestMenuBarStartDefaultsToPortugueseWhenTheListFails(t *testing.T) {
	r := newMenuRig(listStore{err: errors.New("db closed")})
	r.svc.Start()
	if r.rec.starts != 1 || r.rec.gotLang != domain.LangPTBR {
		t.Fatalf("start = %d %q", r.rec.starts, r.rec.gotLang)
	}
}

func TestMenuBarFailedStartShowsTheWindowWithTheMessage(t *testing.T) {
	r := newMenuRig(listStore{})
	r.rec.startErr = errors.New("The speech model is not installed.")
	r.svc.Start()
	evs := r.emit.named("menubar:error")
	if r.shown != 1 || len(evs) != 1 || evs[0].payload["message"] != "The speech model is not installed." {
		t.Fatalf("shown=%d events=%+v", r.shown, evs)
	}
}

func TestMenuBarStopStopsTheActiveMeeting(t *testing.T) {
	r := newMenuRig(listStore{})
	r.rec.active = "m1"
	r.svc.Stop()
	if r.rec.stops != 1 || r.rec.stoppedID != "m1" || r.shown != 0 {
		t.Fatalf("stop = %d %q shown=%d", r.rec.stops, r.rec.stoppedID, r.shown)
	}
}

func TestMenuBarStopWithNothingRunningResetsTheIcon(t *testing.T) {
	r := newMenuRig(listStore{})
	r.svc.Stop()
	if r.rec.stops != 0 || len(r.item.states) != 1 || r.item.states[0] {
		t.Fatalf("stops=%d states=%v", r.rec.stops, r.item.states)
	}
}

func TestMenuBarFailedStopShowsTheWindowWithTheMessage(t *testing.T) {
	r := newMenuRig(listStore{})
	r.rec.active = "m1"
	r.rec.stopErr = errors.New("Meeting not found.")
	r.svc.Stop()
	evs := r.emit.named("menubar:error")
	if r.shown != 1 || len(evs) != 1 || evs[0].payload["message"] != "Meeting not found." {
		t.Fatalf("shown=%d events=%+v", r.shown, evs)
	}
}

func TestMenuBarIconFollowsTheRecordingState(t *testing.T) {
	r := newMenuRig(listStore{})
	r.svc.Attach()
	if r.rec.listener == nil {
		t.Fatal("Attach must subscribe to recording state changes")
	}
	r.rec.listener(true)
	r.rec.listener(false)
	if len(r.item.states) != 2 || !r.item.states[0] || r.item.states[1] {
		t.Fatalf("states = %v", r.item.states)
	}
}
