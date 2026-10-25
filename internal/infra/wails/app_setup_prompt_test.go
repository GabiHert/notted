package wails

import (
	"context"
	"strings"
	"testing"

	"github.com/GabiHert/notted/internal/application/service"
	"github.com/GabiHert/notted/internal/domain"
)

type noSettings struct{}

func (noSettings) GetSettings(context.Context) (domain.Settings, error) {
	return domain.Settings{}, nil
}
func (noSettings) SaveSettings(context.Context, domain.Settings) error { return nil }

func TestGetSetupPromptBinding(t *testing.T) {
	a := &App{emitter: &Emitter{}, setup: service.NewSetupService(noSettings{}, nil)}
	got, err := a.GetSetupPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Archive folder: not chosen yet") {
		t.Fatalf("%q", got)
	}
}
