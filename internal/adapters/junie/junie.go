// Package junie emits configs for JetBrains Junie.
//
// Junie reads every Markdown file under `.junie/rules/` and concatenates
// them automatically; there is no single entry file to assemble. Rules,
// agents, and skills all flatten into that one directory (agents as
// `agent-<name>.md`, skills as `skill-<name>.md`), matching the cline
// adapter's shape. Junie also reads the cross-tool root `AGENTS.md`,
// which is written centrally by `sync` as a slim pointer to the source
// specs; this adapter never writes it. `.junie/guidelines.md` is the
// legacy single-file location and is not emitted here.
//
// MCP servers write to `.junie/mcp/mcp.json` using the standard
// `mcpServers` map schema (the same shape Claude Code and Cursor use).
package junie

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target         = "junie"
	defaultDir     = ".junie/rules"
	defaultMCPFile = ".junie/mcp/mcp.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Junie configs.
type Adapter struct{}

// New returns a Junie adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule, agent, and skill into the rules
// directory, then writes the MCP server file when the bundle has any
// MCP entries.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := emit.RulesDirectory(b, emit.RulesDirOpts{
		Dir:         emit.OutputRulesDir(cfg, target, defaultDir),
		AgentPrefix: "agent-",
	}, dryRun); err != nil {
		return err
	}
	return emit.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap,
		emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}
