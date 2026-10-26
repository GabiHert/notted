package wails

import (
	"context"
	"errors"
	"testing"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/application/service"
	"github.com/GabiHert/notted/internal/domain"
)

type reviewStore struct{ port.MeetingStore }

func (reviewStore) GetMeeting(_ context.Context, id string) (domain.Meeting, error) {
	if id == "none" {
		return domain.Meeting{}, domain.ErrNotFound
	}
	return domain.Meeting{ID: id}, nil
}

func TestReviewSuggestionsWrapsErrorsForTheUI(t *testing.T) {
	svc := service.NewReviewService(service.ReviewDeps{Store: reviewStore{}, Emit: nopEmitter{}})
	a := New(&Emitter{}, Deps{Review: svc})
	if err := a.ReviewSuggestions("none"); err == nil || err.Error() != "Meeting not found." {
		t.Fatalf("err = %v", err)
	}
	if err := a.ReviewSuggestions("m1"); err == nil || err.Error() != "This meeting has no suggestions yet. Generate the summary first." {
		t.Fatalf("err = %v", err)
	}
}

func TestGetReviewStateIdle(t *testing.T) {
	svc := service.NewReviewService(service.ReviewDeps{Store: reviewStore{}, Emit: nopEmitter{}})
	a := New(&Emitter{}, Deps{Review: svc})
	if got := a.GetReviewState("m1"); got.Running {
		t.Fatalf("got %+v", got)
	}
}

type fakeMode struct {
	on     bool
	err    error
	setErr error
	sets   []bool
}

func (f *fakeMode) GetReviewActWithoutAsking(context.Context) (bool, error) { return f.on, f.err }
func (f *fakeMode) SetReviewActWithoutAsking(_ context.Context, on bool) error {
	f.sets = append(f.sets, on)
	return f.setErr
}

func TestReviewSwitchBoundMethods(t *testing.T) {
	mode := &fakeMode{on: true}
	svc := service.NewReviewService(service.ReviewDeps{Store: reviewStore{}, Mode: mode, Emit: nopEmitter{}})
	a := New(&Emitter{}, Deps{Review: svc})
	if on, err := a.GetReviewActWithoutAsking(); err != nil || !on {
		t.Fatalf("get = %v, %v", on, err)
	}
	if err := a.SetReviewActWithoutAsking(true); err != nil || len(mode.sets) != 1 || !mode.sets[0] {
		t.Fatalf("set = %v, %v", mode.sets, err)
	}
	mode.err = errors.New("boom")
	if _, err := a.GetReviewActWithoutAsking(); err == nil || err.Error() != "Notted could not read the review setting: boom" {
		t.Fatalf("get err = %v", err)
	}
	mode.setErr = errors.New("locked")
	if err := a.SetReviewActWithoutAsking(false); err == nil || err.Error() != "Notted could not save the review setting: locked" {
		t.Fatalf("set err = %v", err)
	}
}
