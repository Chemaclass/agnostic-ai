package main

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// readFileShared reads with delete sharing, so agnostic-ai serve can
// replace the file while the companion reads it. os.ReadFile leaves that
// sharing out, and a replace then fails.
func readFileShared(path string) ([]byte, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(handle), path)
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}
