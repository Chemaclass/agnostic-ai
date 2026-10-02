package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// specFileSum is the raw-bytes sum of one spec file and what put those
// bytes where a tool reads them: a sync that rendered the spec for
// Targets, or an import that wrote it from the own files of Sources.
type specFileSum struct {
	Sum     string   `json:"sum"`
	By      string   `json:"by"`
	Targets []string `json:"targets,omitempty"`
	Sources []string `json:"sources,omitempty"`
}

// holders names the tools whose files the recorded bytes came from or
// went to.
func (r specFileSum) holders() []string {
	if r.By == specSumByImport {
		return r.Sources
	}
	return r.Targets
}

const (
	specSumBySync   = "sync"
	specSumByImport = "import"
)

// syncedSpecFileSums returns the spec file sums after a sync that rendered
// b for targets. A spec file still holding the bytes an earlier sync
// rendered for a configured target this one did not cover keeps that
// target, also when no covered target reads it. An import's sum
// outlives the sync for a file sync does not render, such as an overlay.
func syncedSpecFileSums(root string, prev map[string]specFileSum, b spec.Bundle, targets, configured []string) map[string]specFileSum {
	next := map[string]specFileSum{}
	for _, target := range targets {
		for _, e := range b.For(target).All() {
			for _, path := range specEntryFiles(root, e) {
				rec, ok := next[path]
				if !ok {
					data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
					if err != nil {
						continue
					}
					rec = specFileSum{Sum: sha256Hex(data), By: specSumBySync}
				}
				if !slices.Contains(rec.Targets, target) {
					rec.Targets = append(rec.Targets, target)
				}
				next[path] = rec
			}
		}
	}
	// A configured target this sync did not cover still holds what an
	// earlier sync wrote for it, while the spec file keeps those bytes. A
	// target taken out of the config may lose those files, so it drops.
	uncovered := func(old specFileSum) []string {
		return slices.DeleteFunc(slices.Clone(old.Targets), func(t string) bool {
			return slices.Contains(targets, t) || !slices.Contains(configured, t)
		})
	}
	for path, old := range prev {
		rec, ok := next[path]
		switch {
		case !ok && old.By == specSumByImport:
			next[path] = old
		case !ok && old.By == specSumBySync:
			kept := uncovered(old)
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if len(kept) > 0 && err == nil && sha256Hex(data) == old.Sum {
				old.Targets = kept
				next[path] = old
			}
		case ok && old.By == specSumBySync && old.Sum == rec.Sum:
			for _, t := range uncovered(old) {
				if !slices.Contains(rec.Targets, t) {
					rec.Targets = append(rec.Targets, t)
				}
			}
			next[path] = rec
		}
	}
	for path, rec := range next {
		sort.Strings(rec.Targets)
		next[path] = rec
	}
	return next
}

// specEntryFiles lists the files of e under root, slash-form and relative
// to it: its spec file and, for a skill, every file in its folder.
func specEntryFiles(root string, e spec.Entry) []string {
	var files []string
	add := func(path string) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		rel, err := filepath.Rel(absRoot, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			files = append(files, filepath.ToSlash(rel))
		}
	}
	if e.Path == "" {
		return nil
	}
	add(e.Path)
	if dir := e.SkillAssetDir(); dir != "" {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && filepath.Clean(p) != filepath.Clean(e.Path) {
				add(p)
			}
			return nil
		})
	}
	return files
}

// alreadyRead reports whether before, the bytes of an existing spec,
// already came from or reached the files of every source in sources: a
// sync rendered them for each source, or an import of those sources
// wrote them, and nothing changed them since. Replacing such a spec
// brings back an edit made in the tool. A spec another tool's import
// wrote is that tool's, so a different source stops.
func alreadyRead(rec specFileSum, ok bool, before []byte, sources []string) bool {
	if !ok || rec.Sum != sha256Hex(before) || len(sources) == 0 {
		return false
	}
	held := rec.holders()
	return !slices.ContainsFunc(sources, func(s string) bool { return !slices.Contains(held, s) })
}

// replacesSpec reports whether an import write stops the run: it replaces
// an existing spec with different content, the spec is not
// AGNOSTIC_AI.md, which import merges into, and its bytes were not
// already in what the writing sources read. It sets e.heldBy to the
// tools the current bytes came from or went to, for the stop message.
func replacesSpec(e *importPreviewEntry, specDirs []string, sums map[string]specFileSum) bool {
	key := specPathKey(e.path)
	if !e.existed || !e.replaced || bytes.Equal(e.before, e.after) ||
		key == agnosticMainFile || !inSpecDir(e.path, specDirs) {
		return false
	}
	rec, ok := sums[key]
	if ok && rec.Sum == sha256Hex(e.before) {
		e.heldBy = rec.By + ":" + strings.Join(rec.holders(), ", ")
	}
	return !alreadyRead(rec, ok, e.before, e.sources)
}

// overwrites returns the existing specs the import replaces with
// different content, sorted by path.
func (p importPreview) overwrites() []importPreviewEntry {
	var out []importPreviewEntry
	for _, e := range p.entries {
		if e.overwrites {
			out = append(out, e)
		}
	}
	return out
}

