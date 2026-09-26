package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// localImportGuard keeps an import from storing a spec the project local
// layer supplies. Sync renders `.agnostic-ai/local/` specs into the same
// native files import reads back, so without it a personal rule, agent,
// or skill would land in the shared source, and a shared spec the local
// layer extends would be overwritten with the merged content (#1174).
//
// Importers read back what they wrote (a later source merges into an
// earlier one's file, codex re-reads a skill to fix quoting), so the
// writes go through and the guard puts the files back once the run ends.
type localImportGuard struct {
	// dirs maps each kind's shared source directory, absolute, to the
	// kind stored there.
	dirs map[string]spec.Kind
	// names holds, per kind, the names the local layer declares.
	names map[spec.Kind]map[string]bool
	// skipped collects "<kind> <name>" labels for the closing note.
	skipped map[string]bool
	// saved holds, per absolute path of a local spec file the run wrote,
	// the file as it was before the run (nil when it did not exist).
	saved map[string]*savedImportFile
	// created lists the directories the run made for a local spec.
	created []string
}

// savedImportFile is a file's content and mode before an import.
type savedImportFile struct {
	data []byte
	mode fs.FileMode
}

// importLocal is the active guard, or nil when the project has no local
// layer. Sequential use only, like importSandbox.
var importLocal *localImportGuard

// withLocalImportGuard runs fn with a guard built from the project local
// layer under root, puts back every shared file the run wrote for a
// local spec, and prints which local specs it left out.
func withLocalImportGuard(root string, cfg *config.Config, fn func() error) error {
	guard, err := newLocalImportGuard(root, cfg)
	if err != nil {
		return err
	}
	if guard == nil {
		return fn()
	}
	importLocal = guard
	runErr := fn()
	importLocal = nil
	if err := guard.restore(); err != nil {
		return errors.Join(runErr, err)
	}
	guard.printNote()
	return runErr
}

// newLocalImportGuard loads the project local layer. It returns nil when
// the layer does not exist or declares no specs.
func newLocalImportGuard(root string, cfg *config.Config) (*localImportGuard, error) {
	layer, ok := resolveProjectUserLayer(root)
	if !ok {
		return nil, nil
	}
	bundle, err := spec.LoadLayered([]spec.Layer{layer})
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", defaultProjectUser, err)
	}
	entries := bundle.All()
	if len(entries) == 0 {
		return nil, nil
	}
	g := &localImportGuard{
		dirs:    map[string]spec.Kind{},
		names:   map[spec.Kind]map[string]bool{},
		skipped: map[string]bool{},
		saved:   map[string]*savedImportFile{},
	}
	for _, e := range entries {
		if g.names[e.Kind] == nil {
			g.names[e.Kind] = map[string]bool{}
		}
		g.names[e.Kind][e.Name] = true
	}
	for kind, dir := range sourceDirsByKind(cfg.Sources) {
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(root, dir))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		g.dirs[abs] = kind
	}
	return g, nil
}

// sourceDirsByKind pairs each spec kind with its configured directory.
func sourceDirsByKind(s config.Sources) map[spec.Kind]string {
	return map[spec.Kind]string{
		spec.KindAgent:       s.Agents,
		spec.KindSkill:       s.Skills,
		spec.KindRule:        s.Rules,
		spec.KindHook:        s.Hooks,
		spec.KindMCP:         s.MCPs,
		spec.KindCommand:     s.Commands,
		spec.KindSettings:    s.Settings,
		spec.KindReview:      s.Reviews,
		spec.KindEnvironment: s.Environments,
		spec.KindIgnore:      s.Ignore,
	}
}

// track records the state of path before an importer writes it, when
// the write stores a local spec in the shared source. data is nil for a
// directory.
func (g *localImportGuard) track(path string, data []byte, isDir bool) {
	kind, rel, ok := g.locate(path)
	if !ok {
		return
	}
	name, ok := g.localName(kind, rel, data, isDir)
	if !ok {
		return
	}
	g.skipped[string(kind)+" "+name] = true
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	if isDir {
		g.trackDirs(abs)
		return
	}
	if _, seen := g.saved[abs]; seen {
		return
	}
	g.trackDirs(filepath.Dir(abs))
	info, err := os.Stat(abs)
	if err != nil {
		g.saved[abs] = nil
		return
	}
	prior, err := os.ReadFile(abs)
	if err != nil {
		g.saved[abs] = nil
		return
	}
	g.saved[abs] = &savedImportFile{data: prior, mode: info.Mode().Perm()}
}

