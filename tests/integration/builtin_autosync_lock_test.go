//go:build darwin || linux

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestBuiltinAutosync_SessionStartIsSilentWhileAnotherSyncRuns(t *testing.T) {
	dir, _ := autosyncProject(t)
	command := claudeAutosyncCommand(t, dir)
	lock, err := os.OpenFile(filepath.Join(dir, ".agnostic-ai", ".command-lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	must(t, os.Remove(filepath.Join(dir, "AGENTS.md")))

	stdout, stderr := runSessionStart(t, dir, command)
	if stdout != "" || stderr != "" {
		t.Errorf("under lock contention: stdout %q stderr %q, want nothing", stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("synced while another sync held the lock: %v", err)
	}
}
