package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Codex stores subagents and skills under `.agents/`. Older layouts used
// `.codex/agents/` for TOMLs; both are scanned so an upgrade picks up
// either location.
var codexAgentDirs = []string{".agents/agents", ".codex/agents"}

const (
	codexDir        = ".codex"
	codexConfigTOML = ".codex/config.toml"
)

// codexSkillsDirs lists every per-skill folder the importer scans.
// `.codex/skills/` is Codex CLI's native lookup path; `.agents/skills/`
// is the older community shared layout. Both are scanned so an upgrade
// picks up either location.
var codexSkillsDirs = []string{".codex/skills", ".agents/skills"}

// importCodexAgents reads every `<root>/<dir>/*.toml` agent file (where
// <dir> is one of `.agents/agents/` or `.codex/agents/`) and writes one
// `.md` per agent to dstDir. Frontmatter captures `name`, `description`,
// `model`, and codex-specific `x-codex.*` keys; the body comes from
// `developer_instructions`.
func importCodexAgents(root, dstDir string) (int, error) {
	count := 0
	seen := map[string]bool{}
	claudePresent := claudeTreeExists(root)
	for _, sub := range codexAgentDirs {
		dir := filepath.Join(root, sub)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			doc, err := readCodexAgentTOML(path)
			if err != nil {
				return count, err
			}
			tomlName := strings.TrimSuffix(e.Name(), ".toml")
			if n, _ := doc["name"].(string); n != "" {
				tomlName = n
			}
			// Canonicalise filename to dash-case so claude-imported
			// `changelog-keeper.md` and codex-imported
			// `changelog_keeper` resolve to the same spec on disk.
			// The original underscore form is preserved verbatim in
			// the frontmatter's `name:` field so codex emit keeps the
			// TOML `name = "changelog_keeper"` value Codex expects.
			canonical := canonicalSpecSlug(tomlName)
			if seen[canonical] {
				continue
			}
			seen[canonical] = true

			scope := ""
			if claudePresent && !claudeHasAgent(root, canonical) {
				scope = "codex"
			}
			wrote, err := mergeOrWriteCodexAgentSpec(dstDir, canonical, tomlName, doc, scope)
			if err != nil {
				return count, err
			}
			if wrote {
				count++
			}
		}
	}
	return count, nil
}

