package cli

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// companionSysProcAttr starts the companion apart from this console, so
// closing the console does not end it.
func companionSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
}
