package emit

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestIsDirNotEmpty_MatchesTheRealRemoveError pins the one thing that
// broke: the predicate has to recognize the error the running platform
// actually produces, not the one POSIX names.
//
// The guard in removeEmptyDirs used to test errors.Is(err,
// syscall.ENOTEMPTY). On Windows that is dead code. os.Remove returns
// ERROR_DIR_NOT_EMPTY (145) there, while syscall.ENOTEMPTY is a
// synthetic APPLICATION_ERROR value, and syscall.Errno.Is maps neither
// to the other. So the platform whose message the comment quoted was
// the one platform the guard never fired on, and a concurrent write
// into a directory being pruned failed the whole sync (#918).
//
// Rather than assert a constant, provoke the real error and ask the
// predicate about it. That is what makes this test portable and what
// makes it fail on Windows against the old implementation.
func TestIsDirNotEmpty_MatchesTheRealRemoveError(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "occupant.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := os.Remove(child)
	if err == nil {
		t.Fatal("os.Remove on a non-empty directory unexpectedly succeeded")
	}
	if !isDirNotEmpty(err) {
		t.Errorf("isDirNotEmpty(%v) = false, want true", err)
	}
}

// TestIsDirNotEmpty_RejectsUnrelatedErrors keeps the predicate from
// swallowing a genuine failure. A missing directory is handled by its
// own branch and must not read as "someone else filled it".
func TestIsDirNotEmpty_RejectsUnrelatedErrors(t *testing.T) {
	err := os.Remove(filepath.Join(t.TempDir(), "absent"))
	if err == nil {
		t.Fatal("os.Remove of a missing path unexpectedly succeeded")
	}
	if isDirNotEmpty(err) {
		t.Errorf("isDirNotEmpty(%v) = true for a missing path, want false", err)
	}
	if isDirNotEmpty(nil) {
		t.Error("isDirNotEmpty(nil) = true, want false")
	}
}

// TestRemoveEmptyDirs_SurvivesAConcurrentWrite models the sync that
// failed: codex sweeps its legacy `.agents/agents` tree while
// antigravity is still writing `<name>/agent.md` into the same
// directory. The prune must treat a directory that filled up under it
// as not its to remove, rather than failing the run.
func TestRemoveEmptyDirs_SurvivesAConcurrentWrite(t *testing.T) {
	for range 40 {
		root := t.TempDir()
		var names []string
		for i := range 20 {
			name := filepath.Join(root, "agent-"+string(rune('a'+i)))
			if err := os.MkdirAll(name, 0o755); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, name := range names {
				// Errors are expected and ignored: the pruner may have
				// removed the directory already. The assertion is on
				// the pruner, not on this writer.
				_ = os.WriteFile(filepath.Join(name, "agent.md"), []byte("body"), 0o644)
			}
		}()

		if err := removeEmptyDirs(root); err != nil {
			t.Fatalf("removeEmptyDirs raced with a concurrent write: %v", err)
		}
		wg.Wait()
	}
}
