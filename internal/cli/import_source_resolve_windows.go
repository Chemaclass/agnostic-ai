package cli

import (
	"errors"
	"io/fs"
	"path/filepath"
	"syscall"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"golang.org/x/sys/windows"
)

func resolveImportSource(path string) (string, error) {
	return config.ResolveSourceAlias(path)
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
