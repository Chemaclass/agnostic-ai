// Package claude emits Claude Code configs.
//
// Claude Code natively supports all seven spec kinds:
//   - agents   -> <dir>/agents/<name>.md
//   - skills   -> <dir>/skills/<name>/SKILL.md
//   - rules    -> <dir>/rules/<name>.md (one file per rule)
//   - hooks    -> <dir>/settings.json
//   - mcps     -> .mcp.json
//   - commands -> <dir>/commands/<name>.md (slash commands)
//   - settings -> <dir>/settings.json (permissions, model)
//
// Rules emit one file per spec under `.claude/rules/` so a hand-authored
// CLAUDE.md is never clobbered. Claude Code does not auto-load that
// directory, so by default the files are inert. Set
// `outputs.claude.rules-mode: import` to append a sentinel-marked block of
// `@.claude/rules/<name>.md` imports to the CLAUDE.md pointer body, keeping
// the pointer body intact while wiring the rules in. Set
// `outputs.claude.rules-file: CLAUDE.md` to fall back to the legacy
// concatenated single-file layout instead.
package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target             = "claude"
	defaultDir         = ".claude"
	defaultRulesDir    = ".claude/rules"
	defaultCommandsDir = ".claude/commands"
	defaultMCPFile     = ".mcp.json"
	// settingsOverlayPath is the project-relative path to the captured
	// non-hooks portion of `.claude/settings.json`. `agnostic-ai import
	// claude` writes this file; the emitter loads it and layers the
	// spec-derived `hooks` key on top. The overlay survives a wipe of
	// `.claude/`, so a re-sync from a fresh checkout still produces a
	// settings.json with statusLine, enabledPlugins, and any other keys
	// the user had configured.
	settingsOverlayPath = ".agnostic-ai/overlays/claude.settings.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP, spec.KindCommand, spec.KindSettings},
}

// Adapter emits Claude Code configs.
type Adapter struct{}

