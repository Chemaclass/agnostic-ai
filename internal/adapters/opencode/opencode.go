// Package opencode emits configs for the OpenCode (SST) CLI.
//
// The `.opencode/AGENTS.md` entry-point is written centrally by `sync`
// as a slim pointer to the source specs (one body shared with every
// other target's entry-point file). When `outputs.opencode.rules-file`
// is set, this adapter instead writes the legacy concatenated layout
// at that path so users on older workflows keep their behavior.
//
// Agents emit as per-file slash commands at `.opencode/commands/<name>.md`
// with native OpenCode frontmatter (`description`, optional `agent`,
// `model`, `subtask`). Skills are reference-only by default; opt into
// command emission via `outputs.opencode.emit-skills-as-commands: true`.
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
	defaultCommandsDir  = ".opencode/commands"
	defaultMCPFile      = "opencode.json"
	skillFilenamePrefix = "skill-"
	opencodeSchemaURL   = "https://opencode.ai/config.json"
)

// commandFrontmatterKeys names the only frontmatter keys OpenCode reads
// from a custom command file. Anything else in the spec is dropped so
// internal-only fields (globs, tools, ...) do not leak.
var commandFrontmatterKeys = []string{"description", "agent", "model", "subtask"}

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits OpenCode configs.
type Adapter struct{}

// New returns an OpenCode adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one command file per agent (and per skill when opted
// in), `opencode.json` for MCP servers, and—when opted in via
// outputs.opencode.rules-file—a legacy concatenated rules document.
// The `.opencode/AGENTS.md` entry-point is written by `sync`, not
// here.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}

	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)

	if err := emitAgentCommands(b.Agents, commandsDir, dryRun); err != nil {
		return err
	}
	if emit.EmitSkillsAsCommands(cfg, target) {
		if err := emitSkillCommands(b.Skills, commandsDir, dryRun); err != nil {
			return err
		}
	} else {
		emit.NoteCoverageGap(target, spec.KindSkill, len(b.Skills),
			"outputs.opencode.emit-skills-as-commands")
	}
	if err := emit.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{Title: "AGENTS.md"}, dryRun); err != nil {
		return err
	}
	return emitMCPConfig(b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitMCPConfig writes (or merges into) opencode.json with the `mcp`
// map and a `$schema` link. Routes through emit.MergeJSONFile so any
// pre-existing user-managed keys (theme, model, ...) survive the sync;
// only `$schema` and `mcp` are overwritten.
func emitMCPConfig(mcps []spec.Entry, path string, dryRun bool) error {
	if len(mcps) == 0 {
		return nil
	}
	return emit.MergeJSONFile(path, map[string]any{
		"$schema": opencodeSchemaURL,
		"mcp":     buildMCPMap(mcps),
	}, dryRun)
}

// buildMCPMap maps spec MCP entries to OpenCode's `mcp` schema:
//
//	stdio:  {type: "local",  command: ["cmd", "arg1", ...], environment: {...}}
//	http:   {type: "remote", url: "...", headers: {...}}
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

func emitAgentCommands(agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		body := emit.WithHeader(commandFile(a), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func emitSkillCommands(skills []spec.Entry, dir string, dryRun bool) error {
	for _, s := range skills {
		path := filepath.Join(dir, skillFilenamePrefix+s.Name+".md")
		body := emit.WithHeader(commandFile(s), emit.FormatMarkdown)
		if err := emit.WriteFile(path, body, dryRun); err != nil {
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
