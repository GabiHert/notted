package wails

import (
	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/application/service"
)

// App holds every method bound to the frontend.
type App struct {
	emitter   *Emitter
	recording *service.RecordingService
	meetings  port.MeetingStore
}

// Deps are the services the bound methods call.
type Deps struct {
	Recording *service.RecordingService
	Meetings  port.MeetingStore
}

// New builds the app around its event emitter and the services it calls.
func New(e *Emitter, d Deps) *App {
	return &App{emitter: e, recording: d.Recording, meetings: d.Meetings}
}

// AppName is bound so the generated bindings and the build have one method from the start.
func (a *App) AppName() string { return "Notted" }
