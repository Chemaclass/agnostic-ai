package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// guardStoreFromGit keeps facts that were private to a tool out of Git. It
// refuses a store with files Git tracks, and adds the store to
// info/exclude when Git would track new files there, if the store is in
// the working directory's repository. A store outside any repository
// needs nothing.
func guardStoreFromGit(store string) error {
	var err error
	store, err = effectiveMemoryPath(store)
	if err != nil {
		return err
	}
	repo := existingAncestor(store)
	if out, err := exec.Command("git", "-C", repo, "ls-files", "--", store).Output(); err == nil && len(bytes.TrimSpace(out)) > 0 {
		return fmt.Errorf("%s holds files Git tracks, so imported facts would reach a commit; untrack them with `git rm -r --cached %s`", filepath.ToSlash(store), filepath.ToSlash(store))
	}
	// Import writes facts, the index, its record, its lock, and temporary
	// files; a rule that ignores only Markdown leaves the rest committable.
	probes := []string{memoryIndexFile, memoryImportRecord, memoryDeclineRecord, ".lock", ".fact.md.0.tmp"}
	trackable := func() bool {
		for _, name := range probes {
			if gitWouldTrack(repo, filepath.Join(store, name)) {
				return true
			}
		}
		return false
	}
	if !trackable() {
		return nil
	}
	top, err := exec.Command("git", "-C", repo, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("%s: %w", repo, err)
	}
	here, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil || canonicalDir(strings.TrimSpace(string(here))) != canonicalDir(strings.TrimSpace(string(top))) {
		return fmt.Errorf("%s: the Git repository in %s would track it; ignore the folder there first", filepath.ToSlash(store), strings.TrimSpace(string(top)))
	}
	rel, err := filepath.Rel(canonicalDir(strings.TrimSpace(string(top))), canonicalDir(store))
	if err != nil {
		return fmt.Errorf("%s: %w", store, err)
	}
	if err := adapters.ExcludeFromGit("/" + filepath.ToSlash(rel) + "/"); err != nil {
		return err
	}
	if trackable() {
		return fmt.Errorf("%s: Git would still track it; ignore the folder in .gitignore", filepath.ToSlash(store))
	}
	return nil
}

// Resolve existing parents too, since a new store may live below a symlink.
func effectiveMemoryPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	ancestor := abs
	var tail []string
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", fmt.Errorf("%s: %w", ancestor, err)
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", ancestor, err)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("%s: no existing parent", path)
		}
		tail = append(tail, filepath.Base(ancestor))
		ancestor = parent
	}
}

// gitWouldTrack reports whether the repository holding dir would track a
// new file at path: Git reports the path as not ignored. Outside a
// repository it reports false.
func gitWouldTrack(dir, path string) bool {
	err := exec.Command("git", "-C", dir, "check-ignore", "-q", path).Run()
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 1
}

// existingAncestor returns path, or its nearest parent that exists.
func existingAncestor(path string) string {
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}

// memoryStoreLockWait bounds how long import waits for another import
// into the same store.
const memoryStoreLockWait = 5 * time.Second

// lockMemoryStore holds the lock file inside store until the returned
// func runs, so two imports never append index lines at once.
func lockMemoryStore(store string) (func(), error) {
	path := filepath.Join(store, ".lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	deadline := time.Now().Add(memoryStoreLockWait)
	for {
		err := tryLockProjectFile(file)
		if err == nil {
			return func() { _ = file.Close() }, nil
		}
		if !projectLockContended(err) || time.Now().After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
