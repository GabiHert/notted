package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
	"github.com/GabiHert/notted/internal/infra/store"
)

type fakeStopper struct {
	forgot, cancelled, stopped []string
	order                      *[]string
	stopErr                    error
}

func (f *fakeStopper) Forget(id string) { f.forgot = append(f.forgot, id); f.note("forget") }
func (f *fakeStopper) Cancel(id string) { f.cancelled = append(f.cancelled, id); f.note("cancel") }
func (f *fakeStopper) Stop(id string) error {
	f.stopped = append(f.stopped, id)
	f.note("session")
	return f.stopErr
}
func (f *fakeStopper) note(s string) {
	if f.order != nil {
		*f.order = append(*f.order, s)
	}
}

type fakeActive struct{ id string }

func (a fakeActive) ActiveMeetingID() string { return a.id }

type delFixture struct {
	st    *store.Store
	stop  *fakeStopper
	svc   *DeleteService
	audio string // the folder that holds one folder per meeting
	ctx   context.Context
	order []string
}

func newDelFixture(t *testing.T) *delFixture {
	t.Helper()
	t.Setenv("NOTTED_HOME", t.TempDir())
	st, err := store.Open(filepath.Join(t.TempDir(), "meetings.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &delFixture{st: st, stop: &fakeStopper{}, audio: t.TempDir(), ctx: context.Background()}
	f.stop.order = &f.order
	f.svc = NewDeleteService(DeleteDeps{
		Store: st, Recorder: fakeActive{}, Transcription: f.stop, Summary: f.stop, Session: f.stop,
		AudioDir: func(id string) string { return filepath.Join(f.audio, id) },
	})
	return f
}

// meeting stores a meeting with one chunk and one segment and writes its audio folder.
func (f *delFixture) meeting(t *testing.T, id string, state domain.MeetingState) {
	t.Helper()
	m := domain.Meeting{ID: id, Title: "Weekly sync", StartedAt: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC), Language: domain.LangEN, State: state}
	if err := f.st.CreateMeeting(f.ctx, m); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(f.audio, id, "mic-000001.wav")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("wav"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.st.AddChunk(f.ctx, domain.Chunk{MeetingID: id, Track: domain.TrackMic, Index: 1, Path: p}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.AddSegment(f.ctx, domain.Segment{MeetingID: id, Track: domain.TrackMic, Text: "hi"}); err != nil {
		t.Fatal(err)
	}
}

func (f *delFixture) exists(id string) bool {
	_, err := f.st.GetMeeting(f.ctx, id)
	return err == nil
}

func TestDeleteRemovesRowsAndAudioAndStopsWork(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateSummarized)
	f.meeting(t, "m2", domain.StateReady)
	if err := f.svc.Delete(f.ctx, "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if f.exists("m1") {
		t.Fatal("the meeting row is still there")
	}
	if cs, _ := f.st.ListChunks(f.ctx, "m1"); len(cs) != 0 {
		t.Fatalf("chunks left: %d", len(cs))
	}
	if ss, _ := f.st.ListSegments(f.ctx, "m1"); len(ss) != 0 {
		t.Fatalf("segments left: %d", len(ss))
	}
	if _, err := os.Stat(filepath.Join(f.audio, "m1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("audio folder still there: %v", err)
	}
	if !f.exists("m2") {
		t.Fatal("another meeting was deleted")
	}
	if _, err := os.Stat(filepath.Join(f.audio, "m2", "mic-000001.wav")); err != nil {
		t.Fatalf("another meeting's audio was touched: %v", err)
	}
	// Work stops first, then the rows go; the order is what keeps a late write from landing on a deleted meeting.
	if got := strings.Join(f.order, ","); got != "forget,cancel,session" {
		t.Fatalf("stop order = %s", got)
	}
	if len(f.stop.forgot) != 1 || f.stop.forgot[0] != "m1" || f.stop.cancelled[0] != "m1" || f.stop.stopped[0] != "m1" {
		t.Fatalf("stopped = %+v", f.stop)
	}
}

func TestDeleteRefusedWhileRecording(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateRecording)
	wantErr(t, f.svc.Delete(f.ctx, "m1"), "Stop the recording before you delete this meeting.")
	if !f.exists("m1") {
		t.Fatal("the meeting was deleted")
	}
	if _, err := os.Stat(filepath.Join(f.audio, "m1", "mic-000001.wav")); err != nil {
		t.Fatalf("audio removed: %v", err)
	}
	if len(f.order) != 0 {
		t.Fatalf("work was stopped for a refused delete: %v", f.order)
	}
}

func TestDeleteRefusedWhileTheRecorderHoldsTheMeeting(t *testing.T) {
	f := newDelFixture(t)
	f.svc.recorder = fakeActive{id: "m1"}
	f.meeting(t, "m1", domain.StateTranscribing) // the row says transcribing but the helper still runs
	wantErr(t, f.svc.Delete(f.ctx, "m1"), "Stop the recording before you delete this meeting.")
	if !f.exists("m1") {
		t.Fatal("the meeting was deleted")
	}
}

func TestDeleteMissingAudioFolderIsNotAnError(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	if err := os.RemoveAll(filepath.Join(f.audio, "m1")); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(f.ctx, "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if f.exists("m1") {
		t.Fatal("row still there")
	}
}

func TestDeleteMissingAudioParentIsNotAnError(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	f.svc.audioDir = func(id string) string { return filepath.Join(f.audio, "never-made", id) }
	if err := f.svc.Delete(f.ctx, "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDeleteUnknownMeeting(t *testing.T) {
	f := newDelFixture(t)
	err := f.svc.Delete(f.ctx, "ghost")
	wantErr(t, err, "Meeting not found.")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err does not wrap ErrNotFound: %v", err)
	}
}

func TestDeleteInvalidIDTouchesNothing(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	for _, id := range []string{"", ".", "..", "../m1", "m1/..", "a/b", "/abs"} {
		wantErr(t, f.svc.Delete(f.ctx, id), "Meeting not found.")
	}
	if len(f.order) != 0 {
		t.Fatalf("work was stopped for an invalid id: %v", f.order)
	}
	if _, err := os.Stat(filepath.Join(f.audio, "m1", "mic-000001.wav")); err != nil {
		t.Fatalf("audio removed: %v", err)
	}
}

func TestDeleteRefusesAnAudioPathThatIsNotExactlyAudioDirOfTheID(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.svc.audioDir = func(string) string { return other } // base name is not the id
	err := f.svc.Delete(f.ctx, "m1")
	if err == nil || !strings.Contains(err.Error(), "audio folder") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(other, "keep")); err != nil {
		t.Fatalf("a folder outside the audio folder was touched: %v", err)
	}
	if !f.exists("m1") {
		t.Fatal("rows must stay when the path check fails")
	}
}

func TestDeleteDoesNotFollowASymlinkedAudioFolder(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(f.audio, "m1")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.audio, "m1")); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(f.ctx, "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatalf("the symlink target was emptied: %v", err)
	}
}

