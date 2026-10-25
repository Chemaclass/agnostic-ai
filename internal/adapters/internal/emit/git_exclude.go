package emit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ExcludeFromGit adds pattern to the repository's info/exclude, the
// personal ignore list Git never commits, unless a line already holds it.
// It does nothing outside a Git checkout.
func ExcludeFromGit(pattern string) error {
	path, ok := GitExcludePath()
	if !ok {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated, changed := WithGitExclusion(data, pattern)
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WithGitExclusion returns info/exclude content holding pattern, and
// whether that differs from data.
func WithGitExclusion(data []byte, pattern string) ([]byte, bool) {
	text := string(data)
	if slices.Contains(strings.Split(text, "\n"), pattern) {
		return data, false
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text + pattern + "\n"), true
}

// GitExcludePath finds info/exclude in the common Git directory of the
// checkout holding the working directory, so a linked worktree shares
// its main checkout's list.
func GitExcludePath() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		dotGit := filepath.Join(dir, ".git")
		info, err := os.Stat(dotGit)
		if err == nil {
			gitDir := dotGit
			if !info.IsDir() {
				data, err := os.ReadFile(dotGit)
				if err != nil {
					return "", false
				}
				ref, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
				if !found {
					return "", false
				}
				if !filepath.IsAbs(ref) {
					ref = filepath.Join(dir, ref)
				}
				gitDir = ref
			}
			if common, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
				ref := strings.TrimSpace(string(common))
				if !filepath.IsAbs(ref) {
					ref = filepath.Join(gitDir, ref)
				}
				gitDir = ref
			}
			return filepath.Join(gitDir, "info", "exclude"), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
