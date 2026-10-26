package wails

import (
	"context"
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
