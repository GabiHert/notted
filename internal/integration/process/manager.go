// Copied from github.com/brienze1/quant (internal/integration/process/manager.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"unicode/utf8"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/creack/pty"
)

var _ port.TerminalManager = (*Manager)(nil)

// claudeProcess holds the running process and its PTY master. Its pointer is the instance's
// identity: only the instance that m.processes[key] points to may emit events for key.
type claudeProcess struct {
	cmd *exec.Cmd
	ptm *os.File // PTY master
}

// Manager runs one process per key in a pseudo-terminal and streams its output as events.
// Events for a key are emitted under mu and only by the instance that currently owns the key,
// so each instance emits exactly one terminal:exit and nothing after it. emit must not call
// back into the Manager.
type Manager struct {
	emit      port.EventEmitter
	mu        sync.RWMutex
	processes map[string]*claudeProcess // keyed by meeting id
}

// NewManager creates a process manager that emits terminal:data:<key> and terminal:exit:<key>.
func NewManager(emit port.EventEmitter) *Manager {
	return &Manager{emit: emit, processes: make(map[string]*claudeProcess)}
}

// Start runs argv in dir under a PTY. It does nothing when key is already running.
// Nothing starts the process again after it exits.
func (m *Manager) Start(key, dir string, argv []string, env []string) error {
	if len(argv) == 0 {
		return errors.New("no command to start")
	}
	m.mu.Lock()
	if _, running := m.processes[key]; running {
		m.mu.Unlock()
		return nil
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(append([]string{}, env...), "TERM=xterm-256color")

	ptm, err := pty.Start(cmd)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to start in PTY: %w", err)
	}
	_ = pty.Setsize(ptm, &pty.Winsize{Rows: 30, Cols: 120})
	cp := &claudeProcess{cmd: cmd, ptm: ptm}
	m.processes[key] = cp
	m.mu.Unlock()

	// Stream PTY output in a goroutine.
	go func() {
		buf := make([]byte, 32*1024)
		var carry []byte // buffer for incomplete UTF-8 sequences at chunk boundaries

		for {
			n, readErr := ptm.Read(buf)
			if n > 0 {
				data := buf[:n]

				// Prepend any carry from previous read.
				if len(carry) > 0 {
					data = append(carry, data...)
					carry = nil
				}

				// Carry only an incomplete rune at the very end; invalid bytes pass through.
				var tail []byte
				data, tail = splitUTF8(data)
				if len(tail) > 0 {
					carry = tail
				}

				if len(data) > 0 {
					m.emitIfOwner(key, cp, "terminal:data:"+key, string(data))
				}
			}

			if readErr != nil {
				break
			}
		}

		// Wait for process to finish.
		_ = cmd.Wait()
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		_ = ptm.Close()

		// A stopped instance no longer owns the key: Stop already emitted its exit.
		m.mu.Lock()
		if m.processes[key] == cp {
			delete(m.processes, key)
			m.emit.Emit("terminal:exit:"+key, map[string]any{"code": code})
		}
		m.mu.Unlock()
	}()

	return nil
}

// Write sends raw keystrokes to the process.
func (m *Manager) Write(key, data string) error {
	m.mu.RLock()
	cp, exists := m.processes[key]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("no process running for session: %s", key)
	}
	_, err := cp.ptm.Write([]byte(data))
	return err
}

// Stop terminates a running process by key and emits its terminal:exit (code -1) before it
// returns, so a later Start never sees a stale exit. It does nothing for an unknown key.
func (m *Manager) Stop(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp, exists := m.processes[key]
	if !exists {
		return nil
	}
	// Drop the entry so an immediate Start begins a fresh process; the reader goroutine still
	// reaps this one but emits nothing more.
	delete(m.processes, key)

	// Close PTY master — this sends SIGHUP to the process.
	_ = cp.ptm.Close()

	// Also kill the process explicitly in case it doesn't respond to SIGHUP.
	if cp.cmd.Process != nil {
		_ = cp.cmd.Process.Kill()
	}
	m.emit.Emit("terminal:exit:"+key, map[string]any{"code": -1})
	return nil
}

// emitIfOwner emits only while cp still owns key, so a stopped instance's output cannot leak
// into the next instance's stream.
func (m *Manager) emitIfOwner(key string, cp *claudeProcess, name string, payload any) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.processes[key] == cp {
		m.emit.Emit(name, payload)
	}
}

// StopAll stops every running process.
func (m *Manager) StopAll() {
	m.mu.RLock()
	keys := make([]string, 0, len(m.processes))
	for k := range m.processes {
		keys = append(keys, k)
	}
	m.mu.RUnlock()
	for _, k := range keys {
		_ = m.Stop(k)
	}
}

// Running reports whether key has a live process.
func (m *Manager) Running(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.processes[key]
	return ok
}

// Resize resizes the PTY for the given key.
func (m *Manager) Resize(key string, cols, rows uint16) error {
	m.mu.RLock()
	cp, exists := m.processes[key]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("no process running for session: %s", key)
	}
	return pty.Setsize(cp.ptm, &pty.Winsize{Rows: rows, Cols: cols})
}

// splitUTF8 splits off a trailing incomplete rune (at most utf8.UTFMax-1 bytes) to carry to the next read.
// Invalid bytes anywhere else stay in data.
func splitUTF8(data []byte) (emit, carry []byte) {
	for i := 1; i < utf8.UTFMax && i <= len(data); i++ {
		b := data[len(data)-i]
		if !utf8.RuneStart(b) {
			continue
		}
		if !utf8.FullRune(data[len(data)-i:]) {
			c := make([]byte, i)
			copy(c, data[len(data)-i:])
			return data[:len(data)-i], c
		}
		break
	}
	return data, nil
}
