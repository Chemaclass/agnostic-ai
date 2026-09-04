package warp

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitMCP writes .warp/.mcp.json (default; override via
// outputs.warp.mcp-file). No file is written when mcps is empty or every
// entry renders empty. See buildMCPServer for the confirmed schema.
// mcpDisabledNoOpReason explains, in the flushed coverage note, why
// `disabled: true` on an MCP spec never reaches `.warp/.mcp.json`:
// docs.warp.dev/agents/capabilities/mcp publishes two closed property
// tables (CLI Server and URL Server) and neither has a disable key.
// The same page removes most of the need for one: "Project-scoped
// servers never auto-spawn - Warp detects project-scoped MCP config
// files in cloned repos, but requires you to start each server
// manually."
const mcpDisabledNoOpReason = "no file-based way to pre-disable a project-scoped MCP server; project-scoped servers never auto-spawn, so toggle the server on from the MCP servers page when you want it"

func emitMCP(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	mcps = emit.StripMCPDisabled(target, mcps, mcpDisabledNoOpReason)
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
// `description`, `disabled`, and `roots` appear in neither table and
// no longer emit (target-audit 2026-08-27, #641). They were inherited
// from the shared builder this adapter used before it grew its own, and
// carried through the #606 split unexamined. Warp's two tables are
// closed lists, so writing a key from outside them asserts vendor
// support that no vendor sentence backs. `disabled` surfaces a coverage
// note instead of vanishing (see mcpDisabledNoOpReason); `description`
// and `roots` are pure documentation on the spec side and reach the
// file through `x-warp` for anyone who wants them written anyway, the
// same escape hatch warp.go's workflow renderer already offers.
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
	var keys []string
	emit.MergeCustomTargetMeta(out, &keys, e.Meta, target,
		"command", "args", "env", "working_directory", "url", "headers")
	return out
}
