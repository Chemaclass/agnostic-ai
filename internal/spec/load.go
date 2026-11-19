package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// Layer is one tier in a layered spec load. Layers stack low-precedence
// to high-precedence; later layers override earlier layers by
// (Kind, Name).
type Layer struct {
	Name    string
	Root    string
	Sources config.Sources
	// Extends merges a spec into the same-name spec of a lower layer
	// instead of replacing it: the personal `local` layers.
	Extends bool
	// IncludeRoot is the project root that review `@path` lines resolve
	// from. Empty leaves those lines as written.
	IncludeRoot string
	// RuleRoot overrides IncludeRoot for rule folders without expanding review includes.
	RuleRoot string
	// Files stands in for YAML spec files, keyed by the path the walk
	// spells, so a preview loads planned bytes without writing them. A nil
	// value hides the file; a key with no file on disk adds it.
	Files map[string][]byte
}

// LoadBundle walks the source directories under root and returns a
// pre-bucketed Bundle. Single-layer convenience wrapper around
// LoadLayered.
func LoadBundle(root string, cfg *config.Config) (Bundle, error) {
	return LoadLayered([]Layer{{Name: "project", Root: root, Sources: cfg.Sources, RuleRoot: root}})
}

// LoadLayered walks each layer's source directories in order and merges
// the results. For each kind, entries with the same Name from a later
// layer replace earlier ones; new names append. Each Entry is tagged
// with its source Layer.Name for provenance.
func LoadLayered(layers []Layer) (Bundle, error) {
	return loadLayered(layers, nil)
}

func loadLayered(layers []Layer, history *layerHistoryRecorder) (Bundle, error) {
	var b Bundle
	for _, layer := range layers {
		lb, err := loadLayer(layer)
		if err != nil {
			return Bundle{}, err
		}
		for _, m := range []struct {
			into *[]Entry
			from []Entry
		}{
			{&b.Agents, lb.Agents}, {&b.Skills, lb.Skills}, {&b.Rules, lb.Rules},
			{&b.Hooks, lb.Hooks}, {&b.MCPs, lb.MCPs}, {&b.Commands, lb.Commands},
			{&b.Settings, lb.Settings}, {&b.Reviews, lb.Reviews},
			{&b.Environments, lb.Environments}, {&b.Ignores, lb.Ignores},
		} {
			merged, shadowed := mergeEntries(*m.into, m.from, layer.Extends, history)
			*m.into = merged
			b.Shadowed = append(b.Shadowed, shadowed...)
		}
	}
	return b, nil
}

func loadLayer(layer Layer) (Bundle, error) {
	var b Bundle
	ruleRoot := layer.RuleRoot
	if ruleRoot == "" {
		ruleRoot = layer.IncludeRoot
	}
	loaders := []struct {
		src   string
		ext   string
		kind  Kind
		parse func(string) (Entry, error)
		into  *[]Entry
	}{
		{layer.Sources.Agents, ".md", KindAgent, parseMarkdown, &b.Agents},
		{layer.Sources.Skills, ".md", KindSkill, parseMarkdown, &b.Skills},
		{layer.Sources.Rules, ".md", KindRule, parseMarkdown, &b.Rules},
		{layer.Sources.Hooks, ".yaml", KindHook, parseYAML, &b.Hooks},
		{layer.Sources.MCPs, ".yaml", KindMCP, parseYAML, &b.MCPs},
		{layer.Sources.Commands, ".md", KindCommand, parseMarkdown, &b.Commands},
		{layer.Sources.Settings, ".yaml", KindSettings, parseYAML, &b.Settings},
		{layer.Sources.Reviews, ".md", KindReview, parseMarkdown, &b.Reviews},
		{layer.Sources.Environments, ".yaml", KindEnvironment, parseYAML, &b.Environments},
		{layer.Sources.Ignore, ".md", KindIgnore, parseMarkdown, &b.Ignores},
	}
	for _, l := range loaders {
		if l.src == "" {
			continue
		}
		dir := config.ResolveSourcePath(layer.Root, l.src)
		parse := l.parse
		if len(layer.Files) > 0 && l.ext == ".yaml" {
			parse = layer.plannedParse(l.parse)
		}
		entries, err := walkDir(dir, l.ext, l.kind, parse)
		if err == nil && len(layer.Files) > 0 && l.ext == ".yaml" {
			entries, err = layer.addPlannedFiles(entries, dir, l.kind)
		}
		if err != nil {
			if errs.CodeOf(err) == errs.CodeSpecParse {
				return Bundle{}, err // the path already says which spec
			}
			return Bundle{}, fmt.Errorf("load %s [%s]: %w", l.kind, layer.Name, err)
		}
		if l.kind == KindRule {
			for i := range entries {
				entries[i] = entries[i].PathsAsGlobs()
			}
		}
		assignScopes(entries, dir, l.kind, ruleRoot)
		if l.kind == KindReview && layer.IncludeRoot != "" {
			if err := resolveEntryIncludes(entries, layer.IncludeRoot); err != nil {
				return Bundle{}, err
			}
		}
		for i := range entries {
			entries[i].Layer = layer.Name
			entries[i].bodyLayer = layer.Name
		}
		*l.into = entries
	}
	return b, nil
}

