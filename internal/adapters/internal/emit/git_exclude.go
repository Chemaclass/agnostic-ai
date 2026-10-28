package emit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

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
			_, common, ok := gitDirsAt(dir)
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

// gitDirsAt returns the Git directory of a checkout whose top folder is
// dir, and the common directory a linked worktree shares with its main
// checkout. Both are the same folder in a main checkout.
func gitDirsAt(dir string) (gitDir, common string, ok bool) {
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", "", false
	}
	gitDir = dotGit
	if !info.IsDir() {
		data, err := os.ReadFile(dotGit)
		if err != nil {
			return "", "", false
		}
		ref, found := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
		if !found {
			return "", "", false
		}
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(dir, ref)
		}
		gitDir = ref
	}
	common = gitDir
	if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		ref := strings.TrimSpace(string(data))
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(gitDir, ref)
		}
		common = ref
	}
	return gitDir, common, true
}
