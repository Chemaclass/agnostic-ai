// Package opencode emits configs for the OpenCode (SST) CLI.
//
// Rules merge into `.opencode/AGENTS.md`. We intentionally route under
// `.opencode/` rather than the repo root so OpenCode and Codex (which
// natively writes the root `AGENTS.md`) can coexist in the same project.
//
// Agents emit as per-file slash commands at `.opencode/commands/<name>.md`
// with native OpenCode frontmatter (`description`, optional `agent`,
// `model`, `subtask`). Skills are reference-only in the main AGENTS.md
// by default; opt into command emission via
// `outputs.opencode.emit-skills-as-commands: true`.
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
	defaultOutFile      = ".opencode/AGENTS.md"
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

// Emit writes the merged AGENTS.md plus one command file per agent
// (and per skill when opted in).
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
	}
	if err := emitAgentsMd(b, cfg, commandsDir, dryRun); err != nil {
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
		if err := emit.WriteFile(path, commandFile(a), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func emitSkillCommands(skills []spec.Entry, dir string, dryRun bool) error {
	for _, s := range skills {
		path := filepath.Join(dir, skillFilenamePrefix+s.Name+".md")
		if err := emit.WriteFile(path, commandFile(s), dryRun); err != nil {
			return err
		}
	}
	return nil
}

func emitAgentsMd(b spec.Bundle, cfg *config.Config, commandsDir string, dryRun bool) error {
	if len(b.Rules) == 0 && len(b.Agents) == 0 && len(b.Skills) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("# AGENTS.md\n\n")
	sb.WriteString("Generated by agnostic-ai.\n\n")
	writeRulesSection(&sb, b.Rules)
	writeAgentsSection(&sb, b.Agents, commandsDir)
	writeSkillsSection(&sb, b.Skills, commandsDir, emit.EmitSkillsAsCommands(cfg, target))
	return emit.WriteFile(emit.OutputFile(cfg, target, defaultOutFile), sb.String(), dryRun)
}

// commandFile renders a single command markdown file: filtered
// frontmatter (description + optional agent/model/subtask) followed
// by the spec body.
func commandFile(e spec.Entry) string {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickKeys(meta, commandFrontmatterKeys)
	var sb strings.Builder
	sb.WriteString(emit.Frontmatter(front))
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

func writeRulesSection(sb *strings.Builder, rules []spec.Entry) {
	if len(rules) == 0 {
		return
	}
	sb.WriteString("## Rules\n\n")
	for _, r := range rules {
		emit.WriteSection(sb, r.Name, r)
	}
}

// writeAgentsSection lists agents with a pointer to the command file
// path. The real definitions live in `<commandsDir>/<name>.md`.
func writeAgentsSection(sb *strings.Builder, agents []spec.Entry, commandsDir string) {
	if len(agents) == 0 {
		return
	}
	sb.WriteString("## Agents\n\n")
	sb.WriteString("Custom OpenCode slash commands. Definitions live in `" + commandsDir + "/`.\n\n")
	for _, a := range agents {
		emit.WriteReference(sb, a, filepath.Join(commandsDir, a.Name+".md"))
	}
}

func writeSkillsSection(sb *strings.Builder, skills []spec.Entry, commandsDir string, asCommands bool) {
	if len(skills) == 0 {
		return
	}
	sb.WriteString("## Skills\n\n")
	if asCommands {
		sb.WriteString("OpenCode slash commands. Definitions live in `" + commandsDir + "/`.\n\n")
	} else {
		sb.WriteString("Reference material. Read the source file to use a skill.\n\n")
	}
	for _, s := range skills {
		var sourcePath string
		if asCommands {
			sourcePath = filepath.Join(commandsDir, skillFilenamePrefix+s.Name+".md")
		} else {
			sourcePath = s.Path
		}
		emit.WriteReference(sb, s, sourcePath)
	}
}
