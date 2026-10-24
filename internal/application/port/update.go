package port

import "context"

// LineStreamer runs one command to completion and hands each output line to onLine as it arrives.
type LineStreamer interface {
	// Stream runs argv in dir; extraEnv entries override the base environment. It returns nil
	// only when the command exits 0.
	Stream(ctx context.Context, dir string, argv []string, extraEnv []string, onLine func(line string)) error
}

// AppInstaller puts a new app bundle in place of the installed one after Notted quits.
type AppInstaller interface {
	// Start launches the installer detached and returns at once. It waits for pid to exit,
	// replaces installedApp with newApp, reopens installedApp, and logs to logPath.
	Start(pid int, newApp, installedApp, logPath string) error
}
