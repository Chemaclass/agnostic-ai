// Package gitquery answers the Git questions sync asks while it emits:
// where the repository lives, whether it tracks a file, whether it
// ignores a path. Each answer is cached until Reset, so one run starts
// each Git process once however many adapters ask.
package gitquery

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Layout is what Git reports about the repository around one folder.
type Layout struct {
	GitDir    string
	CommonDir string
	// Prefix is the folder's path from the top level, with a trailing slash.
	Prefix string
	// Toplevel is the work tree root. HasToplevel is false in a bare
	// repository, which has none.
	Toplevel    string
	HasToplevel bool
}

var errNoAnswer = errors.New("git gave no usable answer")

type entry[T any] struct {
	once sync.Once
	val  T
}

type memo[T any] struct {
	mu sync.Mutex
	m  map[string]*entry[T]
}

func (c *memo[T]) get(key string, compute func() T) T {
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]*entry[T]{}
	}
	e, ok := c.m[key]
	if !ok {
		e = &entry[T]{}
		c.m[key] = e
	}
	c.mu.Unlock()
	e.once.Do(func() { e.val = compute() })
	return e.val
}

func (c *memo[T]) reset() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

type layoutResult struct {
	layout Layout
	err    error
}

type listResult struct {
	nonEmpty bool
	err      error
}

var (
	layouts memo[layoutResult]
	tracked memo[listResult]
	ignored memo[bool]
)

// Reset forgets every cached answer. Call it when a run starts, so a
// repository that changed since the last run is read again.
func Reset() {
	layouts.reset()
	tracked.reset()
	ignored.reset()
}

func absKey(dir string) string {
	if dir == "" {
		dir = "."
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// LayoutAt returns the repository layout around dir, one Git process per
// folder per run. It fails outside a repository.
func LayoutAt(dir string) (Layout, error) {
	key := absKey(dir)
	r := layouts.get(key, func() layoutResult {
		layout, err := LayoutAtFresh(key)
		return layoutResult{layout: layout, err: err}
	})
	return r.layout, r.err
}

// LayoutAtFresh reads the current repository layout without using the run cache.
func LayoutAtFresh(dir string) (Layout, error) {
	key := absKey(dir)
	const base = "--path-format=absolute"
	out, err := exec.Command("git", "-C", key, "rev-parse", base, "--show-toplevel", "--git-dir", "--git-common-dir", "--show-prefix").Output()
	hasTop := err == nil
	if err != nil {
		// A bare repository has no top level, so the query without it still answers.
		out, err = exec.Command("git", "-C", key, "rev-parse", base, "--git-dir", "--git-common-dir", "--show-prefix").Output()
	}
	if err != nil {
		return Layout{}, err
	}
	parts := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	for i := range parts {
		parts[i] = strings.TrimSuffix(parts[i], "\r")
	}
	var l Layout
	switch {
	case hasTop && len(parts) == 4:
		l = Layout{Toplevel: parts[0], HasToplevel: true, GitDir: parts[1], CommonDir: parts[2], Prefix: parts[3]}
	case !hasTop && len(parts) == 3:
		l = Layout{GitDir: parts[0], CommonDir: parts[1], Prefix: parts[2]}
	default:
		return Layout{}, errNoAnswer
	}
	return l, nil
}

// Toplevel returns the work tree root around dir.
func Toplevel(dir string) (string, error) {
	l, err := LayoutAt(dir)
	if err != nil {
		return "", err
	}
	if !l.HasToplevel {
		return "", errNoAnswer
	}
	return l.Toplevel, nil
}

// Tracked reports whether the index of the repository at root holds a
// file at rel (a slash path). The error is set when Git could not answer.
func Tracked(root, rel string) (bool, error) {
	key := absKey(root) + "\x00" + rel
	r := tracked.get(key, func() listResult {
		out, err := exec.Command("git", "-C", absKey(root), "ls-files", "--", rel).Output()
		return listResult{nonEmpty: len(out) != 0, err: err}
	})
	return r.nonEmpty, r.err
}

// Ignored reports whether Git ignores path, a slash path or a folder
// with a trailing separator, in the repository at root.
func Ignored(root, path string) bool {
	return ignored.get(absKey(root)+"\x00"+path, func() bool {
		return exec.Command("git", "-C", absKey(root), "check-ignore", "-q", "--", path).Run() == nil
	})
}
