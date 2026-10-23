package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeWAV writes a silent 16-bit mono WAV of the given rate and length.
func writeWAV(t *testing.T, path string, rate int, seconds float64) {
	t.Helper()
	n := int(float64(rate) * seconds)
	dataLen := n * 2
	b := make([]byte, 44+dataLen)
	copy(b[0:], "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+dataLen))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(dataLen))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func makeSet(t *testing.T, rate int, durations ...float64) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for i, d := range durations {
		p := filepath.Join(dir, fmt.Sprintf("mic-%06d.wav", i+1))
		writeWAV(t, p, rate, d)
		paths = append(paths, p)
	}
	return paths
}

func TestCheckChunksPasses(t *testing.T) {
	paths := makeSet(t, 16000, 30.0, 30.4, 12.0)
	if got := CheckChunks(paths, 30); len(got) != 0 {
		t.Fatalf("problems = %v", got)
	}
}

func TestCheckChunksShortMiddleFails(t *testing.T) {
	paths := makeSet(t, 16000, 30.0, 28.5, 12.0)
	got := CheckChunks(paths, 30)
	if len(got) != 1 || !strings.HasPrefix(got[0], paths[1]+": ") {
		t.Fatalf("problems = %v", got)
	}
}

func TestCheckChunksWrongRateFails(t *testing.T) {
	paths := makeSet(t, 22050, 30.0)
	if got := CheckChunks(paths, 30); len(got) == 0 {
		t.Fatal("want a problem for 22.05 kHz")
	}
}

func TestCheckChunksReportOnly(t *testing.T) {
	paths := makeSet(t, 22050, 1.0, 99.0)
	if got := CheckChunks(paths, 0); len(got) != 0 {
		t.Fatalf("problems = %v", got)
	}
}

func TestCheckChunksLastTooLongFails(t *testing.T) {
	paths := makeSet(t, 16000, 30.0, 32.0)
	if got := CheckChunks(paths, 30); len(got) != 1 {
		t.Fatalf("problems = %v", got)
	}
}

func TestCheckChunksUnreadable(t *testing.T) {
	got := CheckChunks([]string{filepath.Join(t.TempDir(), "mic-000001.wav")}, 30)
	if len(got) != 1 {
		t.Fatalf("problems = %v", got)
	}
}
