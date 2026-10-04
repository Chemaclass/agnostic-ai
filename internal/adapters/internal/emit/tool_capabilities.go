package emit

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type ToolCapability struct {
	Agent      []string
	Permission []string
}

type ToolCapabilityTable map[string]ToolCapability

func CapabilityToolKey(name string) (string, int) {
	switch name {
	case "Read":
		return "read", 0
	case "Write":
		return "write", 0
	case "Edit":
		return "edit", 0
	case "Delete":
		return "delete", 0
	case "Bash":
		return "shell", 0
	case "WebFetch":
		return "web", 0
	case "WebSearch":
		return "web", 1
	}
	return name, 0
}

func CapabilityTool(table ToolCapabilityTable, name string, permission bool) (string, bool) {
	key, index := CapabilityToolKey(name)
	row, ok := table[key]
	if !ok {
		return "", false
	}
	names := row.Agent
	if permission {
		names = row.Permission
	}
	if index >= len(names) || names[index] == "" {
		return "", false
	}
	return names[index], true
}

func TranslateCapabilityTools(table ToolCapabilityTable, names []string) (out []string, dropped bool) {
	for _, name := range names {
		translated, ok := CapabilityTool(table, name, false)
		if !ok {
			dropped = true
			continue
		}
		if !slices.Contains(out, translated) {
			out = append(out, translated)
		}
	}
	return out, dropped
}

func MCPAtName(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok || rest == "" {
		return "", false
	}
	server, tool, scoped := strings.Cut(rest, "__")
	if scoped {
		return "@" + server + "/" + tool, true
	}
	return "@" + server, true
}

func ReportWidening(target, path, line, mode string) error {
	message := fmt.Sprintf("%s: %s: %s", target, path, line)
	switch mode {
	case OnUnsupportedError:
		return fmt.Errorf("%s", message)
	case OnUnsupportedSilent:
		return nil
	default:
		NoteProject(message)
		return nil
	}
}

func NoteAgentToolsNoOp(target string, agents []spec.Entry, reason string) {
	counts := map[string]int{}
	for _, agent := range agents {
		field := "tools"
		if agent.CapabilityField != "" {
			field = agent.CapabilityField
		}
		counts[field]++
	}
	for _, field := range []string{"tools", "can"} {
		NoteFieldNoOp(target, spec.KindAgent, field, counts[field], reason)
	}
}

type CapabilityTranslation struct {
	Native    []string
	Widening  []string
	Supported bool
	Override  string
}
