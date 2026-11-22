package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const shareAll = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

// rename replaces to with POSIX semantics, so a reader that opened to
// through OpenShared keeps its handle and does not fail the replace.
// os.Rename (MoveFileEx) refuses while any handle to the target is open.
// When POSIX rename fails, os.Rename runs and its error is returned. A
// directory at to goes straight to os.Rename, which refuses to replace it.
func rename(from, to string) error {
	if info, err := os.Lstat(to); err == nil && info.IsDir() {
		return os.Rename(from, to)
	}
	if posixRename(from, to) == nil {
		return nil
	}
	return os.Rename(from, to)
}

func posixRename(from, to string) error {
	source, err := extendedPath(from)
	if err != nil {
		return err
	}
	target, err := extendedName(to)
	if err != nil {
		return err
	}
	name, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(source, windows.DELETE|windows.SYNCHRONIZE, shareAll, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	type renameInfo struct {
		Flags          uint32
		RootDirectory  windows.Handle
		FileNameLength uint32
		FileName       [1]uint16
	}
	offset := unsafe.Offsetof(renameInfo{}.FileName)
	size := offset + uintptr(len(name))*2
	buf := make([]uint64, (size+7)/8)
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	info.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.FileNameLength = uint32((len(name) - 1) * 2)
	copy(unsafe.Slice(&info.FileName[0], len(name)), name)
	return windows.SetFileInformationByHandle(handle, windows.FileRenameInfoEx, (*byte)(unsafe.Pointer(info)), uint32(size))
}

// OpenShared opens path for reading without blocking a Replace or Write of
// it. os.Open withholds FILE_SHARE_DELETE, which makes any rename over the
// file fail while it is open.
func OpenShared(path string) (*os.File, error) {
	name, err := extendedPath(path)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, shareAll, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}

func extendedPath(path string) (*uint16, error) {
	name, err := extendedName(path)
	if err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(name)
}

func extendedName(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, `\\?\`) {
		if strings.HasPrefix(abs, `\\`) {
			abs = `\\?\UNC\` + abs[2:]
		} else {
			abs = `\\?\` + abs
		}
	}
	return abs, nil
}
