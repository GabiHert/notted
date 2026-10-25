package wails

import (
	"context"
	"sync"

	"github.com/GabiHert/notted/internal/application/port"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var _ port.EventEmitter = (*Emitter)(nil)

// Emitter sends events to the frontend through the Wails runtime.
type Emitter struct {
	mu  sync.RWMutex
	ctx context.Context
}

// SetContext gives the emitter the Wails context.
func (e *Emitter) SetContext(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ctx = ctx
}

// Context returns the Wails context, or a background context before startup.
func (e *Emitter) Context() context.Context {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}

// Emit sends an event; before startup it drops the event silently.
func (e *Emitter) Emit(name string, payload any) {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx == nil {
		return
	}
	wruntime.EventsEmit(ctx, name, payload)
}

// Quit closes the app; before startup it does nothing.
func (e *Emitter) Quit() {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx != nil {
		wruntime.Quit(ctx)
	}
}

// Show brings the window back (and the app to the front); before startup it does nothing.
func (e *Emitter) Show() {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx != nil {
		wruntime.WindowShow(ctx)
	}
}
