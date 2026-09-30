//go:build !windows

package hookrun

import (
	"os/exec"
	"syscall"
)

// killTree puts the hook in its own process group, so a timeout kills
// the shell and every command it started.
func killTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
