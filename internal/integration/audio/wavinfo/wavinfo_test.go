package wavinfo

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// buildWAV makes a 16-bit PCM WAV from interleaved samples.
func buildWAV(rate, channels int, samples []int16) []byte {
	dataLen := len(samples) * 2
	b := make([]byte, 44+dataLen)
	copy(b[0:], "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+dataLen))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], uint16(channels))
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(b[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(dataLen))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[44+2*i:], uint16(s))
	}
	return b
}

func sine(rate int, seconds, hz, amp float64) []int16 {
	n := int(float64(rate) * seconds)
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(amp * 32767 * math.Sin(2*math.Pi*hz*float64(i)/float64(rate)))
	}
	return out
}

func TestFromBytesSine(t *testing.T) {
	info, err := FromBytes(buildWAV(16000, 1, sine(16000, 1, 440, 0.5)))
	if err != nil {
		t.Fatal(err)
	}
	if info.SampleRate != 16000 || info.Channels != 1 || info.BitsPerSample != 16 {
		t.Fatalf("format = %+v", info)
	}
	if math.Abs(info.DurationSeconds-1.0) > 0.005 {
		t.Fatalf("duration = %v", info.DurationSeconds)
	}
	if math.Abs(info.RMSdBFS-(-9.0)) > 0.5 {
		t.Fatalf("rms = %v", info.RMSdBFS)
	}
	if info.Silent {
		t.Fatal("tone reported silent")
	}
}

func TestFromBytesSilence(t *testing.T) {
	info, err := FromBytes(buildWAV(16000, 1, make([]int16, 16000)))
	if err != nil {
		t.Fatal(err)
	}
	if info.RMSdBFS != -120 || !info.Silent {
		t.Fatalf("silence = %+v", info)
	}
}

func TestFromBytesStereo8k(t *testing.T) {
	info, err := FromBytes(buildWAV(8000, 2, make([]int16, 16000)))
	if err != nil {
		t.Fatal(err)
	}
	if info.SampleRate != 8000 || info.Channels != 2 {
		t.Fatalf("format = %+v", info)
	}
	if math.Abs(info.DurationSeconds-1.0) > 0.005 {
		t.Fatalf("duration = %v", info.DurationSeconds)
	}
}

func TestFromBytesRejectsGarbage(t *testing.T) {
	if _, err := FromBytes([]byte("nope")); err == nil {
		t.Fatal("want error")
	}
}

func TestParseHeader(t *testing.T) {
	h, err := ParseHeader(buildWAV(16000, 1, make([]int16, 10)))
	if err != nil {
		t.Fatal(err)
	}
	if h.Format != 1 || h.Channels != 1 || h.SampleRate != 16000 || h.BitsPerSample != 16 || h.DataBytes != 20 {
		t.Fatalf("header = %+v", h)
	}
}

func TestDecodeWAVStillWorks(t *testing.T) {
	s, rate, err := DecodeWAV(buildWAV(16000, 1, []int16{16384, -16384}))
	if err != nil || rate != 16000 || len(s) != 2 || s[0] != 0.5 {
		t.Fatalf("got %v %d %v", s, rate, err)
	}
}

func TestInspectorReportsSilence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "z.wav")
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+3200)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 16000)
	binary.LittleEndian.PutUint32(h[28:], 32000)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], 3200)
	if err := os.WriteFile(p, append(h, make([]byte, 3200)...), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Inspector{}.Inspect(p)
	if err != nil || !got.Silent {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := (Inspector{}).Inspect(filepath.Join(dir, "missing.wav")); err == nil {
		t.Fatal("want error")
	}
}
