package qwen

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var toolCapabilities = emit.ToolCapabilityTable{
	"read":  {Agent: []string{"read_file"}, Permission: []string{"Read"}},
	"write": {Agent: []string{"write_file"}, Permission: []string{"Write"}},
	"edit":  {Agent: []string{"edit"}, Permission: []string{"Edit"}},
	"Glob":  {Agent: []string{"glob"}, Permission: []string{"Glob"}},
	"Grep":  {Agent: []string{"grep_search"}, Permission: []string{"Grep"}},
	"shell": {Agent: []string{"run_shell_command"}, Permission: []string{"Bash"}},
	"web":   {Agent: []string{"web_fetch", "web_search"}, Permission: []string{"WebFetch", "WebSearch"}},
}

func (Adapter) ToolCapabilities() emit.ToolCapabilityTable { return toolCapabilities }

// agentFields are the portable agent keys Qwen Code reads as written.
// Its subagents take Claude Code's frontmatter, including per-agent
// `hooks` keyed by Claude event names
// (qwenlm.github.io/qwen-code-docs/en/users/features/sub-agents/).
var agentFields = []string{"model", "color", "maxTurns", "background", "hooks"}

func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	permissionModes := 0
	for _, a := range agents {
		a = emit.WithValidAgentLimits(a, target, emit.AgentMaxTurns, emit.AgentBackground)
		resolved := emit.ResolveMeta(a.Meta, target)
		description, _ := resolved["description"].(string)
		if description == "" {
			description = a.Name
		}
		meta := map[string]any{"name": a.Name, "description": description}
		keys := []string{"name", "description"}
		native, _ := emit.CustomTargetMeta(a.Meta, target)
		for _, key := range agentFields {
			if value, ok := resolved[key]; ok {
				meta[key] = value
				keys = append(keys, key)
			}
		}
		if _, override := native["hooks"]; !override {
			if hooks, ok := meta["hooks"].(map[string]any); ok {
				hooks = agentStopAsSubagentStop(hooks)
				meta["hooks"] = hooks
				for event := range hooks {
					if !slices.Contains(agentHookEvents(), event) {
						emit.NoteFieldNoOp(target, spec.KindAgent, "hooks", 1, "Qwen skips agent hook event "+event)
					}
				}
			}
		}
		if _, override := native["tools"]; !override {
			names := emit.StringSlice(a.Meta["tools"])
			if len(names) > 0 {
				var tools []string
				for _, name := range names {
					if value, ok := emit.CapabilityTool(toolCapabilities, name, false); ok {
						tools = append(tools, value)
					} else {
						tools = append(tools, name)
					}
				}
				// Qwen treats an empty list as inherit-all, so unresolved names stay restrictive.
				meta["tools"] = tools
				keys = append(keys, "tools")
			}
		}
		if _, present := a.Meta["permissionMode"]; present {
			permissionModes++
		}
		emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, append([]string{"name", "description", "effort"}, agentFields...)...)
		body := emit.WithHeader(emit.FrontmatterOrdered(meta, keys)+"\n"+strings.TrimSpace(a.Body)+"\n", emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, a.Name+".md"), body, dryRun); err != nil {
			return err
		}
	}
	// Noted here rather than through caps, since sync --global writes
	// agents without the project coverage report.
	emit.NoteFieldNoOp(target, spec.KindAgent, "permissionMode", permissionModes, "use x-qwen.approvalMode; native modes have different approval semantics")
	return nil
}

func (Adapter) TranslatePermission(_ string, rule string) ([]string, bool) {
	if rule == "Delete" || strings.HasPrefix(rule, "Delete(") {
		return nil, false
	}
	return []string{rule}, true
}

// agentStopAsSubagentStop moves a shared agent `Stop` hook to
// `SubagentStop`, the event Claude Code runs it on: Qwen "Stop is not
// automatically remapped" (qwenlm.github.io/qwen-code-docs/en/users/features/hooks/).
func agentStopAsSubagentStop(hooks map[string]any) map[string]any {
	stop, ok := hooks["Stop"].([]any)
	if !ok {
		return hooks
	}
	out := make(map[string]any, len(hooks))
	for event, groups := range hooks {
		if event != "Stop" {
			out[event] = groups
		}
	}
	existing, _ := out["SubagentStop"].([]any)
	out["SubagentStop"] = append(append([]any(nil), existing...), stop...)
	return out
}
