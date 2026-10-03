//go:build !unix

package hookrun

import "os/exec"

// crushIsolate does nothing off Unix, as in Crush
// (shell/exec_windows.go:15-18).
func crushIsolate(*exec.Cmd) {}
