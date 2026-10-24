package wails

import (
	"github.com/bhlabz/notted/internal/application/port"
	"github.com/bhlabz/notted/internal/application/service"
)

// App holds every method bound to the frontend.
type App struct {
	emitter      *Emitter
	recording    *service.RecordingService
	transcr      *service.TranscriptionService
	archive      *service.ArchiveService
	summary      *service.SummaryService
	session      *service.SessionService
	instructions *service.InstructionsService
	setup        *service.SetupService
	meetings     port.MeetingStore
	models       port.ModelManager
	events       port.EventEmitter
}

// Deps are the services the bound methods call.
type Deps struct {
	Recording     *service.RecordingService
	Transcription *service.TranscriptionService
	Archive       *service.ArchiveService
	Summary       *service.SummaryService
	Session       *service.SessionService
	Instructions  *service.InstructionsService
	Setup         *service.SetupService
	Meetings      port.MeetingStore
	Models        port.ModelManager
	// Events overrides the event sink; it defaults to the emitter given to New.
	Events port.EventEmitter
}

// New builds the app around its event emitter and the services it calls.
func New(e *Emitter, d Deps) *App {
	events := d.Events
	if events == nil {
		events = e
	}
	return &App{emitter: e, recording: d.Recording, transcr: d.Transcription, archive: d.Archive, summary: d.Summary, session: d.Session, instructions: d.Instructions, setup: d.Setup, meetings: d.Meetings, models: d.Models, events: events}
}

// AppName is bound so the generated bindings and the build have one method from the start.
func (a *App) AppName() string { return "Notted" }
