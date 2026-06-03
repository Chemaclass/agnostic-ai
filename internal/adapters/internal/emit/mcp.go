package emit

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// MCPSchema picks the JSON shape used by the target tool.
type MCPSchema int

const (
	// MCPSchemaServersMap wraps entries under {"mcpServers": {...}}.
	// Used by Claude Code (.mcp.json) and Cursor (.cursor/mcp.json).
	MCPSchemaServersMap MCPSchema = iota
	// MCPSchemaVSCodeServers wraps entries under {"servers": {...}} and
	// requires a `type` field per server. Used by VS Code / GitHub Copilot.
	MCPSchemaVSCodeServers
)

// WriteMCPFile renders mcps with the target schema and writes to path.
// No file is written when mcps is empty or every entry produces an empty
// server block. Adapters call this in lieu of hand-rolling the
// guard / render / write triple at every call site.
func WriteMCPFile(mcps []spec.Entry, schema MCPSchema, path string, dryRun bool) error {
	if len(mcps) == 0 {
		return nil
	}
	doc, err := MCPDocument(mcps, schema)
	if err != nil {
		return err
	}
	if doc == "" {
		return nil
	}
	return WriteFile(path, doc, dryRun)
}

// MCPDocument renders an MCP server config file from bundle MCP entries.
//
// Each entry's frontmatter accepts: type (stdio|http|sse, default stdio),
// command, args, env, url, headers, description, disabled, roots.
// Empty entries are skipped.
func MCPDocument(mcps []spec.Entry, schema MCPSchema) (string, error) {
	servers := map[string]map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildServer(e, schema)
		if len(entry) == 0 {
			continue
		}
		servers[e.Name] = entry
	}
	if len(servers) == 0 {
		return "", nil
	}
	root := map[string]any{}
	switch schema {
	case MCPSchemaVSCodeServers:
		root["servers"] = servers
	default:
		root["mcpServers"] = servers
	}
	raw, err := MarshalJSONIndent(root)
	if err != nil {
		return "", fmt.Errorf("marshal mcp: %w", err)
	}
	return string(raw) + "\n", nil
}

func buildServer(e spec.Entry, schema MCPSchema) map[string]any {
	transport := stringField(e.Meta, "type")
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}

	switch transport {
	case "stdio":
		if cmd := stringField(e.Meta, "command"); cmd != "" {
			out["command"] = cmd
		}
		if args := stringSlice(e.Meta, "args"); len(args) > 0 {
			out["args"] = args
		}
	case "http", "sse":
		if url := stringField(e.Meta, "url"); url != "" {
			out["url"] = url
		}
		if h := mapField(e.Meta, "headers"); len(h) > 0 {
			out["headers"] = h
		}
		// Remote servers carry an explicit `type` in every schema. A
		// `.mcp.json` entry with only `url` is ambiguous (http vs sse);
		// stdio stays type-less since it is the inferred default.
		out["type"] = transport
	}

	if env := mapField(e.Meta, "env"); len(env) > 0 {
		out["env"] = env
	}
	if desc := stringField(e.Meta, "description"); desc != "" {
		out["description"] = desc
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["disabled"] = true
	}
	if roots := buildRoots(e.Meta); len(roots) > 0 {
		out["roots"] = roots
	}
	if schema == MCPSchemaVSCodeServers {
		out["type"] = transport
	}
	return out
}

// buildRoots constructs the `roots` array from spec meta. Each element is
// a map with at least a `uri` key; `name` is optional.
func buildRoots(meta map[string]any) []map[string]any {
	raw, _ := meta["roots"].([]any)
	if len(raw) == 0 {
		return nil
	}
	roots := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]any)
		if m == nil {
			continue
		}
		entry := map[string]any{}
		if uri, _ := m["uri"].(string); uri != "" {
			entry["uri"] = uri
		}
		if name, _ := m["name"].(string); name != "" {
			entry["name"] = name
		}
		if len(entry) > 0 {
			roots = append(roots, entry)
		}
	}
	return roots
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func stringSlice(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapField(m map[string]any, key string) map[string]string {
	raw, ok := m[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
