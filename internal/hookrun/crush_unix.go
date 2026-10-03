//go:build unix

package hookrun

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	if err := groups.start(cmd); err != nil {
		return err
	}
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
	mu     sync.Mutex
	closed bool
	pids   []int
}

// errCrushReaped refuses a program a background job starts once the run
// is cleaning up.
var errCrushReaped = errors.New("hook run ended; not starting more programs")

// start starts cmd and records its group, or refuses once reap began.
// Holding the lock across Start leaves no window for a group to escape.
func (g *crushGroups) start(cmd *exec.Cmd) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return errCrushReaped
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	g.pids = append(g.pids, cmd.Process.Pid)
	return nil
}

// reap refuses new programs, then ends every recorded group still
// running: SIGINT, up to crushKillDelay for it to exit, then SIGKILL,
// until none is left.
func (g *crushGroups) reap() {
	g.mu.Lock()
	g.closed = true
	pids := g.pids
	g.mu.Unlock()
	// A group that outlives SIGKILL is past what hook run can do; stop
	// after a few passes rather than hang.
	for range 5 {
		live := 0
		for _, pid := range pids {
			if !groupAlive(pid) {
				continue
			}
			live++
			_ = syscall.Kill(-pid, syscall.SIGINT)
			if waitGroupGone(pid, crushKillDelay) {
				continue
			}
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			waitGroupGone(pid, crushKillDelay)
		}
		if live == 0 {
			return
		}
	}
}

func groupAlive(pid int) bool {
	if errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
		return false
	}
	return groupHasLiveMember(pid, procStats)
}

// groupHasLiveMember reports whether a process in group pgid has not
// exited yet. A zombie, exited but not reaped by a parent that is not
// hook run (or by no init, in a container), still takes signals, so the
// group looks alive to kill(2). Where stats can list processes, as from
// /proc on Linux, zombies do not count; elsewhere the group is alive.
func groupHasLiveMember(pgid int, stats func() ([]string, bool)) bool {
	all, ok := stats()
	if !ok {
		return true
	}
	for _, stat := range all {
		state, group, ok := parseProcStat(stat)
		if ok && group == pgid && state != "Z" {
			return true
		}
	}
	return false
}

// parseProcStat reads the state and process group from a
// /proc/<pid>/stat line: "pid (comm) state ppid pgrp ...", where comm
// may hold spaces and parentheses.
func parseProcStat(stat string) (string, int, bool) {
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return "", 0, false
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 3 {
		return "", 0, false
	}
	group, err := strconv.Atoi(fields[2])
	return fields[0], group, err == nil
}

// procStats lists every /proc/<pid>/stat, or false where there is no
// /proc, as on macOS.
func procStats() ([]string, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	var out []string
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat")); err == nil {
			out = append(out, string(raw))
		}
	}
	return out, true
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
