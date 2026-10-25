package wails

// PermissionsDTO is the Permissions card's state; each value is "allowed", "not_asked" or "denied".
type PermissionsDTO struct {
	Microphone string `json:"microphone"`
	Screen     string `json:"screen"`
}

// GetPermissions reads the microphone and screen recording permissions; it never prompts.
func (a *App) GetPermissions() (PermissionsDTO, error) {
	set, err := a.permissions.Get(a.context())
	if err != nil {
		return PermissionsDTO{}, userFacing(err)
	}
	return PermissionsDTO{Microphone: string(set.Microphone), Screen: string(set.Screen)}, nil
}

// AskPermissions lets macOS prompt for what is still open, then reads both permissions.
func (a *App) AskPermissions() (PermissionsDTO, error) {
	set, err := a.permissions.Ask(a.context())
	if err != nil {
		return PermissionsDTO{}, userFacing(err)
	}
	return PermissionsDTO{Microphone: string(set.Microphone), Screen: string(set.Screen)}, nil
}

// OpenPermissionSettings opens the System Settings privacy pane for "microphone" or "screen".
func (a *App) OpenPermissionSettings(kind string) error {
	return userFacing(a.permissions.OpenSettings(kind))
}
