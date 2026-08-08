package antigravity

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitMCP writes .agents/mcp_config.json (or the outputs.antigravity.mcp-file
// override) with one entry per MCP spec under the root `mcpServers` key.
// No file is written when mcps is empty or every entry renders empty.
func emitMCP(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	doc, err := buildMCPDocument(mcps)
	if err != nil {
		return err
	}
	if doc == "" {
		return nil
	}
	return sess.WriteFile(path, doc, dryRun)
}

// buildMCPDocument renders the {"mcpServers": {...}} document. Entries
// missing their transport's required field render empty and are
// skipped, so a spec with nothing to run or connect to never produces a
// dead entry.
func buildMCPDocument(mcps []spec.Entry) (string, error) {
	servers := map[string]map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPServer(e)
		if len(entry) == 0 {
			continue
		}
		servers[e.Name] = entry
	}
	if len(servers) == 0 {
		return "", nil
	}
	raw, err := emit.MarshalJSONIndent(map[string]any{"mcpServers": servers})
	if err != nil {
		return "", fmt.Errorf("marshal antigravity mcp: %w", err)
	}
	return string(raw) + "\n", nil
}

// buildMCPServer renders one `mcpServers` entry with only the fields
// Antigravity's own doc confirms (antigravity.google/docs/mcp): stdio
// carries `command` plus optional `args` / `env` / `cwd`; remote
// transports carry `serverUrl` plus optional `headers`. Both transports
// accept `disabled` as its own boolean, the vendor's documented name for
// the field (unlike codex and kilo, which map the spec's `disabled` onto
// their own `enabled: false`). The vendor is explicit that the shared
// cross-tool `url` / `httpUrl` field names "are not supported," so this
// builder is deliberately its own schema rather than a case added to the
// shared emit.MCPSchemaServersMap, which still emits `url` for every
// other `mcpServers`-rooted target.
//
// `description`, `roots`, and a `type` discriminant remain unconfirmed
// for Antigravity and are intentionally omitted, the same standard the
// kilo adapter holds its own tool-name mapping to: no vendor-confirmed
// shape, no guess.
func buildMCPServer(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}
	switch transport {
	case "stdio":
		cmd, _ := e.Meta["command"].(string)
		if cmd == "" {
			return nil
		}
		out["command"] = cmd
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			out["args"] = args
		}
		if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
			out["env"] = env
		}
		if cwd, _ := e.Meta["cwd"].(string); cwd != "" {
			out["cwd"] = cwd
		}
	case "http", "sse", "ws":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["serverUrl"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default:
		return nil
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["disabled"] = true
	}
	return out
}
