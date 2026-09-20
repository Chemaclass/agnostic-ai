package amp

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// buildMCPMap renders every named MCP server into the map that becomes
// `amp.mcpServers` in `.amp/settings.json`. The write itself lives in
// settings.go, which merges this map and the settings hatch into that
// one file in a single pass.
func buildMCPMap(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPEntry(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

// buildMCPEntry renders a single MCP server in Amp's settings shape.
// Amp accepts the standard `command`/`args`/`env` for stdio and
// `url`/`headers` for HTTP transports (ampcode.com/docs/customize/mcp).
//
// Any field beyond that set reaches the entry through `x-amp`
// (emit.MergeCustomTargetMeta), the same passthrough the skill
// renderer already gives its own surface. The field this
// unblocks today is `includeTools`, which ampcode.com/docs/customize/skills
// lists under "Common fields": "includeTools (string[], optional but
// recommended) contains tool names or glob patterns used to choose
// which tools are exposed". It stays namespaced rather than mapped
// top-level because ampcode.com/docs/customize/mcp says MCP servers
// "use the same configuration fields as MCP servers in skills" and
// then enumerates without naming it: the clause implies the field, the
// enumeration does not, and a namespaced key is correct either way
// (target-audit 2026-08-27, #634).
func buildMCPEntry(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	entry := map[string]any{}
	switch transport {
	case "stdio":
		if cmd, _ := e.Meta["command"].(string); cmd != "" {
			entry["command"] = cmd
		}
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			entry["args"] = args
		}
	case "http", "sse":
		if url, _ := e.Meta["url"].(string); url != "" {
			entry["url"] = url
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			entry["headers"] = h
		}
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		entry["env"] = env
	}
	var keys []string
	emit.MergeCustomTargetMeta(entry, &keys, e.Meta, target,
		"command", "args", "url", "headers", "env")
	return entry
}
