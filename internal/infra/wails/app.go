package wails

import "context"

// App holds every method bound to the frontend.
type App struct {
	ctx     context.Context
	emitter *Emitter
}

// New builds the app with its event emitter.
func New() *App {
	e := &Emitter{}
	return &App{emitter: e}
}

// Startup stores the Wails context once the window exists.
func (a *App) Startup(ctx context.Context) { a.ctx = ctx; a.emitter.SetContext(ctx) }

// Shutdown runs when the window closes.
func (a *App) Shutdown(ctx context.Context) {}

// AppName is bound so the generated bindings and the build have one method from the start.
func (a *App) AppName() string { return "Notted" }

// Emitter returns the event emitter services use.
func (a *App) Emitter() *Emitter { return a.emitter }
