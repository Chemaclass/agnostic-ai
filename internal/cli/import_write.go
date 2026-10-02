package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// importSandbox is the directory a dry-run import runs in, or "" outside
// one. A write that resolves outside it is recorded but never reaches
// disk, so no path shape can lead a dry-run into the project.
// Sequential test use only.
var importSandbox string

// importSandboxOutsideFiles holds the files and directories of
// importSandbox, relative to it, that the copy took from links leaving the
// project. They still count as outside, so a preview skips what the real
// import skips.
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
// merge marks a write that layers onto the file already there instead
// of replacing it (see withImportMerge).
type importPlannedWrite struct {
	path   string
	source string
	data   []byte
	merge  bool
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
		merge:  importMerging,
	})
}

// importMerging is set while an importer layers a source onto a spec
// that is already there, keeping what it holds, such as codex onto a
// claude skill or agent of the same name. Sequential use only.
var importMerging bool

// withImportMerge runs fn with its writes marked as merges, which do not
// count as replacing an existing spec (see importOverwrites).
func withImportMerge(fn func() error) error {
	prior := importMerging
	importMerging = true
	defer func() { importMerging = prior }()
	return fn()
}

// importWriteFile writes data to path with the given mode, recording it
// for a dry-run report. Replaces os.WriteFile across all importers. A
// write that stores a project local spec in the shared source is undone
// when the run ends (see localImportGuard).
func importWriteFile(path string, data []byte, mode fs.FileMode) error {
	if importLocal != nil && inImportSandbox(path) {
		if err := importLocal.track(path, data, false); err != nil {
			return err
		}
	}
	if importTxn != nil {
		if err := importTxn.saveFile(path); err != nil {
			return err
		}
	}
	if importRecording != nil {
		importRecording.record(path, data)
	}
	if importWritten != nil {
		importWritten[filepath.Clean(path)] = true
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
	if importTxn != nil {
		importTxn.saveDirs(dir)
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

// importTransaction keeps each file a real import writes as it was before
// the run's first write to it, and the directories the run creates, so a
// run that would replace an existing spec can be undone whole. It costs
// one read per written file, never a walk of the project.
type importTransaction struct {
	// files maps each written path, cleaned, to its prior content, or nil
	// when it did not exist.
	files map[string]*savedImportFile
	// dirs lists the directories the run created, in creation order.
	dirs []string
}

// importTxn is the active transaction, or nil outside a guarded import.
// Sequential use only, like importSandbox.
var importTxn *importTransaction

func (t *importTransaction) saveFile(path string) error {
	path = filepath.Clean(path)
	if _, seen := t.files[path]; seen {
		return nil
	}
	t.saveDirs(filepath.Dir(path))
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.files[path] = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	prior, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	t.files[path] = &savedImportFile{data: prior, mode: info.Mode().Perm()}
	return nil
}

// saveDirs records dir and each missing parent the run is about to create.
func (t *importTransaction) saveDirs(dir string) {
	var missing []string
	for dir = filepath.Clean(dir); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) || filepath.Dir(dir) == dir {
			break
		}
		missing = append(missing, dir)
	}
	for _, d := range missing {
		if !slices.Contains(t.dirs, d) {
			t.dirs = append(t.dirs, d)
		}
	}
}

// rollback puts every written file back as it was and removes the
// directories the run created, deepest first and never recursively.
func (t *importTransaction) rollback() error {
	var errs []error
	for path, prior := range t.files {
		var err error
		if prior == nil {
			if err = os.Remove(path); errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
		} else if err = os.WriteFile(path, prior.data, prior.mode); err == nil {
			err = os.Chmod(path, prior.mode)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", path, err))
		}
	}
	dirs := slices.Clone(t.dirs)
	slices.SortFunc(dirs, func(a, b string) int { return len(b) - len(a) })
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
	return errors.Join(errs...)
}
