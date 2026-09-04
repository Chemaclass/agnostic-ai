package qoder

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// qoderMCPKey is the settings.json key holding the server map.
const qoderMCPKey = "mcpServers"

// emitMCP merges the `mcpServers` map into `.qoder/settings.json`
// (default; override via outputs.qoder.mcp-file). Routes through
// MergeJSONFile so the rest of that file (the `mcp` group's
// `enableAllProjectMcpServers` / `enabledProjectMcpServers`,
// permissions, custom models) survives a sync untouched.
//
// No file is written when mcps is empty or every entry renders empty.
func emitMCP(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	servers := buildMCPMap(mcps)
	if len(servers) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, map[string]any{qoderMCPKey: servers}, dryRun)
}

func buildMCPMap(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPServer(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

// buildMCPServer renders one `mcpServers.<name>` entry from the field
// set docs.qoder.com/cli/mcp-reference publishes. The stdio table adds
// `cwd` ("/path/to/dir" in the vendor's own example) on top of
// `command`/`args`/`env`; the http and sse tables carry `url` plus
// `headers` and an explicit `type`. The page's "Common Optional Fields"
// table then adds nine more, every one of which reached the emitted
// file by no route before #641: `timeout` ("Connection/request timeout
// (in milliseconds)"), `description` ("Server description, displayed in
// the management view"), `trust` ("Trusts the server, skipping
// confirmation when its tools are called"), `includeTools` ("Registers
// only the listed tools"), `excludeTools` ("Excludes the listed
// tools"), `disabled` ("Disables the server (keeps the configuration
// without deleting it)"), `alwaysAllow` ("List of tool names that are
// always allowed without confirmation"), and `oauth` ("OAuth
// authorization configuration").
//
// `oauth` passes through as whatever object the spec declares, unlike
// the claude and kiro mappers in the shared builder. The vendor's own
// description is open-ended, "fields include enabled, clientId,
// clientSecret, authorizationUrl, tokenUrl, scopes, callbackPort,
// etc.", so there is no closed list to map against and no
// `additionalProperties: false` to punish a key outside it.
func buildMCPServer(e spec.Entry) map[string]any {
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}

	switch transport {
	case "http", "sse", "streamable-http", "ws":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["type"] = transport
		out["url"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default: // stdio, Qoder's own documented default
		cmd, _ := e.Meta["command"].(string)
		if cmd == "" {
			return nil
		}
		out["command"] = cmd
		if args := emit.StringSlice(e.Meta["args"]); len(args) > 0 {
			out["args"] = args
		}
		if cwd, _ := e.Meta["cwd"].(string); cwd != "" {
			out["cwd"] = cwd
		}
	}

	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		out["env"] = env
	}
	if desc, _ := e.Meta["description"].(string); desc != "" {
		out["description"] = desc
	}
	if timeout, ok := emit.IntField(e.Meta, "timeout"); ok {
		out["timeout"] = timeout
	}
	if trust, _ := e.Meta["trust"].(bool); trust {
		out["trust"] = true
	}
	if include := emit.StringSlice(e.Meta["includeTools"]); len(include) > 0 {
		out["includeTools"] = include
	}
	if exclude := emit.StringSlice(e.Meta["excludeTools"]); len(exclude) > 0 {
		out["excludeTools"] = exclude
	}
	if allow := emit.StringSlice(e.Meta["alwaysAllow"]); len(allow) > 0 {
		out["alwaysAllow"] = allow
	}
	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["disabled"] = true
	}
	if oauth, ok := e.Meta["oauth"].(map[string]any); ok && len(oauth) > 0 {
		out["oauth"] = oauth
	}

	var keys []string
	emit.MergeCustomTargetMeta(out, &keys, e.Meta, target,
		"type", "url", "headers", "command", "args", "cwd", "env",
		"description", "timeout", "trust", "includeTools", "excludeTools",
		"alwaysAllow", "disabled", "oauth")

	return out
}
