package spec

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// Capabilities are the neutral names accepted by tool and permission lists.
var Capabilities = []string{"read", "write", "edit", "delete", "shell", "web"}

// Delete is an internal token; adapters map it only where a native tool exists.
var capabilityTools = map[string][]string{
	"read":   {"Read"},
	"write":  {"Write"},
	"edit":   {"Edit"},
	"delete": {"Delete"},
	"shell":  {"Bash"},
	"web":    {"WebFetch", "WebSearch"},
}

const (
	capabilityKey = "can"
	toolsKey      = "tools"
	shellTool     = "Bash"
	mcpToolPrefix = "mcp__"
)

var mcpToolName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// AgentCapabilityProblem returns why an agent's `can:` cannot be read,
// or "" when it is valid or unset.
func AgentCapabilityProblem(meta map[string]any) string {
	for _, key := range slices.Sorted(maps.Keys(meta)) {
		if custom, ok := meta[key].(map[string]any); ok && strings.HasPrefix(key, "x-") {
			if _, set := custom[capabilityKey]; set {
				return fmt.Sprintf("%s.can is not read; write can: at the top level, or %s.tools with the tool's own names", key, key)
			}
		}
	}
	raw, set := meta[capabilityKey]
	if !set {
		return ""
	}
	if _, both := meta[toolsKey]; both {
		return "sets both can: and tools:; keep one"
	}
	_, problem := capabilityToolNames(raw)
	return problem
}

// CapabilityTools returns the Claude-style tool names a `can:` list
// stands for, or why it cannot be read. A Claude-style name passes
// through as an alias, so a list may mix both forms.
func CapabilityTools(can []string) ([]string, string) {
	var out []string
	for _, c := range can {
		names, problem := capabilityToolsOf(c, agentForm)
		if problem != "" {
			return nil, problem
		}
		out = append(out, names...)
	}
	return out, ""
}

func capabilityToolNames(raw any) ([]string, string) {
	list, ok := raw.([]any)
	if !ok {
		return nil, "can: must be a list, such as [read, shell(git diff *)]"
	}
	can := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Sprintf("can: entry %v is not a capability name", item)
		}
		can = append(can, s)
	}
	return CapabilityTools(can)
}

type capabilityForm struct {
	field string
	paths bool
}

var agentForm = capabilityForm{field: "can:", paths: true}

// pathTools are the Claude Code rules a path pattern scopes in a
// permission list. Claude Code checks file writes against Edit(path)
// rules only and never consults a Write(path) rule, so write takes no
// path: Edit(path) would also grant or deny edits.
var pathTools = map[string]string{"read": "Read", "edit": "Edit"}

func capabilityToolsOf(c string, form capabilityForm) ([]string, string) {
	if names, ok := capabilityTools[c]; ok {
		return slices.Clone(names), ""
	}
	if name, pattern, scoped := strings.Cut(c, "("); scoped && (name == "shell" || form.paths && pathTools[name] != "") {
		pattern, closed := strings.CutSuffix(pattern, ")")
		if !closed {
			return nil, fmt.Sprintf("%s %q is missing its closing parenthesis", form.field, c)
		}
		if strings.TrimSpace(pattern) == "" && name == "shell" {
			return nil, fmt.Sprintf("%s %q needs a command pattern; write shell for every command", form.field, c)
		}
		if strings.TrimSpace(pattern) == "" {
			return nil, fmt.Sprintf("%s %q needs a path pattern; write %s for every file", form.field, c, name)
		}
		tool := shellTool
		if name != "shell" {
			tool = pathTools[name]
		}
		return []string{tool + "(" + pattern + ")"}, ""
	}
	if rest, ok := strings.CutPrefix(c, mcpToolKind); ok {
		server, tool, hasTool := strings.Cut(rest, "/")
		if !mcpServerName.MatchString(server) || (hasTool && !mcpToolName.MatchString(tool)) {
			return nil, fmt.Sprintf("%s %q needs an MCP server and tool name of letters, digits, _, or -", form.field, c)
		}
		if hasTool {
			return []string{mcpToolPrefix + server + "__" + tool}, ""
		}
		return []string{mcpToolPrefix + server}, ""
	}
	if isClaudeToolAlias(c) {
		return []string{c}, ""
	}
	if name, _, scoped := strings.Cut(c, "("); scoped && slices.Contains(Capabilities, name) {
		if !form.paths {
			return nil, fmt.Sprintf("%s %q: only shell takes a pattern", form.field, c)
		}
		if name == "write" {
			return nil, fmt.Sprintf("%s %q: Claude Code checks file writes against Edit rules only, so write takes no path; write edit(<path>), which also covers edits", form.field, c)
		}
		return nil, fmt.Sprintf("%s %q: only shell, read, and edit take a pattern", form.field, c)
	}
	if s := suggest.Name(c, Capabilities); s != "" {
		return nil, fmt.Sprintf("unknown capability %q for %s (did you mean %s?)", c, form.field, s)
	}
	return nil, fmt.Sprintf("unknown capability %q for %s; use one of %s, shell(<pattern>), mcp:<server>, or a Claude Code tool name", c, form.field, strings.Join(Capabilities, ", "))
}

