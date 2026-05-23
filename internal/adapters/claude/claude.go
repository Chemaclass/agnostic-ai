// Package claude emits Claude Code configs.
//
// Claude Code natively supports all six spec kinds:
//   - agents   -> <dir>/agents/<name>.md
//   - skills   -> <dir>/skills/<name>/SKILL.md
//   - rules    -> <dir>/rules/<name>.md (one file per rule)
//   - hooks    -> <dir>/settings.json
//   - mcps     -> .mcp.json
//   - commands -> <dir>/commands/<name>.md (slash commands)
//
// Rules emit one file per spec under `.claude/rules/` so a hand-authored
// CLAUDE.md is never clobbered. Reference the per-rule files from CLAUDE.md
// via `@.claude/rules/<name>.md` imports if you want Claude Code to load
// them. Set `outputs.claude.rules-file: CLAUDE.md` to fall back to the
// legacy concatenated single-file layout.
package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook, spec.KindMCP, spec.KindCommand},
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
		if err := propagateSkillAssets(s, folder, dryRun); err != nil {
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

	if err := writeSettings(b.Hooks, dir, cfg, dryRun); err != nil {
		return err
	}

	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// propagateSkillAssets mirrors every sibling file under the source
// skill directory (`<skills-src>/<name>/`) into the emitted skill
// folder, skipping SKILL.md because the adapter re-renders it from the
// spec frontmatter. Skills authored in agnostic-ai can ship helper
// scripts, fixtures, or nested subdirectories alongside SKILL.md; this
// preserves them across `sync` so the Claude Code skill continues to
// work after a round-trip.
//
// No-op when the source path is unknown (e.g. specs loaded from raw
// bytes in the WASM playground) so adapters stay safe for in-memory
// callers.
func propagateSkillAssets(s spec.Entry, dstDir string, dryRun bool) error {
	if s.Path == "" {
		return nil
	}
	srcDir := filepath.Dir(s.Path)
	return emit.CopyTree(srcDir, dstDir, func(rel string) bool {
		return rel == "SKILL.md"
	}, dryRun)
}

// writeSettings renders `.claude/settings.json` by layering, in order
// of increasing precedence:
//
//  1. Overlay captured during `import claude` (covers anything the user
//     keeps inside `.claude/settings.json` directly). When the overlay
//     is absent, the existing `.claude/settings.json` on disk is used as
//     the base instead so user-edited keys survive until `import claude`
//     captures them.
//  2. First-class fields from `outputs.claude.settings` in
//     `agnostic-ai.yaml` (statusLine, permissions, env, model, etc).
//  3. Spec-derived `hooks` block, emitted via ordered JSON so
//     `{type, command}` and `{matcher, hooks}` stay in lifecycle order
//     instead of alpha-sorted map order.
//
// Short-circuit: all three layers empty -> write nothing.
func writeSettings(hooks []spec.Entry, dir string, cfg *config.Config, dryRun bool) error {
	path := filepath.Join(dir, "settings.json")
	overlay, overlayOK, err := loadSettingsOverlay(dryRun)
	if err != nil {
		return err
	}
	configSettings := buildConfigSettings(cfg)
	hasConfig := len(configSettings) > 0
	hasHooks := len(hooks) > 0
	if !overlayOK && !hasHooks && !hasConfig {
		return nil
	}
	doc := overlay
	if doc == nil {
		// No overlay yet: fall back to the existing settings.json on
		// disk so a user-edited statusLine survives until `import
		// claude` captures it. Skip in dryRun and capture to keep
		// previews and `sync --check` reproducible from sources.
		doc, err = loadSettingsFromDisk(path, dryRun)
		if err != nil {
			return err
		}
		if doc == nil {
			doc = emit.NewOrderedJSON()
		}
	}
	for _, k := range orderedConfigKeys(configSettings) {
		if err := doc.Set(k, configSettings[k]); err != nil {
			return fmt.Errorf("claude settings: marshal %s: %w", k, err)
		}
	}
	if hasHooks {
		if err := doc.Set("hooks", hookSettingsJSON(hooks)); err != nil {
			return fmt.Errorf("claude settings: marshal hooks: %w", err)
		}
	} else {
		doc.Delete("hooks")
	}
	raw, err := emit.MarshalJSONIndent(doc)
	if err != nil {
		return err
	}
	return emit.WriteFile(path, string(raw)+"\n", dryRun)
}

// loadSettingsFromDisk reads an existing `.claude/settings.json` as the
// base when no captured overlay is available. Returns (nil, nil) when
// the file is absent, in dryRun, or under capture so the path is
// reproducible from sources.
func loadSettingsFromDisk(path string, dryRun bool) (*emit.OrderedJSON, error) {
	if dryRun || emit.IsCapturing() {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
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
	if errors.Is(err, fs.ErrNotExist) {
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
		sb.WriteString(emit.Header(emit.FormatMarkdown) + "\n")
		for _, r := range rules {
			sb.WriteString("## " + r.Name + "\n\n" + r.Body + "\n\n")
		}
		return emit.WriteFile(rulesFile, sb.String(), dryRun)
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	for _, r := range rules {
		path := filepath.Join(rulesDir, r.Name+".md")
		body := emit.WithHeader(emit.DocumentStyled(r.Meta, r.MetaKeys, r.MetaStyles, r.Body, target), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// hookSettingsJSON returns the `"hooks"` block as ordered JSON so the
// resulting settings.json mirrors Claude Code's lifecycle order
// (Pre before Post, etc.), with `{type, command}` and `{matcher, hooks}`
// in the order CLAUDE.md examples use. Returns nil when no hooks emit.
func hookSettingsJSON(hooks []spec.Entry) *emit.OrderedJSON {
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
	for _, event := range orderedHookEvents(eventOrder) {
		_ = doc.Set(event, byEvent[event])
	}
	return doc
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
