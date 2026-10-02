package cli

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"golang.org/x/sys/windows"
)

func createImportSourceAlias(target, alias string) error {
	target, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve source preview alias %s: %w", alias, err)
	}
	substitutePath := `\??\` + target
	if strings.HasPrefix(target, `\\?\`) {
		substitutePath = `\??\` + target[4:]
	} else if strings.HasPrefix(target, `\\`) {
		substitutePath = `\??\UNC\` + target[2:]
	}
	substitute, err := windows.UTF16FromString(substitutePath)
	if err != nil {
		return fmt.Errorf("encode source preview alias target %s: %w", target, err)
	}
	printName, err := windows.UTF16FromString(target)
	if err != nil {
		return fmt.Errorf("encode source preview alias target %s: %w", target, err)
	}
	size := 16 + 2*(len(substitute)+len(printName))
	if size > windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE {
		return fmt.Errorf("source preview alias target %s: %w", target, windows.ERROR_FILENAME_EXCED_RANGE)
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

	path, err := config.WindowsSourcePath(alias)
	if err != nil {
		return err
	}
	if err := os.Mkdir(alias, 0o700); err != nil {
		return fmt.Errorf("%s: %w", alias, err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fmt.Errorf("open source preview alias %s: %w", alias, err)
	}
	defer windows.CloseHandle(handle)
	var returned uint32
	if err := windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &returned, nil); err != nil {
		return fmt.Errorf("create source preview junction %s: %w", alias, err)
	}
	return nil
}