// isClaudeToolAlias reports whether name reads as a Claude Code tool
// name, such as Grep, Bash(git diff *), or mcp__github__get_issue.
func isClaudeToolAlias(name string) bool {
	return name != "" && (name[0] >= 'A' && name[0] <= 'Z' || strings.HasPrefix(name, mcpToolPrefix))
}

// NeutralCapability returns the capability a Claude-style tool name
// stands for alone, or false when no capability maps to it one to one.
// CapabilityTools turns the result back into exactly tool.
func NeutralCapability(tool string) (string, bool) {
	for name, native := range pathTools {
		if pattern, ok := strings.CutPrefix(tool, native+"("); ok {
			if p, closed := strings.CutSuffix(pattern, ")"); closed && strings.TrimSpace(p) != "" {
				return name + "(" + p + ")", true
			}
			return "", false
		}
	}
	for _, c := range Capabilities {
		if names := capabilityTools[c]; len(names) == 1 && names[0] == tool {
			return c, true
		}
	}
	if pattern, ok := strings.CutPrefix(tool, shellTool+"("); ok {
		if p, closed := strings.CutSuffix(pattern, ")"); closed && strings.TrimSpace(p) != "" {
			return "shell(" + p + ")", true
		}
		return "", false
	}
	rest, ok := strings.CutPrefix(tool, mcpToolPrefix)
	if !ok {
		return "", false
	}
	server, name, hasTool := strings.Cut(rest, "__")
	if !mcpServerName.MatchString(server) || hasTool && (!mcpToolName.MatchString(name) || strings.Contains(name, "__")) {
		return "", false
	}
	if hasTool {
		return mcpToolKind + server + "/" + name, true
	}
	return mcpToolKind + server, true
}

func (e Entry) NativeTools() (Entry, string) {
	if e.Kind != KindAgent {
		return e, ""
	}
	if problem := AgentCapabilityProblem(e.Meta); problem != "" {
		return e, problem
	}
	raw, set := e.Meta[capabilityKey]
	if !set {
		return e, ""
	}
	names, _ := capabilityToolNames(raw)
	tools := make([]any, len(names))
	for i, n := range names {
		tools[i] = n
	}
	e.CapabilityField = capabilityKey
	meta := maps.Clone(e.Meta)
	delete(meta, capabilityKey)
	meta[toolsKey] = tools
	e.Meta = meta
	if e.MetaKeys != nil {
		keys := slices.Clone(e.MetaKeys)
		if i := slices.Index(keys, capabilityKey); i >= 0 {
			keys[i] = toolsKey
		}
		e.MetaKeys = keys
	}
	if _, styled := e.MetaStyles[capabilityKey]; styled {
		styles := maps.Clone(e.MetaStyles)
		delete(styles, capabilityKey)
		e.MetaStyles = styles
	}
	return e, ""
}
