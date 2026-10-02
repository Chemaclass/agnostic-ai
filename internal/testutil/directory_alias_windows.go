package testutil

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func DirectoryAlias(t *testing.T, target, alias string) {
	t.Helper()
	target, err := filepath.Abs(target)
	if err != nil {
		t.Fatalf("resolve directory alias target %s: %v", target, err)
	}
	substitutePath := `\??\` + target
	if strings.HasPrefix(target, `\\?\`) {
		substitutePath = `\??\` + target[4:]
	} else if strings.HasPrefix(target, `\\`) {
		substitutePath = `\??\UNC\` + target[2:]
	}
	substitute, err := windows.UTF16FromString(substitutePath)
	if err != nil {
		t.Fatalf("encode directory alias target %s: %v", target, err)
	}
	printName, err := windows.UTF16FromString(target)
	if err != nil {
		t.Fatalf("encode directory alias target %s: %v", target, err)
	}
	size := 16 + 2*(len(substitute)+len(printName))
	if size > windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE {
		t.Fatalf("directory alias target is too long: %s", target)
	}
	data := make([]byte, size)
	binary.LittleEndian.PutUint32(data, windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(data[4:], uint16(size-8))
	binary.LittleEndian.PutUint16(data[10:], uint16(2*(len(substitute)-1)))
	binary.LittleEndian.PutUint16(data[12:], uint16(2*len(substitute)))
	binary.LittleEndian.PutUint16(data[14:], uint16(2*(len(printName)-1)))
	for i, char := range append(substitute, printName...) {
		binary.LittleEndian.PutUint16(data[16+2*i:], char)
	}

	name := directoryAliasWindowsPath(t, alias)
	if err := os.Mkdir(alias, 0o700); err != nil {
		t.Fatalf("create directory alias %s: %v", alias, err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatalf("open directory alias %s: %v", alias, err)
	}
	defer windows.CloseHandle(handle)
	var returned uint32
	if err := windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &returned, nil); err != nil {
		t.Fatalf("create directory junction %s: %v", alias, err)
	}
	data = make([]byte, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	if err := windows.DeviceIoControl(handle, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &data[0], uint32(len(data)), &returned, nil); err != nil {
		t.Fatalf("inspect directory junction %s: %v", alias, err)
	}
	if returned < 8 {
		t.Fatalf("directory junction %s returned %d reparse bytes", alias, returned)
	}
	if tag := binary.LittleEndian.Uint32(data); tag != windows.IO_REPARSE_TAG_MOUNT_POINT {
		t.Fatalf("directory alias %s has reparse tag %#x, want mount point", alias, tag)
	}
}

func directoryAliasWindowsPath(t *testing.T, path string) *uint16 {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve directory alias %s: %v", path, err)
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
		t.Fatalf("encode directory alias %s: %v", path, err)
	}
	return name
}
