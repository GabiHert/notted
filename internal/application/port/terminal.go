package port

// TerminalManager runs interactive processes in pseudo-terminals, one per key.
type TerminalManager interface {
	Start(key, dir string, argv []string, env []string) error // key = meeting id
	Write(key, data string) error
	Resize(key string, cols, rows uint16) error
	Stop(key string) error
	Running(key string) bool
	StopAll()
}
