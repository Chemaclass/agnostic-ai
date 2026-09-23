package copilot

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// EmitAgents writes one native Copilot custom-agent profile per agent
// spec at `<agentsDir>/<name>.agent.md`. Copilot (cloud agent and VS
// Code) discovers agent profiles under `.github/agents/`; the
// frontmatter carries `name`, `description` (required), and the
// optional `tools` and `model` keys, with the prompt as the body.
// Arbitrary `x-copilot` keys (target, user-invocable, mcp-servers, ...)
// pass through for the rest of the documented schema. The profile table
// has no effort key, and per-agent `effortLevel` lives only in the
// user-tier `subagents.agents` setting, so a portable `effort` raises a
// coverage note instead of reaching a file (#1066).
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	droppedEffort := 0
	for _, a := range agents {
		body, dropped := agentMarkdown(a)
		if dropped {
			droppedEffort++
		}
		path := filepath.Join(dir, a.Name+agentFileSuffix)
		if err := sess.WriteFile(path, emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "effort", droppedEffort,
		"Copilot agent profiles have no effort key; set a per-agent effortLevel under subagents.agents in ~/.copilot/settings.json")
	return nil
}

// agentMarkdown renders one profile and reports whether a portable
// `effort` was dropped. An explicit `x-copilot.effort` passes through like
// any other x-copilot key, so it is written, not dropped.
func agentMarkdown(e spec.Entry) (string, bool) {
	resolved := emit.ResolveMeta(e.Meta, target)
	custom, _ := e.Meta[emit.XPrefix+target].(map[string]any)
	_, explicit := custom["effort"]
	effort := resolved["effort"]
	dropped := !explicit && effort != nil && effort != ""
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	meta := map[string]any{
		"name":        e.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	if tools := emit.StringSlice(resolved["tools"]); len(tools) > 0 {
		meta["tools"] = tools
		keys = append(keys, "tools")
	}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, "name", "description", "tools", "model")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return front + "\n", dropped
	}
	return front + "\n" + body + "\n", dropped
}
