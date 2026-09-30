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
			// The hook runner spreads a handler's env over the process
			// env, under bash and PowerShell alike (hookRunner.ts).
			env, _ := handler["env"].(map[string]any)
			handler["env"] = emit.WithHookTarget(env, any(target))
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
	args := emit.StringSlice(meta["args"])
	var handlers []map[string]any
	for _, command := range emit.HookCommands(meta["command"]) {
		// No args field: the args fold into the command, quoted for bash,
		// which PowerShell on Windows also reads for args without `'`.
		handler := map[string]any{"type": "command", "command": emit.ExecFormCommand(emit.RewriteHookPath(command, target, meta), args)}
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
		if timeout, ok := HookTimeout(h.Meta); ok {
			handler["timeout"] = timeout
		}
		handlers = append(handlers, handler)
	}
	return handlers
}

// hookSourceCommands returns each handler's command before the path
// rewrite, so the script lookup still sees the tool that owns the stash.
func hookSourceCommands(h spec.Entry) []string {
	native, _ := h.Meta["x-gemini"].(map[string]any)
	if raw, exists := native["hooks"]; exists {
		var commands []string
		for _, meta := range nativeCommandEntries(raw) {
			commands = append(commands, meta["command"].(string))
		}
		return commands
	}
	return emit.HookCommands(emit.ResolveMeta(h.Meta, target)["command"])
}

func nativeCommandEntries(raw any) []map[string]any {
	entries, _ := raw.([]any)
	var matched []map[string]any
	for _, entry := range entries {
		meta, ok := entry.(map[string]any)
		if !ok || meta["type"] != "command" {
			continue
		}
		if command, _ := meta["command"].(string); command != "" {
			matched = append(matched, meta)
		}
	}
	return matched
}

func nativeHookHandlers(raw any) []map[string]any {
	var handlers []map[string]any
	for _, meta := range nativeCommandEntries(raw) {
		handler := map[string]any{"type": "command", "command": emit.RewriteHookPath(meta["command"].(string), target, meta)}
		for _, key := range []string{"name", "description", "timeout", "env"} {
			if value, exists := meta[key]; exists {
				handler[key] = value
			}
		}
		handlers = append(handlers, handler)
	}
	return handlers
}

// HookTimeout returns a hook's timeout in Gemini's milliseconds: the
// spec's seconds times 1000, or `x-gemini.timeout` as written, since
// namespaced fields use native units, including subsecond imports.
func HookTimeout(meta map[string]any) (any, bool) {
	native, _ := meta["x-gemini"].(map[string]any)
	if timeout, exists := native["timeout"]; exists {
		return timeout, true
	}
	if timeout, ok := emit.IntField(meta, "timeout"); ok {
		return timeout * 1000, true
	}
	return nil, false
}
