package config

import (
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func ResolveSourceAlias(path string) (string, error) {
	name, err := WindowsSourcePath(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", &fs.PathError{Op: "open", Path: path, Err: err}
	}
	defer windows.CloseHandle(handle)
	buf := make([]uint16, 260)
	for {
		n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", &fs.PathError{Op: "resolve", Path: path, Err: err}
		}
		if n < uint32(len(buf)) {
			resolved := windows.UTF16ToString(buf[:n])
			if strings.HasPrefix(resolved, `\\?\UNC\`) {
				resolved = `\\` + resolved[8:]
			} else {
				resolved = strings.TrimPrefix(resolved, `\\?\`)
			}
			return filepath.Clean(resolved), nil
		}
		buf = make([]uint16, n+1)
	}
}

func WindowsSourcePath(path string) (*uint16, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, &fs.PathError{Op: "resolve", Path: path, Err: err}
	}
	if !strings.HasPrefix(abs, `\\?\`) {
		if strings.HasPrefix(abs, `\\`) {
			abs = `\\?\UNC\` + abs[2:]
		} else {
			abs = `\\?\` + abs
		}
	}
	name, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return nil, &fs.PathError{Op: "encode", Path: path, Err: err}
	}
	return name, nil
}
