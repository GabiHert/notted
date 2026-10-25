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

// StartMicTest listens to the default input for 10 seconds without saving anything.
// The levels arrive as mictest:level events and the result as mictest:done or mictest:error.
func (a *App) StartMicTest() error {
	return userFacing(a.micTest.Start(a.context()))
}

// StopMicTest ends the running microphone test early; with none running it does nothing.
func (a *App) StopMicTest() error {
	return userFacing(a.micTest.Stop())
}
