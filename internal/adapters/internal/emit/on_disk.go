package emit

import (
	"io/fs"
	"os"
	"path/filepath"
)

// readFile reads a file for onDisk. Tests swap it to count reads.
var readFile = os.ReadFile

// makeParent creates the folder a write lands in. Tests swap it to count
// calls.
var makeParent = func(path string) error { return mkdirAll(filepath.Dir(path), dirPerm) }

// onDisk is the file at path as one write finds it, read and stat-ed at
// most once. The checks before a write (hand edit, backup, rollback,
// up to date) all look at the same bytes. Use it only under the path
// lock, and read the file only through it, or a write reads it twice.
type onDisk struct {
	path string

	readDone bool
	data     []byte
	readErr  error

	lstatDone bool
	linfo     fs.FileInfo
	lstatErr  error
}

func (f *onDisk) bytes() ([]byte, error) {
	if !f.readDone {
		f.data, f.readErr = readFile(f.path)
		f.readDone = true
	}
	return f.data, f.readErr
}

// lstat describes path itself, not what a link points to.
func (f *onDisk) lstat() (fs.FileInfo, error) {
	if !f.lstatDone {
		f.linfo, f.lstatErr = os.Lstat(f.path)
		f.lstatDone = true
	}
	return f.linfo, f.lstatErr
}

// stat describes what path resolves to, as os.Stat does.
func (f *onDisk) stat() (fs.FileInfo, error) {
	info, err := f.lstat()
	if err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return os.Stat(f.path)
	}
	return info, err
}

// found reports whether path was read or stat-ed without error, which
// proves its folder exists.
func (f *onDisk) found() bool {
	return f.readDone && f.readErr == nil || f.lstatDone && f.lstatErr == nil
}

// hasPerm reports whether what path resolves to has the permission bits
// of mode.
func (f *onDisk) hasPerm(mode fs.FileMode) bool {
	info, err := f.stat()
	return err == nil && info.Mode().Perm() == mode.Perm()
}
