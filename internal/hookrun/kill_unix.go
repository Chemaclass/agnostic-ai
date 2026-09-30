//go:build !windows

package hookrun

import (
	"errors"
	"os/exec"
	"syscall"
)

// killTree puts the hook in its own process group, so a timeout kills
// the shell and every command it started. The shell is killed on its
// own first: on macOS, a group kill alone can miss it.
func killTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = cmd.Process.Kill()
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
}
