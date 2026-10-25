package service

import (
	"context"
	"fmt"

	"github.com/GabiHert/notted/internal/application/port"
)

// Privacy panes of System Settings, by the permission's kind.
var settingsPanes = map[string]string{
	"microphone": "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone",
	"screen":     "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture",
}

// PermissionsService shows the recording permissions and helps fix them.
type PermissionsService struct {
	checker port.PermissionChecker
	opener  port.URLOpener
}

// NewPermissionsService builds the service around the helper-backed checker and the URL opener.
func NewPermissionsService(c port.PermissionChecker, o port.URLOpener) *PermissionsService {
	return &PermissionsService{checker: c, opener: o}
}

// Get reads both permissions without prompting.
func (s *PermissionsService) Get(ctx context.Context) (port.PermissionSet, error) {
	return s.checker.Status(ctx)
}

// Ask lets macOS prompt for what is still open, then reads both permissions.
func (s *PermissionsService) Ask(ctx context.Context) (port.PermissionSet, error) {
	return s.checker.Ask(ctx)
}

// OpenSettings opens the privacy pane of one permission ("microphone" or "screen").
func (s *PermissionsService) OpenSettings(kind string) error {
	url, ok := settingsPanes[kind]
	if !ok {
		return fmt.Errorf("Unknown permission %q. Use microphone or screen.", kind)
	}
	return s.opener.Open(url)
}
