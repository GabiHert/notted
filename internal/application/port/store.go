package port

import (
	"context"
	"time"

	"github.com/bhlabz/notted/internal/domain"
)

// MeetingStore persists meetings, their chunks and their segments.
type MeetingStore interface {
	CreateMeeting(ctx context.Context, m domain.Meeting) error
	GetMeeting(ctx context.Context, id string) (domain.Meeting, error) // domain.ErrNotFound
	ListMeetings(ctx context.Context) ([]domain.Meeting, error)        // started_at DESC
	UpdateState(ctx context.Context, id string, s domain.MeetingState) error
	SetEndedAt(ctx context.Context, id string, t time.Time) error
	SetProjectKey(ctx context.Context, id, key string) error
	SetClaudeSessionID(ctx context.Context, id, sessionID string) error // write-once
	SetFolder(ctx context.Context, id, folder string) error             // write-once
	SetSummary(ctx context.Context, id, summaryMD string) error
	SetSuggestions(ctx context.Context, id, suggestionsMD string) error
	AddChunk(ctx context.Context, c domain.Chunk) error                       // duplicate (meeting, track, idx) is an error
	ListChunks(ctx context.Context, meetingID string) ([]domain.Chunk, error) // idx ASC, mic before others
	// PendingChunks lists chunks with transcribed=0 across meetings: meetings.started_at ASC, idx ASC, mic before others.
	PendingChunks(ctx context.Context) ([]domain.Chunk, error)
	MarkChunkTranscribed(ctx context.Context, meetingID string, track domain.Track, idx int) error
	AddSegment(ctx context.Context, s domain.Segment) (int64, error)
	ListSegments(ctx context.Context, meetingID string) ([]domain.Segment, error) // start_ms ASC, mic before others, id ASC
}

// SettingsStore persists the person's settings.
type SettingsStore interface {
	GetSettings(ctx context.Context) (domain.Settings, error)
	SaveSettings(ctx context.Context, s domain.Settings) error // writes all three keys
}
