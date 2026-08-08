// Package factory emits configs for Factory's Droid CLI.
//
// The project-root AGENTS.md is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). Droid CLI reads AGENTS.md directly as
// its single source of truth for project rules, delivered centrally;
// this adapter never writes a rules file of its own.
//
// Custom droids (Droid CLI's subagent surface) emit as one Markdown
// file per agent spec at `.factory/droids/<name>.md` (override via
// outputs.factory.agents-dir), the top-level directory Droid CLI scans
// for droid definitions — nested subdirectories are not read, so every
// droid lands flat in that one folder. Frontmatter carries `name`,
// `description`, optional `model`, and optional `tools`; arbitrary
// `x-factory` keys pass through verbatim so the rest of the documented
// schema is reachable without waiting on this adapter's allowlist.
// Droid CLI's own schema requires a non-empty system prompt after the
// frontmatter ("The body after the frontmatter is the system prompt
// and cannot be empty", docs.factory.ai/harness/subagents), so an
// agent spec with an empty body is skipped rather than written as a
// file Droid CLI itself would call invalid; the skip surfaces through
// a coverage note instead of failing silently.
//
// MCP servers merge into `.factory/mcp.json` (override via
// outputs.factory.mcp-file) under a root `mcpServers` map, the same
// shape emit.MCPSchemaServersMap already produces for Claude Code and
// Cursor: stdio carries `command`/`args`/`env` with no `type`; HTTP
// and SSE carry an explicit `type` plus `url`/`headers`, both
// documented by Factory. This adapter also emits `type: ws` when a
// server's own transport is `ws`; Factory's docs list only `stdio`,
// `http`, and `sse`, so that support is inherited from the shared MCP
// schema (vendor-confirmed for Claude Code, not for Factory) rather
// than confirmed here. Unlike Claude Code, Cursor, and Copilot,
// Factory's own schema documents a working per-server `disabled`
// boolean (default false), so this adapter does not strip it the way
// those three do. Factory's schema also documents `disabledTools`,
// `timeout`, `connectTimeout`, and `oauth`, none of which the
// cross-tool spec carries yet; they are not emitted.
package factory

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "factory"
	defaultDroidsDir = ".factory/droids"
	defaultMCPFile   = ".factory/mcp.json"
)

// droidFrontmatterKeys names the keys Droid CLI reads from a custom
// droid's frontmatter, in the order Factory's docs present them.
var droidFrontmatterKeys = []string{"name", "description", "model", "tools"}

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes a
	// rules file itself: Droid CLI reads project rules exclusively
	// from the shared AGENTS.md entry-point sync writes centrally.
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindMCP},
}

// Adapter emits Factory Droid CLI configs.
type Adapter struct{}

// New returns a Factory adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one droid Markdown file per agent spec under
// `.factory/droids/`, plus a merged `.factory/mcp.json` for MCP
// servers. The project-root AGENTS.md (rules' single source of truth
// for Droid CLI) is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	dir := emit.OutputAgentsDir(cfg, target, defaultDroidsDir)
	if err := emitDroids(sess, b.Agents, dir, dryRun); err != nil {
		return err
	}
	// Factory's schema documents a working `disabled` key (unlike
	// Claude Code, Cursor, and Copilot), so the shared builder's
	// existing `disabled` output is correct here as-is; no strip.
	return sess.WriteMCPFile(b.MCPs, emit.MCPSchemaServersMap, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitDroids writes one `<dir>/<name>.md` per agent spec whose body is
// non-empty. Droid CLI's own schema calls a frontmatter-only body
// invalid, so an agent spec with an empty (or whitespace-only) body is
// skipped instead of written as a file the tool itself would reject;
// the skip count surfaces through a coverage note so a spec left empty
// by accident does not disappear without a trace.
func emitDroids(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	var emptyBody int
	for _, a := range agents {
		if strings.TrimSpace(a.Body) == "" {
			emptyBody++
			continue
		}
		path := filepath.Join(dir, a.Name+".md")
		body := emit.WithHeader(droidMarkdown(a), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	emit.NoteCoverageGap(target, spec.KindAgent, emptyBody,
		"empty spec body; Droid CLI requires a non-empty system prompt")
	return nil
}

// droidMarkdown renders a single droid definition: `name`,
// `description` (falls back to the spec name), optional `model`,
// optional `tools`, plus arbitrary x-factory passthrough, followed by
// the spec body as the droid's system prompt. Callers only reach this
// with a non-empty (trimmed) body; emitDroids skips the empty case
// before this ever runs.
func droidMarkdown(e spec.Entry) string {
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	meta := map[string]any{
		"name":        e.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	if tools := emit.StringSlice(resolved["tools"]); len(tools) > 0 {
		meta["tools"] = tools
		keys = append(keys, "tools")
	}
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, droidFrontmatterKeys...)
	front := emit.FrontmatterOrdered(meta, keys)
	return front + "\n" + strings.TrimSpace(e.Body) + "\n"
}
