package builtins

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func IsIntact(name, root string) bool {
	files, err := load(name)
	return err == nil && intact(root, files, contentHash(files))
}

func Materialize(name string) (string, func() error, error) {
	files, err := load(name)
	if err != nil {
		return "", nil, err
	}
	hash := contentHash(files)
	root, cacheErr := materializeCache(files, hash)
	if cacheErr == nil {
		return root, func() error { return nil }, nil
	}
	root, err = os.MkdirTemp("", "agnostic-ai-builtin-"+name+"-")
	if err != nil {
		return "", nil, fmt.Errorf("materialize builtin %q: %w", name, errors.Join(cacheErr, fmt.Errorf("create temporary builtin directory: %w", err)))
	}
	cleanup := func() error {
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("%s: %w", root, err)
		}
		return nil
	}
	if err := writeTree(root, files, hash); err != nil {
		return "", nil, errors.Join(err, cleanup())
	}
	return root, cleanup, nil
}

func materializeCache(files []builtinFile, hash string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache directory: %w", err)
	}
	info, err := os.Stat(cache)
	if err != nil {
		return "", fmt.Errorf("%s: %w", cache, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s: builtin cache root is not a directory", cache)
	}
	parent := filepath.Join(cache, "agnostic-ai", "builtins")
	root := filepath.Join(parent, hash)
	if intact(root, files, hash) {
		return root, nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("%s: %w", parent, err)
	}
	lockPath := filepath.Join(parent, hash+".lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", fmt.Errorf("%s: %w", lockPath, err)
	}
	defer func() { _ = lock.Close() }()
	if err := lockCache(lock); err != nil {
		return "", fmt.Errorf("lock %s: %w", lockPath, err)
	}
	if intact(root, files, hash) {
		return root, nil
	}
	if err := os.RemoveAll(root); err != nil {
		return "", fmt.Errorf("%s: %w", root, err)
	}
	staging, err := os.MkdirTemp(parent, "."+hash+"-")
	if err != nil {
		return "", fmt.Errorf("create builtin staging directory in %s: %w", parent, err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := writeTree(staging, files, hash); err != nil {
		return "", err
	}
	if err := os.Rename(staging, root); err != nil {
		return "", fmt.Errorf("rename %s to %s: %w", staging, root, err)
	}
	return root, nil
}

func writeTree(root string, files []builtinFile, hash string) error {
	for _, file := range files {
		filename := filepath.Join(root, filepath.FromSlash(file.path))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			return fmt.Errorf("%s: %w", filepath.Dir(filename), err)
		}
		if err := writeReadOnly(filename, file.body); err != nil {
			return err
		}
	}
	return writeReadOnly(filepath.Join(root, ".complete"), []byte(hash+"\n"))
}

func writeReadOnly(filename string, body []byte) error {
	if err := os.WriteFile(filename, body, 0o444); err != nil {
		return fmt.Errorf("%s: %w", filename, err)
	}
	if err := os.Chmod(filename, 0o444); err != nil {
		return fmt.Errorf("%s: %w", filename, err)
	}
	return nil
}

func intact(root string, files []builtinFile, hash string) bool {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	expected := map[string][]byte{".complete": []byte(hash + "\n")}
	dirs := map[string]bool{".": true}
	for _, file := range files {
		expected[file.path] = file.body
		for dir := path.Dir(file.path); dir != "."; dir = path.Dir(dir) {
			dirs[dir] = true
		}
	}
	tree := os.DirFS(root)
	err = fs.WalkDir(tree, ".", func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if !dirs[filename] {
				return fmt.Errorf("%s: unexpected builtin cache directory", filename)
			}
			return nil
		}
		body, exists := expected[filename]
		if !exists {
			return fmt.Errorf("%s: unexpected builtin cache file", filename)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
			return fmt.Errorf("%s: builtin cache file is not a read-only regular file", filename)
		}
		got, err := fs.ReadFile(tree, filename)
		if err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		if !bytes.Equal(got, body) {
			return fmt.Errorf("%s: builtin cache file differs from embedded content", filename)
		}
		delete(expected, filename)
		return nil
	})
	return err == nil && len(expected) == 0
}
