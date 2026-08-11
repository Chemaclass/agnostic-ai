package warp

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitMCP writes .warp/.mcp.json (default; override via
// outputs.warp.mcp-file). No file is written when mcps is empty or every
// entry renders empty. See buildMCPServer for the confirmed schema.
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
		return "", fmt.Errorf("warp: marshal mcp: %w", err)
	}
	return string(raw) + "\n", nil
}

// buildMCPServer renders one `mcpServers` entry. This adapter held the
// shared emit.MCPSchemaServersMapNoType builder until its own schema
// diverged too: docs.warp.dev/agents/capabilities/mcp's CLI Server
// (Command) table documents `command` (required), `args`, `env`, and
// `working_directory` ("Working directory path where the command is
// run, used for resolving relative paths"), the vendor's own name for
// the cross-tool spec's `cwd` field, which the shared builder has no
// field for at all. The doc adds: "Always set `working_directory`
// explicitly when your MCP server command or args include relative
// paths." (#606). The Streamable HTTP or SSE Server (URL) table
// documents only `url` and `headers`, so remote entries carry no
// `working_directory` and, as before, no `type` discriminant either: a
// single tab covers both transports, so the transport is never named in
// config (#592).
//
// `description`, `disabled`, and `roots` are not documented on that
// page but are kept here unchanged from the shared builder this adapter
// used before: this fix maps a confirmed field, not a re-audit of the
// rest.
func buildMCPServer(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}
	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			out["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			out["args"] = args
		}
		if cwd, _ := e.Meta["cwd"].(string); cwd != "" {
			out["working_directory"] = cwd
		}
	case "http", "sse", "ws":
		if url, _ := e.Meta["url"].(string); url != "" {
			out["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		out["env"] = env
	}
	if desc, _ := e.Meta["description"].(string); desc != "" {
		out["description"] = desc
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["disabled"] = true
	}
	if roots := emit.BuildRoots(e.Meta); len(roots) > 0 {
		out["roots"] = roots
	}
	return out
}
