//go:build unix

package hookrun

import (
	"os"
	"path/filepath"
	"runtime"
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
	for _, command := range []string{"./hooks/guard.sh", "hooks/guard.sh"} {
		r := RunCrush(command, dir, os.Environ(), nil, 5*time.Second, runtime.GOOS)
		if r.TimedOut || r.StartErr != nil || (r.Exit != 2 && r.Exit != 128+15) {
			t.Errorf("%s: result = %+v", command, r)
		}
	}
}
