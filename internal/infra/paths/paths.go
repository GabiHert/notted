// Copied from github.com/brienze1/quant (internal/infra/paths/paths.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
// Package paths centralizes resolution of filesystem paths used by Notted.
// All callers that need the data directory funnel through this package so that
// tests can redirect data off ~/.notted via the NOTTED_HOME env var without
// touching user runtime data.
package paths

import (
	"os"
	"path/filepath"
)

// Home returns the root data directory for Notted.
// Order of precedence:
//  1. $NOTTED_HOME if non-empty
//  2. $HOME/.notted
//
// The directory is NOT created here; callers must MkdirAll as needed.
func Home() string {
	if v := os.Getenv("NOTTED_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// last-resort fallback so we never panic on lookup
		return ".notted"
	}
	return filepath.Join(home, ".notted")
}

// DBPath returns the SQLite file path under Home.
func DBPath() string { return filepath.Join(Home(), "meetings.sqlite") }

// AudioDir returns the directory holding one meeting's audio chunks.
func AudioDir(meetingID string) string { return filepath.Join(Home(), "audio", meetingID) }

// ModelsDir returns the directory holding downloaded speech models.
func ModelsDir() string { return filepath.Join(Home(), "models") }

// UpdateLog returns the installer's log file under Home.
func UpdateLog() string { return filepath.Join(Home(), "update.log") }

// InstallSource returns the file make install writes with the path of the clone it ran in.
func InstallSource() string { return filepath.Join(Home(), "install-source") }
