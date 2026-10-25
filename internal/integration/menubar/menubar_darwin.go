//go:build darwin

package menubar

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include "menubar_darwin.h"
*/
import "C"

func install() { C.NottedMenuBarInstall() }

func setRecording(recording bool) {
	v := C.int(0)
	if recording {
		v = 1
	}
	C.NottedMenuBarSetRecording(v)
}

//export nottedMenuBarAction
func nottedMenuBarAction(action C.int) { dispatchActive(int(action)) }
