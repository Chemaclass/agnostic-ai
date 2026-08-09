// Package spec loads and parses agnostic-ai source specs from disk.
//
// Specs come in ten kinds (agent, skill, rule, hook, mcp, command,
// settings, review, environment, ignore). Markdown specs (agent, skill,
// rule, command, review, ignore) use YAML frontmatter; hook, mcp,
// settings, and environment specs are pure YAML.
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
	KindAgent       Kind = "agent"
	KindSkill       Kind = "skill"
	KindRule        Kind = "rule"
	KindHook        Kind = "hook"
	KindMCP         Kind = "mcp"
	KindCommand     Kind = "command"
	KindSettings    Kind = "settings"
	KindReview      Kind = "review"
	KindEnvironment Kind = "environment"
	KindIgnore      Kind = "ignore"
)

// AllKinds is the canonical ordering used by adapters that emit
// per-kind sections.
var AllKinds = []Kind{KindAgent, KindSkill, KindRule, KindHook, KindMCP, KindCommand, KindSettings, KindReview, KindEnvironment, KindIgnore}

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
	// MetaStyles records the YAML scalar style of each top-level
	// frontmatter value as it appeared in source. Adapters pass this to
	// emit.FrontmatterStyled / emit.DocumentStyled so a value that the
	// author hand-quoted with double quotes stays double-quoted on
	// re-emit, while a plain scalar stays plain. Nil for entries built
	// programmatically (e.g. WASM playground) and for keys whose source
	// style was the YAML default (PlainStyle, value 0).
	MetaStyles map[string]yaml.Style
	Body       string
}

// Description returns the entry's description from frontmatter, or "" if
// the field is missing or not a string.
func (e Entry) Description() string {
	d, _ := e.Meta["description"].(string)
	return d
}

// EmitsTo reports whether this entry should emit for the given target.
//
// Entries opt into per-target scoping via four frontmatter fields:
//
//   - `target: <name>` or `targets: [a, b]` — explicit allow-list.
//     Only adapters whose name appears in the list see the entry.
//   - `target-exclude: <name>` or `targets-exclude: [a, b]` — explicit
//     deny-list. The entry emits to every other configured target.
//
// When none of the four are set the entry emits everywhere a supporting
// adapter exists (legacy behavior). Empty target string short-circuits
// to true so callers that do not pass a target (tests, ad-hoc tooling)
// keep historical semantics.
//
// Exclude takes precedence: a target named in both an include AND an
// exclude list is excluded.
func (e Entry) EmitsTo(target string) bool {
	if target == "" {
		return true
	}
	if metaContainsTarget(e.Meta, "target-exclude", "targets-exclude", target) {
		return false
	}
	if hasInclude, matched := evalIncludeTargets(e.Meta, target); hasInclude {
		return matched
	}
	return true
}

// evalIncludeTargets reports whether the entry has any include filter
// set (target or targets) and, if so, whether the target matches it.
func evalIncludeTargets(meta map[string]any, target string) (bool, bool) {
	if s, ok := meta["target"].(string); ok && s != "" {
		return true, s == target
	}
	names, present := stringListField(meta, "targets")
	if !present || len(names) == 0 {
		return false, false
	}
	for _, s := range names {
		if s == target {
			return true, true
		}
	}
	return true, false
}

// metaContainsTarget reports whether either the single-string field or
// the list field names target. Used by exclude evaluation so a single
// helper covers both shapes (target-exclude string + targets-exclude list).
func metaContainsTarget(meta map[string]any, singleKey, listKey, target string) bool {
	if s, ok := meta[singleKey].(string); ok && s == target {
		return true
	}
	names, _ := stringListField(meta, listKey)
	for _, s := range names {
		if s == target {
			return true
		}
	}
	return false
}

// stringListField reads a YAML list of strings from meta[key]. Accepts
// both `[]any` (yaml.v3 default decode) and `[]string` (round-tripped
// through go structs). Returns (nil, false) when the field is absent.
func stringListField(meta map[string]any, key string) ([]string, bool) {
	switch v := meta[key].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	case []string:
		return v, true
	}
	return nil, false
}

// BodyFor returns the entry's body materialized for target, processing
// `::target <name>` / `::targets <a> <b>` / `::end` fences so adapters
// see only the sections meant for them. Closes #293.
//
//   - Lines outside any fence emit for every target.
//   - A fence pinned to one or more targets emits only when target is in
//     the allow-list; the marker lines themselves never emit.
//   - An unterminated fence runs to end-of-body so a missing `::end`
//     does not silently drop the tail of the file.
//   - Empty target returns the raw body unchanged so source-level tooling
//     (round-trip emit, the playground source view) keeps the fences.
//
// Bodies without fences pass through unchanged so the common case stays
// allocation-free.
func (e Entry) BodyFor(target string) string {
	if target == "" || !strings.Contains(e.Body, targetFenceOpen) {
		return e.Body
	}
	return renderBodyForTarget(e.Body, target)
}

