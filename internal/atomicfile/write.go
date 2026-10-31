// Package atomicfile replaces a file whole, so a stop mid-write never
// leaves it short or empty.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
)

const maxLinkHops = 40

// Write replaces path with data. A symlink at path keeps pointing at the
// file it resolves to. The file keeps its mode, or gets defaultMode when new.
// A file without owner write permission is not replaced.
func Write(path string, data []byte, defaultMode fs.FileMode) error {
	target, mode, exists, err := resolve(path, defaultMode)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	tmp, err := createTemp(filepath.Dir(target), "."+filepath.Base(target), mode)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	_, werr := tmp.Write(data)
	var cerr error
	if exists {
		cerr = tmp.Chmod(mode)
	}
	if err := errors.Join(werr, cerr, tmp.Sync(), tmp.Close()); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// createTemp opens a new file the way os.WriteFile creates one, so the
// process umask applies to mode.
func createTemp(dir, prefix string, mode fs.FileMode) (*os.File, error) {
	for try := 0; ; try++ {
		name := filepath.Join(dir, fmt.Sprintf("%s.%d.tmp", prefix, rand.Uint64()))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, fs.ErrExist) && try < 100 {
			continue
		}
		return f, err
	}
}

// resolve returns the file to replace, the mode its replacement gets, and
// whether that file exists. Folders resolve before a ".." in a link is
// applied, as the kernel does.
func resolve(path string, defaultMode fs.FileMode) (string, fs.FileMode, bool, error) {
	target := path
	for hops := 0; ; hops++ {
		var err error
		if target, err = resolveFolder(target); err != nil {
			return "", 0, false, err
		}
		info, err := os.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			return target, defaultMode, false, nil
		}
		if err != nil {
			return "", 0, false, err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			if info.Mode().Perm()&0o200 == 0 {
				return "", 0, false, fs.ErrPermission
			}
			return target, info.Mode().Perm(), true, nil
		}
		if resolved, err := filepath.EvalSymlinks(target); err == nil {
			target = resolved
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", 0, false, err
		}
		if hops == maxLinkHops {
			return "", 0, false, errors.New("too many symbolic links")
		}
		link, err := os.Readlink(target)
		if err != nil {
			return "", 0, false, err
		}
		if !filepath.IsAbs(link) {
			link = target[:strings.LastIndexByte(target, filepath.Separator)+1] + link
		}
		target = link
	}
}

// resolveFolder resolves the folder part of path, leaving its last element.
func resolveFolder(path string) (string, error) {
	i := strings.LastIndexByte(path, filepath.Separator)
	dir, base := path[:i+1], path[i+1:]
	if dir == "" {
		return path, nil
	}
	if base == "" || base == "." || base == ".." {
		return filepath.EvalSymlinks(path)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, base), nil
}
