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
		body := emit.WithHeader(emit.DocumentOrdered(a.Meta, a.MetaKeys, a.Body, target), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}

	for _, s := range b.Skills {
		folder := filepath.Join(dir, "skills", s.Name)
		path := filepath.Join(folder, "SKILL.md")
		body := emit.WithHeader(emit.DocumentOrdered(s.Meta, s.MetaKeys, s.Body, target), emit.FormatMarkdown)
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
		body := emit.WithHeader(emit.DocumentOrdered(c.Meta, c.MetaKeys, c.Body, target), emit.FormatMarkdown)
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
//     keeps inside `.claude/settings.json` directly).
//  2. First-class fields from `outputs.claude.settings` in
//     `agnostic-ai.yaml` (statusLine, permissions, env, model, etc).
//  3. Spec-derived `hooks` block (always wins for the `hooks` key).
//
// Three short-circuits:
//
//   - All three layers empty: write nothing.
//   - Overlay absent but hooks present and config empty: merge into any
//     existing settings.json on disk so a user-edited statusLine added
//     directly to `.claude/settings.json` survives until `import claude`
//     captures it.
//   - Overlay absent but config layer non-empty: start from an empty
//     map. Disk is not consulted, so the config is authoritative.
func writeSettings(hooks []spec.Entry, dir string, cfg *config.Config, dryRun bool) error {
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
	path := filepath.Join(dir, "settings.json")
	if !overlayOK && !hasConfig {
		// Backwards-compat path: no overlay yet, merge hooks into disk
		// so user-edited keys survive until `import claude` captures
		// them into the overlay.
		return emit.MergeJSONFile(path, buildHookSettings(hooks), dryRun)
	}
	doc := overlay
	if doc == nil {
		doc = map[string]any{}
	}
	for k, v := range configSettings {
		doc[k] = v
	}
	if hasHooks {
		for k, v := range buildHookSettings(hooks) {
			doc[k] = v
		}
	} else {
		delete(doc, "hooks")
	}
	raw, err := emit.MarshalJSONIndent(doc)
	if err != nil {
		return err
	}
	return emit.WriteFile(path, string(raw)+"\n", dryRun)
}

// loadSettingsOverlay reads the captured settings overlay from
// `.agnostic-ai/overlays/claude.settings.json`. Returns (doc, true, nil)
// when the overlay exists and parses, (nil, false, nil) when it is
// absent, and (nil, false, err) on a parse failure or unexpected read
// error. Skips disk in dryRun and capture modes so deterministic check
// passes do not depend on the working tree.
func loadSettingsOverlay(dryRun bool) (map[string]any, bool, error) {
	if dryRun || emit.IsCapturing() {
		return nil, false, nil
	}
	data, err := os.ReadFile(settingsOverlayPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false, err
	}
	if doc == nil {
		doc = map[string]any{}
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
		body := emit.WithHeader(emit.DocumentOrdered(r.Meta, r.MetaKeys, r.Body, target), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// buildHookSettings renders {"hooks": {<event>: [{"matcher", "hooks": [...]}]}}
// keyed for `.claude/settings.json`.
//
// Specs with the same event and matcher merge into one matcher block:
// their commands stack inside the inner `hooks` array, matching the
// Claude Code schema where each matcher entry can run multiple commands.
// A spec's `command:` field accepts a string or a list of strings; each
// string becomes one `{type: "command", command: ...}` entry.
func buildHookSettings(hooks []spec.Entry) map[string]any {
	type matcherKey struct{ event, matcher string }

	byKey := map[matcherKey][]map[string]any{}
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
		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		for _, cmd := range cmds {
			byKey[k] = append(byKey[k], map[string]any{"type": "command", "command": cmd})
		}
	}

	byEvent := map[string][]map[string]any{}
	for _, k := range keyOrder {
		byEvent[k.event] = append(byEvent[k.event], map[string]any{
			"matcher": k.matcher,
			"hooks":   byKey[k],
		})
	}
	// Stable iteration: events are emitted by Go in sorted order during
	// JSON encoding, but matcher groups within an event preserve insert
	// order. Sort matcher groups by matcher string for deterministic
	// output across sync runs.
	for event, groups := range byEvent {
		sort.SliceStable(groups, func(i, j int) bool {
			mi, _ := groups[i]["matcher"].(string)
			mj, _ := groups[j]["matcher"].(string)
			return mi < mj
		})
		byEvent[event] = groups
	}
	return map[string]any{"hooks": byEvent}
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