// canonicalSpecSlug returns name with `_` rewritten to `-` and the
// result lowercased. Used to dedupe filenames between tools that pick
// different separator conventions (claude uses dashes, codex uses
// underscores).
func canonicalSpecSlug(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func readCodexAgentTOML(path string) (map[string]any, error) {
	doc := map[string]any{}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

// codexAgentTopLevel are the keys the codex emitter writes at the TOML
// root that map to frontmatter top-level slots on round-trip. Everything
// else gets carried under `x-codex` so unknown / future Codex keys pass
// through without data loss.
var codexAgentTopLevel = map[string]bool{
	"name":                   true,
	"description":            true,
	"model":                  true,
	"developer_instructions": true,
}

// mergeOrWriteCodexAgentSpec writes a codex agent spec at
// `<dstDir>/<canonical>.md`. When a claude-imported file already lives
// at that path the codex-specific keys (`x-codex.*` and any missing
// top-level fields) are layered into it without overwriting the body or
// the existing `name:` slug. Returns (wrote, err) where `wrote` is true
// for both fresh writes and merges so the caller still counts the spec.
//
// Filename canonicalisation collapses claude's dashed convention and
// codex's underscored convention onto the same on-disk file so a
// project that imports both tools no longer carries duplicate specs
// (changelog-keeper.md + changelog_keeper.md). When the codex `name`
// differs from the canonical slug it lands under `x-codex.name` so the
// codex emitter still produces TOML with the runtime-expected
// underscored identifier.
func mergeOrWriteCodexAgentSpec(dstDir, canonical, codexName string, doc map[string]any, scope string) (bool, error) {
	path := filepath.Join(dstDir, canonical+".md")
	existing, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, writeCodexAgentSpec(path, canonical, codexName, doc, scope)
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	merged, err := mergeCodexAgentIntoExisting(string(existing), codexName, doc)
	if err != nil {
		return false, err
	}
	if err := importWriteFile(path, []byte(merged), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// writeCodexAgentSpec renders a codex agent TOML as a fresh
// agnostic-ai spec under path. Known top-level keys map directly into
// the frontmatter; every other key lands under `x-codex`. `name:` uses
// the canonical (dash) slug so the on-disk filename and frontmatter
// stay aligned; when the codex runtime name differs it is preserved
// under `x-codex.name`.
func writeCodexAgentSpec(path, canonical, codexName string, doc map[string]any, scope string) error {
	body, _ := doc["developer_instructions"].(string)
	body = strings.TrimRight(body, "\n")

	fm := map[string]any{"name": canonical}
	if d, _ := doc["description"].(string); d != "" {
		fm["description"] = d
	}
	if m, _ := doc["model"].(string); m != "" {
		fm["model"] = m
	}
	if scope != "" {
		fm["target"] = scope
	}
	xcodex := map[string]any{}
	for key, val := range doc {
		if codexAgentTopLevel[key] {
			continue
		}
		xcodex[key] = val
	}
	if codexName != "" && codexName != canonical {
		xcodex["name"] = codexName
	}
	if len(xcodex) > 0 {
		fm["x-codex"] = xcodex
	}

	raw, err := marshalAgentFrontmatter(fm)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", canonical, err)
	}
	out := "---\n" + string(raw) + "---\n\n" + body + "\n"

	if err := importWriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// mergeCodexAgentIntoExisting parses an already-imported agent spec
// (claude origin) and layers codex-specific frontmatter on top without
// disturbing the body or existing top-level keys. Top-level keys with
// divergent values across the two tools land under `x-codex.<key>` so
// each target emit reproduces its source-of-truth value (#304). Claude's
// richer body survives via mergeAgentBody.
func mergeCodexAgentIntoExisting(existing, codexName string, doc map[string]any) (string, error) {
	front, body, ok := splitCodexAgentFrontmatter(existing)
	if !ok {
		return existing, nil
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return "", fmt.Errorf("parse existing frontmatter: %w", err)
	}
	if _, has := fm["description"]; !has {
		if d, _ := doc["description"].(string); d != "" {
			fm["description"] = d
		}
	}
	if _, has := fm["model"]; !has {
		if m, _ := doc["model"].(string); m != "" {
			fm["model"] = m
		}
	}
	xcodex, _ := fm["x-codex"].(map[string]any)
	if xcodex == nil {
		xcodex = map[string]any{}
	}
	// Top-level frontmatter keys that codex also emits at the TOML root:
	// when the codex value diverges (or is absent while claude has one),
	// record the codex view under `x-codex.<key>` so ResolveMeta(codex)
	// reproduces the codex source-of-truth.
	for _, key := range divergentAgentTopLevelKeys {
		mergeDivergentMetaKey(fm, xcodex, doc, key)
	}
	for key, val := range doc {
		if codexAgentTopLevel[key] {
			continue
		}
		if _, present := xcodex[key]; !present {
			xcodex[key] = val
		}
	}
	if codexName != "" {
		if canonical, _ := fm["name"].(string); codexName != canonical {
			if _, present := xcodex["name"]; !present {
				xcodex["name"] = codexName
			}
		}
	}
	if len(xcodex) > 0 {
		fm["x-codex"] = xcodex
	}
	raw, err := marshalAgentFrontmatter(fm)
	if err != nil {
		return "", fmt.Errorf("re-marshal frontmatter: %w", err)
	}
	codexBody, _ := doc["developer_instructions"].(string)
	mergedBody := mergeAgentBody(body, codexBody)
	if !strings.HasSuffix(mergedBody, "\n") {
		mergedBody += "\n"
	}
	return "---\n" + string(raw) + "---\n\n" + mergedBody, nil
}

// divergentAgentTopLevelKeys are the agent-frontmatter keys whose values
// frequently differ between claude and codex (description, model). Each
// gets compared during the codex merge: if codex's value disagrees with
// claude's, the codex view goes under `x-codex.<key>`; if claude has the
// key but codex does not, `x-codex.<key>: null` marks the deletion so
// ResolveMeta(codex) drops it.
var divergentAgentTopLevelKeys = []string{"description", "model"}

// mergeDivergentMetaKey records a per-target override for key when the
// codex doc value differs from the claude frontmatter value. Used by
// the codex merger to keep both tools' divergent frontmatter intact.
//
//   - codex absent, claude present: x-codex[key] = nil (deletion marker)
//   - codex present, claude absent: claude takes the codex value (no
//     divergence to record — it's just the only source).
//   - codex present and != claude: x-codex[key] = codex value.
//   - equal values: nothing to record.
func mergeDivergentMetaKey(fm, xcodex, doc map[string]any, key string) {
	claudeVal, hasClaude := fm[key]
	docVal, hasDoc := doc[key]
	if hasDoc {
		if s, ok := docVal.(string); ok && s == "" {
			hasDoc = false
		}
	}
	if !hasDoc && hasClaude {
		// Mark the key as deleted-for-codex so emit drops it.
		if _, present := xcodex[key]; !present {
			xcodex[key] = nil
		}
		return
	}
	if !hasDoc {
		return
	}
	if !hasClaude {
		fm[key] = docVal
		return
	}
	if !equalScalar(claudeVal, docVal) {
		if _, present := xcodex[key]; !present {
			xcodex[key] = docVal
		}
	}
}

// equalScalar compares two scalar `any` values without bringing in
// reflect.DeepEqual for the hot import path. Handles the string / bool
// / numeric shapes the codex merger walks.
func equalScalar(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	case int64:
		bv, ok := b.(int64)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	}
	return false
}

// agentFrontmatterOrder is the canonical claude-conventional key order
// for an imported agent spec frontmatter. Keys missing from this list
// land between the known scalars and `x-codex`, sorted alphabetically
// for stability; `x-codex` always trails so the tool-specific subtree
// stays out of the eye-line when reading the file.
var agentFrontmatterOrder = []string{
	"name",
	"description",
	"model",
	"target",
	"allowed_tools",
	"argument-hint",
	"disable-model-invocation",
}

// marshalAgentFrontmatter renders fm as YAML with claude-conventional
// key order so the captured spec matches the format Claude's own
// `.claude/agents/*.md` files use. yaml.Marshal on a map sorts
// alphabetically, which mangles round-tripped hand-authored agents on
// every sync.
func marshalAgentFrontmatter(fm map[string]any) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	seen := map[string]bool{}
	add := func(key string) error {
		val, ok := fm[key]
		if !ok {
			return nil
		}
		seen[key] = true
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
		valNode := &yaml.Node{}
		if err := valNode.Encode(val); err != nil {
			return err
		}
		root.Content = append(root.Content, keyNode, valNode)
		return nil
	}
	for _, k := range agentFrontmatterOrder {
		if err := add(k); err != nil {
			return nil, err
		}
	}
	// Append remaining keys (other than x-codex) in alphabetical order
	// so unknown fields stay deterministic.
	var rest []string
	for k := range fm {
		if seen[k] || k == "x-codex" {
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		if err := add(k); err != nil {
			return nil, err
		}
	}
	if err := add("x-codex"); err != nil {
		return nil, err
	}
	return yaml.Marshal(root)
}

// splitCodexAgentFrontmatter returns the YAML between the first two `---`
// delimiters and the markdown body following them. Returns (_, _,
// false) when the input does not open with a frontmatter block.
func splitCodexAgentFrontmatter(doc string) (string, string, bool) {
	if !strings.HasPrefix(doc, "---\n") {
		return "", "", false
	}
	rest := doc[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", false
	}
	front := rest[:end]
	body := rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	body = strings.TrimPrefix(body, "\n")
	return front, body, true
}

// importCodexSkills walks every dir in codexSkillsDirs under root and
// mirrors each `<dir>/<name>/` skill folder byte-for-byte into
// `<dstDir>/<name>/`. Every file under the skill directory — SKILL.md,
// `agents/openai.yaml`, helper scripts, fixtures, nested subdirectories
// — is preserved so an import then `sync` keeps the full skill payload
// intact across all targets. When the same skill name appears under
// both layouts the first one wins (codex-native path comes first).
func importCodexSkills(root, dstDir string) (int, error) {
	count := 0
	seen := map[string]bool{}
	claudePresent := claudeTreeExists(root)
	for _, sub := range codexSkillsDirs {
		srcDir := filepath.Join(root, sub)
		entries, err := os.ReadDir(srcDir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcDir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if seen[e.Name()] {
				continue
			}
			skillSrc := filepath.Join(srcDir, e.Name())
			if _, err := os.Stat(filepath.Join(skillSrc, "SKILL.md")); errors.Is(err, fs.ErrNotExist) {
				continue
			} else if err != nil {
				return count, fmt.Errorf("stat skill %s: %w", e.Name(), err)
			}
			seen[e.Name()] = true
			skillDst := filepath.Join(dstDir, e.Name())
			merged := dirExists(skillDst)
			if merged {
				if err := mergeCodexSkillIntoExisting(skillSrc, skillDst); err != nil {
					return count, fmt.Errorf("merge skill %s: %w", e.Name(), err)
				}
			} else if err := copyDirTree(skillSrc, skillDst); err != nil {
				return count, fmt.Errorf("copy skill %s: %w", e.Name(), err)
			}
			// Auto-scope `target: codex` only when this is a fresh
			// import (no claude sibling on disk and no claude-merged
			// SKILL.md already at the destination).
			if !merged && claudePresent && !claudeHasSkill(root, e.Name()) {
				if err := injectTargetInSkillMD(filepath.Join(skillDst, "SKILL.md"), "codex"); err != nil {
					return count, fmt.Errorf("scope skill %s: %w", e.Name(), err)
				}
			}
			count++
		}
	}
	return count, nil
}

// mergeCodexSkillIntoExisting layers a codex skill folder on top of an
// already-imported skill (claude origin). Claude frontmatter survives
// (argument-hint, disable-model-invocation, allowed-tools — Claude
// reads these to wire the skill correctly). When the codex SKILL.md
// body diverges from claude's the unique sections get wrapped in
// `::target` fences so both tools' authored prose survives the
// round-trip (#300). Codex-only assets (agents/openai.yaml, scripts/,
// helper files) copy across.
//
// Codex-only top-level entries (anything in `src` not already present
// in `dst`) get recorded in the merged SKILL.md frontmatter under
// `x-codex.assets` so the claude adapter knows to skip them on emit
// (#305).
func mergeCodexSkillIntoExisting(src, dst string) error {
	codexOnlyTopLevel, err := codexOnlyTopLevelEntries(src, dst)
	if err != nil {
		return err
	}
	if err := mergeSkillBodies(filepath.Join(dst, "SKILL.md"), filepath.Join(src, "SKILL.md")); err != nil {
		return err
	}
	if len(codexOnlyTopLevel) > 0 {
		if err := recordCodexSkillAssets(filepath.Join(dst, "SKILL.md"), codexOnlyTopLevel); err != nil {
			return err
		}
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == "SKILL.md" {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		if _, err := os.Stat(target); err == nil {
			// File already imported from claude; keep it.
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
}

// codexOnlyTopLevelEntries lists the top-level names (excluding
// SKILL.md) that exist under src but not under dst. The claude side
// (dst) was imported first, so anything codex adds top-level is by
// definition codex-only.
func codexOnlyTopLevelEntries(src, dst string) ([]string, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", src, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if name == "SKILL.md" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dst, name)); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("stat %s: %w", filepath.Join(dst, name), err)
		}
		out = append(out, name)
	}
	return out, nil
}

// recordCodexSkillAssets appends or merges the given names into
// `x-codex.assets` inside the SKILL.md frontmatter at skillPath. The
// claude adapter consults this list at emit time to skip codex-only
// subtrees instead of leaking them into `.claude/skills/<name>/`.
func recordCodexSkillAssets(skillPath string, names []string) error {
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", skillPath, err)
	}
	front, body, ok := splitCodexAgentFrontmatter(string(data))
	if !ok {
		return nil
	}
	fm := map[string]any{}
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return fmt.Errorf("parse frontmatter: %w", err)
	}
	xcodex, _ := fm["x-codex"].(map[string]any)
	if xcodex == nil {
		xcodex = map[string]any{}
	}
	seen := map[string]bool{}
	var merged []string
	if existing, ok := xcodex["assets"].([]any); ok {
		for _, v := range existing {
			if s, ok := v.(string); ok && !seen[s] {
				seen[s] = true
				merged = append(merged, s)
			}
		}
	}
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			merged = append(merged, n)
		}
	}
	xcodex["assets"] = merged
	fm["x-codex"] = xcodex
	raw, err := marshalAgentFrontmatter(fm)
	if err != nil {
		return fmt.Errorf("re-marshal frontmatter: %w", err)
	}
	out := "---\n" + string(raw) + "---\n\n" + strings.TrimLeft(body, "\n")
	return importWriteFile(skillPath, []byte(out), 0o644)
}

