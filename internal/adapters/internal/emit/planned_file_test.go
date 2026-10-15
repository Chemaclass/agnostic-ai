package emit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestPlannedFile_RecordsOnlyWatchedPaths(t *testing.T) {
	dir := t.TempDir()
	watched := filepath.Join(dir, "opencode.json")
	other := filepath.Join(dir, "other.json")
	sess := NewSession()
	sess.WatchPlannedFile(watched)
	for _, p := range []string{watched, other} {
		if err := sess.WriteFile(p, "{}\n", false); err != nil {
			t.Fatal(err)
		}
	}
	if got, ok := sess.PlannedFile(watched); !ok || got != "{}\n" {
		t.Errorf("PlannedFile(watched) = %q, %t, want the written content", got, ok)
	}
	if _, ok := sess.PlannedFile(other); ok {
		t.Error("an unwatched path must not be recorded")
	}
}

func TestPlannedFile_DryRunRecordsThePreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	sess := NewSession()
	sess.WatchPlannedFile(path)
	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	err := sess.WriteFile(path, "{\"a\":1}\n", true)
	os.Stdout = stdout
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := sess.PlannedFile(path); !ok || got != "{\"a\":1}\n" {
		t.Errorf("PlannedFile = %q, %t, want the previewed content", got, ok)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("dry run wrote the file: %v", err)
	}
}

func TestPlannedFile_KeptHandEditFallsBackToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.WatchPlannedFile(path)
	sess.KeepEditsSince(map[string]string{path: ContentSum("synced\n")})
	if err := sess.WriteFile(path, "new\n", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.PlannedFile(path); ok {
		t.Error("a kept hand edit must leave the caller to read the disk")
	}
}

func TestPlannedFile_UnmanagedPathIsNotRecorded(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	path := "opencode.json"
	sess := NewSession()
	sess.SetUnmanaged([]string{path})
	sess.WatchPlannedFile(path)
	if err := sess.WriteFile(path, "{}\n", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.PlannedFile(path); ok {
		t.Error("an unmanaged path is never written, so nothing is planned")
	}
}
