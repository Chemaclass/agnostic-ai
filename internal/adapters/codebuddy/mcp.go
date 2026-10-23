package codebuddy

import (
	"fmt"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func mcpServers(mcps []spec.Entry) (map[string]any, error) {
	out := map[string]any{}
	for _, e := range mcps {
		m := emit.ResolveMeta(e.Meta, target)
		if value, set := m["disabled"]; set && value != false {
			return nil, fmt.Errorf("%s: codebuddy: MCP disabled is unsupported; no server was enabled", e.Path)
		}
		kind, err := MCPType(m)
		if err != nil {
			return nil, fmt.Errorf("%s: codebuddy: %w", e.Path, err)
		}
		var server map[string]any
		if kind == "stdio" {
			command, _ := m["command"].(string)
			if command == "" {
				return nil, fmt.Errorf("%s: codebuddy: stdio MCP requires command", e.Path)
			}
			if value, set := m["cwd"]; set {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("%s: codebuddy: MCP cwd must be a string", e.Path)
				}
			}
			server = selected(m, "command", "args", "env", "cwd", "description", "defer_loading", "tools")
		} else {
			if _, set := m["cwd"]; set {
				return nil, fmt.Errorf("%s: codebuddy: MCP cwd applies to stdio servers only", e.Path)
			}
			url, _ := m["url"].(string)
			if url == "" {
				return nil, fmt.Errorf("%s: codebuddy: remote MCP requires url", e.Path)
			}
			server = selected(m, "url", "headers", "description", "defer_loading", "tools")
		}
		server["type"] = kind
		out[e.Name] = server
	}
	return out, nil
}

func MCPType(meta map[string]any) (string, error) {
	kind, _ := meta["type"].(string)
	if raw, set := meta["type"]; set {
		if _, ok := raw.(string); !ok {
			return "", fmt.Errorf("MCP type must be a string")
		}
	}
	if kind == "" {
		if command, _ := meta["command"].(string); command != "" {
			kind = "stdio"
		} else if url, _ := meta["url"].(string); url != "" {
			kind = "http"
		} else {
			kind = "stdio"
		}
	}
	if !slices.Contains([]string{"stdio", "http", "sse"}, kind) {
		return "", fmt.Errorf("MCP type must be stdio, http, or sse")
	}
	return kind, nil
}
func (Adapter) UserMCPServers(mcps []spec.Entry) map[string]any {
	out, err := mcpServers(mcps)
	if err != nil {
		emit.NoteProject(err.Error())
		return map[string]any{}
	}
	return out
}

func ValidateMCP(mcps []spec.Entry) error { _, err := mcpServers(mcps); return err }
