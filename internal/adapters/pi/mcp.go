package pi

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func mcpServer(entry spec.Entry) (map[string]any, error) {
	if entry.Name == "" || strings.IndexFunc(entry.Name, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) >= 0 {
		return nil, fmt.Errorf("pi: MCP %q: server names use only letters, digits, underscores, or hyphens", entry.Name)
	}
	meta := emit.ResolveMeta(entry.Meta, target)
	kind, _ := meta["type"].(string)
	if kind == "sse" {
		return nil, fmt.Errorf("pi: MCP %s: SSE is unsupported; use streamable HTTP", entry.Name)
	}
	if kind != "" && kind != "stdio" && kind != "http" && kind != "streamable-http" {
		return nil, fmt.Errorf("pi: MCP %s: unsupported transport %q", entry.Name, kind)
	}
	command, _ := meta["command"].(string)
	url, _ := meta["url"].(string)
	if command != "" && url != "" || kind == "stdio" && url != "" || (kind == "http" || kind == "streamable-http") && command != "" {
		return nil, fmt.Errorf("pi: MCP %s: transport conflicts with command or URL", entry.Name)
	}
	controls := IsControlOnlyMCP(entry)
	if command == "" && url == "" && !controls {
		return nil, fmt.Errorf("pi: MCP %s: a command or URL is required", entry.Name)
	}
	out := map[string]any{}
	keys := []string{"description", "timeout", "exposure", "toolExposure"}
	if command != "" {
		keys = append(keys, "command", "args", "env", "cwd")
	} else {
		keys = append(keys, "url", "headers", "oauth")
	}
	for _, key := range keys {
		if value, ok := meta[key]; ok {
			out[key] = value
		}
	}
	if kind != "" {
		out["type"] = kind
	}
	if disabled, ok := meta["disabled"].(bool); ok {
		out["enabled"] = !disabled
	}
	if enabled, ok := meta["enabled"].(bool); ok {
		out["enabled"] = enabled
	}
	return out, nil
}

func buildMCPServers(entries []spec.Entry) (map[string]any, error) {
	servers := map[string]any{}
	names := map[string]string{}
	for _, entry := range entries {
		server, err := mcpServer(entry)
		if err != nil {
			return nil, err
		}
		normalized := strings.ReplaceAll(entry.Name, "-", "_")
		if previous, exists := names[normalized]; exists {
			return nil, fmt.Errorf("pi: MCP %s and %s resolve to the same native server name", previous, entry.Name)
		}
		names[normalized] = entry.Name
		servers[entry.Name] = server
	}
	return servers, nil
}

func (Adapter) CheckedUserMCPServers(entries []spec.Entry) (map[string]any, error) {
	servers, err := buildMCPServers(entries)
	if err != nil {
		return nil, err
	}
	for name, raw := range servers {
		server, _ := raw.(map[string]any)
		if server["command"] == nil && server["url"] == nil {
			return nil, fmt.Errorf("pi: MCP %s: a user-level server needs a command or URL; control-only entries apply only in a project", name)
		}
	}
	return servers, nil
}

func (Adapter) MCPLaunchView() emit.MCPLaunchView { return emit.MCPLaunchView{Resolved: true} }
