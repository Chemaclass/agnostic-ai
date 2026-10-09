package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// importViewGuard remembers each file an import run writes as it was
// before the run, with the sources that wrote it.
type importViewGuard struct {
	source string
	files  map[string]*importViewFile
}

type importViewFile struct {
	before  []byte
	existed bool
	sources []string
}

// importView is the active guard, or nil outside an import run.
// Sequential use only, like importSandbox.
var importView *importViewGuard

// importDrops maps each spec the last import run rewrote, by
// specPathKey, to what the new bytes lose that the importing tools never
// showed (see replacesSpec). Sequential use only.
var importDrops = map[string][]string{}

func (g *importViewGuard) beginSource(source string) {
	if g != nil {
		g.source = source
	}
}

// note records path before its first write in the run.
func (g *importViewGuard) note(path string) {
	if g == nil {
		return
	}
	path = filepath.Clean(path)
	f, ok := g.files[path]
	if !ok {
		f = &importViewFile{}
		if data, err := os.ReadFile(path); err == nil {
			f.before, f.existed = data, true
		}
		g.files[path] = f
	}
	if g.source != "" && !slices.Contains(f.sources, g.source) {
		f.sources = append(f.sources, g.source)
	}
}

// withImportViewGuard runs fn, then puts back every existing spec the run
// rewrote while each importing tool shows nothing new in it: the tool's
// files are what the old spec renders to, or the old spec renders to
// what the new one does. The spec then keeps what only other tools
// read, such as ::target blocks, workspaces, and comments, and a file
// format an older release wrote.
func withImportViewGuard(root string, fn func() error) error {
	prior := importView
	guard := &importViewGuard{files: map[string]*importViewFile{}}
	importView = guard
	importDrops = map[string][]string{}
	runErr := fn()
	importView = prior
	return errors.Join(runErr, guard.keepUnchanged(root))
}

