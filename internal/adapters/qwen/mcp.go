package qwen

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func buildMCPServers(mcps []spec.Entry) map[string]any {
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
		if cwd, _ := e.Meta["cwd"].(string); cwd != "" {
			out["cwd"] = cwd
		}
	case "http", "streamable-http":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["httpUrl"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	case "sse":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["url"] = url
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default:
		emit.NoteFieldNoOp(target, spec.KindMCP, "type", 1, "unsupported MCP transport "+transport)
		return nil
	}
	if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
		out["env"] = env
	}
	if timeout, ok := emit.IntField(e.Meta, "timeout"); ok {
		out["timeout"] = timeout
	}
	if trust, ok := e.Meta["trust"].(bool); ok {
		out["trust"] = trust
	}
	if desc, _ := e.Meta["description"].(string); desc != "" {
		out["description"] = desc
	}
	if include := emit.StringSlice(e.Meta["includeTools"]); len(include) > 0 {
		out["includeTools"] = include
	}
	if exclude := emit.StringSlice(e.Meta["excludeTools"]); len(exclude) > 0 {
		out["excludeTools"] = exclude
	}
	var keys []string
	emit.MergeCustomTargetMeta(out, &keys, e.Meta, target, "type", "url", "httpUrl", "headers", "command", "args", "cwd", "env", "timeout", "trust", "description", "includeTools", "excludeTools")
	return out
}

func (Adapter) UserMCPServers(mcps []spec.Entry) map[string]any { return buildMCPServers(mcps) }
