package domain

import "time"

// MeetingState is where a meeting is in its life.
type MeetingState string

const (
	StateRecording    MeetingState = "recording"
	StateTranscribing MeetingState = "transcribing"
	StateReady        MeetingState = "ready"
	StateSummarized   MeetingState = "summarized"
)

// Track is one audio source of a meeting.
type Track string

const (
	TrackMic    Track = "mic"
	TrackOthers Track = "others"
)

// Language is the spoken language of a meeting.
type Language string

const (
	LangPTBR Language = "pt-BR"
	LangEN   Language = "en"
)

// Meeting is one recorded meeting.
type Meeting struct {
	ID              string // uuid v4, lower case
	Title           string
	StartedAt       time.Time  // UTC
	EndedAt         *time.Time // nil while recording
	Language        Language
	ClientKey       string // "" = not chosen
	ClaudeSessionID string // "" = none yet
	Folder          string // "" = never exported; else path relative to the archive checkout
	State           MeetingState
	SummaryMD       string // "" = none
	SuggestionsMD   string // "" = none
}

// Chunk is one closed WAV file of one track.
type Chunk struct {
	MeetingID   string
	Track       Track
	Index       int   // 1-based, per track
	StartMs     int64 // relative to the recording start
	EndMs       int64
	Path        string // absolute path of the closed WAV
	Transcribed bool
}

// Segment is one piece of transcribed speech.
type Segment struct {
	ID        int64
	MeetingID string
	Track     Track
	StartMs   int64 // relative to the recording start
	EndMs     int64
	Text      string
}
