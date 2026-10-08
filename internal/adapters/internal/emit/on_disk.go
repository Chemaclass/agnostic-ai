package emit

import (
	"io/fs"
	"os"
)

// readFile reads a file for onDisk. Tests swap it to count reads.
var readFile = os.ReadFile

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
