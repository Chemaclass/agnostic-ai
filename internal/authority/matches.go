package authority

import (
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Bounds of the project walk that resolves file patterns.
const (
	matchFileLimit   = 5000
	matchSampleLimit = 5
)

// skippedDirs are never walked: they hold dependencies, build output,
// or version control data rather than project files.
var skippedDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true}

// projectFiles lists up to matchFileLimit files under root, as
// slash-separated paths relative to root, and whether the walk stopped
// at the limit.
func projectFiles(root string) ([]string, bool) {
	var files []string
	truncated := false
	errStop := errors.New("limit reached")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		if d.IsDir() {
			if path != root && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) == matchFileLimit {
			truncated = true
			return errStop
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return nil, false
	}
	return files, truncated
}

// matchPattern resolves a file pattern against the walked files. A file
// matches when it or one of its parent directories matches, as a
// protected path covers every file under it.
func matchPattern(pattern string, files []string, truncated bool) *Matches {
	re, err := regexp.Compile(spec.ProtectPatternRegexp(pattern))
	if err != nil {
		return nil
	}
	m := &Matches{Samples: []string{}, Truncated: truncated}
	for _, f := range files {
		if !matchesWithParents(re, f) {
			continue
		}
		m.Count++
		if len(m.Samples) < matchSampleLimit {
			m.Samples = append(m.Samples, f)
		}
	}
	return m
}
