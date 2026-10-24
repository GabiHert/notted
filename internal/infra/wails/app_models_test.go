package wails

import (
	"context"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

type fakeModels struct {
	status port.ModelStatus
	steps  []port.ModelProgress
}

func (f fakeModels) Status(lang domain.Language) port.ModelStatus {
	s := f.status
	s.Language = lang
	return s
}

func (f fakeModels) Download(_ context.Context, lang domain.Language, on func(port.ModelProgress)) error {
	for _, p := range f.steps {
		p.Language = lang
		on(p)
	}
	return nil
}

type recEmitter struct{ ch chan [2]any }

func (r recEmitter) Emit(name string, payload any) { r.ch <- [2]any{name, payload} }

func TestGetModelStatus(t *testing.T) {
	a := New(&Emitter{}, Deps{Models: fakeModels{status: port.ModelStatus{Missing: "speech models are not installed", DownloadBytes: 42}}})
	got, err := a.GetModelStatus("pt-BR")
	if err != nil || got.Language != "pt-BR" || got.Ready || got.DownloadBytes != 42 || got.Missing == "" {
		t.Fatalf("status = %+v, %v", got, err)
	}
	if _, err := a.GetModelStatus("fr"); err == nil || err.Error() != `Unknown language "fr". Use pt-BR or en.` {
		t.Fatalf("unknown language err = %v", err)
	}
}

func TestDownloadModelsEmitsProgressEvents(t *testing.T) {
	ch := make(chan [2]any, 8)
	a := New(&Emitter{}, Deps{
		Events: recEmitter{ch},
		Models: fakeModels{steps: []port.ModelProgress{
			{Name: "whisper small.en", Phase: "download", Done: 5, Total: 10},
			{Phase: "error", Err: "boom"},
		}},
	})
	if err := a.DownloadModels("en"); err != nil {
		t.Fatal(err)
	}
	if err := a.DownloadModels("xx"); err == nil || err.Error() != `Unknown language "xx". Use pt-BR or en.` {
		t.Fatalf("unknown language err = %v", err)
	}
	for i, want := range []ModelProgressDTO{
		{Language: "en", Name: "whisper small.en", Phase: "download", BytesDone: 5, BytesTotal: 10},
		{Language: "en", Phase: "error", Error: "boom"},
	} {
		select {
		case ev := <-ch:
			if ev[0] != "models:progress" || ev[1] != want {
				t.Fatalf("event %d = %v, want %+v", i, ev, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("event %d never arrived", i)
		}
	}
}