// renderBodyForTarget walks the body line-by-line, keeping every line
// outside a fence and every line inside a matching fence. Marker lines
// are dropped. See BodyFor for the documented semantics.
func renderBodyForTarget(body, target string) string {
	lines := strings.Split(body, "\n")
	var out strings.Builder
	out.Grow(len(body))
	keep := true // outside a fence: every target sees the line
	for _, line := range lines {
		marker, allow := parseFenceMarker(line)
		switch marker {
		case fenceTargetOpen:
			keep = inAllowList(allow, target)
			continue
		case fenceTargetClose:
			keep = true
			continue
		}
		if keep {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	// strings.Split appends an empty trailing element for the final '\n';
	// the loop already wrote a '\n' for the line before it. Trim the
	// stray newline so the rendered body matches the source ending.
	s := out.String()
	if strings.HasSuffix(body, "\n") {
		s = strings.TrimSuffix(s, "\n")
	} else {
		s = strings.TrimSuffix(strings.TrimSuffix(s, "\n"), "\n")
	}
	// Dropped fences leave the surrounding blank lines stacked. Collapse
	// any run of 3 or more newlines back down to a paragraph break so
	// the rendered body reads cleanly for the active target.
	return collapseBlankRuns(s)
}

// collapseBlankRuns rewrites runs of 3+ consecutive '\n' bytes as
// exactly 2 so a dropped fence does not leave a wider-than-paragraph
// gap. 2 newlines (one paragraph break) is the maximum.
func collapseBlankRuns(s string) string {
	if !strings.Contains(s, "\n\n\n") {
		return s
	}
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

const (
	targetFenceOpen  = "::target"
	fenceTargetOpen  = 1
	fenceTargetClose = 2
)

// parseFenceMarker classifies a line as a fence opener, closer, or
// regular content. Returns the marker kind and the allow-list parsed
// off an opener. Marker lines must start at column 0; leading whitespace
// is treated as regular content so authors do not accidentally fence
// indented code blocks.
func parseFenceMarker(line string) (int, []string) {
	if line == "::end" || strings.HasPrefix(line, "::end ") {
		return fenceTargetClose, nil
	}
	switch {
	case strings.HasPrefix(line, "::target "):
		return fenceTargetOpen, strings.Fields(line[len("::target "):])
	case strings.HasPrefix(line, "::targets "):
		return fenceTargetOpen, strings.Fields(line[len("::targets "):])
	case line == "::target" || line == "::targets":
		// Bare opener with no targets — degenerate; treat as "no allow",
		// so the fence body is dropped for every target.
		return fenceTargetOpen, nil
	}
	return 0, nil
}

// inAllowList reports whether target appears in names. An empty list
// means "no target allowed" so a degenerate fence drops its body.
func inAllowList(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
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
	Agents       []Entry
	Skills       []Entry
	Rules        []Entry
	Hooks        []Entry
	MCPs         []Entry
	Commands     []Entry
	Settings     []Entry
	Reviews      []Entry
	Environments []Entry
	Ignores      []Entry
	// Shadowed holds entries dropped because a peer in the same layer
	// declared the same name. They reach no target; `lint` reports them
	// (#582). Cross-layer overrides are intentional and never listed here.
	Shadowed []Entry
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
		case KindSettings:
			b.Settings = append(b.Settings, e)
		case KindReview:
			b.Reviews = append(b.Reviews, e)
		case KindEnvironment:
			b.Environments = append(b.Environments, e)
		case KindIgnore:
			b.Ignores = append(b.Ignores, e)
		}
	}
	return b
}

// All returns every entry in canonical kind order.
func (b Bundle) All() []Entry {
	out := make([]Entry, 0, len(b.Agents)+len(b.Skills)+len(b.Rules)+len(b.Hooks)+len(b.MCPs)+len(b.Commands)+len(b.Settings)+len(b.Reviews)+len(b.Environments)+len(b.Ignores))
	out = append(out, b.Agents...)
	out = append(out, b.Skills...)
	out = append(out, b.Rules...)
	out = append(out, b.Hooks...)
	out = append(out, b.MCPs...)
	out = append(out, b.Commands...)
	out = append(out, b.Settings...)
	out = append(out, b.Reviews...)
	out = append(out, b.Environments...)
	out = append(out, b.Ignores...)
	return out
}

// HooksFor returns the subset of b.Hooks that should emit for target,
// per Entry.EmitsTo. Use this in adapter Emit() loops to honor hook
// `target:` / `targets:` scoping. Empty target returns b.Hooks verbatim.
func (b Bundle) HooksFor(target string) []Entry {
	if target == "" {
		return b.Hooks
	}
	return filterEntriesFor(b.Hooks, target)
}

// For returns a copy of b with every kind filtered by Entry.EmitsTo
// and each surviving entry's body materialized for target via
// Entry.BodyFor. Adapters that loop over `bundle.Agents` / `Skills` /
// `Rules` / `MCPs` / `Commands` see only entries scoped to their
// target via `target:` / `targets:` / `target-exclude:` /
// `targets-exclude:` frontmatter, AND see bodies with `::target` fences
// already resolved for the active target. Empty target string
// short-circuits to identity. Closes #292 + #293.
func (b Bundle) For(target string) Bundle {
	if target == "" {
		return b
	}
	return Bundle{
		Agents:       filterEntriesFor(b.Agents, target),
		Skills:       filterEntriesFor(b.Skills, target),
		Rules:        filterEntriesFor(b.Rules, target),
		Hooks:        filterEntriesFor(b.Hooks, target),
		MCPs:         filterEntriesFor(b.MCPs, target),
		Commands:     filterEntriesFor(b.Commands, target),
		Settings:     filterEntriesFor(b.Settings, target),
		Reviews:      filterEntriesFor(b.Reviews, target),
		Environments: filterEntriesFor(b.Environments, target),
		Ignores:      filterEntriesFor(b.Ignores, target),
	}
}

// filterEntriesFor returns the subset of entries whose EmitsTo(target)
// is true, with each survivor's Body materialized for target via
// BodyFor (a no-op when the body carries no `::target` fences).
func filterEntriesFor(entries []Entry, target string) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if !e.EmitsTo(target) {
			continue
		}
		if resolved := e.BodyFor(target); resolved != e.Body {
			e.Body = resolved
		}
		out = append(out, e)
	}
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
	case KindSettings:
		return len(b.Settings) > 0
	case KindReview:
		return len(b.Reviews) > 0
	case KindEnvironment:
		return len(b.Environments) > 0
	case KindIgnore:
		return len(b.Ignores) > 0
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
		for _, m := range []struct {
			into *[]Entry
			from []Entry
		}{
			{&b.Agents, lb.Agents}, {&b.Skills, lb.Skills}, {&b.Rules, lb.Rules},
			{&b.Hooks, lb.Hooks}, {&b.MCPs, lb.MCPs}, {&b.Commands, lb.Commands},
			{&b.Settings, lb.Settings}, {&b.Reviews, lb.Reviews},
			{&b.Environments, lb.Environments}, {&b.Ignores, lb.Ignores},
		} {
			merged, shadowed := mergeEntries(*m.into, m.from)
			*m.into = merged
			b.Shadowed = append(b.Shadowed, shadowed...)
		}
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
		{layer.Sources.Settings, ".yaml", KindSettings, parseYAML, &b.Settings},
		{layer.Sources.Reviews, ".md", KindReview, parseMarkdown, &b.Reviews},
		{layer.Sources.Environments, ".yaml", KindEnvironment, parseYAML, &b.Environments},
		{layer.Sources.Ignore, ".md", KindIgnore, parseMarkdown, &b.Ignores},
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
//
// The second return holds entries that were replaced by a peer from the
// same layer. Replacing across layers is the documented way to override a
// spec, so those are not reported. Two files in one layer declaring the
// same name is an authoring mistake instead: one silently wins and the
// other's body never reaches any target (#582).
func mergeEntries(base, src []Entry) ([]Entry, []Entry) {
	if len(src) == 0 {
		return base, nil
	}
	var shadowed []Entry
	idx := make(map[string]int, len(base))
	for i, e := range base {
		idx[e.Name] = i
	}
	for _, e := range src {
		if i, ok := idx[e.Name]; ok {
			if base[i].Layer == e.Layer {
				shadowed = append(shadowed, base[i])
			}
			base[i] = e
			continue
		}
		idx[e.Name] = len(base)
		base = append(base, e)
	}
	return base, shadowed
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

// checkSpecName rejects a spec name that is not a single safe path
// segment. Every adapter uses the name as a filename or directory segment
// in its output, so a value carrying path separators or `..` could make
// sync write outside the target directory (path traversal). Rejecting it
// at load fails the run with a clear message instead of emitting an
// escaping file. A name derived from a filename stem is always safe; the
// guard matters for an author-supplied `name:` frontmatter value.
func checkSpecName(name string) error {
	if name != filepath.Base(name) || name == "." || name == ".." ||
		strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return fmt.Errorf("invalid spec name %q: must be a single path segment (no %q, %q, or path separators)", name, "/", "..")
	}
	return nil
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
		if kind == KindSkill && d.Name() != "SKILL.md" && insideFolderSkill(path, dir) {
			// A non-SKILL.md markdown file inside a folder-based skill is a
			// bundled asset, copied verbatim by the adapters. Promoting it
			// to its own skill spawns a phantom skill (#431).
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
		if err := checkSpecName(entry.Name); err != nil {
			return fmt.Errorf("%s: %w", path, err)
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
	meta, keys, styles, body, err := splitFrontmatter(normalizeLineEndings(data))
	if err != nil {
		return Entry{}, err
	}
	name, _ := meta["name"].(string)
	return Entry{
		Kind:       kind,
		Name:       name,
		Meta:       meta,
		MetaKeys:   keys,
		MetaStyles: styles,
		Body:       body,
	}, nil
}

// ParseYAMLBytes parses an in-memory hook or MCP spec (pure YAML, no
// frontmatter) and returns the Entry.
func ParseYAMLBytes(kind Kind, data []byte) (Entry, error) {
	meta, keys, styles, err := decodeYAMLOrdered(data)
	if err != nil {
		return Entry{}, err
	}
	name, _ := meta["name"].(string)
	return Entry{
		Kind:       kind,
		Name:       name,
		Meta:       meta,
		MetaKeys:   keys,
		MetaStyles: styles,
	}, nil
}

func parseMarkdown(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	meta, keys, styles, body, err := splitFrontmatter(normalizeLineEndings(data))
	if err != nil {
		// Frontmatter starts at line 2 (line 1 is the opening `---`).
		return Entry{}, formatYAMLError(path, err, 1)
	}
	name, _ := meta["name"].(string)
	return Entry{
		Path:       path,
		Name:       name,
		Meta:       meta,
		MetaKeys:   keys,
		MetaStyles: styles,
		Body:       body,
	}, nil
}

func parseYAML(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	meta, keys, styles, err := decodeYAMLOrdered(data)
	if err != nil {
		return Entry{}, formatYAMLError(path, err, 0)
	}
	name, _ := meta["name"].(string)
	return Entry{
		Path:       path,
		Name:       name,
		Meta:       meta,
		MetaKeys:   keys,
		MetaStyles: styles,
	}, nil
}

// decodeYAMLOrdered parses a YAML document into a map, an ordered key
// slice, and per-key value styles so adapters can re-emit the
// frontmatter in source order with the author's original scalar style
// (plain vs. double-quoted, etc.). Empty input returns empty map and
// nil keys/styles.
func decodeYAMLOrdered(data []byte) (map[string]any, []string, map[string]yaml.Style, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, nil, nil, err
	}
	meta, keys, styles := nodeToOrderedMap(&node)
	if meta == nil {
		meta = map[string]any{}
	}
	return meta, keys, styles, nil
}

// nodeToOrderedMap converts a YAML mapping node to (map, ordered keys,
// per-key value styles). Non-mapping inputs return (nil, nil, nil); the
// DocumentNode wrapper is peeled when present. Errors decoding
// individual values cause the affected key to be dropped silently —
// yaml.Unmarshal would have already surfaced the syntactic failure at
// parse time. The styles map only carries entries for scalar values
// whose Style was non-default; missing keys mean "let the encoder
// choose" at re-emit time.
func nodeToOrderedMap(n *yaml.Node) (map[string]any, []string, map[string]yaml.Style) {
	if n == nil {
		return nil, nil, nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil, nil, nil
		}
		return nodeToOrderedMap(n.Content[0])
	}
	if n.Kind != yaml.MappingNode {
		return nil, nil, nil
	}
	meta := make(map[string]any, len(n.Content)/2)
	keys := make([]string, 0, len(n.Content)/2)
	var styles map[string]yaml.Style
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
		if valNode.Kind == yaml.ScalarNode && valNode.Style != 0 {
			if styles == nil {
				styles = make(map[string]yaml.Style)
			}
			styles[keyNode.Value] = valNode.Style
		}
	}
	return meta, keys, styles
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
func splitFrontmatter(data []byte) (map[string]any, []string, map[string]yaml.Style, string, error) {
	const delim = "---"
	empty := map[string]any{}

	if !bytes.HasPrefix(data, []byte(delim)) {
		return empty, nil, nil, string(data), nil
	}
	rest := data[len(delim):]
	idx := bytes.Index(rest, []byte("\n"+delim))
	if idx < 0 {
		return empty, nil, nil, string(data), nil
	}
	yamlPart := rest[:idx]
	body := bytes.TrimLeft(rest[idx+len("\n"+delim):], "\n")

	meta, keys, styles, err := decodeYAMLOrdered(yamlPart)
	if err != nil {
		return nil, nil, nil, "", err
	}
	return meta, keys, styles, string(body), nil
}
