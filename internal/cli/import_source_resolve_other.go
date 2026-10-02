//go:build !windows

package cli

import (
	"io/fs"
	"path/filepath"
)

func resolveImportSource(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

func resolveImportSourceExisting(path string) string {
	return resolveExisting(path)
}

func importSourceDirectoryLink(info fs.FileInfo) bool {
	return false
}
