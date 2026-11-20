//go:build darwin || linux

package conversations

import (
	"errors"
	"syscall"
)

func signalShellGroup(group int, force bool) error {
	if group <= 1 || group == syscall.Getpgrp() {
		return ErrShellRefused
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	err := syscall.Kill(-group, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func signalSession(pid int, force bool) error {
	if pid <= 1 || pid == syscall.Getpid() {
		return ErrSessionRefused
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	err := syscall.Kill(pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
