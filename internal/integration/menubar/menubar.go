// Package menubar puts Notted's icon and menu in the macOS menu bar.
package menubar

import "sync"

// Handlers are called, each on its own goroutine, when the user picks a menu item.
type Handlers struct {
	Start func()
	Stop  func()
	Open  func()
	Quit  func()
}

// Menu actions, shared with the Objective-C side.
const (
	actionStart = iota
	actionStop
	actionOpen
	actionQuit
)

// Item is the menu bar icon.
type Item struct {
	mu       sync.Mutex
	handlers Handlers
}

// New returns an item that shows nothing until Install is called.
func New() *Item { return &Item{} }

// Install shows the icon and routes the menu to h. Call it once.
func (i *Item) Install(h Handlers) {
	i.mu.Lock()
	i.handlers = h
	i.mu.Unlock()
	active.Store(i)
	install()
}

// SetRecording switches the icon and the first menu line between idle and recording.
func (i *Item) SetRecording(recording bool) { setRecording(recording) }

// dispatch runs the handler for action off the caller's thread, so the main thread never waits on it.
func (i *Item) dispatch(action int) {
	i.mu.Lock()
	h := i.handlers
	i.mu.Unlock()
	var fn func()
	switch action {
	case actionStart:
		fn = h.Start
	case actionStop:
		fn = h.Stop
	case actionOpen:
		fn = h.Open
	case actionQuit:
		fn = h.Quit
	}
	if fn != nil {
		go fn()
	}
}
