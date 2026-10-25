package wails

// SegmentDTO is one transcript line as the frontend sees it.
type SegmentDTO struct {
	ID      int64  `json:"id"`
	Track   string `json:"track"` // "mic" | "others"
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

// GetTranscript lists a meeting's segments, start ASC, mic before others on a tie.
func (a *App) GetTranscript(id string) ([]SegmentDTO, error) {
	segs, err := a.transcr.Transcript(a.context(), id)
	if err != nil {
		return nil, err
	}
	out := make([]SegmentDTO, 0, len(segs))
	for _, s := range segs {
		out = append(out, SegmentDTO{ID: s.ID, Track: string(s.Track), StartMs: s.StartMs, EndMs: s.EndMs, Text: s.Text})
	}
	return out, nil
}
