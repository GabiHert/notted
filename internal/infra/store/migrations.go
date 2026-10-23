package store

import (
	"database/sql"
	"fmt"
	"time"
)

// migrations are applied in order; the version is the index plus one. Never edit an applied one.
var migrations = []string{
	`CREATE TABLE meetings (
  id                TEXT PRIMARY KEY,
  title             TEXT NOT NULL,
  started_at        TEXT NOT NULL,
  ended_at          TEXT,
  language          TEXT NOT NULL CHECK (language IN ('pt-BR','en')),
  client_key        TEXT,
  claude_session_id TEXT,
  folder            TEXT,
  state             TEXT NOT NULL CHECK (state IN ('recording','transcribing','ready','summarized')),
  summary_md        TEXT,
  suggestions_md    TEXT
);
CREATE TABLE chunks (
  meeting_id  TEXT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  track       TEXT NOT NULL CHECK (track IN ('mic','others')),
  idx         INTEGER NOT NULL CHECK (idx >= 1),
  start_ms    INTEGER NOT NULL,
  end_ms      INTEGER NOT NULL,
  path        TEXT NOT NULL,
  transcribed INTEGER NOT NULL DEFAULT 0 CHECK (transcribed IN (0,1)),
  PRIMARY KEY (meeting_id, track, idx)
);
CREATE TABLE segments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  meeting_id TEXT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
  track      TEXT NOT NULL CHECK (track IN ('mic','others')),
  start_ms   INTEGER NOT NULL,
  end_ms     INTEGER NOT NULL,
  text       TEXT NOT NULL
);
CREATE INDEX segments_by_meeting_time ON segments(meeting_id, start_ms);
CREATE TABLE settings (
  key   TEXT PRIMARY KEY CHECK (key IN ('profile','checkout_path','project_roots')),
  value TEXT NOT NULL
);
CREATE TRIGGER meetings_session_id_write_once
BEFORE UPDATE OF claude_session_id ON meetings
WHEN OLD.claude_session_id IS NOT NULL AND NEW.claude_session_id IS NOT OLD.claude_session_id
BEGIN SELECT RAISE(ABORT, 'claude_session_id is write-once'); END;
CREATE TRIGGER meetings_folder_write_once
BEFORE UPDATE OF folder ON meetings
WHEN OLD.folder IS NOT NULL AND NEW.folder IS NOT OLD.folder
BEGIN SELECT RAISE(ABORT, 'folder is write-once'); END;`,
}

func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	for i, ddl := range migrations {
		version := i + 1
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ddl); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
