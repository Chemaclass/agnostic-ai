// Package atomicfile replaces a file whole, so a stop mid-write never
// leaves it short or empty.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const maxLinkHops = 40

// Write replaces path with data. A symlink at path keeps pointing at the
// file it resolves to. The file keeps its mode, or gets defaultMode when new.
// A file without owner write permission is not replaced.
func Write(path string, data []byte, defaultMode fs.FileMode) error {
	target, mode, err := resolve(path, defaultMode)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*.tmp")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	_, werr := tmp.Write(data)
	if err := errors.Join(werr, tmp.Chmod(mode), tmp.Sync(), tmp.Close()); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func resolve(path string, defaultMode fs.FileMode) (string, fs.FileMode, error) {
	target := path
	for hops := 0; ; hops++ {
		info, err := os.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			return target, defaultMode, nil
		}
		if err != nil {
			return "", 0, err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			if info.Mode().Perm()&0o200 == 0 {
				return "", 0, fs.ErrPermission
			}
			return target, info.Mode().Perm(), nil
		}
		if hops == maxLinkHops {
			return "", 0, errors.New("too many symbolic links")
		}
		link, err := os.Readlink(target)
		if err != nil {
			return "", 0, err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(target), link)
		}
		target = link
	}
}
