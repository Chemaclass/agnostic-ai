// Package zed emits configs for the Zed editor's agent.
//
// Zed 1.4.2 retired its rules library in favor of Agent Skills: one
// folder per skill at `.agents/skills/<name>/SKILL.md` (the cross-tool
// path Codex and Amp share) with bundled assets. Always-on project
// instructions live in the root `AGENTS.md`, which is written centrally
// by `sync` as a slim pointer body with rule bodies inlined. When
// `outputs.zed.rules-file` is set, this adapter instead writes the
// legacy merged `.rules`-style document at that path so users on older
// Zed versions keep their behavior.
//
// MCP servers are configured in `.zed/settings.json` under the
// `context_servers` key (note: not `mcpServers`), with a flat
// `command`/`args`/`env` shape for stdio and a native `url`/`headers`
// shape for remote (HTTP / SSE) servers.
//
// When `outputs.zed.tasks-file` is set, hook specs additionally emit
// as Zed Tasks (https://zed.dev/docs/tasks): one entry per hook in the
// configured tasks JSON, runnable on demand from the command palette
// (Zed has no lifecycle-hook surface).
package zed

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "zed"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = ".zed/settings.json"
	zedMCPKey        = "context_servers"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindHook},
}

// Adapter emits Zed configs.
type Adapter struct{}

// New returns a Zed adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one native skill folder per skill under .agents/skills/,
// the legacy merged rules document when opted in via
// outputs.zed.rules-file, Zed Tasks when outputs.zed.tasks-file is set,
// and `.zed/settings.json` with the `context_servers` map when MCP
// entries exist. The root AGENTS.md entry-point (with rule bodies
// inlined) is written by `sync`, not here.
func (Adapter) Emit(b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	for _, s := range b.Skills {
		if err := emitSkill(s, skillsDir, dryRun); err != nil {
			return err
		}
	}
	// Agents have no per-agent surface in current Zed; only the legacy
	// merged document carries their bodies.
	if emit.OutputRulesFile(cfg, target, "") == "" {
		emit.NoteCoverageGap(target, spec.KindAgent, len(b.Agents), "outputs.zed.rules-file")
	}
	if err := emit.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{
		Title:              "Project rules",
		AgentSectionPrefix: "Agent: ",
	}, dryRun); err != nil {
		return err
	}
	if err := emitTasks(b.HooksFor(target), emit.OutputTasksFile(cfg, target, ""), dryRun); err != nil {
		return err
	}
	return emitContextServers(b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitTasks writes one Zed Task per hook spec into the configured
// tasks JSON file. No-op when the tasks-file is unset, so existing
// setups are unaffected. Each hook becomes a `sh -c "<command>"` task
// whose label is the hook's name; the description (when present)
// prefixes the label so it shows up in the command palette.
func emitTasks(hooks []spec.Entry, path string, dryRun bool) error {
	if path == "" {
		emit.NoteCoverageGap(target, spec.KindHook, len(hooks), "outputs.zed.tasks-file")
		return nil
	}
	if len(hooks) == 0 {
		return nil
	}
	tasks := make([]map[string]any, 0, len(hooks))
	for _, h := range hooks {
		cmd, _ := h.Meta["command"].(string)
		if cmd == "" {
			continue
		}
		label := h.Name
		if d := h.Description(); d != "" {
			label = h.Name + " — " + d
		}
		tasks = append(tasks, map[string]any{
			"label":   label,
			"command": "sh",
			"args":    []string{"-c", cmd},
		})
	}
	if len(tasks) == 0 {
		return nil
	}
	raw, err := emit.MarshalJSONIndent(tasks)
	if err != nil {
		return fmt.Errorf("marshal zed tasks: %w", err)
	}
	return emit.WriteFile(path, string(raw)+"\n", dryRun)
}

// emitContextServers writes (or merges into) .zed/settings.json with
// the `context_servers` map. Routes through emit.MergeJSONFile so any
// pre-existing user-managed Zed settings survive the sync; only
// `context_servers` is overwritten.
func emitContextServers(mcps []spec.Entry, path string, dryRun bool) error {
	servers := buildContextServers(mcps)
	if len(servers) == 0 {
		return nil
	}
	return emit.MergeJSONFile(path, map[string]any{zedMCPKey: servers}, dryRun)
}

func buildContextServers(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildContextServer(e)
		if entry == nil {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

// buildContextServer renders one Zed context_server entry. Stdio specs
// produce a flat `command`/`args`/`env` block; HTTP / SSE specs produce
// a native `url`/`headers` block. Both shapes match Zed's current
// `context_servers` schema (https://zed.dev/docs/ai/mcp).
func buildContextServer(e spec.Entry) map[string]any {
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
	case "http", "sse":
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
