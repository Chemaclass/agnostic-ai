package claude

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// allowlistTools take a pattern Claude Code reads in an agent's `tools`:
// "use Agent(agent_type) syntax in the tools field", with Task as its
// older name. Every other pattern there is read as the bare tool. A
// live check on Claude Code 2.1.303 (#2325) found an agent with
// Bash(git diff *) ran git status under bypassPermissions, and one with
// Read(src/**) or Edit(src/**) read and edited files outside src/.
var allowlistTools = []string{"Agent", "Task"}

// widenedAgentTools returns the agent's `tools` with each scoped entry
// written as the tool Claude Code grants, plus one line per entry naming
// the widening. An `x-claude.tools` list is the agent's own native
// choice and is left alone.
func widenedAgentTools(a spec.Entry) (any, []string) {
	if custom, _ := a.Meta[emit.XPrefix+target].(map[string]any); custom != nil {
		if _, native := custom["tools"]; native {
			return nil, nil
		}
	}
	raw := a.Meta["tools"]
	var names []string
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, nil
			}
			names = append(names, s)
		}
	case string:
		names = spec.SplitToolList(v)
	default:
		return nil, nil
	}
	var out, lines []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		tool, ok := scopedTool(name)
		if ok {
			lines = append(lines, widenedLine(a, name, tool))
			name = tool
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	if _, scalar := raw.(string); scalar {
		return strings.Join(out, ", "), lines
	}
	items := make([]any, len(out))
	for i, name := range out {
		items[i] = name
	}
	return items, lines
}

// scopedTool returns the tool a scoped entry such as Bash(git diff *)
// names, or false for a bare name or an allowlist tool.
func scopedTool(name string) (string, bool) {
	tool, _, scoped := strings.Cut(name, "(")
	if !scoped || !strings.HasSuffix(name, ")") || slices.Contains(allowlistTools, tool) {
		return "", false
	}
	return tool, true
}

func widenedLine(a spec.Entry, name, tool string) string {
	label := name
	if a.CapabilityField != "" {
		if c, ok := spec.NeutralCapability(name); ok {
			label = c
		}
	}
	return fmt.Sprintf("%s becomes %s; Claude Code does not limit an agent's tools to a pattern", label, tool)
}

// CapabilityWidening names the access a scoped capability gains on
// Claude Code, for explain.
func (Adapter) CapabilityWidening(names []string) []string {
	var out []string
	for _, name := range names {
		if tool, ok := scopedTool(name); ok {
			label := name
			if c, neutral := spec.NeutralCapability(name); neutral {
				label = c
			}
			out = append(out, fmt.Sprintf("%s becomes %s; Claude Code does not limit an agent's tools to a pattern", label, tool))
		}
	}
	return out
}

// reportWidenedTools prints, or fails on under on-unsupported: error,
// each scoped agent capability Claude Code reads as the whole tool.
func reportWidenedTools(agents []spec.Entry, mode string) error {
	for _, a := range agents {
		_, lines := widenedAgentTools(a)
		for _, line := range lines {
			if err := emit.ReportWidening(target, a.Path, line, mode); err != nil {
				return err
			}
		}
	}
	return nil
}

// WidenedAgentTools returns the `tools` list sync writes for a, as written
// in its spec, when a scoped capability widens it, or false when nothing
// widens.
func WidenedAgentTools(a spec.Entry) ([]string, bool) {
	a, problem := a.NativeTools()
	if problem != "" {
		return nil, false
	}
	tools, lines := widenedAgentTools(a)
	if lines == nil {
		return nil, false
	}
	names := emit.StringSlice(tools)
	if s, ok := tools.(string); ok {
		names = spec.SplitToolList(s)
	}
	// Sync drops Delete, which has no Claude Code tool, before writing.
	return slices.DeleteFunc(names, func(name string) bool { return name == "Delete" }), true
}

// ReportAgentWidening reports each scoped agent capability for user-level
// sync, under the home's on-unsupported mode.
func (Adapter) ReportAgentWidening(agents []spec.Entry, mode string) error {
	return reportWidenedTools(agents, mode)
}

// TranslateAgentCapability names the tool Claude Code grants an agent for
// one Claude-style name: a scoped entry is the bare tool. Delete has no
// Claude Code tool.
func (Adapter) TranslateAgentCapability(name string) ([]string, bool) {
	if tool, ok := scopedTool(name); ok {
		return []string{tool}, true
	}
	return []string{name}, name != "Delete"
}