// codexConfigDoc mirrors the relevant `.codex/config.toml` shape: nested
// `[[hooks.<event>]]` arrays and `[mcp_servers.<name>]` tables.
type codexConfigDoc struct {
	Hooks      map[string][]codexHookEntry `toml:"hooks"`
	MCPServers map[string]codexMCPEntry    `toml:"mcp_servers"`
}

type codexHookEntry struct {
	Matcher       string `toml:"matcher"`
	Command       string `toml:"command"`
	Timeout       int    `toml:"timeout"`
	StatusMessage string `toml:"statusMessage"`
}

type codexMCPEntry struct {
	Command           string            `toml:"command"`
	Args              []string          `toml:"args"`
	Env               map[string]string `toml:"env"`
	URL               string            `toml:"url"`
	BearerTokenEnvVar string            `toml:"bearer_token_env_var"`
	HTTPHeaders       map[string]string `toml:"http_headers"`
}

// importCodexConfig reads `<root>/.codex/config.toml` plus the
// standalone `<root>/.codex/hooks.json` (if present) and writes one
// yaml per discovered hook to hooksDst and one yaml per `[mcp_servers.<name>]`
// table to mcpsDst. Hooks declared in both files are deduped by
// (event, matcher, command); the hooks.json variant wins because it
// can carry timeout + statusMessage.
//
// Returns (hooks, mcps).
func importCodexConfig(root, hooksDst, mcpsDst string) (int, int, error) {
	hooksByKey, mcpServers, err := readCodexConfigTOML(root)
	if err != nil {
		return 0, 0, err
	}
	if err := mergeCodexHooksJSON(root, hooksByKey); err != nil {
		return 0, 0, err
	}

	hooks, err := writeCodexHooksFromMap(hooksByKey, hooksDst)
	if err != nil {
		return hooks, 0, err
	}
	mcps, err := writeCodexMCPs(mcpServers, mcpsDst)
	if err != nil {
		return hooks, mcps, err
	}
	return hooks, mcps, nil
}