// mergeEntries appends src onto base, replacing any entry in base with
// the same Name. Order preserved: existing names keep their slot, new
// names append.
//
// The second return holds entries that were replaced by a peer from the
// same layer. Replacing across layers is the documented way to override a
// spec, so those are not reported. Two files in one layer declaring the
// same name is an authoring mistake instead: one silently wins and the
// other's body never reaches any target (#582).
//
// In an extending layer, an entry from a lower layer is merged into
// instead (see extendEntry).
func mergeEntries(base, src []Entry, extends bool, history *layerHistoryRecorder) ([]Entry, []Entry) {
	if len(src) == 0 {
		return base, nil
	}
	src, shadowed := dedupeLayer(src)
	idx := make(map[string]int, len(base))
	for i, e := range base {
		idx[e.Name] = i
	}
	for _, e := range src {
		if i, ok := idx[e.Name]; ok {
			if extends {
				if history != nil {
					history.record(e, "extend")
				}
				base[i] = extendEntry(base[i], e)
			} else {
				if history != nil {
					history.record(e, "replace")
				}
				base[i] = e
			}
			continue
		}
		if history != nil {
			history.record(e, "base")
		}
		if extends {
			if expanded := expandParent(e.Body, ""); expanded != e.Body {
				e.Body, e.BodyLine = expanded, 0
			}
		}
		idx[e.Name] = len(base)
		base = append(base, e)
	}
	return base, shadowed
}

// dedupeLayer keeps the last entry of each name in one layer, at the
// first one's position, and returns the ones it dropped. Resolving the
// clash before merging lets the winner extend the lower layer exactly
// once.
func dedupeLayer(src []Entry) ([]Entry, []Entry) {
	var shadowed []Entry
	idx := make(map[string]int, len(src))
	out := make([]Entry, 0, len(src))
	for _, e := range src {
		if i, ok := idx[e.Name]; ok {
			shadowed = append(shadowed, out[i])
			out[i] = e
			continue
		}
		idx[e.Name] = len(out)
		out = append(out, e)
	}
	return out, shadowed
}

// assignScopes derives Entry.Scope from the source layout. For markdown
// kinds (rules, agents, skills), the scope is the relative directory
// from the source root. Skill nested layout (`skills/<name>/SKILL.md`)
// is a special case: the immediate parent IS the skill name, not a
// scope, so skill scope is derived from the grandparent only.
//
// A rule folder scopes only a rule without frontmatter `scope`, and with
// a projectRoot, only when it names a directory there.
func assignScopes(entries []Entry, dir string, kind Kind, projectRoot string) {
	for i := range entries {
		rel, err := filepath.Rel(dir, entries[i].Path)
		if err != nil {
			continue
		}
		parent := filepath.ToSlash(filepath.Dir(rel))
		if kind == KindSkill && filepath.Base(entries[i].Path) == "SKILL.md" {
			parent = filepath.ToSlash(filepath.Dir(filepath.Dir(rel)))
		}
		if parent == "." {
			parent = ""
		}
		if kind == KindRule {
			entries[i].ruleFolder = parent
			if parent != "" && (projectRoot == "" || isDir(filepath.Join(projectRoot, filepath.FromSlash(parent)))) {
				entries[i].Folder = parent
			}
			entries[i].Scope = entries[i].folderScope()
			if len(entries[i].NativeRuleTargets()) > 0 {
				entries[i].Scope = parent
			}
			continue
		}
		entries[i].Scope = parent
	}
}

