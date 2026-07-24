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
// Frontmatter carries `name`, `description`, optional `model`, and
// optional `tools`; arbitrary `x-kilo` keys pass through verbatim.
//
// MCP servers merge into the project `kilo.jsonc` (override via
// outputs.kilo.mcp-file) under an `mcpServers` map: stdio entries
// render as `{"command": ..., "args": [...], "env": {...}}`; HTTP /
// SSE / remote entries render as `{"url": ..., "headers": {...}}`.
// kilo.jsonc also holds user-managed keys (models, providers, ...);
// the merge only touches `mcpServers` so those survive a sync. This
// adapter writes plain JSON: JSONC is a superset of JSON, so every
// JSONC parser accepts the output, and agnostic-ai never needs to
// emit (or preserve) comments of its own.
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

// agentFrontmatterKeys names the keys Kilo Code reads from an agent's
// frontmatter.
var agentFrontmatterKeys = []string{"name", "description", "model", "tools"}

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

// agentMarkdown renders a single agent definition: `name`,
// `description` (falls back to the spec name), optional `model`,
// optional `tools`, plus arbitrary x-kilo passthrough, followed by the
// spec body as the agent's system prompt.
func agentMarkdown(e spec.Entry) string {
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
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, agentFrontmatterKeys...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

// emitMCPConfig merges the `mcpServers` map into kilo.jsonc. Routes
// through emit.MergeJSONFile so any pre-existing user-managed keys
// (models, providers, ...) survive the sync; only `mcpServers` is
// overwritten. No file is written when there are no MCP entries (or
// every entry renders empty).
func emitMCPConfig(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	servers := buildMCPMap(mcps)
	if len(servers) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, map[string]any{"mcpServers": servers}, dryRun)
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

// buildMCPEntry renders one kilo.jsonc mcpServers entry. Stdio specs
// produce a command/args/env block; HTTP / SSE / remote specs produce
// a url/headers block. An entry missing its transport's required
// field (command for stdio, url for remote) is dropped: there is
// nothing for Kilo Code to run or connect to.
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
		out["command"] = cmd
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			out["args"] = args
		}
		if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
			out["env"] = env
		}
	case "http", "sse", "remote":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["url"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default:
		return nil
	}

	return out
}
