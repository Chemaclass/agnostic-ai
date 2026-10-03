//go:build unix

package hookrun

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A hook that signals its whole process group must not reach hook run:
// Crush starts every program in its own session.
func TestRunCrush_HookCannotSignalHookRunsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ntrap 'kill -TERM 0' EXIT\nexit 2\n"
	if err := os.WriteFile(filepath.Join(dir, "hooks", "guard.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// The shell may exit 2 first, or die from its own SIGTERM, as dash
	// does. Crush reads that death as exit 1 for a ./ script it starts
	// through the shebang (dispatch.go:203-208) and as 128+15 for a
	// program it starts itself (exec_unix.go:95-99).
	for command, exits := range map[string][]int{"./hooks/guard.sh": {2, 1}, "hooks/guard.sh": {2, 128 + 15}} {
		r := RunCrush(command, dir, os.Environ(), nil, 5*time.Second, runtime.GOOS)
		if r.TimedOut || r.StartErr != nil || !slices.Contains(exits, r.Exit) {
			t.Errorf("%s: result = %+v, want exit in %v", command, r, exits)
		}
	}
}

// Crush interrupts a timed-out hook before it kills it, so a hook that
// traps INT and exits 2 still blocks.
func TestRunCrush_InterruptsBeforeKillingOnTimeout(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ntrap 'exit 2' INT\nwhile :; do sleep 0.05; done\n"
	if err := os.WriteFile(filepath.Join(dir, "hooks", "slow.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := RunCrush("hooks/slow.sh", dir, os.Environ(), nil, time.Second, runtime.GOOS)
	if r.TimedOut || r.Exit != 2 || DecideHandler("crush", "PreToolUse", Handler{}, r) != Block {
		t.Errorf("result = %+v", r)
	}
}

// hook run exits right after a run, so it must end every process the
// hook started, even one that ignores SIGINT.
func TestRunCrush_LeavesNoProcessBehind(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ntrap '' INT\nsleep 30 &\necho $! > child.pid\nwait\n"
	if err := os.WriteFile(filepath.Join(dir, "hooks", "stuck.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"hooks/stuck.sh", "./hooks/stuck.sh", `sh -c "trap '' INT; sleep 30 & echo \$! > child.pid; wait"`} {
		_ = os.Remove(filepath.Join(dir, "child.pid"))
		r := RunCrush(command, dir, os.Environ(), nil, time.Second, runtime.GOOS)
		if !r.TimedOut {
			t.Errorf("%s: result = %+v", command, r)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "child.pid"))
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Errorf("%s: child %d survived the run: %v", command, pid, err)
		}
	}
}

// The interpreter does not wait for background jobs, so one can start a
// program while hook run is ending the others; that program must not
// escape.
func TestRunCrush_BackgroundJobCannotStartAfterCleanupBegins(t *testing.T) {
	dir := t.TempDir()
	stuck := `sh -c 'trap "" INT; echo $$ >> pids; exec sleep 30'`
	command := stuck + " & (sleep 0.2; " + stuck + ") & sleep 0.1"
	RunCrush(command, dir, os.Environ(), nil, 5*time.Second, runtime.GOOS)
	time.Sleep(500 * time.Millisecond)
	raw, err := os.ReadFile(filepath.Join(dir, "pids"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range strings.Fields(string(raw)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Errorf("process %d survived the run: %v", pid, err)
		}
	}
}
