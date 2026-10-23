package grok

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

var mcpKeys = []string{"command", "args", "env", "cwd", "url", "type", "headers", "bearer_token_env_var", "bearer_token_file", "oauth_client_id", "oauth_client_secret_env_var", "oauth_scopes", "startup_timeout_sec", "tool_timeout_sec", "tool_timeouts", "enabled", "expose_image_base64", "oauth", "setup"}

func mcpEntry(e spec.Entry) (map[string]any, error) {
	meta := emit.ResolveMeta(e.Meta, target)
	out := selected(meta, mcpKeys)
	if disabled, ok := meta["disabled"].(bool); ok {
		out["enabled"] = !disabled
	}
	if out["command"] != nil {
		delete(out, "type")
	} else if out["url"] != nil {
		if transport, ok := out["type"].(string); ok && !slices.Contains([]string{"http", "sse", "streamable-http"}, transport) {
			return nil, fmt.Errorf("grok MCP %s: unsupported transport %q", e.Name, transport)
		}
	} else {
		return nil, fmt.Errorf("grok MCP %s: command or url is required", e.Name)
	}
	for _, field := range []string{"description", "timeout"} {
		if meta[field] != nil {
			emit.NoteFieldNoOp(target, spec.KindMCP, field, 1, "use x-grok native fields; Grok has no portable "+field+" key")
		}
	}
	return out, nil
}
func (Adapter) UserMCPServerTables(mcps []spec.Entry) map[string]string {
	out := map[string]string{}
	for _, e := range mcps {
		meta, err := mcpEntry(e)
		if err != nil {
			continue
		}
		var b strings.Builder
		b.WriteString("[mcp_servers." + emit.TOMLKey(e.Name) + "]\n")
		for _, key := range mcpKeys {
			if value, ok := meta[key]; ok {
				if literal, ok := emit.TOMLLiteral(value); ok {
					b.WriteString(emit.TOMLKey(key) + " = " + literal + "\n")
				}
			}
		}
		out[e.Name] = b.String()
	}
	return out
}
func emitConfig(sess *emit.Session, mcps, settings []spec.Entry, path string, dryRun bool) error {
	permissions := emit.SettingsPermissions(settings)
	for _, e := range settings {
		if e.Meta["model"] != nil {
			emit.NoteFieldNoOp(target, spec.KindSettings, "model", 1, "Grok reads model settings only from the user config; use sync --global")
		}
		if p, _ := e.Meta["permissions"].(map[string]any); p["default-mode"] != nil {
			emit.NoteFieldNoOp(target, spec.KindSettings, "permissions.default-mode", 1, "Grok project config reads permission lists only")
		}
	}
	custom := emit.SettingsCustomKeys(settings, target)
	for key := range custom {
		if key != "permission" && key != "plugins" {
			return fmt.Errorf("%s: x-grok.%s is not a supported project setting; only permission and plugins are read", path, key)
		}
	}
	var values []emit.TOMLValue
	for _, key := range []string{"allow", "ask", "deny"} {
		if list := stringList(permissions[key]); len(list) > 0 {
			values = append(values, emit.TOMLValue{Path: []string{"permission", key}, Value: list, MergeList: true})
		}
	}
	for key, value := range custom {
		if key == "permission" {
			native, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: x-grok.permission must be a table", path)
			}
			for field, list := range native {
				if !slices.Contains([]string{"allow", "ask", "deny"}, field) {
					return fmt.Errorf("%s: project permission.%s is unsupported", path, field)
				}
				values = append(values, emit.TOMLValue{Path: []string{"permission", field}, Value: list, MergeList: true})
			}
		} else {
			values = append(values, emit.TOMLValue{Path: []string{key}, Value: value})
		}
	}
	for _, e := range mcps {
		entry, err := mcpEntry(e)
		if err != nil {
			return err
		}
		values = append(values, emit.TOMLValue{Path: []string{"mcp_servers", e.Name}, Value: entry})
	}
	return sess.MergeTOMLFile(path, values, dryRun)
}

func ValidateMCPs(mcps []spec.Entry) error {
	for _, entry := range mcps {
		if _, err := mcpEntry(entry); err != nil {
			return err
		}
	}
	return nil
}