// folderScope is the scope a rule takes from its folder: none when the
// frontmatter sets `scope`, which wins.
func (e Entry) folderScope() string {
	if _, explicit := e.Meta["scope"]; explicit {
		return ""
	}
	return e.Folder
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// LoadAll is a convenience wrapper that returns a flat slice. Prefer
// LoadBundle in new code.
func LoadAll(root string, cfg *config.Config) ([]Entry, error) {
	b, err := LoadBundle(root, cfg)
	if err != nil {
		return nil, err
	}
	return b.All(), nil
}

// Filter returns the subset of entries with the given kind.
func Filter(entries []Entry, kind Kind) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// insideFolderSkill reports whether a markdown file at path lives within a
// folder-based skill: an ancestor directory, up to and including root, that
// holds a SKILL.md. Such files are bundled assets of that skill, not skills
// of their own. Flat-file skills (`skills/<name>.md`, including scoped ones
// like `skills/backend/foo.md`) have no SKILL.md ancestor and return false.
func insideFolderSkill(path, root string) bool {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
			return true
		}
		if dir == root || dir == filepath.Dir(dir) {
			return false
		}
	}
}

// ValidateName rejects names that could escape an adapter's output tree.
// MCP names may contain forward slashes because vendor identifiers use
// them and MCPFileName encodes per-server filenames. Other spec kinds use
// the name directly as a filename or directory segment.
func ValidateName(kind Kind, name string) error {
	if kind == KindMCP {
		if name == "" || hasDotPathSegment(name) || strings.ContainsRune(name, '\\') || strings.ContainsRune(name, '\x00') {
			return fmt.Errorf("invalid MCP name %q: must not be empty, contain dot path segments, a backslash, or a NUL byte", name)
		}
		if slices.ContainsFunc(strings.Split(name, "/"), config.DotGitName) {
			return fmt.Errorf("invalid MCP name %q: names the Git folder", name)
		}
		return nil
	}
	if name != filepath.Base(name) || name == "." || name == ".." ||
		strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') ||
		strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("invalid spec name %q: must be a single path segment (no %q, %q, path separators, or a NUL byte)", name, "/", "..")
	}
	if config.GitDirName(name) {
		return fmt.Errorf("invalid spec name %q: names the Git folder", name)
	}
	return nil
}

func hasDotPathSegment(name string) bool {
	for _, segment := range strings.Split(name, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func walkDir(dir, ext string, kind Kind, parse func(string) (Entry, error)) ([]Entry, error) {
	var entries []Entry
	var files []string
	modes := map[string]fs.FileMode{}
	err := WalkSourceRoot(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == dir && errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if kind == KindSkill && d.Type().IsRegular() {
			info, err := d.Info()
			if errors.Is(err, fs.ErrNotExist) {
				// Gone since the folder was read, such as an editor's
				// temporary save file.
				return nil
			}
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			files = append(files, path)
			modes[path] = info.Mode()
		}
		if d.IsDir() || filepath.Ext(path) != ext {
			return nil
		}
		if kind == KindSkill && d.Name() != "SKILL.md" && insideFolderSkill(path, dir) {
			// A non-SKILL.md markdown file inside a folder-based skill is a
			// bundled asset, copied verbatim by the adapters. Promoting it
			// to its own skill spawns a phantom skill (#431).
			return nil
		}
		entry, err := parse(path)
		if errors.Is(err, errPlannedRemoval) {
			return nil
		}
		if err != nil {
			if errs.CodeOf(err) == errs.CodeSpecParse {
				return err // already names path:line:col
			}
			return fmt.Errorf("%s: %w", path, err)
		}
		entry.Kind = kind
		if entry.Name == "" {
			if kind == KindSkill && d.Name() == "SKILL.md" {
				// Skills nest under `<skill>/SKILL.md` so the parent
				// directory is the authoritative name, not the filename
				// stem. Mirrors the special case in assignScopes.
				entry.Name = filepath.Base(filepath.Dir(path))
			} else {
				entry.Name = strings.TrimSuffix(d.Name(), ext)
			}
		}
		if err := ValidateName(kind, entry.Name); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if kind == KindSkill {
		listSkillAssets(entries, dir, files, modes)
	}
	return entries, nil
}

// listSkillAssets records, on each folder skill, every file under its
// folder in walk order, as CopyTree would visit them.
func listSkillAssets(entries []Entry, root string, files []string, modes map[string]fs.FileMode) {
	byDir := map[string][]string{}
	for _, f := range files {
		for d := filepath.Dir(f); ; d = filepath.Dir(d) {
			byDir[d] = append(byDir[d], f)
			if d == root || d == filepath.Dir(d) {
				break
			}
		}
	}
	for i := range entries {
		dir := entries[i].SkillAssetDir()
		if dir == "" {
			continue
		}
		list := []AssetFile{}
		for _, f := range byDir[dir] {
			rel, err := filepath.Rel(dir, f)
			if err != nil {
				continue
			}
			list = append(list, AssetFile{Rel: filepath.ToSlash(rel), Mode: modes[f]})
		}
		entries[i].Assets = &list
	}
}
