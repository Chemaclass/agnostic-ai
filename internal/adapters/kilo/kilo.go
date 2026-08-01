// Package kilo emits configs for Kilo Code.
//
// The project-root AGENTS.md is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). Kilo Code reads AGENTS.md as its
// recommended rules file for new projects, so this adapter never
// writes a rules file of its own. The legacy `.kilocode/rules/` tree
// is Kilo's own auto-migration target for pre-AGENTS.md projects; this
// adapter intentionally never emits it.
//
// Agents emit as one Markdown file per agent spec at
// `.kilo/agents/<name>.md` (override via outputs.kilo.agents-dir).
// Kilo Code takes the agent's name from the filename, not from
// frontmatter, so `name:` is never written. Frontmatter otherwise
// carries `description` (falls back to the spec name) and optional
// `model`; arbitrary `x-kilo` keys pass through verbatim. `tools` is
// never written under any key, including `x-kilo`: Kilo Code's full
// agent option table has no `tools` field, so a spec's `tools`
// allowlist would be a silent no-op there, and the agent would keep
// its default (typically full) permissions while looking restricted.
// Kilo Code's real access control is a per-tool `permission` map
// (`allow` / `ask` / `deny`), but this adapter has no vendor-confirmed
// mapping from agnostic-ai's generic tool names onto Kilo's own tool
// identifiers, so it does not guess one. An author who needs per-tool
// restriction writes `x-kilo: {permission: {...}}` directly; an agent
// spec that sets `tools` instead surfaces a coverage note rather than
// silently dropping the restriction.
//
// MCP servers merge into the project `kilo.jsonc` (override via
// outputs.kilo.mcp-file) under an `mcp` map, the key current Kilo Code
// reads (`mcpServers` is the deprecated MCP-spec 2025-03-26 form).
// Stdio entries combine `command` + `args` into one `command` array
// and set `"type": "local"`; HTTP / SSE / remote entries render as
// `{"type": "remote", "url": ..., "headers": {...}}`. `environment`
// (not `env`) carries a stdio server's environment variables. A spec's
// `disabled: true` writes `"enabled": false`, the key Kilo Code's own
// documented MCP example carries; `disabled` itself is never written,
// and an enabled server (the common case) gets no explicit key at all.
// kilo.jsonc also holds user-managed keys (models, providers, ...);
// the merge only touches `mcp` so those survive a sync. This adapter
// writes plain JSON: JSONC is a superset of JSON, so every JSONC
// parser accepts the output, and agnostic-ai never needs to emit (or
// preserve) comments of its own.
package kilo

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "kilo"
	defaultAgentsDir = ".kilo/agents"
	defaultMCPFile   = "kilo.jsonc"
)

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes a
	// rules file itself: Kilo Code reads project rules exclusively
	// from the shared AGENTS.md entry-point sync writes centrally.
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindMCP},
}

// Adapter emits Kilo Code configs.
type Adapter struct{}

// New returns a Kilo adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one agent Markdown file per agent spec under
// `.kilo/agents/`, plus a merged `kilo.jsonc` for MCP servers. The
// project-root AGENTS.md (rules' single source of truth for Kilo
// Code) is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	dir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, dir, dryRun); err != nil {
		return err
	}
	return emitMCPConfig(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitAgents writes one `<dir>/<name>.md` per agent spec. Agents whose
// spec declares `tools` get no restriction on Kilo Code (see
// agentMarkdown), so the whole batch surfaces one coverage note
// instead of a silent drop.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	withTools := 0
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, hadTools := agentMarkdown(a)
		if hadTools {
			withTools++
		}
		body := emit.WithHeader(md, emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	emit.NoteCoverageGap(target, spec.KindAgent, withTools,
		"tools has no Kilo Code key; use x-kilo.permission for native per-tool access control")
	return nil
}

// agentMarkdown renders a single agent definition: `description`
// (falls back to the spec name) and optional `model`, plus arbitrary
// x-kilo passthrough, followed by the spec body as the agent's system
// prompt. Kilo Code takes the agent name from the filename, so `name`
// is never written; `tools` is never written either (see the package
// doc). Both stay excluded from the x-kilo passthrough too, so an
// escape-hatch attempt cannot reintroduce a confirmed no-op key.
// hadTools reports whether the spec declared a tools list, so the
// caller can fold it into one coverage note per sync instead of a
// silent drop.
func agentMarkdown(e spec.Entry) (body string, hadTools bool) {
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	meta := map[string]any{
		"description": desc,
	}
	keys := []string{"description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	hadTools = len(emit.StringSlice(resolved["tools"])) > 0
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, "description", "model", "name", "tools")
	front := emit.FrontmatterOrdered(meta, keys)
	trimmed := strings.TrimSpace(e.Body)
	if trimmed == "" {
		return front + "\n", hadTools
	}
	return front + "\n" + trimmed + "\n", hadTools
}

// emitMCPConfig merges the `mcp` map into kilo.jsonc. Routes through
// emit.MergeJSONFile so any pre-existing user-managed keys (models,
// providers, ...) survive the sync; only `mcp` is overwritten. No file
// is written when there are no MCP entries (or every entry renders
// empty).
func emitMCPConfig(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	servers := buildMCPMap(mcps)
	if len(servers) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, map[string]any{"mcp": servers}, dryRun)
}

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

// buildMCPEntry renders one kilo.jsonc `mcp` entry. Stdio specs
// combine `command` + `args` into a single `command` array and set
// `"type": "local"`; HTTP / SSE / remote specs set `"type": "remote"`
// with a `url`/`headers` block. `environment` (not `env`) carries a
// stdio server's environment variables. An entry missing its
// transport's required field (command for stdio, url for remote) is
// dropped: there is nothing for Kilo Code to run or connect to.
//
// A spec's `disabled: true` maps to Kilo Code's own `enabled: false`
// key (B9, target-audit 2026-08-01 follow-up): the documented MCP
// example carries `"enabled": true` alongside type/command/environment/
// timeout, so the vendor concept exists under that name. Kilo Code's
// own default (enabled) needs no explicit key, matching the codex
// adapter's identical convention for its `enabled` field.
func buildMCPEntry(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}

	switch transport {
	case "stdio":
		cmd, _ := e.Meta["command"].(string)
		if cmd == "" {
			return nil
		}
		out["type"] = "local"
		out["command"] = combineCommand(cmd, e.Meta)
		if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
			out["environment"] = env
		}
	case "http", "sse", "remote":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["type"] = "remote"
		out["url"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default:
		return nil
	}

	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["enabled"] = false
	}

	return out
}

// combineCommand folds Kilo Code's expected `command: [cmd, arg1,
// ...]` array out of agnostic-ai's separate `command` + `args` fields.
func combineCommand(cmd string, meta map[string]any) []string {
	parts := []string{cmd}
	parts = append(parts, emit.StringSlice(meta["args"])...)
	return parts
}
