// Package opencode emits configs for the OpenCode (SST) CLI.
//
// The root `AGENTS.md` entry-point is written centrally by `sync` as a
// slim pointer to the source specs, with every rule body inlined under
// a sentinel-marked block (one body shared with codex, amp, warp, and
// the other AGENTS.md consumers, so the write dedupes). That path is
// the only one OpenCode reads: opencode.ai/docs/rules documents an
// upward walk from the current directory for `AGENTS.md` and
// `CLAUDE.md`, and the vendor's own source file
// `packages/core/src/instruction-context.ts` on branch `dev` walks with
// `fs.up({ targets: ["AGENTS.md"] })`. Until #623 the entry point sat at
// `.opencode/AGENTS.md`, which no vendor doc or code path names, so
// rules reached OpenCode only when another AGENTS.md target happened
// to be enabled too. A managed leftover there is swept on sync.
//
// When `outputs.opencode.rules-file` is set, this adapter instead
// writes the legacy concatenated layout at that path so users on older
// workflows keep their behavior.
//
// Agents emit natively as subagent definitions at
// `.opencode/agents/<name>.md` (frontmatter `description`, `mode`,
// `model`, `temperature`, `permission`). Skills emit natively as one
// folder per skill at `.opencode/skills/<name>/SKILL.md` with bundled
// assets; `outputs.opencode.emit-skills-as-commands: true` additionally
// writes the command form. Command specs emit at
// `.opencode/commands/<name>.md`.
package opencode

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target              = "opencode"
	defaultAgentsDir    = ".opencode/agents"
	defaultSkillsDir    = ".opencode/skills"
	defaultCommandsDir  = ".opencode/commands"
	defaultMCPFile      = "opencode.json"
	skillFilenamePrefix = "skill-"
	opencodeSchemaURL   = "https://opencode.ai/config.json"
	// legacyEntryPointFile is the pre-#623 entry-point default. OpenCode
	// walks up for files named exactly `AGENTS.md` and never opens a
	// `.opencode/` copy, so sync moved the write to the root file and
	// sweeps a managed leftover here (same treatment codex gives its
	// v0.26..v0.42 `.codex/skills/` tree).
	legacyEntryPointFile = ".opencode/AGENTS.md"
)

// commandFrontmatterKeys names the only frontmatter keys OpenCode reads
// from a custom command file. Anything else in the spec is dropped so
// internal-only fields (globs, tools, ...) do not leak.
var commandFrontmatterKeys = []string{"description", "agent", "model", "subtask"}

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindCommand},
}

// Adapter emits OpenCode configs.
type Adapter struct{}

// New returns an OpenCode adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one native agent definition per agent, one native skill
// folder per skill (plus the command form when opted in), one command
// file per command spec, `opencode.json` for MCP servers, and—when
// opted in via outputs.opencode.rules-file—a legacy concatenated rules
// document. The root `AGENTS.md` entry-point is written by `sync`, not
// here; this Emit only sweeps the stale `.opencode/AGENTS.md` a
// pre-#623 sync left behind.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sweepLegacyEntryPoint(sess, cfg, dryRun); err != nil {
		return err
	}

	if err := emitAgents(sess, b.Agents, emit.OutputAgentsDir(cfg, target, defaultAgentsDir), dryRun); err != nil {
		return err
	}
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	if err := emitCommands(sess, b.Commands, commandsDir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		if err := emitSkillCommands(sess, b.Skills, commandsDir, dryRun); err != nil {
			return err
		}
	}
	if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun); err != nil {
		return err
	}
	return emitMCPConfig(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// sweepLegacyEntryPoint removes the agnostic-ai-managed entry-point a
// pre-#623 sync left at `.opencode/AGENTS.md`, now that the write moved
// to the root `AGENTS.md` OpenCode actually walks up for. Files without
// the provenance header are hand-authored and stay. The sweep is
// skipped when the user pointed the entry-point
// (`outputs.opencode.file`) or the legacy concatenated rules document
// (`outputs.opencode.rules-file`) back at that path, since then it is a
// live write rather than a leftover.
func sweepLegacyEntryPoint(sess *emit.Session, cfg *config.Config, dryRun bool) error {
	if emit.EntryPointPath(cfg, target) == legacyEntryPointFile {
		return nil
	}
	if emit.OutputRulesFile(cfg, target, "") == legacyEntryPointFile {
		return nil
	}
	return sess.RemoveGenerated(legacyEntryPointFile, dryRun)
}

// emitMCPConfig writes (or merges into) opencode.json with the `mcp`
// map and a `$schema` link. Routes through emit.MergeJSONFile so any
// pre-existing user-managed keys (theme, model, ...) survive the sync;
// only `$schema` and `mcp` are overwritten.
func emitMCPConfig(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	if len(mcps) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, map[string]any{
		"$schema": opencodeSchemaURL,
		"mcp":     buildMCPMap(mcps),
	}, dryRun)
}

