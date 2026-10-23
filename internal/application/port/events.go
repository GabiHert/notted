package port

// EventEmitter sends a named event with a payload to the UI.
type EventEmitter interface {
	Emit(name string, payload any)
}
