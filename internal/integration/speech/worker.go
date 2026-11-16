package speech

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
)

// The speech model runs in a child process, the transcribe worker. A loaded Whisper turbo takes
// about 2 GB, and sherpa-onnx keeps more with each call (about 90 MB per 30 s chunk in a real
// meeting) and does not give it all back on unload. A worker that exits returns all of it, so
// the worker is stopped once a recording is transcribed, and replaced when it grows past
// MaxFootprint.
//
// Wire format, both ways: a 4-byte little-endian length, then that many bytes of JSON. A
// request's JSON is followed by Samples float32 values, little-endian.

var _ port.SpeechEngine = (*ProcessEngine)(nil)

type workerRequest struct {
	Lang       domain.Language `json:"lang"`
	SampleRate int             `json:"sampleRate"`
	Samples    int             `json:"samples"`
}

type workerReply struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
	// Footprint is the worker's memory in bytes after the call, 0 when unknown.
	Footprint uint64 `json:"footprint,omitempty"`
}

// DefaultMaxFootprint is the worker memory past which it is replaced: the model loads at about
// 2 GB, and a fresh worker loads it again in about 1.5 s.
const DefaultMaxFootprint = 3 << 30

// maxWorkerSamples bounds one request: 10 minutes at 48 kHz, far above the 20 s spans sent.
const maxWorkerSamples = 10 * 60 * 48000

func writeFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
	if _, err := w.Write(n[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readFrame(r io.Reader, v any) error {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(n[:])
	if size > 1<<20 {
		return fmt.Errorf("transcribe worker: frame of %d bytes", size)
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeSamples(w io.Writer, samples []float32) error {
	buf := make([]byte, 4*4096)
	for len(samples) > 0 {
		n := min(len(samples), 4096)
		for i, s := range samples[:n] {
			binary.LittleEndian.PutUint32(buf[4*i:], math.Float32bits(s))
		}
		if _, err := w.Write(buf[:4*n]); err != nil {
			return err
		}
		samples = samples[n:]
	}
	return nil
}

func readSamples(r io.Reader, n int) ([]float32, error) {
	b := make([]byte, 4*n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	out := make([]float32, n)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out, nil
}

// Transcriber is what the worker serves; *Engine is one.
type Transcriber interface {
	TranscribeSamples(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error)
}

// ServeWorker answers requests from r on w until r ends. It is the worker's main loop.
func ServeWorker(r io.Reader, w io.Writer, t Transcriber) error {
	return serveWorker(r, w, t, footprint)
}

func serveWorker(r io.Reader, w io.Writer, t Transcriber, mem func() uint64) error {
	br, bw := bufio.NewReader(r), bufio.NewWriter(w)
	for {
		var req workerRequest
		if err := readFrame(br, &req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if req.Samples < 0 || req.Samples > maxWorkerSamples {
			return fmt.Errorf("transcribe worker: %d samples", req.Samples)
		}
		samples, err := readSamples(br, req.Samples)
		if err != nil {
			return err
		}
		var rep workerReply
		rep.Text, err = t.TranscribeSamples(context.Background(), samples, req.SampleRate, req.Lang)
		if err != nil {
			rep.Error = err.Error()
		}
		rep.Footprint = mem()
		if err := writeFrame(bw, rep); err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
}

// ProcessEngine runs transcription in a worker process it starts on first use and stops on
// Unload or after IdleUnload without a call. Calls are serialized, like the in-process engine.
type ProcessEngine struct {
	// Command builds the worker command; ProcessEngine sets its stdin and stdout.
	Command func() *exec.Cmd
	// Files reports whether a language's model files are on disk, without loading anything.
	Files interface {
		ReadyFor(lang domain.Language) (bool, string)
	}
	IdleUnload time.Duration
	// MaxFootprint replaces a worker that grew past it, in bytes; 0 means DefaultMaxFootprint.
	MaxFootprint uint64

	mu       sync.Mutex
	cmd      *exec.Cmd
	in       io.WriteCloser
	out      *bufio.Reader
	exited   chan struct{}
	idle     *time.Timer
	lastUsed time.Time
}

// Ready satisfies port.SpeechEngine: the model files for lang are on disk.
func (p *ProcessEngine) Ready(lang domain.Language) (bool, string) { return p.Files.ReadyFor(lang) }

// Transcribe sends the samples to the worker, starting it when it is not running. A worker that
// fails or dies is stopped, and the next call starts a new one.
func (p *ProcessEngine) Transcribe(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(samples) == 0 {
		return "", fmt.Errorf("empty audio")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.startLocked(); err != nil {
		return "", err
	}
	defer p.touchLocked()

	type result struct {
		rep workerReply
		err error
	}
	done := make(chan result, 1)
	in, out := p.in, p.out
	go func() {
		err := writeFrame(in, workerRequest{Lang: lang, SampleRate: sampleRate, Samples: len(samples)})
		if err == nil {
			err = writeSamples(in, samples)
		}
		var rep workerReply
		if err == nil {
			err = readFrame(out, &rep)
		}
		done <- result{rep, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			p.stopLocked()
			return "", fmt.Errorf("the transcribe worker stopped: %w", r.err)
		}
		if limit := p.maxFootprint(); r.rep.Footprint > limit {
			p.stopLocked() // the next call starts a fresh worker
		}
		if r.rep.Error != "" {
			return "", errors.New(r.rep.Error)
		}
		return r.rep.Text, nil
	case <-ctx.Done():
		p.stopLocked()
		<-done
		return "", ctx.Err()
	}
}

func (p *ProcessEngine) maxFootprint() uint64 {
	if p.MaxFootprint > 0 {
		return p.MaxFootprint
	}
	return DefaultMaxFootprint
}

// Unload stops the worker, which gives the model's memory back to the system. Idempotent.
func (p *ProcessEngine) Unload() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

// Running reports whether a worker process is up.
func (p *ProcessEngine) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil
}

func (p *ProcessEngine) startLocked() error {
	if p.cmd != nil {
		select {
		case <-p.exited:
			p.stopLocked() // it died between calls
		default:
			return nil
		}
	}
	cmd := p.Command()
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the transcribe worker: %w", err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	p.cmd, p.in, p.out, p.exited = cmd, in, bufio.NewReader(out), exited
	return nil
}

// stopLocked closes the worker's stdin so it exits, and kills it when it has not within 5 s.
func (p *ProcessEngine) stopLocked() {
	if p.idle != nil {
		p.idle.Stop()
		p.idle = nil
	}
	if p.cmd == nil {
		return
	}
	_ = p.in.Close()
	select {
	case <-p.exited:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.exited
	}
	p.cmd, p.in, p.out, p.exited = nil, nil, nil, nil
}

func (p *ProcessEngine) touchLocked() {
	if p.cmd == nil {
		return
	}
	p.lastUsed = time.Now()
	d := p.IdleUnload
	if d <= 0 {
		d = defaultIdleUnload
	}
	if p.idle == nil {
		p.idle = time.AfterFunc(d, p.idleUnload)
		return
	}
	p.idle.Reset(d)
}

func (p *ProcessEngine) idleUnload() {
	p.mu.Lock()
	defer p.mu.Unlock()
	d := p.IdleUnload
	if d <= 0 {
		d = defaultIdleUnload
	}
	if time.Since(p.lastUsed) < d {
		return
	}
	p.stopLocked()
}
