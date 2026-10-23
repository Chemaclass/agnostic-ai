package kimi

import (
	"fmt"
	"maps"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) MCPLaunchView() emit.MCPLaunchView { return emit.MCPLaunchView{Resolved: true} }
func (Adapter) PrepareMCPReferences(mcps []spec.Entry) []spec.Entry {
	out := make([]spec.Entry, 0, len(mcps))
	for _, e := range mcps {
		m := emit.ResolveMeta(e.Meta, target)
		for _, key := range []string{"env", "headers"} {
			if values, ok := m[key].(map[string]string); ok {
				normalized := make(map[string]any, len(values))
				for k, v := range values {
					normalized[k] = v
				}
				m[key] = normalized
			}
		}
		if headers, ok := m["headers"].(map[string]any); ok {
			headers = maps.Clone(headers)
			for key, v := range headers {
				if !strings.EqualFold(key, "Authorization") {
					continue
				}
				value, _ := v.(string)
				if token, ok := strings.CutPrefix(value, "Bearer "); ok {
					if name, whole := spec.WholeEnvRef(token); whole && m["bearerTokenEnvVar"] == nil {
						m["bearerTokenEnvVar"] = name
						delete(headers, key)
					}
				}
			}
			m["headers"] = headers
		}
		incomplete := false
		var check func(any)
		check = func(v any) {
			switch v := v.(type) {
			case string:
				if spec.HasEnvRef(v) {
					incomplete = true
				}
			case []any:
				for _, x := range v {
					check(x)
				}
			case []string:
				for _, x := range v {
					check(x)
				}
			case map[string]any:
				for _, x := range v {
					check(x)
				}
			}
		}
		for _, key := range []string{"command", "args", "env", "cwd", "url", "headers"} {
			check(m[key])
		}
		if incomplete {
			emit.NoteFieldNoOp(target, spec.KindMCP, "environment references", 1, "server "+e.Name+" requires unsupported interpolation; use bearerTokenEnvVar or literal escapes")
			emit.NoteEntryOmitted(target, spec.KindMCP, e.Name)
			continue
		}
		e.Meta = m
		out = append(out, e)
	}
	return out
}
func (Adapter) RewriteMCPReferences(mcps []spec.Entry) []spec.Entry {
	out := emit.RewriteMCPEnvRefs(target, (Adapter{}).MCPLaunchView(), (Adapter{}).PrepareMCPReferences(mcps))
	for i := range out {
		for _, key := range []string{"command", "cwd"} {
			if s, ok := out[i].Meta[key].(string); ok {
				out[i].Meta[key] = spec.DecodeEnvRefEscapes(s)
			}
		}
	}
	return out
}
func (Adapter) CheckedUserMCPServers(mcps []spec.Entry) (map[string]any, error) {
	out := map[string]any{}
	for _, e := range mcps {
		m := emit.ResolveMeta(e.Meta, target)
		d := map[string]any{}
		for _, key := range []string{"command", "args", "env", "cwd", "executor", "runtime_id", "url", "headers", "auth", "bearerTokenEnvVar", "enabled", "deferred", "startupTimeoutMs", "toolTimeoutMs", "enabledTools", "disabledTools"} {
			if v, ok := m[key]; ok {
				d[key] = v
			}
		}
		transport, _ := m["transport"].(string)
		if transport == "" {
			transport, _ = m["type"].(string)
		}
		if transport == "streamable-http" {
			transport = "http"
		}
		if transport == "" {
			transport = "stdio"
			if d["url"] != nil {
				transport = "http"
			}
		}
		if transport != "stdio" && transport != "http" && transport != "sse" {
			return nil, fmt.Errorf("kimi: MCP %s unsupported transport %s", e.Name, transport)
		}
		d["transport"] = transport
		_, nativeEnabled := m["enabled"]
		if disabled, ok := m["disabled"].(bool); ok && !nativeEnabled {
			d["enabled"] = !disabled
		}
		if transport == "stdio" {
			if cmd, _ := d["command"].(string); strings.TrimSpace(cmd) == "" {
				return nil, fmt.Errorf("kimi: MCP %s requires command", e.Name)
			}
		} else {
			if url, _ := d["url"].(string); strings.TrimSpace(url) == "" {
				return nil, fmt.Errorf("kimi: MCP %s requires URL", e.Name)
			}
		}
		for _, key := range []string{"startupTimeoutMs", "toolTimeoutMs"} {
			if v, ok := d[key]; ok {
				n, valid := nativeInteger(v)
				if !valid || n <= 0 || n > 2147483647 {
					return nil, fmt.Errorf("kimi: MCP %s %s must be a positive 32-bit integer", e.Name, key)
				}
			}
		}
		if err := validateMCP(d); err != nil {
			return nil, fmt.Errorf("kimi: MCP %s: %w", e.Name, err)
		}
		out[e.Name] = d
	}
	return out, nil
}
