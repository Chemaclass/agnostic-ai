// Package spec loads and parses agnostic-ai source specs from disk.
//
// Specs come in ten kinds (agent, skill, rule, hook, mcp, command,
// settings, review, environment, ignore). Markdown specs (agent, skill,
// rule, command, review, ignore) use YAML frontmatter; hook, mcp,
// settings, and environment specs are pure YAML.
package spec

import (
	"io/fs"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
	// CapabilityField preserves the source name after tool normalization.
	CapabilityField string
	Kind            Kind
	Name            string
	Path            string
	// Scope is the relative directory under the source kind directory in
	// which the spec lives, with forward slashes. A spec at
	// `rules/backend/auth.md` has Scope "backend"; a spec at the root of
	// `rules/` has Scope "". A rule takes no Scope from a folder when its
	// frontmatter sets `scope` or the folder names no project directory.
	// Native file selectors keep the folder for placement without adding activation.
	Scope string
	// Keep the loader's folder for placement after local metadata merges.
	ruleFolder string
	// globsFromPaths marks `globs` read from a rule's `paths` alias.
	globsFromPaths bool
	// Folder is a rule's subfolder under `rules/` when that folder names a
	// project directory, whether or not Scope took it.
	Folder string
	// Layer names the source layer this entry came from
	// ("pack:<name>", "project", "project-user"). Empty when loaded
	// outside the layered loader (legacy path).
	Layer     string
	bodyLayer string
	// Base is the lower-layer spec a local layer entry extends, and
	// LocalMeta the keys the local file itself writes. Both are nil
	// unless the entry extends one.
	Base      *Entry
	LocalMeta map[string]any
	Meta      map[string]any
	// MetaKeys preserves the order in which keys appeared in the source
	// frontmatter (or pure YAML for hooks/MCPs). Adapters pass this to
	// emit.DocumentOrdered / emit.FrontmatterOrdered so a round-trip
	// through agnostic-ai keeps the author's key order. Nil for entries
	// built programmatically (e.g. WASM playground inputs without a
	// preserved key order); emit falls back to alphabetical order in
	// that case.
	MetaKeys []string
	// OpeningBlockKept is set when a closed leading `---` block was not
	// frontmatter and stays in Body.
	OpeningBlockKept bool
	// DuplicateKeys lists the top-level keys the source writes more than
	// once, which Meta holds once with the last value. Other YAML parsers
	// reject such a file, so lint reports it. A local layer that extends
	// the entry replaces the list with its own.
	DuplicateKeys []string
	// NestedKeys records the key order of each object below the top
	// level of a pure YAML spec, by key path. Read it through KeyOrder.
	NestedKeys map[string][]string
	// MetaStyles records the YAML scalar style of each top-level
	// frontmatter value as it appeared in source. Adapters pass this to
	// emit.FrontmatterStyled / emit.DocumentStyled so a value that the
	// author hand-quoted with double quotes stays double-quoted on
	// re-emit, while a plain scalar stays plain. Nil for entries built
	// programmatically (e.g. WASM playground) and for keys whose source
	// style was the YAML default (PlainStyle, value 0).
	MetaStyles map[string]yaml.Style
	// Literals holds, by top-level field, the keys whose value a YAML
	// spec tags LiteralTag. Read it through MarkedLiteral.
	Literals map[string]map[string]bool
	Body     string
	// BodyLine is the 1-based line of Path where Body starts, or 0 once
	// Body no longer maps line for line onto the file (an include,
	// ::parent, or ::target fence rewrote it).
	BodyLine int
	// AssetDir is the skill folder whose assets ship when it is not the
	// one holding Path: a local skill that only edits fields keeps the
	// shared folder's assets. Read it through SkillAssetDir.
	AssetDir string
	// Assets lists the regular files under SkillAssetDir as the load
	// found them, so a skill emitted to many targets walks its folder
	// once. Nil means a skill built without a listing, which walks it.
	Assets *[]AssetFile
	// ModelTier names the `models:` tier the spec's `model` resolved
	// through, or "" for a literal model. See Bundle.ApplyModelTiers.
	ModelTier string
	// PortableOn is the `on:` a hook was translated from by NativeHook,
	// or "" for a hook written in the native form.
	HookAssetDirs map[string]string
	PortableOn    string
	// PortableMatch is the `match:` it was translated from, and
	// PortableDecision whether it set `decision: stdout`.
	PortableMatch    string
	PortableDecision bool
}

// AssetFile is one file in a skill folder: its path relative to the
// folder, with forward slashes, and its mode.
type AssetFile struct {
	Rel  string
	Mode fs.FileMode
}

func (e Entry) BodySourceLayer() string {
	if e.bodyLayer != "" {
		return e.bodyLayer
	}
	return e.Layer
}

// SkillAssetDir returns the folder whose sibling files ship with a skill,
// or "" for a flat-file or in-memory skill.
func (e Entry) SkillAssetDir() string {
	if e.AssetDir != "" {
		return e.AssetDir
	}
	if e.Path != "" && filepath.Base(e.Path) == "SKILL.md" {
		return filepath.Dir(e.Path)
	}
	return ""
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
	return FilterFences(e.Body, []string{target})
}

// EffectiveScope returns the routing prefix for the entry: the layout
// Scope, or the frontmatter `scope: <relpath>`, which the loader keeps
// ahead of a rule's folder. An empty result means root.
func (e Entry) EffectiveScope() string {
	if e.Scope != "" {
		return e.Scope
	}
	if s, ok := e.Meta["scope"].(string); ok && s != "" {
		return strings.Trim(filepath.ToSlash(s), "/")
	}
	return ""
}