// codexHookKey identifies a hook for dedupe across sources. Hooks with
// the same (event, matcher, command) are considered the same entry.
type codexHookKey struct {
	event, matcher, command string
}

// codexHookSlot is the merged hook representation built up by reading
// every codex hook source. The order field preserves discovery order
// so emitted spec files stay byte-stable across re-imports.
type codexHookSlot struct {
	order         int
	entry         codexHookEntry
	event         string
	fromHooksJSON bool
}

// readCodexConfigTOML reads `.codex/config.toml` and returns a
// dedupe-keyed map of hooks plus the MCP servers section. The dedupe key
// uses (event, matcher, command) so the standalone hooks.json layer can
// overwrite TOML-defined entries that carry less information.
func readCodexConfigTOML(root string) (map[codexHookKey]*codexHookSlot, map[string]codexMCPEntry, error) {
	hooks := map[codexHookKey]*codexHookSlot{}
	path := filepath.Join(root, codexConfigTOML)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return hooks, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc codexConfigDoc
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, event := range sortedMapKeys(doc.Hooks) {
		for _, h := range doc.Hooks[event] {
			if h.Command == "" {
				continue
			}
			k := codexHookKey{event: event, matcher: h.Matcher, command: h.Command}
			if _, exists := hooks[k]; exists {
				continue
			}
			hooks[k] = &codexHookSlot{order: len(hooks), entry: h, event: event}
		}
	}
	return hooks, doc.MCPServers, nil
}

