// Package spec loads and parses agnostic-ai source specs from disk.
//
// Specs come in six kinds (agent, skill, rule, hook, mcp, command).
// Markdown specs (agent, skill, rule, command) use YAML frontmatter;
// hook and mcp specs are pure YAML.
package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// Kind names a spec category.
type Kind string

// Spec kinds.
const (
	KindAgent   Kind = "agent"
	KindSkill   Kind = "skill"
	KindRule    Kind = "rule"
	KindHook    Kind = "hook"
	KindMCP     Kind = "mcp"
	KindCommand Kind = "command"
)

// AllKinds is the canonical ordering used by adapters that emit
// per-kind sections.
var AllKinds = []Kind{KindAgent, KindSkill, KindRule, KindHook, KindMCP, KindCommand}

// Entry is a single loaded spec.
type Entry struct {
	Kind Kind
	Name string
	Path string
	// Scope is the relative directory under the source kind directory in
	// which the spec lives, with forward slashes. A spec at
	// `rules/backend/auth.md` has Scope "backend"; a spec at the root of
	// `rules/` has Scope "".
	//
	// Adapters that produce nested per-directory outputs (Codex, Cursor,
	// Cline, Windsurf, Continue) honor Scope. Single-document adapters
	// (Claude CLAUDE.md, Gemini, Aider, Copilot) merge regardless.
	Scope string
	// Layer names the source layer this entry came from
	// ("user-global", "project", "project-user"). Empty when loaded
	// outside the layered loader (legacy path).
	Layer string
	Meta  map[string]any
	// MetaKeys preserves the order in which keys appeared in the source
	// frontmatter (or pure YAML for hooks/MCPs). Adapters pass this to
	// emit.DocumentOrdered / emit.FrontmatterOrdered so a round-trip
	// through agnostic-ai keeps the author's key order. Nil for entries
	// built programmatically (e.g. WASM playground inputs without a
	// preserved key order); emit falls back to alphabetical order in
	// that case.
	MetaKeys []string
	Body     string
}

// Description returns the entry's description from frontmatter, or "" if
// the field is missing or not a string.
func (e Entry) Description() string {
	d, _ := e.Meta["description"].(string)
	return d
}

// Globs returns the entry's globs frontmatter as a string, or "" if
// missing or not a string.
func (e Entry) Globs() string {
	g, _ := e.Meta["globs"].(string)
	return g
}

// EffectiveScope returns the routing prefix for the entry. A non-empty
// Scope (derived from source layout) wins over a frontmatter override
// (`scope: <relpath>`); the override wins over a globs prefix; an empty
// result means root.
func (e Entry) EffectiveScope() string {
	if e.Scope != "" {
		return e.Scope
	}
	if s, ok := e.Meta["scope"].(string); ok && s != "" {
		return strings.Trim(filepath.ToSlash(s), "/")
	}
	return ""
}

// Bundle is a pre-bucketed view of loaded entries. Adapters consume this
// directly to avoid repeated Filter calls.
type Bundle struct {
	Agents   []Entry
	Skills   []Entry
	Rules    []Entry
	Hooks    []Entry
	MCPs     []Entry
	Commands []Entry
}

// NewBundle groups a flat slice of entries by kind. Useful in tests and
// when adapting external sources of Entry slices.
func NewBundle(entries []Entry) Bundle {
	var b Bundle
	for _, e := range entries {
		switch e.Kind {
		case KindAgent:
			b.Agents = append(b.Agents, e)
		case KindSkill:
			b.Skills = append(b.Skills, e)
		case KindRule:
			b.Rules = append(b.Rules, e)
		case KindHook:
			b.Hooks = append(b.Hooks, e)
		case KindMCP:
			b.MCPs = append(b.MCPs, e)
		case KindCommand:
			b.Commands = append(b.Commands, e)
		}
	}
	return b
}

// All returns every entry in canonical kind order.
func (b Bundle) All() []Entry {
	out := make([]Entry, 0, len(b.Agents)+len(b.Skills)+len(b.Rules)+len(b.Hooks)+len(b.MCPs)+len(b.Commands))
	out = append(out, b.Agents...)
	out = append(out, b.Skills...)
	out = append(out, b.Rules...)
	out = append(out, b.Hooks...)
	out = append(out, b.MCPs...)
	out = append(out, b.Commands...)
	return out
}

