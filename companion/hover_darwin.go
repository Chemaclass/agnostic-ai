//go:build darwin && cgo

package main

/*
void companionWatchPointer(void *window);
int companionMousePressedOver(void *window);
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var pointer struct {
	sync.Mutex
	changed func(over bool)
}

// watchPointer calls changed when the pointer enters or leaves the
// window, even while another app is in front. The web view's own hover
// cannot be trusted there: the circle's window is a panel that is rarely
// the key window.
func watchPointer(window unsafe.Pointer, changed func(over bool)) {
	pointer.Lock()
	pointer.changed = changed
	pointer.Unlock()
	C.companionWatchPointer(window)
}

//export companionPointerChanged
func companionPointerChanged(over C.int) {
	pointer.Lock()
	changed := pointer.changed
	pointer.Unlock()
	if changed != nil {
		changed(over != 0)
	}
}

func mousePressedOver(window unsafe.Pointer) bool {
	return application.InvokeSyncWithResult(func() bool {
		return C.companionMousePressedOver(window) != 0
	})
}
