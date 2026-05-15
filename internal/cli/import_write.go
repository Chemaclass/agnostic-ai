package cli

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
)

// importDryRun gates writes during `import --dry-run`. Set before any
// importer call via newImportCmd, cleared afterward. Safe for sequential
// (non-parallel) test use.
var importDryRun bool

// importDryRunPaths collects every path importWriteFile / importMkdirAll
// would have touched in dry-run mode. Drained and printed as a planning
// summary by reportImportDryRun. Sequential test use only — `import`
// invokes one importer at a time.
var importDryRunPaths []string

// importWriteFile writes data to path with the given mode, or in dry-run
// mode records the path for a planning summary without touching disk.
// Replaces os.WriteFile across all importers.
func importWriteFile(path string, data []byte, mode fs.FileMode) error {
	if importDryRun {
		importDryRunPaths = append(importDryRunPaths, path)
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

// resetImportDryRunPaths clears the collector before each importer run.
func resetImportDryRunPaths() {
	importDryRunPaths = nil
}

// reportImportDryRun prints a planning summary instead of file contents.
// Output: one line per path the importer would write, sorted, ending
// with a count. Equivalent in shape to `sync --plan`.
func reportImportDryRun() {
	paths := append([]string(nil), importDryRunPaths...)
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Printf("  would write %s\n", p)
	}
	fmt.Printf("dry-run: %d file(s) would be written\n", len(paths))
}
