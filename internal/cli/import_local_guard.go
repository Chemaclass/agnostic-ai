package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// localImportGuard keeps an import from storing a spec the project local
// layer supplies. Sync renders `.agnostic-ai/local/` specs into the same
// native files import reads, so without it a personal rule, agent, or
// skill would land in the shared source, and a shared spec the local
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
	// hooks indexes the local hook specs by content, the only identity
	// native hook settings keep.
	hooks []localHook
	// overlays maps an overlay file, absolute, to the kind whose specs
	// sync renders into the native file the overlay captures.
	overlays map[string]spec.Kind
	// skipped collects "<kind> <name>" labels for the closing note.
	skipped map[string]bool
	// kept lists the merged kinds whose import the run undid.
	kept map[spec.Kind]bool
	// saved holds, per absolute path of a local spec file the run wrote,
	// the file as it was before the run (nil when it did not exist).
	saved map[string]*savedImportFile
	// created lists the directories the run made for a local spec.
	created []string
	// skillFiles and skillDirs hold every skill write. Which skill a file
	// belongs to is known only once its folder holds a SKILL.md, so they
	// are sorted out when the run ends.
	skillFiles map[string]*savedImportFile
	skillDirs  []string
}

// savedImportFile is a file's content and mode before an import.
type savedImportFile struct {
	data []byte
	mode fs.FileMode
}

// mergedKinds are the kinds every target merges into one native file
// that keeps no spec names. A local spec of such a kind cannot be told
// apart from the shared ones on import, so the whole kind stays as it
// was.
var mergedKinds = map[spec.Kind]string{
	spec.KindSettings:    "settings",
	spec.KindReview:      "reviews",
	spec.KindEnvironment: "environments",
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
		dirs:       map[string]spec.Kind{},
		names:      map[spec.Kind]map[string]bool{},
		hooks:      newLocalHooks(bundle.Hooks),
		overlays:   map[string]spec.Kind{},
		skipped:    map[string]bool{},
		kept:       map[spec.Kind]bool{},
		saved:      map[string]*savedImportFile{},
		skillFiles: map[string]*savedImportFile{},
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
	// The Claude and Codex overlays capture the native settings file,
	// model and permissions included.
	for _, path := range []string{claudeOverlayPath(root), codexOverlayPath(root)} {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		g.overlays[abs] = spec.KindSettings
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
// the write may store a local spec in the shared source. data is nil for
// a directory. A destination it cannot read fails the write: taking it
// for a missing file would delete it when the run ends.
func (g *localImportGuard) track(path string, data []byte, isDir bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if kind, ok := g.overlays[abs]; ok {
		if len(g.names[kind]) == 0 {
			return nil
		}
		g.kept[kind] = true
		return g.save(abs, g.saved, &g.created)
	}
	kind, rel, _, ok := g.locate(abs)
	if !ok {
		return nil
	}
	switch {
	case kind == spec.KindSkill && len(g.names[kind]) > 0:
		if isDir {
			g.skillDirs = g.appendMissingDirs(g.skillDirs, abs)
			return nil
		}
		return g.save(abs, g.skillFiles, &g.skillDirs)
	case mergedKinds[kind] != "" && len(g.names[kind]) > 0:
		g.kept[kind] = true
	default:
		label, ok := g.localName(kind, rel, data, isDir)
		if !ok {
			return nil
		}
		g.skipped[label] = true
	}
	if isDir {
		g.created = g.appendMissingDirs(g.created, abs)
		return nil
	}
	return g.save(abs, g.saved, &g.created)
}

// save keeps the content of abs as it is now in files, once, and records
// the directories an importer will create for it in dirs.
func (g *localImportGuard) save(abs string, files map[string]*savedImportFile, dirs *[]string) error {
	if _, seen := files[abs]; seen {
		return nil
	}
	*dirs = g.appendMissingDirs(*dirs, filepath.Dir(abs))
	info, err := os.Stat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		files[abs] = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", abs, err)
	}
	prior, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("%s: %w", abs, err)
	}
	files[abs] = &savedImportFile{data: prior, mode: info.Mode().Perm()}
	return nil
}

// leaves reports whether path is a local spec's file in the shared
// source, and names the spec in the closing note. An importer calls it
// to skip a file up front instead of warning about it. Nil-safe.
func (g *localImportGuard) leaves(path string) bool {
	if g == nil {
		return false
	}
	kind, rel, _, ok := g.locate(path)
	if !ok || kind == spec.KindSkill {
		return false
	}
	label, ok := g.localName(kind, rel, nil, false)
	if ok {
		g.skipped[label] = true
	}
	return ok
}

