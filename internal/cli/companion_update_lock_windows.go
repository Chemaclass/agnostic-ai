package cli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
)

func lockCompanionFolder(folder string) (func(), error) {
	path := strings.ToLower(resolvedPath(folder))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`Global\agnostic-ai.companion.%x`, sha256.Sum256([]byte(path))))
	if err != nil {
		return nil, fmt.Errorf("lock the companion in %s: %w", folder, err)
	}
	mutex, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, fmt.Errorf("lock the companion in %s: %w", folder, err)
	}
	// A mutex must be released on the same OS thread that acquired it.
	runtime.LockOSThread()
	status, err := windows.WaitForSingleObject(mutex, 0)
	if err == nil && (status == windows.WAIT_OBJECT_0 || status == windows.WAIT_ABANDONED) {
		return func() {
			_ = windows.ReleaseMutex(mutex)
			_ = windows.CloseHandle(mutex)
			runtime.UnlockOSThread()
		}, nil
	}
	_ = windows.CloseHandle(mutex)
	runtime.UnlockOSThread()
	if err != nil {
		return nil, fmt.Errorf("lock the companion in %s: %w", folder, err)
	}
	return nil, fmt.Errorf("another install or update of the companion in %s is running; wait for it to finish", folder)
}
