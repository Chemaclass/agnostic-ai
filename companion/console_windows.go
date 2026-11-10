package main

import (
	"os/exec"
	"syscall"
)

// withoutConsoleWindow keeps Windows from opening a console window for
// a command-line program started by this windowed one.
func withoutConsoleWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
