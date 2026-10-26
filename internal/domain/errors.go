package domain

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrSessionIDAlreadySet = errors.New("the meeting already has a Claude Code session id")
	ErrFolderAlreadySet    = errors.New("the meeting already has an archive folder")
	ErrAppNotRunning       = errors.New("Notted is not running")
)
