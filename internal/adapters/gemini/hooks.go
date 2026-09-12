package gemini

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Gemini discards definitions without a nested hooks array. Keep a command
// list together so sequential execution applies to the whole group.
func buildHooks(hooks []spec.Entry) map[string]any {
	byEvent := map[string][]map[string]any{}
	for _, h := range hooks {
		meta := emit.ResolveMeta(h.Meta, target)
		event, _ := meta["event"].(string)
		handlers := hookHandlers(h)
		if event == "" || len(handlers) == 0 {
			continue
		}
		for _, handler := range handlers {
			command, _ := handler["command"].(string)
			handler["command"] = emit.RewriteHookPath(command, target)
		}
		definition := map[string]any{"hooks": handlers}
		if matcher, _ := meta["matcher"].(string); matcher != "" {
			definition["matcher"] = matcher
		}
		if sequential, ok := meta["sequential"].(bool); ok {
			definition["sequential"] = sequential
		}
		byEvent[event] = append(byEvent[event], definition)
	}
	out := map[string]any{}
	for event, definitions := range byEvent {
		out[event] = definitions
	}
	return out
}

func hookHandlers(h spec.Entry) []map[string]any {
	native, _ := h.Meta["x-gemini"].(map[string]any)
	if raw, exists := native["hooks"]; exists {
		return nativeHookHandlers(raw)
	}
	meta := emit.ResolveMeta(h.Meta, target)
	var handlers []map[string]any
	for _, command := range emit.HookCommands(meta["command"]) {
		handler := map[string]any{"type": "command", "command": command}
		if description, _ := meta["description"].(string); description != "" {
			handler["description"] = description
		}
		// A spec's filename-safe name is distinct from Gemini's display name.
		if name, _ := native["name"].(string); name != "" {
			handler["name"] = name
		}
		if env, ok := native["env"].(map[string]any); ok {
			handler["env"] = env
		}
		if timeout, ok := emit.IntField(h.Meta, "timeout"); ok {
			handler["timeout"] = timeout * 1000
		}
		// Namespaced fields use native units, including subsecond imports.
		if timeout, exists := native["timeout"]; exists {
			handler["timeout"] = timeout
		}
		handlers = append(handlers, handler)
	}
	return handlers
}

func nativeHookHandlers(raw any) []map[string]any {
	entries, _ := raw.([]any)
	var handlers []map[string]any
	for _, entry := range entries {
		meta, ok := entry.(map[string]any)
		if !ok || meta["type"] != "command" {
			continue
		}
		command, _ := meta["command"].(string)
		if command == "" {
			continue
		}
		handler := map[string]any{"type": "command", "command": command}
		for _, key := range []string{"name", "description", "timeout", "env"} {
			if value, exists := meta[key]; exists {
				handler[key] = value
			}
		}
		handlers = append(handlers, handler)
	}
	return handlers
}
