//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// withoutConsoleWindow does nothing here: only Windows opens a console
// window for a command-line program started by a windowed one.
func withoutConsoleWindow(*exec.Cmd) {}

// detached starts command in its own session, so it outlives the
// companion even when the system ends the companion's process group.
func detached(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
