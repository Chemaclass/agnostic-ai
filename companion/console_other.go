//go:build !windows

package main

import "os/exec"

// withoutConsoleWindow does nothing here: only Windows opens a console
// window for a command-line program started by a windowed one.
func withoutConsoleWindow(*exec.Cmd) {}
