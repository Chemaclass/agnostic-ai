// Package gitquery answers the Git questions sync asks while it emits:
// where the repository lives, whether it tracks a file, whether it
// ignores a path. Each answer is cached until Reset, so one run starts
// each Git process once however many adapters ask.
package gitquery

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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

type matchResult struct {
	matched map[string]bool
	err     error
}

var (
	layouts       memo[layoutResult]
	tracked       memo[listResult]
	ignored       memo[bool]
	ignoredBy     memo[matchResult]
	excludesFiles memo[string]
)

// Reset forgets every cached answer. Call it when a run starts, so a
// repository that changed since the last run is read again.
func Reset() {
	layouts.reset()
	tracked.reset()
	ignored.reset()
	ignoredBy.reset()
	excludesFiles.reset()
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

// TrackedAnyCase is Tracked for a disk that reads one file under any letter
// case, so .claude/Settings.Local.json counts for .claude/settings.local.json.
// The disk decides, not core.ignorecase, which a clone can set either way.
// On a case-sensitive disk it is Tracked.
func TrackedAnyCase(root, rel string) (bool, error) {
	if !diskIgnoresCase(root) {
		return Tracked(root, rel)
	}
	return Tracked(root, ":(literal,icase)"+rel)
}

// diskIgnoresCase reports whether dir answers to its own name in another
// letter case. A name without letters cannot tell, so it counts as yes:
// the extra match only blocks more.
func diskIgnoresCase(dir string) bool {
	abs := absKey(dir)
	base := filepath.Base(abs)
	swapped := strings.ToUpper(base)
	if swapped == base {
		swapped = strings.ToLower(base)
	}
	if swapped == base {
		return true
	}
	info, err := os.Stat(abs)
	if err != nil {
		return true
	}
	other, err := os.Stat(filepath.Join(filepath.Dir(abs), swapped))
	return err == nil && os.SameFile(info, other)
}

// Ignored reports whether Git ignores path, a slash path or a folder
// with a trailing separator, in the repository at root.
func Ignored(root, path string) bool {
	return ignored.get(absKey(root)+"\x00"+path, func() bool {
		return exec.Command("git", "-C", absKey(root), "check-ignore", "-q", "--", path).Run() == nil
	})
}

// IgnoredBy reports which of paths, slash paths from a work tree root,
// the gitignore patterns match on their own, without the index or any
// other ignore file. One Git check answers every path, once per run for
// the same patterns and paths.
func IgnoredBy(patterns []byte, paths []string) (map[string]bool, error) {
	key := string(patterns) + "\x00" + strings.Join(paths, "\x00")
	r := ignoredBy.get(key, func() matchResult {
		matched, err := ignoredByFresh(patterns, paths)
		return matchResult{matched: matched, err: err}
	})
	return r.matched, r.err
}

func ignoredByFresh(patterns []byte, paths []string) (map[string]bool, error) {
	matched := map[string]bool{}
	if len(paths) == 0 {
		return matched, nil
	}
	scratch, err := os.MkdirTemp("", "agnostic-ai-ignore-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if out, err := exec.Command("git", "-c", "init.templateDir=", "init", "--quiet", scratch).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w (%s)", err, bytes.TrimSpace(out))
	}
	if err := os.WriteFile(filepath.Join(scratch, ".gitignore"), patterns, 0600); err != nil {
		return nil, err
	}
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}
	cmd := exec.Command("git", "-C", scratch, "-c", "core.excludesFile=", "check-ignore", "--no-index", "-z", "--stdin")
	cmd.Stdin = &stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// check-ignore exits 1 when it matches nothing.
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return nil, fmt.Errorf("%w (%s)", err, bytes.TrimSpace(stderr.Bytes()))
		}
	}
	for _, p := range strings.Split(stdout.String(), "\x00") {
		if p != "" {
			matched[p] = true
		}
	}
	return matched, nil
}

// ExcludesFile returns the core.excludesFile the global or else the
// system Git config names, or "" when neither does.
func ExcludesFile() string {
	var key strings.Builder
	for _, name := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM"} {
		key.WriteString(os.Getenv(name))
		key.WriteByte(0)
	}
	return excludesFiles.get(key.String(), func() string {
		for _, scope := range []string{"--global", "--system"} {
			if out, err := exec.Command("git", "config", scope, "--path", "--get", "core.excludesFile").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
				return strings.TrimSpace(string(out))
			}
		}
		return ""
	})
}
