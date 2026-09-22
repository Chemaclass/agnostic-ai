package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// importSandbox is the directory a dry-run import runs in, or "" outside
// one. A write that resolves outside it is recorded but never reaches
// disk, so an absolute source path cannot lead a dry-run into the
// project. Sequential test use only.
var importSandbox string

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

// importPlannedWrite is one importer write seen by an import preview:
// the destination, the source being imported, and the bytes proposed.
type importPlannedWrite struct {
	path   string
	source string
	data   []byte
}

// importRecorder collects every importer write of an `import --dry-run`
// run, attributed to the source that made it.
// order lists the sources in the sequence they ran.
type importRecorder struct {
	order  []string
	writes []importPlannedWrite
}

// importRecording is the active recorder, or nil outside a preview.
// Sequential use only, like importSandbox.
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

// importWriteFile writes data to path with the given mode, recording it
// for a dry-run report. Replaces os.WriteFile across all importers.
func importWriteFile(path string, data []byte, mode fs.FileMode) error {
	if importRecording != nil {
		importRecording.record(path, data)
	}
	if !inImportSandbox(path) {
		return nil
	}
	return os.WriteFile(path, data, mode)
}

// importMkdirAll creates dir and its parents, unless a dry-run is active
// and dir resolves outside its sandbox.
func importMkdirAll(dir string, perm fs.FileMode) error {
	if !inImportSandbox(dir) {
		return nil
	}
	return os.MkdirAll(dir, perm)
}

// inImportSandbox reports whether path may be written: always outside a
// dry-run, and only under importSandbox during one.
func inImportSandbox(path string) bool {
	if importSandbox == "" {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(importSandbox, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// dryRunImport runs the import for args in a copy of the project and
// prints a planning summary instead of file contents: one line per path
// the importer would write, sorted and listed once however many stages
// write it, ending with a count. Equivalent in shape to `sync --plan`.
// The summary prints even when an importer fails.
func dryRunImport(args []string) error {
	rec, err := runImportInCopy(args, func(string, string, *importRecorder) error { return nil })
	paths := make([]string, 0, len(rec.writes))
	for _, w := range rec.writes {
		paths = append(paths, filepath.FromSlash(w.path))
	}
	sort.Strings(paths)
	paths = slices.Compact(paths)
	for _, p := range paths {
		fmt.Printf("  would write %s\n", p)
	}
	fmt.Printf("dry-run: %d file(s) would be written\n", len(paths))
	return err
}
