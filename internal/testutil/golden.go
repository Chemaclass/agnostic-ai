package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// WalkRel returns every file under root as a slash-form path relative to
// root, in walk order. Directories are skipped. It fails the test on a
// walk error.
func WalkRel(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// LoadExpectedTree reads every file under root into a map keyed by the
// slash-form path relative to root. It is the loader half of a golden
// snapshot comparison.
func LoadExpectedTree(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out, err
}

// CopyEmittedTree copies every file under srcDir to dstDir, keeping the
// relative layout. Files under `.agnostic-ai/` are always skipped: they
// belong to the source tree, not the emitted output. Any slash-form
// relative path in skipRel is skipped too, for entry-point files that
// sync owns rather than the adapter (e.g. CLAUDE.md, AGENTS.md). It
// backs the UPDATE_GOLDEN=1 refresh path of adapter golden tests.
func CopyEmittedTree(srcDir, dstDir string, skipRel ...string) error {
	skip := make(map[string]bool, len(skipRel))
	for _, s := range skipRel {
		skip[s] = true
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if skip[relSlash] || strings.HasPrefix(relSlash, ".agnostic-ai/") {
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}