// appendMissingDirs adds dir and each missing parent inside a kind
// directory to dirs. A directory it cannot stat counts as present, so it
// is never removed.
func (g *localImportGuard) appendMissingDirs(dirs []string, dir string) []string {
	for {
		if _, _, _, inside := g.locate(dir); !inside {
			return dirs
		}
		if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
			return dirs
		}
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
		dir = filepath.Dir(dir)
	}
}

// restore puts every local spec file back as it was before the run,
// removes the directories the run made for local specs, and drops the
// writes from a dry-run recording.
func (g *localImportGuard) restore() error {
	g.claimLocalSkills()
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

// claimLocalSkills moves the skill writes that belong to a local skill
// into saved and created. A skill is the folder holding its SKILL.md, or
// a flat `<name>.md`; its name is that folder's or file's.
func (g *localImportGuard) claimLocalSkills() {
	var roots []string
	for path, prior := range g.skillFiles {
		root, name := g.skillOwner(path)
		if root == "" {
			continue
		}
		g.saved[path] = prior
		g.skipped[string(spec.KindSkill)+" "+name] = true
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	for _, dir := range g.skillDirs {
		for _, root := range roots {
			if dir == root || strings.HasPrefix(dir, root+string(filepath.Separator)) {
				g.created = append(g.created, dir)
				break
			}
		}
	}
}

// skillOwner returns the local skill a skill file belongs to, as its
// root and name, or "" when it belongs to no local skill.
func (g *localImportGuard) skillOwner(path string) (string, string) {
	_, _, base, ok := g.locate(path)
	if !ok {
		return "", ""
	}
	names := g.names[spec.KindSkill]
	for dir := filepath.Dir(path); dir != base && strings.HasPrefix(dir, base); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
			if name := filepath.Base(dir); names[name] {
				return dir, name
			}
			return "", ""
		}
	}
	if filepath.Ext(path) != ".md" {
		return "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	for _, name := range specNamesOf(spec.KindSkill, filepath.Base(path), data) {
		if names[name] {
			return path, name
		}
	}
	return "", ""
}

// locate finds the shared source directory holding path and returns its
// kind, the path relative to it with forward slashes, and the directory.
func (g *localImportGuard) locate(path string) (spec.Kind, string, string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", "", false
	}
	// The deepest match wins, so a kind directory nested in another
	// still claims its own files.
	var kind spec.Kind
	var rel, base string
	for dir, k := range g.dirs {
		r, err := filepath.Rel(dir, abs)
		if err != nil || r == "." || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			continue
		}
		if rel == "" || len(r) < len(rel) {
			kind, rel, base = k, r, dir
		}
	}
	return kind, filepath.ToSlash(rel), base, rel != ""
}

// localName returns the "<kind> <name>" label of the local spec a write
// under a kind directory belongs to. A spec is one file named after it
// or declaring its name. An imported hook has a generated name, so it
// also matches on content. Skills are sorted out by claimLocalSkills.
func (g *localImportGuard) localName(kind spec.Kind, rel string, data []byte, isDir bool) (string, bool) {
	names := g.names[kind]
	if len(names) == 0 || isDir {
		return "", false
	}
	base := rel[strings.LastIndex(rel, "/")+1:]
	for _, name := range specNamesOf(kind, base, data) {
		if names[name] {
			return string(kind) + " " + name, true
		}
	}
	if kind == spec.KindHook {
		if name, ok := g.localHookName(data); ok {
			return string(kind) + " " + name, true
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

// printNote lists the local specs the import left out and the merged
// kinds it kept as they were, if any.
func (g *localImportGuard) printNote() {
	if len(g.skipped) > 0 {
		labels := slices.Sorted(maps.Keys(g.skipped))
		_, _ = fmt.Fprintf(os.Stdout,
			"  note: left %d local spec(s) out of the shared source; edit them under %s/: %s\n",
			len(labels), defaultProjectUser, strings.Join(labels, ", "))
	}
	if len(g.kept) > 0 {
		kinds := make(map[string]bool, len(g.kept))
		for k := range g.kept {
			kinds[mergedKinds[k]] = true
		}
		_, _ = fmt.Fprintf(os.Stdout,
			"  note: kept the shared %s as they were; %s/ feeds the same native files, so import cannot tell its specs apart\n",
			strings.Join(slices.Sorted(maps.Keys(kinds)), ", "), defaultProjectUser)
	}
}