// importSpecDirs lists the spec directories of the project at root, the
// working directory: the source base and every configured source
// directory, each in both forms specPathForms gives.
func importSpecDirs(root string) []string {
	paths := []string{filepath.Join(root, config.SourceBaseDir)}
	if cfg, err := config.Load(root); err == nil {
		for _, d := range sourceDirsByKind(cfg.Sources) {
			switch {
			case d == "":
			case filepath.IsAbs(d):
				paths = append(paths, d)
			default:
				paths = append(paths, filepath.Join(root, d))
			}
		}
	}
	var dirs []string
	for _, p := range paths {
		for _, form := range specPathForms(p) {
			if !slices.Contains(dirs, form) {
				dirs = append(dirs, form)
			}
		}
	}
	return dirs
}

// inSpecDir reports whether path lies in a spec directory, as written or
// with its links resolved: a skill folder linked out of the source still
// holds a spec, and a source directory reached through a link still
// counts.
func inSpecDir(path string, specDirs []string) bool {
	return slices.ContainsFunc(specPathForms(path), func(form string) bool { return underAny(form, specDirs) })
}

// specPathKey names path the way spec file sums key it, lexically like
// specEntryFiles: slash form, relative to the working directory when
// inside it, and absolute otherwise.
func specPathKey(path string) string {
	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return relativeKey(wd, abs)
}

// specPathForms returns path as specPathKey names it, and again with
// every link in it resolved against the resolved working directory, so
// /var and /private/var on macOS name one directory.
func specPathForms(path string) []string {
	forms := []string{specPathKey(path)}
	abs, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return forms
	}
	wd, err := os.Getwd()
	if err != nil {
		return forms
	}
	if real, err := filepath.EvalSymlinks(wd); err == nil {
		wd = real
	}
	if resolved := relativeKey(wd, resolveExisting(abs)); resolved != forms[0] {
		forms = append(forms, resolved)
	}
	return forms
}

// relativeKey is abs in slash form, relative to wd when inside it.
func relativeKey(wd, abs string) string {
	if rel, err := filepath.Rel(wd, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

// underAny reports whether the slash-form path lies inside one of dirs.
func underAny(path string, dirs []string) bool {
	return slices.ContainsFunc(dirs, func(d string) bool {
		return strings.HasPrefix(path, d+"/")
	})
}

// printImportOverwrites lists, in an import preview, the existing specs
// an import with --overwrite replaces.
func printImportOverwrites(w io.Writer, entries []importPreviewEntry) {
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "  ! replaces %s, which holds different content (%s)\n", e.path, strings.Join(e.sources, ", "))
	}
}

// importOverwriteError lists the existing specs an import would replace
// and ends with remedy for the sources that wrote them.
func importOverwriteError(entries []importPreviewEntry, remedy func(sources []string) string) error {
	var b strings.Builder
	var sources []string
	fmt.Fprintf(&b, "import would replace %d existing spec(s) with different content, so no spec was written:\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(&b, "  %s (from %s%s)\n", e.path, strings.Join(e.sources, ", "), heldByNote(e.heldBy))
		for _, s := range e.sources {
			if !slices.Contains(sources, s) {
				sources = append(sources, s)
			}
		}
	}
	b.WriteString(remedy(sources))
	return errs.Coded(errs.CodeImportWouldReplace, "%s", b.String())
}

// heldByNote says where the bytes an import would replace came from.
func heldByNote(heldBy string) string {
	by, tools, ok := strings.Cut(heldBy, ":")
	switch {
	case !ok || tools == "":
		return ""
	case by == specSumByImport:
		return "; now holds what import " + tools + " wrote"
	default:
		return "; now holds what sync wrote for " + tools
	}
}

// importOverwriteRemedy names the two ways past an import that would
// replace existing specs, for the import command args.
func importOverwriteRemedy(args []string) string {
	return fmt.Sprintf("rename a spec to keep both, or run agnostic-ai import %s --overwrite to replace them", strings.Join(args, " "))
}

// runGuardedImport runs a real import and, unless overwrite is set, puts
// every file it wrote back and returns an error when it replaced an
// existing spec with different content (see replacesSpec). Only files
// there before the run count, so a later source in the same run still
// replaces what an earlier one wrote. The run's output is held back and
// dropped on a stop, which would otherwise report imports it undid. A
// finished run records the sums of the spec files it wrote.
func runGuardedImport(overwrite bool, remedy func(sources []string) string, run func() error) error {
	rec := &importRecorder{}
	txn := &importTransaction{files: map[string]*savedImportFile{}}
	return withHeldOutput(func() (bool, error) {
		importRecording, importTxn = rec, txn
		stopOnSignal := txn.rollbackOnSignal()
		runErr := run()
		stopOnSignal()
		importRecording, importTxn = nil, nil
		entries, err := txn.entries(rec)
		if err != nil {
			return true, errors.Join(runErr, err)
		}
		specDirs := importSpecDirs(".")
		sums := readStateFile(".").SpecFileSums
		var overwrites []importPreviewEntry
		for _, e := range entries {
			if replacesSpec(&e, specDirs, sums) {
				overwrites = append(overwrites, e)
			}
		}
		if len(overwrites) > 0 && !overwrite {
			return false, errors.Join(runErr, importOverwriteError(overwrites, remedy), txn.rollback())
		}
		return true, errors.Join(runErr, recordImportedSpecFiles(".", entries, specDirs))
	})
}

