// Package qoder emits .qoder/rules/*.md and .qoder/agents/*.md for
// Alibaba's Qoder IDE.
//
// Qoder reads project rules from `.qoder/rules/*.md`: one file per
// rule, each carrying a name and description, and Qoder's own docs
// state this native rules content takes precedence over `AGENTS.md`
// when both are present. Qoder also reads the cross-tool root
// `AGENTS.md`, which is written centrally by `sync` as a slim pointer
// to the source specs, not by this adapter.
//
// Agents emit as one Markdown file per agent spec at
// `.qoder/agents/<name>.md` (override via outputs.qoder.agents-dir):
// docs.qoder.com/extensions/subagent documents `name` and
// `description` as required frontmatter, plus optional `model`,
// `tools`, `skills`, and `mcpServers`. `tools` renders as a
// comma-separated string (`tools: Read, Grep, Bash`), the only form the
// vendor doc shows; no list syntax is documented. Qoder's built-in tool
// vocabulary is Claude-style (`Bash`, `Edit`, `Write`, `Glob`, `Grep`,
// `Read`, `WebFetch`, `WebSearch`), so agnostic-ai's generic `tools`
// list passes through safely here: joined into the comma form on emit
// and split back into a list on `import qoder`. That is the opposite of
// kilo and augment, whose vendor tool vocabularies differ from
// agnostic-ai's and which drop a generic `tools` list with a coverage
// note instead of guessing a translation; qoder is the one target where
// passthrough is confirmed safe. Qoder's skill surface still has no
// documented file format, so skills continue to the unsupported-kind
// warning channel.
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
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "qoder"
	defaultDir       = ".qoder/rules"
	defaultAgentsDir = ".qoder/agents"
	defaultMCPFile   = ".mcp.json"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindMCP},
}

// Adapter emits Qoder configs.
type Adapter struct{}

// New returns a Qoder adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one .md per rule into the rules directory (default
// `.qoder/rules`), one .md per agent into the agents directory (default
// `.qoder/agents`), plus a merged `.mcp.json` for MCP servers. Skills
// have no native Qoder surface, so they are left to ReportUnsupported
// rather than flattened into the rules directory.
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
	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, agentsDir, dryRun); err != nil {
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

// emitAgents writes one `<dir>/<name>.md` per agent spec.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		body := emit.WithHeader(agentMarkdown(a), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// agentMarkdown renders one `.qoder/agents/<name>.md` file. `name`
// (from the spec name) and `description` (falls back to the spec name)
// are Qoder's required frontmatter keys; `model`, `tools`, `skills`,
// and `mcpServers` are its documented optional ones (see the package
// doc). `tools` joins agnostic-ai's generic list into Qoder's
// comma-separated string form; a spec that already wrote a plain
// string (e.g. via x-qoder) passes through unchanged. `skills` and
// `mcpServers` pass through whatever shape the spec declares, since
// Qoder defines no agnostic-ai-native format for either.
func agentMarkdown(a spec.Entry) string {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{
		"name":        a.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	if tools := qoderToolsString(resolved["tools"]); tools != "" {
		meta["tools"] = tools
		keys = append(keys, "tools")
	}
	for _, k := range []string{"skills", "mcpServers"} {
		if v, ok := resolved[k]; ok {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target,
		"name", "description", "model", "tools", "skills", "mcpServers")
	front := emit.FrontmatterOrdered(meta, keys)
	trimmed := strings.TrimSpace(a.Body)
	if trimmed == "" {
		return front + "\n"
	}
	return front + "\n" + trimmed + "\n"
}

// qoderToolsString renders the generic `tools` field as Qoder's
// documented comma-separated string: `tools: Read, Grep, Bash`, no
// list syntax. A list joins with ", "; a spec that already wrote a
// plain string (typically via x-qoder.tools) passes through unchanged.
// Returns "" when tools is absent or empty, so the caller omits the key
// entirely rather than writing an empty one.
func qoderToolsString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return strings.Join(emit.StringSlice(v), ", ")
}
