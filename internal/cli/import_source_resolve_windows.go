package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func resolveImportSource(path string) (string, error) {
	name, err := importSourceWindowsPath(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	defer windows.CloseHandle(handle)
	buf := make([]uint16, 260)
	for {
		n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", path, err)
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

func resolveImportSourceExisting(path string) string {
	rest := ""
	for p := path; ; p = filepath.Dir(p) {
		resolved, err := resolveImportSource(p)
		if err == nil {
			return filepath.Join(resolved, rest)
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(p) == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}

func importSourceDirectoryLink(info fs.FileInfo) bool {
	attrs, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && !info.IsDir() && attrs.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) ==
		windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT
}

func importSourceWindowsPath(path string) (*uint16, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
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
		return nil, fmt.Errorf("encode %s: %w", path, err)
	}
	return name, nil
}
