//go:build darwin

package menubar

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include "menubar_darwin.h"
*/
import "C"

func install() { C.PhononMenuBarInstall() }

func setRecording(recording bool) {
	v := C.int(0)
	if recording {
		v = 1
	}
	C.PhononMenuBarSetRecording(v)
}

//export phononMenuBarAction
func phononMenuBarAction(action C.int) { dispatchActive(int(action)) }
