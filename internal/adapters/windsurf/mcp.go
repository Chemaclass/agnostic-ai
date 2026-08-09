package windsurf

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitMCP writes .devin/mcp_config.json (default; override via
// outputs.windsurf.mcp-file). No file is written when mcps is empty or
// every entry renders empty. See the package doc for the confirmed
// schema.
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

// buildMCPDocument renders the {"mcpServers": {...}} document Devin
// Local's project-scoped MCP config file expects. Entries missing
// their transport's required field render empty and are skipped, so a
// spec with nothing to run or connect to never produces a dead entry.
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
		return "", fmt.Errorf("windsurf: marshal mcp: %w", err)
	}
	return string(raw) + "\n", nil
}

// buildMCPServer renders one `mcpServers` entry with the fields
// docs.devin.ai/cli/extensibility/mcp/configuration confirms for
// project scope (`.devin/mcp_config.json`). Local (stdio) carries
// `command` (required) plus optional `args` / `env`. Remote (Streamable
// HTTP by default, or legacy SSE) carries `url` (required) plus
// optional `transport` (`http` or `sse`), `headers`, `oauthClientId`,
// `oauthClientSecret`, and `oauthResource`. Both transports accept
// `disabled`: the CLI's own `devin mcp enable|disable` commands toggle
// this literal key on the same file, unlike the trae and antigravity
// adapters, where the same key is unconfirmed and stripped before it
// ever reaches the file (see StripMCPDisabled in the shared emit
// package).
//
// The shared emit.MCPSchemaServersMap builder always writes the
// transport discriminant under the key `type`. Devin's own field is
// spelled `transport`, so this adapter holds its own schema here
// rather than reuse that builder, the same reason trae and antigravity
// hold their own.
func buildMCPServer(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}
	switch transport {
	case "http", "sse", "ws":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["url"] = url
		out["transport"] = transport
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
		if id, _ := e.Meta["oauthClientId"].(string); id != "" {
			out["oauthClientId"] = id
		}
		if secret, _ := e.Meta["oauthClientSecret"].(string); secret != "" {
			out["oauthClientSecret"] = secret
		}
		// oauthResource has three vendor-documented states: unset
		// (default, the server URL), a non-empty override, and an
		// explicit "" that omits the OAuth resource parameter
		// entirely. The `ok` check (rather than a non-empty guard)
		// keeps that third state distinguishable from "not set".
		if resource, ok := e.Meta["oauthResource"].(string); ok {
			out["oauthResource"] = resource
		}
	default: // stdio
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
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["disabled"] = true
	}
	return out
}
