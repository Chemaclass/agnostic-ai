package trae

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// mcpDisabledNoOpReason explains, in the flushed coverage note, why
// `disabled: true` on an MCP spec never reaches `.trae/mcp.json`:
// docs.trae.ai/ide/add-mcp-servers documents no per-server disable key,
// only the project-level MCP on/off toggle under Settings > MCP (see
// the package doc for the fetch that confirmed this).
const mcpDisabledNoOpReason = "no confirmed file-based way to pre-disable a project-scoped MCP server; toggle it off in Settings > MCP instead"

// emitMCP writes .trae/mcp.json (default; override via
// outputs.trae.mcp-file). No file is written when mcps is empty or
// every entry renders empty. See the package doc for the confirmed
// schema and the fields this adapter deliberately does not emit.
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
		return "", fmt.Errorf("trae: marshal mcp: %w", err)
	}
	return string(raw) + "\n", nil
}

// buildMCPServer renders one `mcpServers` entry with only the fields
// docs.trae.ai/ide/add-mcp-servers confirms: stdio carries `command`
// (required) plus optional `args` / `env`; HTTP carries `url`
// (required) plus optional `headers`. Neither transport carries a
// `type` discriminant: the vendor page's own examples never key one in,
// Trae tells stdio and HTTP apart purely by whether `command` or `url`
// is present, and the shared emit.MCPSchemaServersMap always writes
// `type` on a remote entry, so this builder is deliberately its own
// schema (the antigravity adapter holds the same line for its own
// vendor-specific reasons). `description`, `disabled`, `roots`, and
// `autoApprove` are equally undocumented and stay out for the same
// reason; disabled specs are stripped before this function ever sees
// them (see emitMCP).
//
// Two vendor quirks, confirmed on the same fetch, deliberately produce
// no code here: the docs caution "The command must not contain spaces,
// otherwise parsing errors will occur" (a Windows path with spaces
// needs a wrapper, which is on the spec author, not this adapter), and
// the timeout knobs are `env.START_MCP_TIMEOUT_MS` /
// `env.RUN_MCP_TIMEOUT_MS` for stdio, with the HTTP example reusing
// those same two names nested under `headers` instead of a timeout
// field of its own. That looks like a copy-paste in the vendor's own
// docs; a spec author who sets it that way gets exactly what they typed
// via the generic `env` / `headers` passthrough below, nothing more.
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
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
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
	return out
}
