//go:build unix

package conversations

import (
	"errors"
	"os/exec"
	"syscall"
)

// ownCodexTree starts the app-server in a process group of its own, and
// makes the deadline kill the app-server first, since on macOS a group
// kill alone can miss it, and then the whole group.
func ownCodexTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = cmd.Process.Kill()
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
}

// reapCodexTree kills what the app-server left running in its process
// group once it has exited, so no program it started outlives the
// rename.
func reapCodexTree(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