// New returns a Claude adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes the agents, skills, rules, and hook settings.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	dir := emit.OutputDir(cfg, target, defaultDir)

	for _, a := range b.Agents {
		path := filepath.Join(dir, "agents", a.Name+".md")
		body := emit.WithHeader(emit.DocumentStyled(a.Meta, a.MetaKeys, a.MetaStyles, a.Body, target), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}

	for _, s := range b.Skills {
		folder := filepath.Join(dir, "skills", s.Name)
		path := filepath.Join(folder, "SKILL.md")
		body := emit.WithHeader(emit.DocumentStyled(s.Meta, s.MetaKeys, s.MetaStyles, s.Body, target), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
		if err := emit.PropagateSkillAssets(s, folder, claudeSkillSkipFor(s), dryRun); err != nil {
			return err
		}
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	for _, c := range b.Commands {
		path := filepath.Join(commandsDir, c.Name+".md")
		body := emit.WithHeader(emit.DocumentStyled(c.Meta, c.MetaKeys, c.MetaStyles, c.Body, target), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}

	if err := writeRules(b.Rules, cfg, dryRun); err != nil {
		return err
	}

	hooks := b.HooksFor(target)
	if err := writeSettings(hooks, b.Settings, dir, cfg, dryRun); err != nil {
		return err
	}

	if err := materializeHookScripts(hooks, dryRun); err != nil {
		return err
	}

	if err := emit.RestoreHelperFiles(target, dryRun); err != nil {
		return err
	}

	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// materializeHookScripts copies each hook's stashed script body from
// `.agnostic-ai/scripts/` into `.<target>/hooks/`. The lookup keys off
// the spec's original `command:` path so a script imported via claude
// still materializes when the same hook syncs out to claude.
//
// Hooks whose command field is a free-form shell expression carry no
// stashed body and skip silently — there is nothing to copy.
func materializeHookScripts(hooks []spec.Entry, dryRun bool) error {
	for _, h := range hooks {
		cmds := hookCommands(h.Meta["command"])
		for _, raw := range cmds {
			sourceTool, _ := emit.SourceToolFromHookCommand(raw)
			rewritten := emit.RewriteHookPath(raw, target)
			if err := emit.MaterializeHookScript(rewritten, target, sourceTool, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}

// claudeSkillSkipFor returns a per-skill skip predicate honoring the
// hardcoded codex-only entries (`SKILL.md`, `agents/`) plus any top-
// level paths the importer recorded as codex-only under
// `x-codex.assets`. Callers pass the predicate to emit.CopyTree.
func claudeSkillSkipFor(s spec.Entry) func(string) bool {
	codexOnly := codexOnlyAssets(s.Meta)
	return func(rel string) bool {
		if isClaudeSkillSkippedAsset(rel) {
			return true
		}
		for _, top := range codexOnly {
			if rel == top || strings.HasPrefix(rel, top+"/") {
				return true
			}
		}
		return false
	}
}

// codexOnlyAssets reads the list under `meta["x-codex"]["assets"]` and
// returns each entry as a string. Returns nil for the common case (no
// codex-only assets recorded). Accepts both `[]any` (yaml.v3 decode)
// and `[]string` (round-tripped through structs).
func codexOnlyAssets(meta map[string]any) []string {
	x, ok := meta["x-codex"].(map[string]any)
	if !ok {
		return nil
	}
	switch xs := x["assets"].(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, v := range xs {
			if s, ok := v.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// isClaudeSkillSkippedAsset reports whether the given skill-relative path
// should be omitted from `.claude/skills/<name>/`. SKILL.md gets re-rendered
// from the spec, and `agents/openai.yaml` (plus the entire `agents/`
// subtree it lives in) is a codex-only artifact Claude does not read.
func isClaudeSkillSkippedAsset(rel string) bool {
	if rel == "SKILL.md" {
		return true
	}
	if rel == "agents" || strings.HasPrefix(rel, "agents/") {
		return true
	}
	return false
}

// writeSettings renders `.claude/settings.json` by layering, in order
// of increasing precedence:
//
//  1. Overlay captured during `import claude` (covers anything the user
//     keeps inside `.claude/settings.json` directly). When the overlay
//     is absent, the existing `.claude/settings.json` on disk is used as
//     the base instead so user-edited keys survive until `import claude`
//     captures them.
//  2. Agnostic `settings` specs (`.agnostic-ai/settings/*.yaml`), the
//     cross-tool source for permissions and default model.
//  3. First-class fields from `outputs.claude.settings` in
//     `agnostic-ai.yaml` (statusLine, permissions, env, model, etc).
//     Being claude-specific, these override the generic spec.
//  4. Spec-derived `hooks` block, emitted via ordered JSON so
//     `{type, command}` and `{matcher, hooks}` stay in lifecycle order
//     instead of alpha-sorted map order.
//
// Short-circuit: all layers empty -> write nothing.
func writeSettings(hooks, settings []spec.Entry, dir string, cfg *config.Config, dryRun bool) error {
	path := filepath.Join(dir, "settings.json")
	overlay, overlayOK, err := loadSettingsOverlay(dryRun)
	if err != nil {
		return err
	}
	specSettings := buildSpecSettings(settings)
	configSettings := buildConfigSettings(cfg)
	hasSpec := len(specSettings) > 0
	hasConfig := len(configSettings) > 0
	hasHooks := len(hooks) > 0
	if !overlayOK && !hasHooks && !hasConfig && !hasSpec {
		return nil
	}
	doc := overlay
	if doc == nil {
		// No overlay yet: fall back to the existing settings.json on
		// disk so a user-edited statusLine survives until `import
		// claude` captures it. Skipped only in dryRun; capture reads it
		// so `sync --check` and `doctor --fix` keep the user's keys.
		doc, err = loadSettingsFromDisk(path, dryRun)
		if err != nil {
			return err
		}
		if doc == nil {
			doc = emit.NewOrderedJSON()
		}
	}
	// `permissions` is a nested object whose allow/deny/ask lists are
	// additive security rules. A wholesale key replace would drop rules a
	// lower layer authored (e.g. config setting only `deny` would erase a
	// spec `allow`). Union them across overlay (base), spec, then config so
	// no layer silently loses another's rules. Scalars keep last-wins.
	mergedPerms := mergePermissions(docPermissions(doc), mapOf(specSettings["permissions"]), mapOf(configSettings["permissions"]))
	delete(specSettings, "permissions")
	delete(configSettings, "permissions")
	if len(mergedPerms) > 0 {
		specSettings["permissions"] = mergedPerms
	}
	for _, k := range orderedConfigKeys(specSettings) {
		if err := doc.Set(k, specSettings[k]); err != nil {
			return fmt.Errorf("claude settings: marshal %s: %w", k, err)
		}
	}
	for _, k := range orderedConfigKeys(configSettings) {
		if err := doc.Set(k, configSettings[k]); err != nil {
			return fmt.Errorf("claude settings: marshal %s: %w", k, err)
		}
	}
	if hasHooks {
		preferred := loadCapturedHookEventOrder()
		if err := doc.Set("hooks", hookSettingsJSONWithOrder(hooks, preferred)); err != nil {
			return fmt.Errorf("claude settings: marshal hooks: %w", err)
		}
	} else {
		doc.Delete("hooks")
	}
	indent := detectSettingsIndent(path)
	raw, err := emit.MarshalJSONIndentWith(doc, indent)
	if err != nil {
		return err
	}
	return emit.WriteFile(path, string(raw)+"\n", dryRun)
}

// detectSettingsIndent sniffs the indent style of the overlay (preferred,
// since it captures the author's original byte-for-byte) and falls back
// to the existing on-disk settings.json. Returns "" when neither file
// has a discoverable indent, in which case MarshalJSONIndentWith uses
// its 2-space default. Lets a user with a 4-space settings.json keep
// 4-space indent across a round-trip.
func detectSettingsIndent(settingsPath string) string {
	candidates := []string{
		filepath.Join(".agnostic-ai", "overlays", "claude.settings.json"),
		settingsPath,
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if indent := emit.DetectJSONIndent(data); indent != "" {
			return indent
		}
	}
	return ""
}

// loadCapturedHookEventOrder reads the sidecar file
// `.agnostic-ai/overlays/claude.settings.hook-events.json` written by
// import. Returns nil when the file is absent or malformed; the caller
// then falls back to canonical lifecycle order.
func loadCapturedHookEventOrder() []string {
	data, err := os.ReadFile(filepath.Join(".agnostic-ai", "overlays", "claude.settings.hook-events.json"))
	if err != nil {
		return nil
	}
	var events []string
	if err := json.Unmarshal(data, &events); err != nil {
		return nil
	}
	return events
}

// loadSettingsFromDisk reads an existing `.claude/settings.json` as the
// base when no captured overlay is available. Returns (nil, nil) when
// the file is absent or in dryRun. Capture mode (sync --check, doctor,
// status) still reads it: the user's keys are part input, not a pure
// output, so the captured bytes must match what sync writes. Skipping
// the read reported false drift and let `doctor --fix` delete those keys
// for users who never ran `import claude` (#465). Mirrors the overlay
// reader, which already reads during capture for the same reason (#215).
func loadSettingsFromDisk(path string, dryRun bool) (*emit.OrderedJSON, error) {
	if dryRun {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if emit.IsAbsent(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc := emit.NewOrderedJSON()
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

// orderedConfigKeys returns the keys of `configSettings` in a stable
// emit order. agnostic-ai-managed keys land in a canonical sequence so
// the diff stays predictable when none of them exist in the overlay yet.
func orderedConfigKeys(m map[string]any) []string {
	const canonical = "statusLine,permissions,enabledPlugins,env,model,outputStyle,apiKeyHelper,cleanupPeriodDays,includeCoAuthoredBy"
	out := make([]string, 0, len(m))
	seen := make(map[string]bool, len(m))
	for _, k := range strings.Split(canonical, ",") {
		if _, ok := m[k]; ok && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	for k := range m {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// loadSettingsOverlay reads the captured settings overlay from
// `.agnostic-ai/overlays/claude.settings.json`. Returns (doc, true, nil)
// when the overlay exists and parses, (nil, false, nil) when it is
// absent, and (nil, false, err) on a parse failure or unexpected read
// error. Skips disk in dryRun so `--dry-run` previews remain pure.
// Capture mode (used by `sync --check` and `doctor`) still reads the
// overlay because it is a project-source input under `.agnostic-ai/`,
// not a previously emitted output — skipping it would cause capture
// output to diverge from real sync output and trigger false drift.
func loadSettingsOverlay(dryRun bool) (*emit.OrderedJSON, bool, error) {
	if dryRun {
		return nil, false, nil
	}
	data, err := os.ReadFile(settingsOverlayPath)
	if emit.IsAbsent(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	doc := emit.NewOrderedJSON()
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, false, err
	}
	return doc, true, nil
}

// writeRules emits rules per-file under `.claude/rules/<name>.md` by
// default. Setting `outputs.claude.rules-file` switches back to the
// legacy single-file concatenated layout (typically CLAUDE.md).
func writeRules(rules []spec.Entry, cfg *config.Config, dryRun bool) error {
	if len(rules) == 0 {
		return nil
	}
	if rulesFile := emit.OutputRulesFile(cfg, target, ""); rulesFile != "" {
		var sb strings.Builder
		sb.WriteString(emit.HeaderBlock(emit.FormatMarkdown))
		for _, r := range rules {
			sb.WriteString("## " + r.Name + "\n\n" + r.Body + "\n\n")
		}
		return emit.WriteFile(rulesFile, sb.String(), dryRun)
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	for _, r := range rules {
		path := filepath.Join(rulesDir, r.EffectiveScope(), r.Name+".md")
		body := emit.WithHeader(emit.DocumentStyled(r.Meta, r.MetaKeys, r.MetaStyles, r.Body, target), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// hookSettingsJSONWithOrder returns the `"hooks"` block as ordered JSON.
// preferred is the event-key order captured at import time (sidecar
// `.agnostic-ai/overlays/claude.settings.hook-events.json`). Events in
// preferred come first in that order; the remainder follow canonical
// lifecycle order; new events not in either land at the tail in
// first-seen order.
//
// Returns nil when no hooks emit.
func hookSettingsJSONWithOrder(hooks []spec.Entry, preferred []string) *emit.OrderedJSON {
	doc := emit.NewOrderedJSON()
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]hookCommandEntry{}
	keyOrder := []matcherKey{}
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		if event == "" {
			continue
		}
		cmds := hookCommands(h.Meta["command"])
		if len(cmds) == 0 {
			continue
		}
		timeout := hookIntMeta(h.Meta, "timeout")
		statusMessage, _ := h.Meta["statusMessage"].(string)
		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, cmd := range cmds {
			byKey[k] = append(byKey[k], hookCommandEntry{
				Type:          "command",
				Command:       emit.RewriteHookPath(cmd, target),
				Timeout:       timeout,
				StatusMessage: statusMessage,
			})
		}
	}
	byEvent := map[string][]matcherGroup{}
	eventOrder := []string{}
	for _, k := range keyOrder {
		if _, seen := byEvent[k.event]; !seen {
			eventOrder = append(eventOrder, k.event)
		}
		byEvent[k.event] = append(byEvent[k.event], matcherGroup{Matcher: k.matcher, Hooks: byKey[k]})
	}
	for event, groups := range byEvent {
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Matcher < groups[j].Matcher })
		byEvent[event] = groups
	}
	for _, event := range mergePreferredAndLifecycle(preferred, eventOrder) {
		if _, ok := byEvent[event]; !ok {
			continue
		}
		_ = doc.Set(event, byEvent[event])
	}
	return doc
}

// mergePreferredAndLifecycle returns the union of preferred and seen
// in the order: preferred-list-order first (those that appear in seen),
// then canonical lifecycle order for the remainder.
func mergePreferredAndLifecycle(preferred, seen []string) []string {
	if len(preferred) == 0 {
		return orderedHookEvents(seen)
	}
	have := map[string]bool{}
	for _, e := range seen {
		have[e] = true
	}
	out := make([]string, 0, len(seen))
	emitted := map[string]bool{}
	for _, e := range preferred {
		if have[e] && !emitted[e] {
			out = append(out, e)
			emitted[e] = true
		}
	}
	// Remainder via canonical lifecycle.
	var rest []string
	for _, e := range seen {
		if !emitted[e] {
			rest = append(rest, e)
		}
	}
	out = append(out, orderedHookEvents(rest)...)
	return out
}

// hookCommandEntry mirrors the `{type, command}` JSON object Claude Code
// expects inside a matcher group's `hooks` array. Using a struct lets
// `encoding/json` emit the fields in declaration order rather than the
// alpha-sorted order map iteration would produce.
//
// `Timeout` and `StatusMessage` are optional Claude schema fields that
// propagate from the spec's `timeout` / `statusMessage` Meta keys. They
// `omitempty` so specs that don't set them produce the historic minimal
// `{type, command}` payload.
type hookCommandEntry struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

// matcherGroup mirrors the `{matcher, hooks}` JSON object in
// settings.json's hook event arrays. Same rationale as
// hookCommandEntry: ordered struct fields beat sorted map keys.
type matcherGroup struct {
	Matcher string             `json:"matcher"`
	Hooks   []hookCommandEntry `json:"hooks"`
}

// hookEventLifecycleOrder names the canonical sequence Claude Code
// documentation uses when listing hook events. Events that appear in
// the user's spec but are not in this list fall through to the tail in
// the order they were first encountered.
var hookEventLifecycleOrder = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"Notification",
	"PreCompact",
	"Stop",
	"SubagentStop",
	"SessionEnd",
}

func orderedHookEvents(seen []string) []string {
	have := make(map[string]bool, len(seen))
	for _, e := range seen {
		have[e] = true
	}
	out := make([]string, 0, len(seen))
	emitted := make(map[string]bool, len(seen))
	for _, e := range hookEventLifecycleOrder {
		if have[e] {
			out = append(out, e)
			emitted[e] = true
		}
	}
	for _, e := range seen {
		if !emitted[e] {
			out = append(out, e)
			emitted[e] = true
		}
	}
	return out
}

// hookIntMeta extracts an integer field from a hook spec's Meta map.
// YAML decodes numerics as int/int64/float64 depending on representation,
// so each plausible concrete type is checked. Returns 0 when the key is
// absent, an unparseable string, or any other type.
func hookIntMeta(meta map[string]any, key string) int {
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var n int
		_, err := fmt.Sscanf(v, "%d", &n)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// hookCommands normalizes a `command:` field that may be a string or a
// list of strings into a single []string. Empty strings drop out.
func hookCommands(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
