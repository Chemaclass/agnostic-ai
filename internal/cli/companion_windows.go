package cli

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// companionRunKey holds what starts at login, one value per program.
const companionRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// companionSysProcAttr starts the companion apart from this console, so
// closing the console does not end it.
func companionSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
}

// removeCompanionRunValue deletes the value that starts the companion
// at login, even one whose program is gone. It reports whether one was there.
func removeCompanionRunValue() (bool, error) {
	return removeRunValue(registry.CURRENT_USER, companionRunKey, companionName)
}

func removeRunValue(root registry.Key, path, name string) (bool, error) {
	key, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = key.Close() }()
	err = key.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
