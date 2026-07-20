// Package crush emits configs for Charm's Crush CLI.
//
// The project-root AGENTS.md is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). Crush reads that file natively for
// project instructions, and rules reach it exclusively through that
// shared entry-point: Crush has no per-rule directory, so this adapter
// never writes rules directly.
//
// Skills emit as one folder per skill under `.agents/skills/<name>/SKILL.md`,
// the first location Crush scans (ahead of `.crush/skills`,
// `.claude/skills`, and `.cursor/skills`). The renderer matches the
// codex/amp/zed output byte-for-byte so the shared tree dedupes.
//
// MCP servers are configured in the project `crush.json` under the
// `mcp` map: stdio entries render as `{"type": "stdio", "command":
// ..., "args": [...], "env": {...}}`; HTTP / SSE / remote entries
// render as `{"type": "http", "url": ..., "headers": {...}}`. crush.json
// also holds user-managed keys (models, providers, lsp, options); the
// merge only touches the `mcp` key so those survive a sync.
package crush

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "crush"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = "crush.json"
)

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes rules
	// itself: they reach Crush through the shared AGENTS.md entry-point
	// sync writes centrally. KindAgent is absent; Crush has no agent
	// surface, so the unsupported warning is accurate.
	Supports: []spec.Kind{spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits Crush configs.
type Adapter struct{}

// New returns a Crush adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one native skill folder per skill under .agents/skills/
// and crush.json for MCP servers. The project-root AGENTS.md (with
// rule bodies inlined) is written by `sync`, not here.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := emit.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	return emitMCPConfig(b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitMCPConfig writes (or merges into) crush.json with the `mcp` map.
// Routes through emit.MergeJSONFile so any pre-existing user-managed
// keys (models, providers, lsp, options, ...) survive the sync; only
// `mcp` is overwritten.
func emitMCPConfig(mcps []spec.Entry, path string, dryRun bool) error {
	servers := buildMCPMap(mcps)
	if len(servers) == 0 {
		return nil
	}
	return emit.MergeJSONFile(path, map[string]any{"mcp": servers}, dryRun)
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

// buildMCPEntry renders one Crush mcp entry. Stdio specs produce a
// command/args/env block tagged `type: "stdio"`; HTTP / SSE / remote
// specs produce a url/headers block tagged `type: "http"`.
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
		out["type"] = "stdio"
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
		out["type"] = "http"
		out["url"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default:
		return nil
	}

	return out
}
