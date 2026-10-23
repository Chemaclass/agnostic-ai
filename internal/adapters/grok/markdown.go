package grok

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var agentKeys = []string{"name", "description", "model", "effort", "maxTurns", "background", "tools", "disallowedTools", "skills", "permissionMode", "isolation", "color", "initialPrompt", "mcpServers", "hooks", "memory"}
var skillKeys = []string{"name", "description", "when-to-use", "paths", "allowed-tools", "argument-hint", "user-invocable", "disable-model-invocation", "metadata", "license", "compatibility"}

func selected(meta map[string]any, keys []string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if value, ok := meta[key]; ok {
			out[key] = value
		}
	}
	return out
}
func agentMeta(e spec.Entry) (map[string]any, error) {
	resolved := emit.ResolveMeta(e.Meta, target)
	custom, _ := e.Meta["x-grok"].(map[string]any)
	out := selected(resolved, agentKeys)
	if out["name"] == nil {
		out["name"] = e.Name
	}
	if out["description"] == nil {
		out["description"] = e.Name
	}
	if name, ok := out["name"].(string); !ok || spec.ValidateName(spec.KindAgent, name) != nil {
		return nil, fmt.Errorf("grok agent %s: invalid native name", e.Name)
	}
	if effort, ok := out["effort"]; ok && !slices.Contains([]string{"low", "medium", "high", "xhigh", "max"}, fmt.Sprint(effort)) {
		return nil, fmt.Errorf("grok agent %s: effort %q is unsupported; use low, medium, high, xhigh, or max", e.Name, effort)
	}
	if raw, ok := resolved["can"]; ok {
		tools := stringList(raw)
		if len(tools) == 0 {
			return nil, fmt.Errorf("grok agent %s: empty can list would inherit all tools", e.Name)
		}
		var native []string
		for _, tool := range tools {
			switch tool {
			case "read", "Read":
				native = append(native, "Read")
			case "shell", "Bash":
				native = append(native, "Bash")
			case "write", "Write":
				native = append(native, "Write")
			case "edit", "Edit":
				native = append(native, "Edit")
			case "web":
				native = append(native, "WebFetch", "WebSearch")
			case "WebFetch", "WebSearch":
				native = append(native, tool)
			default:
				return nil, fmt.Errorf("grok agent %s: capability %q has no exact native restriction", e.Name, tool)
			}
		}
		if custom["tools"] == nil {
			out["tools"] = native
		}
	}
	if raw, present := out["tools"]; present {
		tools := stringList(raw)
		if len(tools) == 0 {
			return nil, fmt.Errorf("grok agent %s: empty tools would inherit all tools", e.Name)
		}
		for i, tool := range tools {
			tool = strings.TrimSpace(tool)
			tools[i] = tool
			if !slices.Contains(nativeAllowlistTools, tool) {
				return nil, fmt.Errorf("grok agent %s: unknown tools entry %q would restore all tools; use a verified native alias such as Read or Bash", e.Name, tool)
			}
		}
		out["tools"] = tools
		if custom["tools"] == nil {
			deny := stringList(out["disallowedTools"])
			for _, bridge := range []string{"search_tool", "use_tool"} {
				if !slices.Contains(tools, bridge) && !slices.Contains(deny, bridge) {
					deny = append(deny, bridge)
				}
			}
			out["disallowedTools"] = deny
		}
	}
	if raw, present := out["disallowedTools"]; present {
		deny := stringList(raw)
		for i, tool := range deny {
			tool = strings.TrimSpace(tool)
			deny[i] = tool
			if !slices.Contains(nativeToolIDs, tool) {
				return nil, fmt.Errorf("grok agent %s: disallowedTools entry %q is not a verified native tool ID", e.Name, tool)
			}
		}
		out["disallowedTools"] = deny
	}

	return out, nil
}
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	var all []map[string]any
	for _, e := range agents {
		if err := spec.ValidateName(spec.KindAgent, e.Name); err != nil {
			return fmt.Errorf("grok agent %s: %w", e.Name, err)
		}
		meta, err := agentMeta(e)
		if err != nil {
			return err
		}
		all = append(all, meta)
	}
	for i, e := range agents {
		scoped, err := emit.ScopedSkillsDir(e.Scope, dir)
		if err != nil {
			return err
		}
		if err := sess.WriteFile(filepath.Join(scoped, e.Name+".md"), emit.WithHeader(emit.DocumentOrdered(all[i], agentKeys, e.Body, target), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}
func (Adapter) SkillMarkdown(e spec.Entry) string {
	meta := selected(emit.ResolveMeta(e.Meta, target), skillKeys)
	if meta["name"] == nil {
		meta["name"] = e.Name
	}
	if meta["description"] == nil {
		meta["description"] = e.Name
	}
	return emit.DocumentOrdered(meta, skillKeys, e.Body, target)
}
func commandMarkdown(e spec.Entry) string {
	meta := maps.Clone(emit.ResolveMeta(e.Meta, target))
	meta = selected(meta, []string{"description", "argument-hint", "allowed-tools", "user-invocable", "disable-model-invocation"})
	return emit.DocumentOrdered(meta, []string{"description", "argument-hint", "allowed-tools", "user-invocable", "disable-model-invocation"}, e.Body, target)
}

func stringList(v any) []string {
	if list, ok := v.([]string); ok {
		return list
	}
	if text, ok := v.(string); ok {
		return strings.FieldsFunc(text, func(r rune) bool { return r == ',' })
	}
	return emit.StringSlice(v)
}

var nativeAllowlistTools = []string{"Read", "Write", "Edit", "MultiEdit", "NotebookEdit", "Bash", "PowerShell", "Grep", "Glob", "LS", "LSP", "WebSearch", "WebFetch", "DeployApp", "TodoWrite", "AskUserQuestion", "TaskOutput", "BashOutput", "BashOutputTool", "AgentOutputTool", "TaskStop", "KillShell", "KillBash", "Skill", "ToolSearch", "read_file", "grep", "web_fetch", "GrokBuild:read_file", "GrokBuild:grep", "GrokBuild:web_fetch", "search_tool", "use_tool", "task", "get_task_output", "kill_task", "wait_tasks"}
var nativeToolIDs = []string{"read_file", "grep", "web_fetch", "GrokBuild:read_file", "GrokBuild:grep", "GrokBuild:web_fetch", "search_tool", "use_tool", "task", "get_task_output", "kill_task", "wait_tasks"}
