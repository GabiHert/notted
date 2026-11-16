// Package whispercpp runs whisper.cpp (pinned and built as static libraries by
// scripts/build-whisper.sh) through a small C wrapper. A Model is not safe for concurrent use.
package whispercpp

/*
#cgo CFLAGS: -I${SRCDIR}/../../../../helpers/whisper/build/install/include -mmacosx-version-min=14.0
#cgo LDFLAGS: -L${SRCDIR}/../../../../helpers/whisper/build/install/lib -lwhisper -lggml -lggml-base -lggml-cpu -lggml-metal -lggml-blas -lc++
#cgo LDFLAGS: -framework Metal -framework MetalKit -framework Foundation -framework Accelerate
#include <stdlib.h>
#include "wrapper.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// SampleRate is the only rate whisper.cpp takes.
const SampleRate = 16000

// Segment is one piece of text with its times in milliseconds from the start of the input.
type Segment struct {
	StartMs int64
	EndMs   int64
	Text    string
}

// Params configures one Transcribe call.
type Params struct {
	// Language is a Whisper code such as "pt" or "en"; it is never detected.
	Language string
	// VADModel is the path of whisper.cpp's Silero VAD model; empty turns VAD off.
	VADModel string
	// Threads is the CPU thread count; 0 means 4.
	Threads int
}

// Model is a loaded whisper.cpp model.
type Model struct {
	ctx *C.struct_whisper_context
}

// Load reads a ggml model file. gpu runs it on Metal.
func Load(path string, gpu bool) (*Model, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	g := C.int(0)
	if gpu {
		g = 1
	}
	ctx := C.phonon_whisper_init(cpath, g)
	if ctx == nil {
		return nil, fmt.Errorf("could not load the speech model %s", path)
	}
	return &Model{ctx: ctx}, nil
}

// Close frees the model. Idempotent.
func (m *Model) Close() {
	if m.ctx != nil {
		C.phonon_whisper_free(m.ctx)
		m.ctx = nil
	}
}

// Transcribe runs 16 kHz mono samples in [-1, 1] through the model.
func (m *Model) Transcribe(samples []float32, p Params) ([]Segment, error) {
	if m.ctx == nil {
		return nil, errors.New("the speech model is closed")
	}
	if len(samples) == 0 {
		return nil, nil
	}
	threads := p.Threads
	if threads <= 0 {
		threads = 4
	}
	lang := C.CString(p.Language)
	defer C.free(unsafe.Pointer(lang))
	vad := C.CString(p.VADModel)
	defer C.free(unsafe.Pointer(vad))
	rc := C.phonon_whisper_full(m.ctx, (*C.float)(unsafe.Pointer(&samples[0])), C.int(len(samples)), lang, vad, C.int(threads))
	if rc != 0 {
		return nil, fmt.Errorf("whisper.cpp failed (code %d)", int(rc))
	}
	n := int(C.phonon_whisper_n_segments(m.ctx))
	out := make([]Segment, 0, n)
	for i := range n {
		ci := C.int(i)
		out = append(out, Segment{
			StartMs: int64(C.phonon_whisper_segment_t0(m.ctx, ci)) * 10,
			EndMs:   int64(C.phonon_whisper_segment_t1(m.ctx, ci)) * 10,
			Text:    C.GoString(C.phonon_whisper_segment_text(m.ctx, ci)),
		})
	}
	return out, nil
}
