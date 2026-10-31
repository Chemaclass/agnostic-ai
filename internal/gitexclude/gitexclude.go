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

var pathLocks = struct {
	sync.Mutex
	entries map[string]*pathLock
}{entries: make(map[string]*pathLock)}

type pathLock struct {
	available chan struct{}
	refs      int
}

type Guard struct {
	path        string
	file        *os.File
	releasePath func()
}

// Acquire holds the process and file locks for path until Release.
func Acquire(path string) (*Guard, error) {
	dir := filepath.Dir(path)
	lockPath, err := filepath.Abs(filepath.Join(dir, "agnostic-ai-exclude.lock"))
	if err != nil {
		return nil, fmt.Errorf("lock path %s: %w", path, err)
	}
	pathLocks.Lock()
	entry := pathLocks.entries[lockPath]
	if entry == nil {
		entry = &pathLock{available: make(chan struct{}, 1)}
		entry.available <- struct{}{}
		pathLocks.entries[lockPath] = entry
	}
	entry.refs++
	pathLocks.Unlock()
	dropRef := func() {
		pathLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(pathLocks.entries, lockPath)
		}
		pathLocks.Unlock()
	}
	deadline := time.Now().Add(LockWait)
	timer := time.NewTimer(LockWait)
	defer timer.Stop()
	select {
	case <-entry.available:
	case <-timer.C:
		dropRef()
		return nil, fmt.Errorf("lock %s: timed out waiting for another update", lockPath)
	}
	releasePath := func() { entry.available <- struct{}{}; dropRef() }
	if err := os.MkdirAll(dir, 0o755); err != nil {
		releasePath()
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		releasePath()
		return nil, fmt.Errorf("%s: %w", lockPath, err)
	}
	for {
		err := tryLock(lock)
		if err == nil {
			break
		}
		if !contended(err) || time.Now().After(deadline) {
			_ = lock.Close()
			releasePath()
			return nil, fmt.Errorf("lock %s: %w", lockPath, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return &Guard{path: lockPath, file: lock, releasePath: releasePath}, nil
}

func (g *Guard) Release() { _ = g.file.Close(); g.releasePath() }

// Lock holds the lock beside path until its returned function is called.
func Lock(path string) (func(), error) {
	guard, err := Acquire(path)
	if err != nil {
		return nil, err
	}
	return guard.Release, nil
}

// Covers reports whether this guard holds the lock for path.
func (g *Guard) Covers(path string) bool {
	if g == nil {
		return false
	}
	lockPath, err := filepath.Abs(filepath.Join(filepath.Dir(path), "agnostic-ai-exclude.lock"))
	return err == nil && lockPath == g.path
}

// Update updates path while this guard holds its lock.
func (g *Guard) Update(path string, edit func(string) string) error {
	lockPath, err := filepath.Abs(filepath.Join(filepath.Dir(path), "agnostic-ai-exclude.lock"))
	if err != nil {
		return fmt.Errorf("lock path %s: %w", path, err)
	}
	if lockPath != g.path {
		return fmt.Errorf("%s: lock does not cover this file", path)
	}
	return update(path, edit)
}

// Update reads and rewrites the file at path with edit under
// its lock. Every read goes through it, so none sees a half-written file.
func Update(path string, edit func(string) string) error {
	release, err := Lock(path)
	if err != nil {
		return err
	}
	defer release()
	return update(path, edit)
}

func update(path string, edit func(string) string) error {
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