func (g *importViewGuard) keepUnchanged(root string) error {
	paths := g.rewrittenSpecs(root)
	if len(paths) == 0 {
		return nil
	}
	cfg, after, err := loadProject(root)
	if err != nil {
		return nil
	}
	var before spec.Bundle
	if err := g.withBefore(paths, func() error {
		var loadErr error
		_, before, loadErr = loadProject(root)
		return loadErr
	}); err != nil {
		return nil
	}
	adapters.SetWarner(io.Discard)
	defer func() {
		adapters.ResetCapabilityWarnings()
		adapters.SetWarner(os.Stderr)
	}()
	r := &importViewRenderer{guard: g, cfg: cfg, after: after, full: map[string]map[string]string{}}
	var keep, others []string
	for _, path := range paths {
		f := g.files[path]
		newEntry, newOK := entryAt(after, path)
		oldEntry, oldOK := entryAt(before, path)
		switch {
		case newOK && oldOK:
			if r.specShowsSame(f, oldEntry, newEntry) {
				keep = append(keep, path)
				continue
			}
			lost, err := r.keepHidden(path, f, oldEntry)
			if err != nil {
				return err
			}
			if len(lost) > 0 {
				importDrops[specPathKey(path)] = lost
			}
		case !newOK && !oldOK && !inSkillFolder(after, path):
			others = append(others, path)
		}
	}
	keep = append(keep, r.othersShowSame(others)...)
	for _, path := range keep {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := importWriteFile(path, g.files[path].before, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

// rewrittenSpecs lists the files in a spec directory, other than
// AGNOSTIC_AI.md, that the run changed and that existed before it. A
// file the run reached through a link or under a second name is left
// out: its bytes before the run are not known for each name.
func (g *importViewGuard) rewrittenSpecs(root string) []string {
	specDirs := importSpecDirs(root)
	infos := map[string]os.FileInfo{}
	for path := range g.files {
		if info, err := os.Stat(path); err == nil {
			infos[path] = info
		}
	}
	shared := func(path string) bool {
		if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
		for other, info := range infos {
			if other != path && os.SameFile(info, infos[path]) {
				return true
			}
		}
		return false
	}
	var paths []string
	for path, f := range g.files {
		if !f.existed || infos[path] == nil || specPathKey(path) == agnosticMainFile || !inSpecDir(path, specDirs) || shared(path) {
			continue
		}
		if now, err := os.ReadFile(path); err == nil && !bytes.Equal(now, f.before) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths
}

// withBefore runs fn with the old bytes of paths on disk, and puts the
// new bytes back after it, even when it fails.
func (g *importViewGuard) withBefore(paths []string, fn func() error) error {
	now := make(map[string][]byte, len(paths))
	var errs []error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			break
		}
		now[path] = data
		if err := writeKeepingMode(path, g.files[path].before); err != nil {
			errs = append(errs, err)
			break
		}
	}
	if len(errs) == 0 {
		errs = append(errs, fn())
	}
	for path, data := range now {
		errs = append(errs, writeKeepingMode(path, data))
	}
	return errors.Join(errs...)
}

func writeKeepingMode(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := os.WriteFile(path, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// importViewRenderer renders specs for the importing tools in memory.
type importViewRenderer struct {
	guard *importViewGuard
	cfg   *config.Config
	after spec.Bundle
	// full caches each tool's render of the whole project after the run.
	full map[string]map[string]string
}

// specShowsSame reports whether every tool that wrote a spec shows
// nothing new in it. The spec renders on its own.
func (r *importViewRenderer) specShowsSame(f *importViewFile, oldEntry, newEntry spec.Entry) bool {
	tools := importingTools(f.sources)
	if len(tools) == 0 {
		return false
	}
	for _, a := range tools {
		oldFiles, err := r.render(a, spec.NewBundle([]spec.Entry{oldEntry}))
		if err != nil {
			return false
		}
		newFiles, err := r.render(a, spec.NewBundle([]spec.Entry{newEntry}))
		if err != nil || len(newFiles) == 0 {
			return false
		}
		if !covers(oldFiles, newFiles) && !onDisk(oldFiles, newFiles) {
			return false
		}
	}
	return true
}

// othersShowSame returns the files among paths, such as overlays, whose
// old bytes give each tool that wrote them the files the new bytes do.
// They render with the whole project, first together, since one overlay
// can depend on another, then one by one.
func (r *importViewRenderer) othersShowSame(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	var sources []string
	for _, path := range paths {
		for _, s := range r.guard.files[path].sources {
			if !slices.Contains(sources, s) {
				sources = append(sources, s)
			}
		}
	}
	if r.wholeShowsSame(paths, sources) {
		return paths
	}
	if len(paths) == 1 {
		return nil
	}
	var same []string
	for _, path := range paths {
		if r.wholeShowsSame([]string{path}, r.guard.files[path].sources) {
			same = append(same, path)
		}
	}
	return same
}

func (r *importViewRenderer) wholeShowsSame(paths, sources []string) bool {
	tools := importingTools(sources)
	if len(tools) == 0 {
		return false
	}
	for _, a := range tools {
		newFiles, err := r.renderWhole(a)
		if err != nil {
			return false
		}
		var oldFiles map[string]string
		var renderErr error
		if err := r.guard.withBefore(paths, func() error {
			oldFiles, renderErr = r.render(a, r.after)
			return nil
		}); err != nil || renderErr != nil || !covers(oldFiles, newFiles) {
			return false
		}
	}
	return true
}

func (r *importViewRenderer) renderWhole(a adapters.Adapter) (map[string]string, error) {
	if files, ok := r.full[a.Name()]; ok {
		return files, nil
	}
	files, err := r.render(a, r.after)
	if err == nil {
		r.full[a.Name()] = files
	}
	return files, err
}

func (r *importViewRenderer) render(a adapters.Adapter, b spec.Bundle) (map[string]string, error) {
	captured, err := captureAdapterFiles(adapters.NewSession(), a, b, r.cfg)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string, len(captured))
	for _, f := range captured {
		files[filepath.ToSlash(filepath.Clean(f.Path))] = f.Content
	}
	return files, nil
}

// importingTools resolves each source to its target, or returns nil when
// one has none, so no render speaks for it.
func importingTools(sources []string) []adapters.Adapter {
	var tools []adapters.Adapter
	for _, s := range sources {
		a, err := adapters.Resolve(s)
		if err != nil {
			return nil
		}
		tools = append(tools, a)
	}
	return tools
}

// onDisk reports whether the tool's files that the new spec renders to
// hold exactly what the old spec renders to, so the import read nothing
// the old spec does not already say.
func onDisk(old, now map[string]string) bool {
	for path := range now {
		if _, ok := old[path]; !ok {
			return false
		}
	}
	for path, content := range old {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil || string(data) != content {
			return false
		}
	}
	return true
}

// covers reports whether old holds every file of now, at least one, with
// the same content. Old may hold more: a workspace copy the import cannot
// read back is still what the tool showed. A spec that gives the tool no
// file did not come from it.
func covers(old, now map[string]string) bool {
	if len(now) == 0 {
		return false
	}
	for path, content := range now {
		if got, ok := old[path]; !ok || got != content {
			return false
		}
	}
	return true
}

func entryAt(b spec.Bundle, path string) (spec.Entry, bool) {
	key := specPathKey(path)
	for _, e := range b.All() {
		if e.Path != "" && specPathKey(e.Path) == key {
			return e, true
		}
	}
	return spec.Entry{}, false
}

// inSkillFolder reports whether path is an asset of a skill: the tool's
// own bytes, copied as they are.
func inSkillFolder(b spec.Bundle, path string) bool {
	key := specPathKey(path)
	for _, e := range b.Skills {
		if dir := e.SkillAssetDir(); dir != "" && strings.HasPrefix(key, specPathKey(dir)+"/") {
			return true
		}
	}
	return false
}

// keepHidden carries back into a rewritten spec what its old bytes hold
// that the tools which wrote it never showed, so no edit in them removed
// it, and returns what it cannot carry. A top-level key those tools'
// files do not depend on goes back into a markdown spec's frontmatter.
// ::target blocks and YAML comments cannot: a tool's file holds that
// tool's view of the body, so no import carries an edit back into a
// body with blocks for other tools.
func (r *importViewRenderer) keepHidden(path string, f *importViewFile, old spec.Entry) ([]string, error) {
	now, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var hidden []string
	nowKeys := topLevelKeys(yamlPart(path, now))
	for _, key := range topLevelKeys(yamlPart(path, f.before)) {
		if !slices.Contains(nowKeys, key) && r.hidesKey(f.sources, old, key) {
			hidden = append(hidden, key)
		}
	}
	if len(hidden) > 0 && filepath.Ext(path) == ".md" {
		omitted := map[string]bool{}
		for _, key := range hidden {
			omitted[key] = true
		}
		merged, err := mergeSpecFrontmatter(f.before, now, specFields{all: true, omitted: omitted})
		if err == nil && !slices.ContainsFunc(hidden, func(k string) bool { return !slices.Contains(topLevelKeys(yamlPart(path, merged)), k) }) {
			info, err := os.Stat(path)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			if err := importWriteFile(path, merged, info.Mode().Perm()); err != nil {
				return nil, err
			}
			now, hidden = merged, nil
		}
	}
	var lost []string
	if filepath.Ext(path) == ".md" {
		_, beforeBody, _ := splitFrontmatter(f.before)
		_, nowBody, _ := splitFrontmatter(now)
		if len(spec.FenceTargets(beforeBody)) > 0 && nowBody != beforeBody {
			lost = append(lost, "::target blocks")
		}
	}
	lost = append(lost, hidden...)
	nowYAML := string(yamlPart(path, now))
	for _, line := range strings.Split(string(yamlPart(path, f.before)), "\n") {
		if c := strings.TrimSpace(line); strings.HasPrefix(c, "#") && !strings.Contains(nowYAML, c) {
			lost = append(lost, "comments")
			break
		}
	}
	return lost, nil
}

// hidesKey reports whether each tool renders e the same without key.
func (r *importViewRenderer) hidesKey(sources []string, e spec.Entry, key string) bool {
	tools := importingTools(sources)
	if len(tools) == 0 {
		return false
	}
	without := e
	without.Meta = maps.Clone(e.Meta)
	delete(without.Meta, key)
	without.MetaKeys = slices.DeleteFunc(slices.Clone(e.MetaKeys), func(k string) bool { return k == key })
	for _, a := range tools {
		with, err := r.render(a, spec.NewBundle([]spec.Entry{e}))
		if err != nil {
			return false
		}
		got, err := r.render(a, spec.NewBundle([]spec.Entry{without}))
		if err != nil || !maps.Equal(with, got) {
			return false
		}
	}
	return true
}

// yamlPart is the frontmatter of a markdown spec, the whole of a YAML
// one, and nothing for any other file.
func yamlPart(path string, data []byte) []byte {
	switch filepath.Ext(path) {
	case ".md":
		yamlBytes, _, _ := splitFrontmatter(data)
		return yamlBytes
	case ".yaml", ".yml":
		return data
	}
	return nil
}

func topLevelKeys(yamlBytes []byte) []string {
	m, err := frontmatterMapping(yamlBytes)
	if err != nil || m == nil {
		return nil
	}
	keys := make([]string, 0, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		keys = append(keys, m.Content[i].Value)
	}
	return keys
}
