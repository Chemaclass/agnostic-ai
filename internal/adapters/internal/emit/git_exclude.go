package emit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitexclude"
)

// ExcludeFromGit adds pattern to the repository's info/exclude, the
// personal ignore list Git never commits, unless a line already holds it.
// It does nothing outside a Git checkout.
func ExcludeFromGit(pattern string) error {
	path, ok := GitExcludePath()
	if !ok {
		return nil
	}
	return gitexclude.Update(path, func(content string) string {
		updated, _ := WithGitExclusion([]byte(content), pattern)
		return string(updated)
	})
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
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			_, common, ok := config.GitDirs(dir)
			if !ok {
				return "", false
			}
			return filepath.Join(common, "info", "exclude"), true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
