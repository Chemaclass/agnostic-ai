// Package gitexclude serializes every read and write of a repository's
// info/exclude, which syncs of nested projects and linked worktrees share.
package gitexclude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LockWait bounds how long a writer waits for another process to finish
// its info/exclude update, as a worktree and its main checkout may.
var LockWait = 10 * time.Second

// heldExcludeLocks counts this process's holds on each info/exclude
// lock, so a web change that holds one can revalidate through code that
// takes it again. A second descriptor would block on its own process.
var heldExcludeLocks = struct {
	sync.Mutex
	files  map[string]*os.File
	counts map[string]int
}{files: map[string]*os.File{}, counts: map[string]int{}}

// Lock takes the lock beside the info/exclude at path, waiting
// for another process up to LockWait, so concurrent syncs and web
// changes never lose each other's blocks. Call release when done.
func Lock(path string) (release func(), err error) {
	dir := filepath.Dir(path)
	lockPath := filepath.Join(dir, "agnostic-ai-exclude.lock")
	held := &heldExcludeLocks
	held.Lock()
	defer held.Unlock()
	unlock := func() {
		held.Lock()
		defer held.Unlock()
		if held.counts[lockPath]--; held.counts[lockPath] == 0 {
			_ = held.files[lockPath].Close()
			delete(held.files, lockPath)
			delete(held.counts, lockPath)
		}
	}
	if held.counts[lockPath] > 0 {
		held.counts[lockPath]++
		return unlock, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", lockPath, err)
	}
	deadline := time.Now().Add(LockWait)
	for {
		err := tryLock(lock)
		if err == nil {
			break
		}
		if !contended(err) || time.Now().After(deadline) {
			_ = lock.Close()
			return nil, fmt.Errorf("lock %s: %w", lockPath, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	held.files[lockPath], held.counts[lockPath] = lock, 1
	return unlock, nil
}

// Update reads and rewrites the file at path with edit under
// its lock. Every read goes through it, so none sees a half-written file.
func Update(path string, edit func(string) string) error {
	release, err := Lock(path)
	if err != nil {
		return err
	}
	defer release()
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := edit(string(existing))
	if updated == string(existing) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
