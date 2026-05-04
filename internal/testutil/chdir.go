// Package testutil holds helpers shared across test files.
package testutil

import (
	"os"
	"testing"
)

// Chdir switches the process working directory to dir for the duration of
// the test, restoring the original on cleanup. Process-global; the test
// must not call t.Parallel().
func Chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

// TempCwd creates a fresh t.TempDir(), chdirs into it, and returns the
// path. Combines the most common test-setup pair.
func TempCwd(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	Chdir(t, dir)
	return dir
}
