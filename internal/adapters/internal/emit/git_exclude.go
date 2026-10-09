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
	path, _, ok := gitExcludePath()
	if !ok {
		return nil
	}
	return addExcludeLine(path, pattern)
}

// ExcludeOutputFromGit adds the file at path, relative to the working
// directory, to the repository's info/exclude, anchored at the root of
// the checkout so it matches that file alone. It does nothing outside a
// Git checkout or for a path outside it.
func ExcludeOutputFromGit(path string) error {
	exclude, root, ok := gitExcludePath()
	if !ok {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	return addExcludeLine(exclude, "/"+escapeIgnorePattern(filepath.ToSlash(rel)))
}

// escapeIgnorePattern escapes the characters gitignore reads as a glob,
// and trailing spaces, which it would drop, so path matches itself alone.
func escapeIgnorePattern(path string) string {
	var sb strings.Builder
	for _, r := range path {
		if strings.ContainsRune(`\*?[`, r) {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	escaped := sb.String()
	trimmed := strings.TrimRight(escaped, " ")
	return trimmed + strings.Repeat(`\ `, len(escaped)-len(trimmed))
}

func addExcludeLine(path, pattern string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if slices.Contains(strings.Split(string(data), "\n"), pattern) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := os.WriteFile(path, []byte(text+pattern+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// gitExcludePath finds info/exclude in the common Git directory of the
// checkout holding the working directory, so a linked worktree shares
// its main checkout's list, and the root of that checkout.
func gitExcludePath() (exclude, root string, ok bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", false
	}
	for {
		dotGit := filepath.Join(dir, ".git")
		info, err := os.Stat(dotGit)
		if err == nil {
			gitDir := dotGit
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
			if common, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
				ref := strings.TrimSpace(string(common))
				if !filepath.IsAbs(ref) {
					ref = filepath.Join(gitDir, ref)
				}
				gitDir = ref
			}
			return filepath.Join(gitDir, "info", "exclude"), dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}
