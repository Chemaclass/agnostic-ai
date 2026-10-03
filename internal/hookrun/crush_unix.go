//go:build unix

package hookrun

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// crushKillDelay is how long Crush waits between interrupting a hook's
// process group and killing it (shell/exec_unix.go:18).
const crushKillDelay = 2 * time.Second

// crushIsolate starts the program in its own session, as Crush does for
// every program a hook starts (shell/exec_unix.go:20-31, dispatch.go:200),
// so a hook that signals its process group cannot reach hook run.
func crushIsolate(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

// crushStart runs cmd as shell/exec_unix.go:62-80 does: in its own
// session, and on cancellation SIGINT to its process group, then
// SIGKILL once crushKillDelay passes, so a hook that traps INT can still
// exit with its own status.
func crushStart(ctx context.Context, cmd *exec.Cmd) error {
	crushIsolate(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		time.Sleep(crushKillDelay)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})
	defer stop()
	return cmd.Wait()
}
