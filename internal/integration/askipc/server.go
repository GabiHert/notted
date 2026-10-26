// Package askipc carries one form from a `Notted mcp` process to the running
// app over a unix socket: one JSON line each way per connection.
package askipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

const (
	maxSocketPath   = 103
	maxRequestBytes = 2 << 20
	maxReplyBytes   = 4 << 20
	ioDeadline      = 10 * time.Second
	protocolVersion = 1
)

type request struct {
	V         int             `json:"v"`
	Token     string          `json:"token"`
	MeetingID string          `json:"meeting_id"`
	Kind      string          `json:"kind"`
	Form      json.RawMessage `json:"form"`
}

type reply struct {
	OK     bool               `json:"ok"`
	Answer *domain.FormAnswer `json:"answer,omitempty"`
	Error  string             `json:"error,omitempty"`
}

// Server listens on a unix socket and hands each request to a FormHandler.
type Server struct {
	path     string
	listener net.Listener
	handler  port.FormHandler
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	once     sync.Once
}

// Listen starts serving on socketPath.
func Listen(socketPath string, h port.FormHandler) (*Server, error) {
	if len(socketPath) > maxSocketPath {
		return nil, fmt.Errorf("the socket path is longer than %d bytes: %s", maxSocketPath, socketPath)
	}
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{path: socketPath, listener: l, handler: h, ctx: ctx, cancel: cancel}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serve(c)
		}()
	}
}

// Close stops accepting, cancels in-flight handlers, waits for them and removes the socket.
func (s *Server) Close() error {
	var err error
	s.once.Do(func() {
		err = s.listener.Close()
		s.cancel()
		s.wg.Wait()
		if rerr := os.Remove(s.path); rerr != nil && !os.IsNotExist(rerr) && err == nil {
			err = rerr
		}
	})
	return err
}

var errTooLarge = errors.New("the request is larger than 2 MB")

// readLine reads one line of at most limit bytes.
func readLine(r *bufio.Reader, limit int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > limit {
			return nil, errTooLarge
		}
		switch {
		case err == nil:
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(buf) > 0:
			return buf, nil
		default:
			return nil, err
		}
	}
}

// encodeLine returns v as one JSON line without HTML escaping, so a form that
// passed ParseForm keeps its size on the wire.
func encodeLine(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v) // appends exactly one newline
	return buf.Bytes(), err
}

func writeReply(c net.Conn, rep reply) {
	b, _ := encodeLine(rep)
	_ = c.SetWriteDeadline(time.Now().Add(ioDeadline))
	_, _ = c.Write(b)
}

func fail(c net.Conn, msg string) { writeReply(c, reply{Error: msg}) }

func (s *Server) serve(c net.Conn) {
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(ioDeadline))
	line, err := readLine(bufio.NewReaderSize(c, 64<<10), maxRequestBytes)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			fail(c, err.Error())
		} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
			fail(c, "the request was not received in time")
		}
		return
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		fail(c, "the request is not valid JSON")
		return
	}
	if req.V != protocolVersion {
		fail(c, "unsupported request version")
		return
	}
	kind := domain.FormKind(req.Kind)
	if kind != domain.FormAsk && kind != domain.FormPermission {
		fail(c, "unknown request kind")
		return
	}
	form, err := domain.ParseForm(req.Form)
	if err != nil {
		fail(c, err.Error())
		return
	}
	_ = c.SetReadDeadline(time.Time{})

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	go func() {
		var one [1]byte
		_, _ = c.Read(one[:])
		cancel()
	}()
	ans, err := s.handler.Handle(ctx, port.FormRequest{MeetingID: req.MeetingID, Token: req.Token, Kind: kind, Form: form})
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		fail(c, err.Error())
		return
	}
	writeReply(c, reply{OK: true, Answer: &ans})
}
