package cli

import (
	"fmt"
	"io/fs"
	"os"
)

// importDryRun gates writes during `import --dry-run`. Set before any
// importer call via newImportCmd, cleared afterward. Safe for sequential
// (non-parallel) test use.
var importDryRun bool

// importWriteFile writes data to path with the given mode, or in dry-run
// mode prints what would be written without touching disk. Replaces
// os.WriteFile across all importers.
func importWriteFile(path string, data []byte, mode fs.FileMode) error {
	if importDryRun {
		fmt.Printf("--- %s ---\n%s\n", path, string(data))
		return nil
	}
	return os.WriteFile(path, data, mode)
}

// importMkdirAll creates dir and its parents unless dry-run mode is
// active, in which case it is a no-op (no directories are created on
// disk during a dry-run preview).
func importMkdirAll(dir string, perm fs.FileMode) error {
	if importDryRun {
		return nil
	}
	return os.MkdirAll(dir, perm)
}
