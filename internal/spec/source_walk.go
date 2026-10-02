package spec

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// WalkSourceRoot follows only the root alias and retains lexical callback paths.
func WalkSourceRoot(root string, fn fs.WalkDirFunc) error {
	var resolved string
	var err error
	if root == "" {
		err = fs.ErrNotExist
	} else {
		resolved, err = config.ResolveSourceAlias(root)
	}
	if err != nil {
		err = fn(root, nil, sourceWalkError(root, err))
		if err == fs.SkipDir || err == fs.SkipAll {
			return nil
		}
		return err
	}
	return filepath.WalkDir(resolved, func(path string, d fs.DirEntry, walkErr error) error {
		lexical := root
		if path != resolved {
			rel, err := filepath.Rel(resolved, path)
			if err != nil {
				return sourceWalkError(root, err)
			}
			lexical = filepath.Join(root, rel)
		} else if d != nil {
			d = sourceRootEntry{DirEntry: d, name: filepath.Base(root)}
		}
		if walkErr != nil {
			walkErr = sourceWalkError(lexical, walkErr)
		}
		return fn(lexical, d, walkErr)
	})
}

func sourceWalkError(path string, err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return &fs.PathError{Op: pathErr.Op, Path: path, Err: pathErr.Err}
	}
	return &fs.PathError{Op: "walk", Path: path, Err: err}
}

type sourceRootEntry struct {
	fs.DirEntry
	name string
}

func (d sourceRootEntry) Name() string { return d.name }

func (d sourceRootEntry) Info() (fs.FileInfo, error) {
	info, err := d.DirEntry.Info()
	if err != nil {
		return nil, err
	}
	return sourceRootInfo{FileInfo: info, name: d.name}, nil
}

type sourceRootInfo struct {
	fs.FileInfo
	name string
}

func (info sourceRootInfo) Name() string { return info.name }
