package emit

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestMkdirAll_CreatesAndIsIdempotent covers the ordinary contract: the
// directory and its parents appear, and a second call on an existing
// directory is not an error.
func TestMkdirAll_CreatesAndIsIdempotent(t *testing.T) {
	dir := testutil.TempCwd(t)
	target := filepath.Join("a", "b", "c")

	if err := mkdirAll(target, dirPerm); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(dir, target)); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: stat err %v", err)
	}
	if err := mkdirAll(target, dirPerm); err != nil {
		t.Fatalf("second call on an existing directory: %v", err)
	}
}

// TestMkdirAll_ConcurrentUnderSharedParent guards #526.
//
// `sync --jobs` emits targets in parallel, and pathLocks keys on a file
// path, so two targets writing different files under one not-yet-existing
// parent call into this helper at the same time. Bare os.MkdirAll
// transiently rejects that with EINVAL or ENOENT: it only absorbs a
// failed Mkdir when its follow-up Lstat finds a directory, and that Lstat
// can also fail while the winning thread is still mid-create.
//
// That is what `.agents/` hit once antigravity added `.agents/rules`
// beside the skills tree codex, amp, zed, crush, openhands, windsurf,
// augment, and kilo already write into.
//
// The underlying failure is kernel-timing dependent, so a single pass
// proves little. Many rounds against a fresh shared parent, each with a
// starting gun, reproduced it reliably before the retry landed. With the
// retry in place this must never fail.
func TestMkdirAll_ConcurrentUnderSharedParent(t *testing.T) {
	const (
		rounds  = 50
		writers = 16
	)
	testutil.TempCwd(t)

	for round := range rounds {
		// A fresh parent per round: the race only exists while the shared
		// component does not yet exist.
		parent := fmt.Sprintf("shared-%d", round)

		var ready, done sync.WaitGroup
		ready.Add(writers)
		done.Add(writers)
		start := make(chan struct{})
		errs := make([]error, writers)

		for i := range writers {
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				errs[i] = mkdirAll(filepath.Join(parent, fmt.Sprintf("child-%d", i)), dirPerm)
			}()
		}

		ready.Wait()
		close(start)
		done.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d writer %d: concurrent create under a shared parent failed: %v",
					round, i, err)
			}
		}
	}
}
