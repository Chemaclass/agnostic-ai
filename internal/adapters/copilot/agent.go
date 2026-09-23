package copilot

import (
	"fmt"
	"path/filepath"
	"slices"
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
// user-tier `subagents.agents` setting. Project sync raises a coverage
// note for a portable `effort`; a user-tier session leaves every
// supported level to AgentEffortLevels and notes only the rest.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	droppedEffort := 0
	for _, a := range agents {
		body, dropped := agentMarkdown(a)
		if dropped && (!sess.UserTier() || !supportedEffort(a)) {
			droppedEffort++
		}
		path := filepath.Join(dir, a.Name+agentFileSuffix)
		if err := sess.WriteFile(path, emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "effort", droppedEffort,
		"Copilot agent profiles have no effort key; set a per-agent effortLevel of low, medium, high, or xhigh under subagents.agents in ~/.copilot/settings.json")
	return nil
}

// effortLevels are the values Copilot documents for effortLevel.
var effortLevels = []string{"low", "medium", "high", "xhigh"}

// AgentEffortLevels maps each agent name to the portable effort that
// belongs in `subagents.agents.<name>.effortLevel`, skipping agents with
// an explicit x-copilot.effort and values Copilot does not accept.
func (Adapter) AgentEffortLevels(agents []spec.Entry) map[string]string {
	levels := map[string]string{}
	for _, a := range agents {
		if supportedEffort(a) {
			levels[a.Name], _ = droppedEffort(a)
		}
	}
	return levels
}

func supportedEffort(e spec.Entry) bool {
	level, dropped := droppedEffort(e)
	return dropped && slices.Contains(effortLevels, level)
}

// droppedEffort returns the portable effort a profile cannot carry. An
// explicit x-copilot.effort passes through like any other x-copilot key,
// so it is written, not dropped.
func droppedEffort(e spec.Entry) (string, bool) {
	custom, _ := e.Meta[emit.XPrefix+target].(map[string]any)
	if _, explicit := custom["effort"]; explicit {
		return "", false
	}
	effort := emit.ResolveMeta(e.Meta, target)["effort"]
	if effort == nil || effort == "" {
		return "", false
	}
	return fmt.Sprint(effort), true
}

// agentMarkdown renders one profile and reports whether a portable
// `effort` was dropped.
func agentMarkdown(e spec.Entry) (string, bool) {
	resolved := emit.ResolveMeta(e.Meta, target)
	_, dropped := droppedEffort(e)
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