// Has reports whether the bundle contains any entry of the given kind.
func (b Bundle) Has(k Kind) bool {
	switch k {
	case KindAgent:
		return len(b.Agents) > 0
	case KindSkill:
		return len(b.Skills) > 0
	case KindRule:
		return len(b.Rules) > 0
	case KindHook:
		return len(b.Hooks) > 0
	case KindMCP:
		return len(b.MCPs) > 0
	case KindCommand:
		return len(b.Commands) > 0
	}
	return false
}

// Layer is one tier in a layered spec load. Layers stack low-precedence
// to high-precedence; later layers override earlier layers by
// (Kind, Name).
type Layer struct {
	Name    string
	Root    string
	Sources config.Sources
}

// LoadBundle walks the source directories under root and returns a
// pre-bucketed Bundle. Single-layer convenience wrapper around
// LoadLayered.
func LoadBundle(root string, cfg *config.Config) (Bundle, error) {
	return LoadLayered([]Layer{{Name: "project", Root: root, Sources: cfg.Sources}})
}

// LoadLayered walks each layer's source directories in order and merges
// the results. For each kind, entries with the same Name from a later
// layer replace earlier ones; new names append. Each Entry is tagged
// with its source Layer.Name for provenance.
func LoadLayered(layers []Layer) (Bundle, error) {
	var b Bundle
	for _, layer := range layers {
		lb, err := loadLayer(layer)
		if err != nil {
			return Bundle{}, err
		}
		b.Agents = mergeEntries(b.Agents, lb.Agents)
		b.Skills = mergeEntries(b.Skills, lb.Skills)
		b.Rules = mergeEntries(b.Rules, lb.Rules)
		b.Hooks = mergeEntries(b.Hooks, lb.Hooks)
		b.MCPs = mergeEntries(b.MCPs, lb.MCPs)
		b.Commands = mergeEntries(b.Commands, lb.Commands)
	}
	return b, nil
}

func loadLayer(layer Layer) (Bundle, error) {
	var b Bundle
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
	}
	for _, l := range loaders {
		if l.src == "" {
			continue
		}
		dir := filepath.Join(layer.Root, l.src)
		entries, err := walkDir(dir, l.ext, l.kind, l.parse)
		if err != nil {
			return Bundle{}, fmt.Errorf("load %s [%s]: %w", l.kind, layer.Name, err)
		}
		assignScopes(entries, dir, l.kind)
		for i := range entries {
			entries[i].Layer = layer.Name
		}
		*l.into = entries
	}
	return b, nil
}

// mergeEntries appends src onto base, replacing any entry in base with
// the same Name. Order preserved: existing names keep their slot, new
// names append.
func mergeEntries(base, src []Entry) []Entry {
	if len(src) == 0 {
		return base
	}
	idx := make(map[string]int, len(base))
	for i, e := range base {
		idx[e.Name] = i
	}
	for _, e := range src {
		if i, ok := idx[e.Name]; ok {
			base[i] = e
			continue
		}
		idx[e.Name] = len(base)
		base = append(base, e)
	}
	return base
}

// assignScopes derives Entry.Scope from the source layout. For markdown
// kinds (rules, agents, skills), the scope is the relative directory
// from the source root. Skill nested layout (`skills/<name>/SKILL.md`)
// is a special case: the immediate parent IS the skill name, not a
// scope, so skill scope is derived from the grandparent only.
func assignScopes(entries []Entry, dir string, kind Kind) {
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
		entries[i].Scope = parent
	}
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