// mergeCodexHooksJSON layers `.codex/hooks.json` over the config.toml
// dedupe map. When the same (event, matcher, command) appears in both,
// the JSON entry wins so timeout + statusMessage propagate even when
// the TOML copy carried only matcher + command.
func mergeCodexHooksJSON(root string, hooks map[codexHookKey]*codexHookSlot) error {
	path := filepath.Join(root, codexDir, "hooks.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var s claudeSettings // identical shape: hooks[<event>][n].{matcher, hooks[m].{...}}
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for _, event := range sortedMapKeys(s.Hooks) {
		for _, g := range s.Hooks[event] {
			for _, h := range g.Hooks {
				if h.Command == "" {
					continue
				}
				k := codexHookKey{event: event, matcher: g.Matcher, command: h.Command}
				slot, exists := hooks[k]
				entry := codexHookEntry{
					Matcher:       g.Matcher,
					Command:       h.Command,
					Timeout:       h.Timeout,
					StatusMessage: h.StatusMessage,
				}
				if !exists {
					hooks[k] = &codexHookSlot{
						order:         len(hooks),
						entry:         entry,
						event:         event,
						fromHooksJSON: true,
					}
					continue
				}
				slot.entry = entry
				slot.fromHooksJSON = true
			}
		}
	}
	return nil
}

// writeCodexHooksFromMap emits one yaml per discovered hook. Hooks are
// written in (event, order) order so a re-import produces byte-stable
// output regardless of which source file changed.
func writeCodexHooksFromMap(hooks map[codexHookKey]*codexHookSlot, dstDir string) (int, error) {
	slots := make([]*codexHookSlot, 0, len(hooks))
	for _, slot := range hooks {
		slots = append(slots, slot)
	}
	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].event != slots[j].event {
			return slots[i].event < slots[j].event
		}
		return slots[i].order < slots[j].order
	})

	count := 0
	for _, slot := range slots {
		h := slot.entry
		name := hookSpecName(slot.event, h.Matcher, []string{h.Command})
		doc := map[string]any{
			"name":    name,
			"event":   slot.event,
			"command": h.Command,
			"target":  "codex",
		}
		if h.Matcher != "" {
			doc["matcher"] = h.Matcher
		}
		if h.Timeout != 0 {
			doc["timeout"] = h.Timeout
		}
		if h.StatusMessage != "" {
			doc["statusMessage"] = h.StatusMessage
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return count, fmt.Errorf("marshal hook %s: %w", name, err)
		}
		path := filepath.Join(dstDir, name+".yaml")
		if err := importWriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// sortedMapKeys returns a copy of map[string]V's keys sorted for
// deterministic iteration. Local to the codex importer to avoid
// colliding with the validate.go variant that takes a set.
func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func writeCodexMCPs(servers map[string]codexMCPEntry, dstDir string) (int, error) {
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)

	count := 0
	for _, name := range names {
		s := servers[name]
		doc := map[string]any{"name": name}
		switch {
		case s.URL != "":
			doc["type"] = "http"
			doc["url"] = s.URL
			if s.BearerTokenEnvVar != "" {
				doc["bearer_token_env_var"] = s.BearerTokenEnvVar
			}
			if len(s.HTTPHeaders) > 0 {
				doc["headers"] = s.HTTPHeaders
			}
		default:
			doc["type"] = "stdio"
			if s.Command != "" {
				doc["command"] = s.Command
			}
			if len(s.Args) > 0 {
				doc["args"] = s.Args
			}
		}
		if len(s.Env) > 0 {
			doc["env"] = s.Env
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return count, fmt.Errorf("marshal mcp %s: %w", name, err)
		}
		path := filepath.Join(dstDir, name+".yaml")
		if err := importWriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}
