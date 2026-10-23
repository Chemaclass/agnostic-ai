package qwen

import (
	"path/filepath"
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

func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		a = emit.WithValidAgentLimits(a, target, emit.AgentMaxTurns)
		resolved := emit.ResolveMeta(a.Meta, target)
		description, _ := resolved["description"].(string)
		if description == "" {
			description = a.Name
		}
		meta := map[string]any{"name": a.Name, "description": description}
		keys := []string{"name", "description"}
		for _, key := range []string{"model", "color", "maxTurns"} {
			if value, ok := resolved[key]; ok {
				meta[key] = value
				keys = append(keys, key)
			}
		}
		native, _ := emit.CustomTargetMeta(a.Meta, target)
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
			emit.NoteFieldNoOp(target, spec.KindAgent, "permissionMode", 1, "use x-qwen.approvalMode; native modes have different approval semantics")
		}
		emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description", "model", "color", "maxTurns", "effort")
		body := emit.WithHeader(emit.FrontmatterOrdered(meta, keys)+"\n"+strings.TrimSpace(a.Body)+"\n", emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, a.Name+".md"), body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func (Adapter) TranslatePermission(_ string, rule string) ([]string, bool) {
	if rule == "Delete" || strings.HasPrefix(rule, "Delete(") {
		return nil, false
	}
	return []string{rule}, true
}