func walkDir(dir, ext string, kind Kind, parse func(string) (Entry, error)) ([]Entry, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var entries []Entry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ext {
			return nil
		}
		entry, err := parse(path)
		if err != nil {
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
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// ParseMarkdownBytes parses an in-memory spec document and returns the
// resulting Entry. Used by callers that have a spec body in hand and
// no on-disk path (e.g. the WASM playground) — Path is left empty so
// adapters that emit per-file outputs fall back to the entry's Name.
//
// The data must be UTF-8 with optional `---` frontmatter on top.
func ParseMarkdownBytes(kind Kind, data []byte) (Entry, error) {
	meta, keys, body, err := splitFrontmatter(normalizeLineEndings(data))
	if err != nil {
		return Entry{}, err
	}
	name, _ := meta["name"].(string)
	return Entry{
		Kind:     kind,
		Name:     name,
		Meta:     meta,
		MetaKeys: keys,
		Body:     body,
	}, nil
}

// ParseYAMLBytes parses an in-memory hook or MCP spec (pure YAML, no
// frontmatter) and returns the Entry.
func ParseYAMLBytes(kind Kind, data []byte) (Entry, error) {
	meta, keys, err := decodeYAMLOrdered(data)
	if err != nil {
		return Entry{}, err
	}
	name, _ := meta["name"].(string)
	return Entry{
		Kind:     kind,
		Name:     name,
		Meta:     meta,
		MetaKeys: keys,
	}, nil
}

func parseMarkdown(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	meta, keys, body, err := splitFrontmatter(normalizeLineEndings(data))
	if err != nil {
		// Frontmatter starts at line 2 (line 1 is the opening `---`).
		return Entry{}, formatYAMLError(path, err, 1)
	}
	name, _ := meta["name"].(string)
	return Entry{
		Path:     path,
		Name:     name,
		Meta:     meta,
		MetaKeys: keys,
		Body:     body,
	}, nil
}

func parseYAML(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	meta, keys, err := decodeYAMLOrdered(data)
	if err != nil {
		return Entry{}, formatYAMLError(path, err, 0)
	}
	name, _ := meta["name"].(string)
	return Entry{
		Path:     path,
		Name:     name,
		Meta:     meta,
		MetaKeys: keys,
	}, nil
}

// decodeYAMLOrdered parses a YAML document into both a map and an
// ordered key slice so adapters can re-emit the frontmatter in source
// order. Empty input returns an empty map and nil keys.
func decodeYAMLOrdered(data []byte) (map[string]any, []string, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, nil, err
	}
	meta, keys := nodeToOrderedMap(&node)
	if meta == nil {
		meta = map[string]any{}
	}
	return meta, keys, nil
}

// nodeToOrderedMap converts a YAML mapping node to (map, ordered keys).
// Non-mapping inputs return (nil, nil); the DocumentNode wrapper is
// peeled when present. Errors decoding individual values cause the
// affected key to be dropped silently — yaml.Unmarshal would have
// already surfaced the syntactic failure at parse time.
func nodeToOrderedMap(n *yaml.Node) (map[string]any, []string) {
	if n == nil {
		return nil, nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil, nil
		}
		return nodeToOrderedMap(n.Content[0])
	}
	if n.Kind != yaml.MappingNode {
		return nil, nil
	}
	meta := make(map[string]any, len(n.Content)/2)
	keys := make([]string, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		valNode := n.Content[i+1]
		var v any
		if err := valNode.Decode(&v); err != nil {
			continue
		}
		if _, dup := meta[keyNode.Value]; !dup {
			keys = append(keys, keyNode.Value)
		}
		meta[keyNode.Value] = v
	}
	return meta, keys
}

// normalizeLineEndings converts CRLF to LF so the frontmatter splitter
// works on Windows-authored files.
func normalizeLineEndings(b []byte) []byte {
	if !bytes.Contains(b, []byte("\r\n")) {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// splitFrontmatter parses a leading `---` block as YAML and returns the
// remaining body along with the ordered map of frontmatter keys. A file
// that does not start with `---`, or whose closing `---` is missing,
// is treated as body-only with empty meta. Malformed YAML inside a
// fully delimited block is surfaced as an error rather than silently
// swallowed.
func splitFrontmatter(data []byte) (map[string]any, []string, string, error) {
	const delim = "---"
	empty := map[string]any{}

	if !bytes.HasPrefix(data, []byte(delim)) {
		return empty, nil, string(data), nil
	}
	rest := data[len(delim):]
	idx := bytes.Index(rest, []byte("\n"+delim))
	if idx < 0 {
		return empty, nil, string(data), nil
	}
	yamlPart := rest[:idx]
	body := bytes.TrimLeft(rest[idx+len("\n"+delim):], "\n")

	meta, keys, err := decodeYAMLOrdered(yamlPart)
	if err != nil {
		return nil, nil, "", err
	}
	return meta, keys, string(body), nil
}
