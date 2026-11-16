package service

import (
	"time"

	"github.com/GabiHert/phonon/internal/application/port"
	"github.com/GabiHert/phonon/internal/domain"
)

// levelMinInterval spaces the live levels of one track: the helper sends 15 a second per track,
// so this only drops a burst and caps the frontend at about 30 events a second for both tracks.
const levelMinInterval = 50 * time.Millisecond

// SetVisibility attaches the window's visibility; live levels are sent only while it reports true.
// Without it the window counts as visible.
func (s *RecordingService) SetVisibility(fn func() bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible = fn
}

// forwardLevel sends one helper level to the window as "recording:level", for the live wave.
// Nothing goes out while the window is hidden, before the meeting exists, once it is stopping,
// or sooner than levelMinInterval after the track's last one.
func (s *RecordingService) forwardLevel(a *activeRecording, ev port.HelperEvent) {
	if ev.Track != domain.TrackMic && ev.Track != domain.TrackOthers {
		return
	}
	s.mu.Lock()
	visible := s.visible
	ok := a.created && !a.stopping && s.active == a
	s.mu.Unlock()
	if !ok || (visible != nil && !visible()) {
		return
	}
	now := s.now()
	s.mu.Lock()
	if a.lastLevel == nil {
		a.lastLevel = map[domain.Track]time.Time{}
	}
	last, seen := a.lastLevel[ev.Track]
	if seen && now.Sub(last) < levelMinInterval && now.Sub(last) >= 0 {
		s.mu.Unlock()
		return
	}
	a.lastLevel[ev.Track] = now
	s.mu.Unlock()
	s.emit.Emit("recording:level", map[string]any{
		"meetingId": a.meetingID, "track": string(ev.Track), "rmsDb": ev.RMSdB, "peakDb": ev.PeakDB,
	})
}