// entries folds the transaction and the recorded writes into one entry
// per written file, with its bytes before the run and on disk now.
func (t *importTransaction) entries(rec *importRecorder) ([]importPreviewEntry, error) {
	byPath := map[string]*importPreviewEntry{}
	for _, w := range rec.writes {
		e, ok := byPath[w.path]
		if !ok {
			e = &importPreviewEntry{path: w.path}
			byPath[w.path] = e
		}
		if !slices.Contains(e.sources, w.source) {
			e.sources = append(e.sources, w.source)
		}
		e.replaced = e.replaced || !w.merge
	}
	var out []importPreviewEntry
	for path, prior := range t.files {
		e, ok := byPath[filepath.ToSlash(path)]
		if !ok {
			continue
		}
		if prior != nil {
			e.existed, e.before = true, prior.data
		}
		after, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		e.after = after
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

// recordImportedSpecFiles stores the raw-bytes sum of each spec file the
// run wrote, as written by import of its sources, so a later import of
// them may replace it while nothing else has touched it. A file whose
// record already holds its bytes keeps that record, with the sources
// added to the tools that hold them.
func recordImportedSpecFiles(root string, entries []importPreviewEntry, specDirs []string) error {
	// An unreadable ledger still counts as one (#1334); a stub written
	// over it would not, so the records wait for the next sync.
	state, err := readStateFileStrict(root)
	if err != nil {
		return nil
	}
	changed := false
	for _, e := range entries {
		key := specPathKey(e.path)
		if !inSpecDir(e.path, specDirs) {
			continue
		}
		if state.SpecFileSums == nil {
			state.SpecFileSums = map[string]specFileSum{}
		}
		if e.after == nil {
			delete(state.SpecFileSums, key)
			changed = true
			continue
		}
		sum := sha256Hex(e.after)
		rec, ok := state.SpecFileSums[key]
		if !ok || rec.Sum != sum {
			state.SpecFileSums[key] = specFileSum{Sum: sum, By: specSumByImport, Sources: slices.Sorted(slices.Values(e.sources))}
			changed = true
			continue
		}
		// The bytes are what the record holds, and now also what these
		// sources hold.
		holders := slices.Clone(rec.holders())
		for _, s := range e.sources {
			if !slices.Contains(holders, s) {
				holders = append(holders, s)
			}
		}
		if len(holders) == len(rec.holders()) {
			continue
		}
		sort.Strings(holders)
		if rec.By == specSumByImport {
			rec.Sources = holders
		} else {
			rec.Targets = holders
		}
		state.SpecFileSums[key] = rec
		changed = true
	}
	if !changed {
		return nil
	}
	return replaceStateFile(root, state)
}

// releaseHeldOutput gives back the streams withHeldOutput holds and
// removes its files, or is nil outside it. Sequential use only.
var releaseHeldOutput func()

// withHeldOutput runs fn with standard output, standard error, and the
// summary log held back, then prints them when fn says to keep them.
// Output it cannot hold prints as it comes.
func withHeldOutput(fn func() (keep bool, err error)) error {
	out, err := os.CreateTemp("", "agnostic-ai-import-out-")
	if err != nil {
		_, err := fn()
		return err
	}
	defer func() { _ = out.Close(); _ = os.Remove(out.Name()) }()
	errOut, err := os.CreateTemp("", "agnostic-ai-import-err-")
	if err != nil {
		_, err := fn()
		return err
	}
	defer func() { _ = errOut.Close(); _ = os.Remove(errOut.Name()) }()
	stdout, stderr, log := os.Stdout, os.Stderr, logOut
	var logBuf bytes.Buffer
	os.Stdout, os.Stderr = out, errOut
	if log == io.Writer(stdout) {
		logOut = out
	} else {
		logOut = &logBuf
	}
	restore := func() { os.Stdout, os.Stderr, logOut = stdout, stderr, log }
	// A signal exits without running the defers, so the handler restores
	// the streams and removes the held files through this.
	releaseHeldOutput = func() {
		restore()
		_ = out.Close()
		_ = os.Remove(out.Name())
		_ = errOut.Close()
		_ = os.Remove(errOut.Name())
	}
	defer func() { releaseHeldOutput = nil }()
	keep, runErr := func() (bool, error) {
		defer restore()
		return fn()
	}()
	if keep {
		for _, held := range []struct {
			from *os.File
			to   io.Writer
		}{{out, stdout}, {errOut, stderr}} {
			if _, err := held.from.Seek(0, io.SeekStart); err == nil {
				_, _ = io.Copy(held.to, held.from)
			}
		}
		_, _ = log.Write(logBuf.Bytes())
	}
	return runErr
}
