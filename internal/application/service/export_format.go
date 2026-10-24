package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bhlabz/notted/internal/domain"
)

func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

func meetingFile(date string, m domain.Meeting, profile string) string {
	return "---\n" +
		"date: " + date + "\n" +
		"project: " + m.ProjectKey + "\n" +
		"title: " + jsonString(m.Title) + "\n" +
		"participants: []\n" +
		"source: app\n" +
		"language: " + string(m.Language) + "\n" +
		"meetily_id: \"\"\n" +
		"claude_session_id: " + jsonString(m.ClaudeSessionID) + "\n" +
		"session_owner: " + jsonString(profile) + "\n" +
		"---\n\n" +
		"# " + m.Title + "\n\n" +
		"Participants are typed into `participants` above by hand. Notted gives only `mic` (the person\n" +
		"who recorded) and `others`.\n"
}

func transcriptFile(title string, segs []domain.Segment) string {
	var b strings.Builder
	b.WriteString("# Transcript: " + title + "\n\n")
	b.WriteString(transcriptLines(segs))
	return b.String()
}

// transcriptLines is one "[hh:mm:ss] (track) text" line per segment, each ending in a newline.
func transcriptLines(segs []domain.Segment) string {
	var b strings.Builder
	for _, s := range segs {
		text := strings.Join(strings.Fields(s.Text), " ")
		n := s.StartMs / 1000
		fmt.Fprintf(&b, "[%02d:%02d:%02d] (%s) %s\n", n/3600, n%3600/60, n%60, s.Track, text)
	}
	return b.String()
}

func oneNewline(s string) string { return strings.TrimRight(s, "\n") + "\n" }
