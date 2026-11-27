//go:build !windows

package cli

import "syscall"

// companionSysProcAttr gives the companion a session of its own, so
// closing the terminal that started it does not end it.
func companionSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// removeCompanionRunValue does nothing here: only Windows keeps start at
// login in the registry.
func removeCompanionRunValue() (bool, error) { return false, nil }

func readLegacyCompanionRunValue() (companionLegacyRunValue, error) {
	return companionLegacyRunValue{}, nil
}
