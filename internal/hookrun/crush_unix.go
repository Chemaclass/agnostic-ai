//go:build unix

package hookrun

import (
	"context"
	"errors"
	"os/exec"
	"sync"
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

// crushStart runs cmd in its own session and records its process group.
// With interrupt, cancellation sends SIGINT to the group, then SIGKILL
// once crushKillDelay passes, as shell/exec_unix.go:62-80 does, so a
// hook that traps INT can still exit with its own status. Without it,
// the caller's exec.CommandContext kills the process alone, as Crush's
// shebang dispatch does (dispatch.go:191-202).
func crushStart(ctx context.Context, cmd *exec.Cmd, groups *crushGroups, interrupt bool) error {
	crushIsolate(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	groups.add(cmd.Process.Pid)
	if interrupt {
		stop := context.AfterFunc(ctx, func() {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
			time.Sleep(crushKillDelay)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		})
		defer stop()
	}
	return cmd.Wait()
}

// crushGroups are the process groups one hook run started.
type crushGroups struct {
	mu   sync.Mutex
	pids []int
}

func (g *crushGroups) add(pid int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pids = append(g.pids, pid)
}

// reap ends every group still running: SIGINT, up to crushKillDelay for
// it to exit, then SIGKILL, and a bounded wait for it to go.
func (g *crushGroups) reap() {
	g.mu.Lock()
	pids := g.pids
	g.mu.Unlock()
	for _, pid := range pids {
		if !groupAlive(pid) {
			continue
		}
		_ = syscall.Kill(-pid, syscall.SIGINT)
		if waitGroupGone(pid, crushKillDelay) {
			continue
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		waitGroupGone(pid, crushKillDelay)
	}
}

func groupAlive(pid int) bool {
	return !errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH)
}

func waitGroupGone(pid int, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if !groupAlive(pid) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !groupAlive(pid)
}
