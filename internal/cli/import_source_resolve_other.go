//go:build !windows

package cli

import (
	"io/fs"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func resolveImportSource(path string) (string, error) {
	return config.ResolveSourceAlias(path)
}

func resolveImportSourceExisting(path string) string {
	return resolveExisting(path)
}

func importSourceDirectoryLink(info fs.FileInfo) bool {
	return false
}
