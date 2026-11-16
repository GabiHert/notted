package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
)

func (f *fixture) level(track domain.Track, rms, peak float64) {
	f.rec.onEvent(port.HelperEvent{Kind: "level", Track: track, RMSdB: rms, PeakDB: peak})
}

func TestLevelIsForwardedWithMeetingAndTrack(t *testing.T) {
	f := newFixture(t)
	m, err := f.svc.Start(context.Background(), "a", domain.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	f.level(domain.TrackMic, -30.5, -9.5)
	f.level(domain.TrackOthers, -120, -120)
	ev := f.emit.named("recording:level")
	if len(ev) != 2 {
		t.Fatalf("level events = %+v", ev)
	}
	want := map[string]any{"meetingId": m.ID, "track": "mic", "rmsDb": -30.5, "peakDb": -9.5}
	for k, v := range want {
		if ev[0].payload[k] != v {
			t.Fatalf("payload[%s] = %v, want %v (%+v)", k, ev[0].payload[k], v, ev[0].payload)
		}
	}
	if len(ev[0].payload) != len(want) {
		t.Fatalf("payload has extra keys: %+v", ev[0].payload)
	}
	if ev[1].payload["track"] != "others" || ev[1].payload["rmsDb"] != -120.0 {
		t.Fatalf("others payload = %+v", ev[1].payload)
	}
}

func TestLevelsAreThrottledPerTrack(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Start(context.Background(), "a", domain.LangEN); err != nil {
		t.Fatal(err)
	}
	f.level(domain.TrackMic, -30, -10)
	f.level(domain.TrackMic, -31, -11) // same instant: dropped
	f.level(domain.TrackOthers, -40, -20)
	f.now = f.now.Add(30 * time.Millisecond)
	f.level(domain.TrackMic, -32, -12) // 30 ms later: dropped
	f.now = f.now.Add(40 * time.Millisecond)
	f.level(domain.TrackMic, -33, -13) // 70 ms after the first: sent
	ev := f.emit.named("recording:level")
	if len(ev) != 3 {
		t.Fatalf("got %d level events, want 3: %+v", len(ev), ev)
	}
	if ev[2].payload["rmsDb"] != -33.0 {
		t.Fatalf("last level = %+v", ev[2].payload)
	}
	// One second of the helper's 15 a second per track never exceeds 30 events.
	f2 := newFixture(t)
	if _, err := f2.svc.Start(context.Background(), "a", domain.LangEN); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ { // a misbehaving helper: 60 a second per track
		f2.level(domain.TrackMic, -30, -10)
		f2.level(domain.TrackOthers, -30, -10)
		f2.now = f2.now.Add(time.Second / 60)
	}
	if n := len(f2.emit.named("recording:level")); n > 2*20+2 {
		t.Fatalf("%d level events in one second, want at most about 40", n)
	}
}

func TestNoLevelsWhileTheWindowIsHidden(t *testing.T) {
	f := newFixture(t)
	var visible atomic.Bool
	f.svc.SetVisibility(visible.Load)
	if _, err := f.svc.Start(context.Background(), "a", domain.LangEN); err != nil {
		t.Fatal(err)
	}
	f.level(domain.TrackMic, -30, -10)
	if n := len(f.emit.named("recording:level")); n != 0 {
		t.Fatalf("hidden window got %d levels", n)
	}
	visible.Store(true)
	f.level(domain.TrackMic, -30, -10)
	if n := len(f.emit.named("recording:level")); n != 1 {
		t.Fatalf("visible window got %d levels, want 1", n)
	}
	visible.Store(false)
	f.now = f.now.Add(time.Second)
	f.level(domain.TrackMic, -30, -10)
	if n := len(f.emit.named("recording:level")); n != 1 {
		t.Fatalf("hidden again, got %d levels, want 1", n)
	}
}

func TestNoLevelsBeforeTheMeetingExistsOrAfterStop(t *testing.T) {
	f := newFixture(t)
	f.rec.earlyEvt = &port.HelperEvent{Kind: "level", Track: domain.TrackMic, RMSdB: -30, PeakDB: -10}
	m, err := f.svc.Start(context.Background(), "a", domain.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond) // the early level ran on its own goroutine
	for _, e := range f.emit.named("recording:level") {
		if e.payload["meetingId"] != m.ID {
			t.Fatalf("level for another meeting: %+v", e.payload)
		}
	}
	before := len(f.emit.named("recording:level"))
	onEvent := f.rec.onEvent
	if err := f.svc.Stop(context.Background(), m.ID); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Second)
	onEvent(port.HelperEvent{Kind: "level", Track: domain.TrackMic, RMSdB: -30, PeakDB: -10})
	if n := len(f.emit.named("recording:level")); n != before {
		t.Fatalf("level after stop: %d events, want %d", n, before)
	}
}

func TestLevelWithAnUnknownTrackIsDropped(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Start(context.Background(), "a", domain.LangEN); err != nil {
		t.Fatal(err)
	}
	f.level("", -30, -10)
	f.level("speaker", -30, -10)
	if n := len(f.emit.named("recording:level")); n != 0 {
		t.Fatalf("got %d levels for unknown tracks", n)
	}
}
