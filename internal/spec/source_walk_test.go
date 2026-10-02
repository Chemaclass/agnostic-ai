package spec

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestWalkSourceRoot_LinkedSourceRetainsLexicalPathsAndSkipsNestedAlias(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	target, nested := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(target, "assets", "data.txt"), "Data.\n")
	mustWrite(t, filepath.Join(nested, "outside.txt"), "Do not descend.\n")
	testutil.DirectoryAlias(t, nested, filepath.Join(target, "nested-alias"))
	testutil.DirectoryAlias(t, target, "direct-alias")
	testutil.DirectoryAlias(t, "direct-alias", "linked-root")
	root := "linked-root" + string(filepath.Separator)
	seen := map[string]bool{}
	err := WalkSourceRoot(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		seen[path] = true
		if path == root {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !d.IsDir() || !info.IsDir() || d.Name() != "linked-root" || info.Name() != "linked-root" {
				t.Errorf("root entry = %v, info = %v, want lexical directory metadata", d, info)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{root, filepath.Join(root, "assets"), filepath.Join(root, "assets", "data.txt"), filepath.Join(root, "nested-alias")}
	if len(seen) != len(want) {
		t.Errorf("walk paths = %v, want only %v", seen, want)
	}
	for _, path := range want {
		if !seen[path] {
			t.Errorf("walk omitted lexical path %q: %v", path, seen)
		}
	}
	calls := 0
	if err := WalkSourceRoot(root, func(string, fs.DirEntry, error) error {
		calls++
		return fs.SkipDir
	}); err != nil || calls != 1 {
		t.Errorf("root SkipDir: calls=%d err=%v, want one call and nil", calls, err)
	}
}

func TestWalkSourceRoot_LinkedSourceMissingRootPreservesCallbackError(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	root := "missing-alias" + string(filepath.Separator)
	err := WalkSourceRoot(root, func(path string, d fs.DirEntry, err error) error {
		var pathErr *fs.PathError
		if path != root || d != nil || !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pathErr) || pathErr.Path != root {
			t.Errorf("missing root callback: path=%q entry=%v err=%v", path, d, err)
		}
		return err
	})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("walk error = %v, want missing source", err)
	}
	if err := WalkSourceRoot(root, func(string, fs.DirEntry, error) error { return fs.SkipAll }); err != nil {
		t.Errorf("missing root SkipAll = %v, want nil", err)
	}
}

func TestWalkSourceRoot_EmptyRootIsMissing(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWrite(t, "present.txt", "Do not walk the current directory.\n")
	calls := 0
	err := WalkSourceRoot("", func(path string, d fs.DirEntry, err error) error {
		calls++
		var pathErr *fs.PathError
		if path != "" || d != nil || !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pathErr) || pathErr.Path != "" {
			t.Errorf("empty root callback: path=%q entry=%v err=%v", path, d, err)
		}
		return nil
	})
	if err != nil || calls != 1 {
		t.Errorf("empty root walk: calls=%d err=%v, want one callback and nil", calls, err)
	}
}

func TestWalkSourceRoot_FileRootKeepsLexicalMetadata(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	target := t.TempDir()
	mustWrite(t, filepath.Join(target, "instructions.md"), "Read this.\n")
	testutil.DirectoryAlias(t, target, "linked-dir")
	root := filepath.Join("linked-dir", "instructions.md")
	calls := 0
	err := WalkSourceRoot(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		calls++
		info, err := d.Info()
		if err != nil {
			return err
		}
		if path != root || d.IsDir() || !info.Mode().IsRegular() || d.Name() != "instructions.md" {
			t.Errorf("file root callback: path=%q entry=%v info=%v", path, d, info)
		}
		return nil
	})
	if err != nil || calls != 1 {
		t.Errorf("file root walk: calls=%d err=%v", calls, err)
	}
}

func TestLoad_LinkedSourceDanglingRootIsEmptyAndCycleReportsPath(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	testutil.DirectoryAlias(t, "cycle-b", "cycle-a")
	if entries, err := walkDir("cycle-a", ".md", KindRule, parseMarkdown); err != nil || len(entries) != 0 {
		t.Errorf("dangling root: entries=%v err=%v, want empty source", entries, err)
	}
	testutil.DirectoryAlias(t, "cycle-a", "cycle-b")
	_, err := walkDir("cycle-a", ".md", KindRule, parseMarkdown)
	var pathErr *fs.PathError
	if err == nil || !errors.As(err, &pathErr) || pathErr.Path != "cycle-a" {
		t.Errorf("cycle error = %v, want configured lexical path", err)
	}
}
