package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// importSandbox is the directory a dry-run import runs in, or "" outside
// one. A write that resolves outside it is recorded but never reaches
// disk, so no path shape can lead a dry-run into the project.
// Sequential test use only.
var importSandbox string

// importSandboxOutsideFiles holds the files of importSandbox, relative to
// it, that the copy took from links leaving the project. They still count
// as outside, so a preview skips what the real import skips.
var importSandboxOutsideFiles map[string]bool

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

// importRecording is the active recorder, or nil outside a dry-run.
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
// for a dry-run report. Replaces os.WriteFile across all importers. A
// write that stores a project local spec in the shared source is undone
// when the run ends (see localImportGuard).
func importWriteFile(path string, data []byte, mode fs.FileMode) error {
	if importLocal != nil && inImportSandbox(path) {
		data = importLocal.stripLocalHandlers(path, data)
		if err := importLocal.track(path, data, false); err != nil {
			return err
		}
	}
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
	if importLocal != nil && inImportSandbox(dir) {
		if err := importLocal.track(dir, nil, true); err != nil {
			return err
		}
	}
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
