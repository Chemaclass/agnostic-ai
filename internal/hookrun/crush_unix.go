//go:build unix

package hookrun

import (
	"os/exec"
	"syscall"
)

// crushIsolate starts the program in its own session, as Crush does for
// every program a hook starts (shell/exec_unix.go:20-31, dispatch.go:200),
// so a hook that signals its process group cannot reach hook run.
func crushIsolate(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = false
	cmd.SysProcAttr.Setsid = true
}
