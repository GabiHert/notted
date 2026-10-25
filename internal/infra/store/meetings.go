package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/GabiHert/notted/internal/domain"
)

const meetingCols = `id, title, started_at, ended_at, language, project_key, claude_session_id, folder, state, summary_md, suggestions_md`

const trackOrder = `CASE track WHEN 'mic' THEN 0 ELSE 1 END`

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

type scanner interface{ Scan(dest ...any) error }

func scanMeeting(r scanner) (domain.Meeting, error) {
	var m domain.Meeting
	var started string
	var ended, project, session, folder, summary, sugg sql.NullString
	var lang, state string
	if err := r.Scan(&m.ID, &m.Title, &started, &ended, &lang, &project, &session, &folder, &state, &summary, &sugg); err != nil {
		return m, err
	}
	t, err := time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return m, err
	}
	m.StartedAt = t.UTC()
	if ended.Valid {
		e, err := time.Parse(time.RFC3339Nano, ended.String)
		if err != nil {
			return m, err
		}
		e = e.UTC()
		m.EndedAt = &e
	}
	m.Language = domain.Language(lang)
	m.State = domain.MeetingState(state)
	m.ProjectKey, m.ClaudeSessionID, m.Folder = project.String, session.String, folder.String
	m.SummaryMD, m.SuggestionsMD = summary.String, sugg.String
	return m, nil
}

func (s *Store) CreateMeeting(ctx context.Context, m domain.Meeting) error {
	var ended any
	if m.EndedAt != nil {
		ended = fmtTime(*m.EndedAt)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO meetings (`+meetingCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.Title, fmtTime(m.StartedAt), ended, string(m.Language), nullable(m.ProjectKey),
		nullable(m.ClaudeSessionID), nullable(m.Folder), string(m.State), nullable(m.SummaryMD), nullable(m.SuggestionsMD))
	return err
}

func (s *Store) GetMeeting(ctx context.Context, id string) (domain.Meeting, error) {
	m, err := scanMeeting(s.db.QueryRowContext(ctx, `SELECT `+meetingCols+` FROM meetings WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Meeting{}, domain.ErrNotFound
	}
	return m, err
}

func (s *Store) ListMeetings(ctx context.Context) ([]domain.Meeting, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+meetingCols+` FROM meetings ORDER BY started_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Meeting
	for rows.Next() {
		m, err := scanMeeting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMeeting removes a meeting with its segments and chunks in one transaction.
func (s *Store) DeleteMeeting(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // a no-op once committed
	for _, q := range []string{`DELETE FROM segments WHERE meeting_id = ?`, `DELETE FROM chunks WHERE meeting_id = ?`} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM meetings WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit()
}

// updateOne runs an UPDATE for one meeting and returns ErrNotFound when no row matches.
func (s *Store) updateOne(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateState(ctx context.Context, id string, st domain.MeetingState) error {
	return s.updateOne(ctx, `UPDATE meetings SET state = ? WHERE id = ?`, string(st), id)
}

func (s *Store) SetEndedAt(ctx context.Context, id string, t time.Time) error {
	return s.updateOne(ctx, `UPDATE meetings SET ended_at = ? WHERE id = ?`, fmtTime(t), id)
}

func (s *Store) SetProjectKey(ctx context.Context, id, key string) error {
	return s.updateOne(ctx, `UPDATE meetings SET project_key = ? WHERE id = ?`, nullable(key), id)
}

func (s *Store) SetSummary(ctx context.Context, id, summaryMD string) error {
	return s.updateOne(ctx, `UPDATE meetings SET summary_md = ? WHERE id = ?`, nullable(summaryMD), id)
}

func (s *Store) SetSuggestions(ctx context.Context, id, suggestionsMD string) error {
	return s.updateOne(ctx, `UPDATE meetings SET suggestions_md = ? WHERE id = ?`, nullable(suggestionsMD), id)
}

func (s *Store) SetClaudeSessionID(ctx context.Context, id, sessionID string) error {
	return s.setOnce(ctx, "claude_session_id", id, sessionID, domain.ErrSessionIDAlreadySet)
}

func (s *Store) SetFolder(ctx context.Context, id, folder string) error {
	return s.setOnce(ctx, "folder", id, folder, domain.ErrFolderAlreadySet)
}

// setOnce sets a write-once column. col is a fixed identifier from this file, never user input.
func (s *Store) setOnce(ctx context.Context, col, id, value string, already error) error {
	var cur sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT `+col+` FROM meetings WHERE id = ?`, id).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if cur.String == value {
		return nil
	}
	if cur.String != "" {
		return already
	}
	return s.updateOne(ctx, `UPDATE meetings SET `+col+` = ? WHERE id = ?`, nullable(value), id)
}

func (s *Store) AddChunk(ctx context.Context, c domain.Chunk) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chunks (meeting_id, track, idx, start_ms, end_ms, path, transcribed) VALUES (?,?,?,?,?,?,?)`,
		c.MeetingID, string(c.Track), c.Index, c.StartMs, c.EndMs, c.Path, boolInt(c.Transcribed))
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) queryChunks(ctx context.Context, query string, args ...any) ([]domain.Chunk, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Chunk
	for rows.Next() {
		var c domain.Chunk
		var track string
		var tr int
		if err := rows.Scan(&c.MeetingID, &track, &c.Index, &c.StartMs, &c.EndMs, &c.Path, &tr); err != nil {
			return nil, err
		}
		c.Track, c.Transcribed = domain.Track(track), tr == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListChunks(ctx context.Context, meetingID string) ([]domain.Chunk, error) {
	return s.queryChunks(ctx, `SELECT meeting_id, track, idx, start_ms, end_ms, path, transcribed FROM chunks
WHERE meeting_id = ? ORDER BY idx, `+trackOrder, meetingID)
}

func (s *Store) PendingChunks(ctx context.Context) ([]domain.Chunk, error) {
	return s.queryChunks(ctx, `SELECT c.meeting_id, c.track, c.idx, c.start_ms, c.end_ms, c.path, c.transcribed
FROM chunks c JOIN meetings m ON m.id = c.meeting_id
WHERE c.transcribed = 0
ORDER BY m.started_at, c.meeting_id, c.idx, CASE c.track WHEN 'mic' THEN 0 ELSE 1 END`)
}

func (s *Store) MarkChunkTranscribed(ctx context.Context, meetingID string, track domain.Track, idx int) error {
	return s.updateOne(ctx, `UPDATE chunks SET transcribed = 1 WHERE meeting_id = ? AND track = ? AND idx = ?`, meetingID, string(track), idx)
}

func (s *Store) AddSegment(ctx context.Context, sg domain.Segment) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO segments (meeting_id, track, start_ms, end_ms, text) VALUES (?,?,?,?,?)`,
		sg.MeetingID, string(sg.Track), sg.StartMs, sg.EndMs, sg.Text)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListSegments(ctx context.Context, meetingID string) ([]domain.Segment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, meeting_id, track, start_ms, end_ms, text FROM segments
WHERE meeting_id = ? ORDER BY start_ms, `+trackOrder+`, id`, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Segment
	for rows.Next() {
		var sg domain.Segment
		var track string
		if err := rows.Scan(&sg.ID, &sg.MeetingID, &track, &sg.StartMs, &sg.EndMs, &sg.Text); err != nil {
			return nil, err
		}
		sg.Track = domain.Track(track)
		out = append(out, sg)
	}
	return out, rows.Err()
}