// buildMCPMap maps spec MCP entries to OpenCode's `mcp` schema:
//
//	stdio:  {type: "local",  command: ["cmd", "arg1", ...], environment: {...}}
//	http:   {type: "remote", url: "...", headers: {...}}
//
// A spec's `disabled: true` maps to OpenCode's own `enabled: false` key
// (#555): opencode.ai/docs/mcp-servers documents "You can also disable
// a server by setting `enabled` to `false`" for both local and remote
// servers. OpenCode's own default (enabled) needs no explicit key, so
// an enabled server gets no `enabled` key at all, matching the codex
// and kilo adapters' identical convention for their own `enabled`
// fields.
func buildMCPMap(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPEntry(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

// buildMCPEntry renders one `mcp.<name>` entry from this fixed field
// set: type, command (stdio) or url/headers (remote), environment, and
// enabled. Any other documented field beyond that set (`oauth` on a
// "Pre-registered" remote entry, opencode.ai/docs/mcp-servers/) or any
// field OpenCode adds next reaches the entry through `x-opencode`
// (emit.MergeCustomTargetMeta) instead of staying unreachable: commands
// and agents already get that passthrough (commandFile, agent.go), so
// the MCP builder matches rather than being the one exception (#588).
func buildMCPEntry(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	entry := map[string]any{}
	switch transport {
	case "stdio":
		entry["type"] = "local"
		entry["command"] = combineCommand(e.Meta)
	case "http", "sse", "remote":
		entry["type"] = "remote"
		if url, _ := e.Meta["url"].(string); url != "" {
			entry["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			entry["headers"] = h
		}
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		entry["environment"] = env
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		entry["enabled"] = false
	}
	var keys []string
	emit.MergeCustomTargetMeta(entry, &keys, e.Meta, target,
		"type", "command", "url", "headers", "environment", "enabled")
	return entry
}

// combineCommand turns OpenCode's expected `command: [cmd, arg1, ...]`
// shape from agnostic-ai's separate `command` + `args` fields.
func combineCommand(meta map[string]any) []string {
	cmd, _ := meta["command"].(string)
	parts := []string{}
	if cmd != "" {
		parts = append(parts, cmd)
	}
	parts = append(parts, emit.StringSlice(meta["args"])...)
	return parts
}

// emitCommands writes one command markdown file per command spec under
// `.opencode/commands/`. Commands are the native slash-prompt surface,
// so they emit with their bare name (no `skill-` prefix).
func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, c := range commands {
		path := filepath.Join(dir, c.Name+".md")
		body := emit.WithHeader(commandFile(c), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func emitSkillCommands(sess *emit.Session, skills []spec.Entry, dir string, dryRun bool) error {
	for _, s := range skills {
		path := filepath.Join(dir, skillFilenamePrefix+s.Name+".md")
		body := emit.WithHeader(commandFile(s), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// commandFile renders a single command markdown file: filtered
// frontmatter (description + optional agent/model/subtask) followed
// by the spec body.
func commandFile(e spec.Entry) string {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickKeys(meta, commandFrontmatterKeys)
	keys := append([]string{}, commandFrontmatterKeys...)
	// Pass through arbitrary x-opencode keys beyond the documented set so
	// an author can declare command metadata OpenCode adds later without
	// waiting on the allowlist. Excludes the allowlisted keys (handled by
	// pickKeys) so nothing is emitted twice. See #367.
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, commandFrontmatterKeys...)
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String()
}

// pickKeys returns the subset of meta whose keys are in allowed.
// Empty string values are dropped so absent fields stay absent.
func pickKeys(meta map[string]any, allowed []string) map[string]any {
	out := make(map[string]any, len(allowed))
	for _, k := range allowed {
		v, ok := meta[k]
		if !ok {
			continue
		}
		if s, isStr := v.(string); isStr && s == "" {
			continue
		}
		out[k] = v
	}
	return out
}