type failDeleteStore struct {
	port.MeetingStore
	err error
}

func (s failDeleteStore) DeleteMeeting(context.Context, string) error { return s.err }

func TestDeleteRowFailureKeepsRowsAndAudio(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	f.svc.store = failDeleteStore{MeetingStore: f.st, err: errors.New("disk is full")}
	wantErr(t, f.svc.Delete(f.ctx, "m1"), "The meeting was not deleted: disk is full")
	if !f.exists("m1") {
		t.Fatal("row gone")
	}
	if _, err := os.Stat(filepath.Join(f.audio, "m1", "mic-000001.wav")); err != nil {
		t.Fatalf("audio removed although the rows stayed: %v", err)
	}
}

func TestDeleteAudioFailureAfterRowsSaysSo(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	if err := os.Chmod(filepath.Join(f.audio, "m1"), 0o500); err != nil { // the file inside cannot be unlinked
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(f.audio, "m1"), 0o700) })
	err := f.svc.Delete(f.ctx, "m1")
	if err == nil || !strings.HasPrefix(err.Error(), "The meeting was deleted, but its audio folder could not be removed: ") {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, ErrAudioLeft) {
		t.Fatalf("err does not wrap ErrAudioLeft: %v", err)
	}
	if f.exists("m1") {
		t.Fatal("row must be gone")
	}
}

func TestDeleteSessionStopFailureDoesNotBlock(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	f.stop.stopErr = errors.New("pty gone")
	if err := f.svc.Delete(f.ctx, "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDeleteLeavesTheArchiveFolderAlone(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateSummarized)
	archive := t.TempDir()
	folder := filepath.Join(archive, "meetings", "acme", "2026-10-25-weekly-sync")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"summary.md", "transcript.md", "suggestions.md"} {
		if err := os.WriteFile(filepath.Join(folder, n), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.st.SetFolder(f.ctx, "m1", "meetings/acme/2026-10-25-weekly-sync"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(f.ctx, "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for _, n := range []string{"summary.md", "transcript.md", "suggestions.md"} {
		if b, err := os.ReadFile(filepath.Join(folder, n)); err != nil || string(b) != n {
			t.Fatalf("archive file %s changed: %q %v", n, b, err)
		}
	}
}

func TestPlanCountsAudioFilesAndNamesTheArchiveFolder(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateSummarized)
	if err := f.st.AddChunk(f.ctx, domain.Chunk{MeetingID: "m1", Track: domain.TrackOthers, Index: 1, Path: "/x/o"}); err != nil {
		t.Fatal(err)
	}
	p, err := f.svc.Plan(f.ctx, "m1")
	if err != nil || p.AudioFiles != 2 || p.ArchiveFolder != "" {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	if err := f.st.SetFolder(f.ctx, "m1", "meetings/acme/x"); err != nil {
		t.Fatal(err)
	}
	p, _ = f.svc.Plan(f.ctx, "m1")
	if p.ArchiveFolder != "meetings/acme/x" {
		t.Fatalf("plan = %+v", p)
	}
}

func TestPlanUnknownOrInvalidMeeting(t *testing.T) {
	f := newDelFixture(t)
	_, err := f.svc.Plan(f.ctx, "ghost")
	wantErr(t, err, "Meeting not found.")
	_, err = f.svc.Plan(f.ctx, "../x")
	wantErr(t, err, "Meeting not found.")
}

func TestDeleteCancelsTheReview(t *testing.T) {
	f := newDelFixture(t)
	f.meeting(t, "m1", domain.StateReady)
	review := &fakeStopper{order: &f.order}
	f.svc.review = review
	if err := f.svc.Delete(f.ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	if len(review.cancelled) != 1 || review.cancelled[0] != "m1" {
		t.Fatalf("review cancelled = %v", review.cancelled)
	}
}
