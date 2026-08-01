// Package qoder emits .qoder/rules/*.md for Alibaba's Qoder IDE.
//
// Qoder reads project rules from `.qoder/rules/*.md`: one file per
// rule, each carrying a name and description, and Qoder's own docs
// state this native rules content takes precedence over `AGENTS.md`
// when both are present. Qoder's agent and skill surfaces have no
// documented file format yet, so this adapter declares only KindRule
// and KindMCP and leaves agents and skills to the unsupported-kind
// warning channel. Qoder also reads the cross-tool root `AGENTS.md`,
// which is written centrally by `sync` as a slim pointer to the
// source specs, not by this adapter.
//
// MCP servers merge into `.mcp.json` (override via
// outputs.qoder.mcp-file) under a root `mcpServers` map: stdio carries
// `command`/`args`/`env` with no `type`; remote carries an explicit
// `type` plus `url`/`headers`. That is the exact shape and the exact
// literal project-root path Claude Code already writes: two targets
// sharing one file, deduplicated by sync when both are enabled. Qoder's
// own support for a per-server `disabled` key is not vendor-confirmed,
// and `.mcp.json` is the same file Claude Code reads there (which
// ignores the key entirely), so this adapter strips `disabled` the way
// the claude adapter does rather than risk two targets writing
// different bytes to one shared path.
package qoder

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target         = "qoder"
	defaultDir     = ".qoder/rules"
	defaultMCPFile = ".mcp.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule, spec.KindMCP},
}

// Adapter emits Qoder configs.
type Adapter struct{}

// New returns a Qoder adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule into the rules directory (default
// `.qoder/rules`), plus a merged `.mcp.json` for MCP servers. Agents
// and skills have no native Qoder surface, so they are left to
// ReportUnsupported rather than flattened into the rules directory.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        emit.OutputRulesDir(cfg, target, defaultDir),
		SkipAgents: true,
		SkipSkills: true,
	}, dryRun); err != nil {
		return err
	}
	mcps := emit.StripMCPDisabled(target, b.MCPs, mcpDisabledNoOpReason)
	return sess.WriteMCPFile(mcps, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// mcpDisabledNoOpReason explains, in the flushed coverage note, why
// `disabled: true` on an MCP spec never reaches `.mcp.json`: the file
// is shared byte-for-byte with Claude Code, which has no per-server
// disable key there (see the package doc comment).
const mcpDisabledNoOpReason = "no confirmed file-based way to pre-disable a project-scoped MCP server; .mcp.json is the same file Claude Code reads there, and Claude Code ignores a per-server disabled key"
