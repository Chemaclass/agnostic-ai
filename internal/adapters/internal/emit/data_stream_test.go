package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSession_RefusesToWriteAColonNameOnWindowsButRemovesAnOwnedOne(t *testing.T) {
	testutilChdir(t, t.TempDir())
	sess := NewSession()
	sess.SetProjectRoot(".")
	path := filepath.Join(".claude", "commands", "git:commit.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, dryRun := range []bool{true, false} {
		err := sess.WriteFile(path, "y\n", dryRun)
		if runtime.GOOS == "windows" {
			if err == nil || !strings.Contains(err.Error(), "hidden stream") {
				t.Errorf("WriteFile(%s, dryRun=%v) = %v, want the colon refusal", path, dryRun, err)
			}
		} else if err != nil {
			t.Errorf("WriteFile(%s, dryRun=%v) = %v, want a write", path, dryRun, err)
		}
	}

	capture := NewSession()
	capture.StartCapture()
	if err := capture.WriteFile(path, "y\n", false); (err != nil) != (runtime.GOOS == "windows") {
		t.Errorf("captured WriteFile(%s) = %v, want the colon refusal on Windows only", path, err)
	}

	// An older release wrote this name on Windows, where it is a stream
	// of the file `git`; its ledger entry must still clean up.
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := sess.RemoveOwnedOrphan(path, ContentSum("x\n"))
	if err != nil || !removed {
		t.Errorf("RemoveOwnedOrphan(%s) = %v, %v, want it removed", path, removed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s still exists: %v", path, err)
	}
}
