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
		for _, key := range []string{"disabled", "cwd", "description"} {
			if value, ok := m[key]; ok {
				if key == "description" {
					emit.NoteFieldNoOp(target, spec.KindMCP, key, 1, "the native server schema has no description field")
					continue
				}
				if key == "disabled" && value == false {
					continue
				}
				return nil, fmt.Errorf("%s: codebuddy: MCP %s is unsupported; no server was enabled", e.Path, key)
			}
		}
		kind, _ := m["type"].(string)
		if raw, set := m["type"]; set {
			if _, ok := raw.(string); !ok {
				return nil, fmt.Errorf("%s: codebuddy: MCP type must be a string", e.Path)
			}
		}
		if kind == "" {
			if m["url"] != nil {
				kind = "http"
			} else {
				kind = "stdio"
			}
		}
		if !slices.Contains([]string{"stdio", "http", "sse"}, kind) {
			return nil, fmt.Errorf("%s: codebuddy: unsupported MCP transport %q", e.Path, kind)
		}
		var server map[string]any
		if kind == "stdio" {
			command, _ := m["command"].(string)
			if command == "" {
				return nil, fmt.Errorf("%s: codebuddy: stdio MCP requires command", e.Path)
			}
			server = selected(m, "command", "args", "env", "defer_loading", "tools")
		} else {
			url, _ := m["url"].(string)
			if url == "" {
				return nil, fmt.Errorf("%s: codebuddy: remote MCP requires url", e.Path)
			}
			server = selected(m, "url", "headers", "defer_loading", "tools")
		}
		server["type"] = kind
		out[e.Name] = server
	}
	return out, nil
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
