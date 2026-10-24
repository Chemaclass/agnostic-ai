package cli

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

func snapshotBuiltinFile(root *os.Root, path string) (editFileSnapshot, error) {
	info, err := root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return editFileSnapshot{}, nil
	}
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return editFileSnapshot{}, fmt.Errorf("%s: is not a regular project file", path)
	}
	file, err := root.Open(path)
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return editFileSnapshot{}, fmt.Errorf("%s: changed while opening", path)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	after, err := root.Lstat(path)
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if !os.SameFile(opened, after) || opened.Mode() != after.Mode() || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return editFileSnapshot{}, fmt.Errorf("%s: changed while reading", path)
	}
	return editFileSnapshot{info: after, data: data}, nil
}

func checkBuiltinFile(root *os.Root, path string, before editFileSnapshot) error {
	now, err := snapshotBuiltinFile(root, path)
	if err != nil {
		return err
	}
	if !sameBuiltinFile(before, now) {
		return fmt.Errorf("%s: changed since it was read; keeping the current file", path)
	}
	return nil
}

func replaceBuiltinFile(root *os.Root, parents map[string]os.FileInfo, path string, before editFileSnapshot, data []byte, mode os.FileMode) (editFileSnapshot, error) {
	if err := checkBuiltinParents(root, path, parents, false); err != nil {
		return editFileSnapshot{}, err
	}
	if err := checkBuiltinFile(root, path, before); err != nil {
		return editFileSnapshot{}, err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: create pending edit name: %w", path, err)
	}
	temporary := filepath.Join(filepath.Dir(path), ".agnostic-ai-edit-"+hex.EncodeToString(token[:]))
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = file.Close(); _ = root.Remove(temporary) }()
	if err := file.Chmod(mode); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	pending, err := root.Lstat(temporary)
	if err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: pending edit: %w", path, err)
	}
	if !os.SameFile(info, pending) {
		return editFileSnapshot{}, fmt.Errorf("%s: pending edit changed", path)
	}
	if err := checkBuiltinFile(root, path, before); err != nil {
		return editFileSnapshot{}, err
	}
	if err := checkBuiltinParents(root, path, parents, false); err != nil {
		return editFileSnapshot{}, err
	}
	if err := root.Rename(temporary, path); err != nil {
		return editFileSnapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	return editFileSnapshot{data: slices.Clone(data), info: pending}, nil
}

func builtinParentPaths(path string) []string {
	var paths []string
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		paths = append(paths, parent)
		if parent == "." {
			break
		}
	}
	slices.Reverse(paths)
	return paths
}

func captureBuiltinParents(root *os.Root, changes []builtinFileChange) (map[string]os.FileInfo, error) {
	parents := map[string]os.FileInfo{}
	for _, change := range changes {
		for _, path := range builtinParentPaths(change.Path) {
			if _, ok := parents[path]; ok {
				continue
			}
			info, err := root.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				parents[path] = nil
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			if !info.IsDir() {
				return nil, fmt.Errorf("%s: parent is not a real directory", path)
			}
			file, err := root.Open(path)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			opened, err := file.Stat()
			_ = file.Close()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			if !os.SameFile(info, opened) {
				return nil, fmt.Errorf("%s: parent changed while reading", path)
			}
			parents[path] = opened
		}
	}
	return parents, nil
}

func checkBuiltinParents(root *os.Root, path string, parents map[string]os.FileInfo, allowMissing bool) error {
	for _, parent := range builtinParentPaths(path) {
		before, known := parents[parent]
		if !known {
			return fmt.Errorf("%s: parent was not recorded before the change", parent)
		}
		now, err := root.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) && before == nil && allowMissing {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", parent, err)
		}
		if before == nil || !now.IsDir() || !os.SameFile(before, now) || before.Mode() != now.Mode() {
			return fmt.Errorf("%s: parent directory changed; keeping its files", parent)
		}
	}
	return nil
}
