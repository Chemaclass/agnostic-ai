package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// importDryRun gates writes during `import --dry-run`. Set before any
// importer call via newImportCmd, cleared afterward. Safe for sequential
// (non-parallel) test use.
var importDryRun bool

// importRunSources names every source of a multi-source `import` run
// (`import claude codex`, `import all`). Empty for a single-source run.
// An importer reads it to leave a file another source in the same run
// owns: the root AGENTS.md is claude's documented fallback and codex's
// own main file, so only one of the two may claim it. Set before the
// first importer call, cleared afterward. Sequential test use only.
var importRunSources []string

// setImportRunSources records the sources of the current `import` run.
func setImportRunSources(sources []string) {
	importRunSources = append([]string(nil), sources...)
}

// importDryRunPaths collects every path importWriteFile / importMkdirAll
// would have touched in dry-run mode. Drained and printed as a planning
// summary by reportImportDryRun. Sequential test use only — `import`
// invokes one importer at a time.
var importDryRunPaths []string

// importPlannedWrite is one importer write seen by an import preview:
// the destination, the source being imported, and the bytes proposed.
type importPlannedWrite struct {
	path   string
	source string
	data   []byte
}

// importRecorder collects every importer write of an
// `import --dry-run --diff` run, attributed to the source that made it.
// order lists the sources in the sequence they ran.
type importRecorder struct {
	order  []string
	writes []importPlannedWrite
}

// importRecording is the active recorder, or nil outside a preview.
// Sequential use only, like importDryRun.
var importRecording *importRecorder

// beginSource marks source as the one now importing.
func (r *importRecorder) beginSource(source string) {
	r.order = append(r.order, source)
}

// record keeps a copy of data proposed for path by the current source.
func (r *importRecorder) record(path string, data []byte) {
	var source string
	if len(r.order) > 0 {
		source = r.order[len(r.order)-1]
	}
	r.writes = append(r.writes, importPlannedWrite{
		path:   filepath.ToSlash(filepath.Clean(path)),
		source: source,
		data:   bytes.Clone(data),
	})
}

// importWriteFile writes data to path with the given mode, or in dry-run
// mode records the path for a planning summary without touching disk.
// Replaces os.WriteFile across all importers.
func importWriteFile(path string, data []byte, mode fs.FileMode) error {
	if importRecording != nil {
		importRecording.record(path, data)
	}
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
