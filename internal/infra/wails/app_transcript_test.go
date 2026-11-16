package wails

import (
	"context"
	"testing"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/application/service"
	"github.com/GabiHert/phonon/internal/domain"
)

type segStore struct {
	port.MeetingStore
	segs []domain.Segment
}

func (s segStore) ListSegments(context.Context, string) ([]domain.Segment, error) { return s.segs, nil }

func TestGetTranscriptMapsSegments(t *testing.T) {
	tr := service.NewTranscriptionService(segStore{segs: []domain.Segment{
		{ID: 7, Track: domain.TrackOthers, StartMs: 1000, EndMs: 2500, Text: "hi"},
	}}, nil, nil, nil)
	a := New(&Emitter{}, Deps{Transcription: tr})
	got, err := a.GetTranscript("m1")
	want := SegmentDTO{ID: 7, Track: "others", StartMs: 1000, EndMs: 2500, Text: "hi"}
	if err != nil || len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v err %v", got, err)
	}
	empty := New(&Emitter{}, Deps{Transcription: service.NewTranscriptionService(segStore{}, nil, nil, nil)})
	if got, _ := empty.GetTranscript("m1"); got == nil || len(got) != 0 {
		t.Fatalf("empty = %#v, want non-nil empty slice", got)
	}
}

type recordingStore struct{ port.MeetingStore }

func (recordingStore) GetMeeting(_ context.Context, id string) (domain.Meeting, error) {
	return domain.Meeting{ID: id, State: domain.StateRecording}, nil
}

func TestRetranscribeRefusesARecordingAndReportsNoJob(t *testing.T) {
	a := New(&Emitter{}, Deps{Transcription: service.NewTranscriptionService(recordingStore{}, nil, nil, nil)})
	if err := a.Retranscribe("m1"); err == nil || err.Error() != "Only a finished meeting can be transcribed again." {
		t.Fatalf("err = %v", err)
	}
	if got := a.GetRetranscribeStatus("m1"); got != (RetranscribeDTO{}) {
		t.Fatalf("status = %+v", got)
	}
}