// leaves reports whether path is a local spec's file in the shared
// source, and names the spec in the closing note. An importer calls it
// to skip a file up front instead of warning about it. Nil-safe.
func (g *localImportGuard) leaves(path string) bool {
	if g == nil {
		return false
	}
	kind, rel, ok := g.locate(path)
	if !ok {
		return false
	}
	name, ok := g.localName(kind, rel, nil, false)
	if ok {
		g.skipped[string(kind)+" "+name] = true
	}
	return ok
}

// trackDirs records dir and each missing parent inside a kind directory.
func (g *localImportGuard) trackDirs(dir string) {
	for {
		if _, _, inside := g.locate(dir); !inside {
			return
		}
		if _, err := os.Stat(dir); err == nil {
			return
		}
		if !slices.Contains(g.created, dir) {
			g.created = append(g.created, dir)
		}
		dir = filepath.Dir(dir)
	}
}

// restore puts every tracked file back as it was before the run, removes
// the directories the run made for local specs, and drops the writes
// from a dry-run recording.
func (g *localImportGuard) restore() error {
	var errs []error
	for path, prior := range g.saved {
		if !inImportSandbox(path) {
			continue
		}
		var err error
		if prior == nil {
			err = os.Remove(path)
			if errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
		} else {
			err = os.WriteFile(path, prior.data, prior.mode)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", path, err))
		}
	}
	// Deepest first, and never recursive: a folder that still holds a
	// file the run did not track stays.
	slices.SortFunc(g.created, func(a, b string) int { return len(b) - len(a) })
	for _, dir := range g.created {
		if inImportSandbox(dir) {
			_ = os.Remove(dir)
		}
	}
	if importRecording != nil {
		importRecording.writes = slices.DeleteFunc(importRecording.writes, func(w importPlannedWrite) bool {
			abs, err := filepath.Abs(filepath.FromSlash(w.path))
			_, local := g.saved[abs]
			return err == nil && local
		})
	}
	return errors.Join(errs...)
}

// locate finds the shared source directory holding path and returns its
// kind and path relative to it, with forward slashes.
func (g *localImportGuard) locate(path string) (spec.Kind, string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", false
	}
	// The deepest match wins, so a kind directory nested in another
	// still claims its own files.
	var kind spec.Kind
	var rel string
	for dir, k := range g.dirs {
		r, err := filepath.Rel(dir, abs)
		if err != nil || r == "." || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			continue
		}
		if rel == "" || len(r) < len(rel) {
			kind, rel = k, r
		}
	}
	return kind, filepath.ToSlash(rel), rel != ""
}

// localName returns the local spec name a write under a kind directory
// belongs to. A skill owns its whole folder, assets included; any other
// spec is one file named after it or declaring its name.
func (g *localImportGuard) localName(kind spec.Kind, rel string, data []byte, isDir bool) (string, bool) {
	names := g.names[kind]
	if len(names) == 0 {
		return "", false
	}
	segments := strings.Split(rel, "/")
	if kind == spec.KindSkill {
		folders := segments
		if !isDir {
			folders = segments[:len(segments)-1]
		}
		if i := slices.IndexFunc(folders, func(s string) bool { return names[s] }); i >= 0 {
			return folders[i], true
		}
	}
	if isDir {
		return "", false
	}
	for _, name := range specNamesOf(kind, segments[len(segments)-1], data) {
		if names[name] {
			return name, true
		}
	}
	return "", false
}

// specNamesOf returns the names a spec file can carry: its file stem and
// the `name` it declares.
func specNamesOf(kind spec.Kind, base string, data []byte) []string {
	names := []string{strings.TrimSuffix(base, filepath.Ext(base))}
	var e spec.Entry
	var err error
	switch filepath.Ext(base) {
	case ".md":
		e, err = spec.ParseMarkdownBytes(kind, data)
	case ".yaml", ".yml":
		e, err = spec.ParseYAMLBytes(kind, data)
	default:
		return names
	}
	if err == nil && e.Name != "" {
		names = append(names, e.Name)
	}
	return names
}

// printNote lists the local specs the import left out, if any.
func (g *localImportGuard) printNote() {
	if len(g.skipped) == 0 {
		return
	}
	labels := make([]string, 0, len(g.skipped))
	for l := range g.skipped {
		labels = append(labels, l)
	}
	slices.Sort(labels)
	_, _ = fmt.Fprintf(os.Stdout,
		"  note: left %d local spec(s) out of the shared source; edit them under %s/: %s\n",
		len(labels), defaultProjectUser, strings.Join(labels, ", "))
}
