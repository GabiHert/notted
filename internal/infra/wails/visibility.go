package wails

import "sync/atomic"

// Visibility remembers whether the window's page is visible, as the frontend reports it.
type Visibility struct{ hidden atomic.Bool } // zero value: visible

// Set records whether the window's page is visible.
func (v *Visibility) Set(visible bool) { v.hidden.Store(!visible) }

// Visible reports whether the window's page is visible.
func (v *Visibility) Visible() bool { return !v.hidden.Load() }
