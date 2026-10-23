package qwen

import (
	"maps"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func buildHooks(hooks []spec.Entry) map[string]any {
	byEvent := map[string][]map[string]any{}
	for _, h := range hooks {
		meta := emit.ResolveMeta(h.Meta, target)
		event, _ := meta["event"].(string)
		if !slices.Contains(HookEvents(), event) {
			if event != "" {
				emit.NoteFieldNoOp(target, spec.KindHook, "event", 1, "unknown Qwen hook event "+event)
			}
			continue
		}
		handlers := hookHandlers(h)
		if event == "" || len(handlers) == 0 {
			continue
		}
		for _, handler := range handlers {
			if handler["type"] != "command" {
				continue
			}
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
	native, _ := h.Meta["x-qwen"].(map[string]any)
	if raw, exists := native["hooks"]; exists {
		return nativeHookHandlers(h, raw)
	}
	meta := emit.ResolveMeta(h.Meta, target)
	if kind, _ := meta["type"].(string); kind != "" && kind != "command" {
		handler := map[string]any{"type": kind}
		switch kind {
		case "http":
			for _, key := range []string{"url", "headers", "allowedEnvVars", "timeout", "if", "name", "description", "statusMessage", "once"} {
				if value, ok := meta[key]; ok {
					handler[key] = value
				}
			}
		case "prompt":
			for _, key := range []string{"prompt", "model", "timeout", "name", "description", "statusMessage"} {
				if value, ok := meta[key]; ok {
					handler[key] = value
				}
			}
		default:
			emit.NoteFieldNoOp(target, spec.KindHook, "type", 1, "unsupported hook handler "+kind)
			return nil
		}
		return []map[string]any{handler}
	}
	var handlers []map[string]any
	for _, command := range emit.HookCommands(meta["command"]) {
		handler := map[string]any{"type": "command", "command": emit.WrapPortableHook(h, target, emit.ShellHookCommand(command, target, meta))}
		if description, _ := meta["description"].(string); description != "" {
			handler["description"] = description
		}
		if name, _ := native["name"].(string); name != "" {
			handler["name"] = name
		}
		if env, ok := native["env"].(map[string]any); ok {
			handler["env"] = env
		}
		if value, ok := native["async"]; ok {
			handler["async"] = value
		}
		if timeout, ok := HookTimeout(h.Meta); ok {
			handler["timeout"] = timeout
		}
		handlers = append(handlers, handler)
	}
	return handlers
}

func hookSourceCommands(h spec.Entry) []string {
	native, _ := h.Meta["x-qwen"].(map[string]any)
	if raw, exists := native["hooks"]; exists {
		var commands []string
		for _, meta := range emit.HookCommandEntries(raw) {
			commands = append(commands, meta["command"].(string))
		}
		return commands
	}
	return emit.HookCommands(emit.ResolveMeta(h.Meta, target)["command"])
}

func nativeHookHandlers(h spec.Entry, raw any) []map[string]any {
	var handlers []map[string]any
	entries, _ := raw.([]any)
	if typed, ok := raw.([]map[string]any); ok {
		for _, value := range typed {
			entries = append(entries, value)
		}
	}
	for _, raw := range entries {
		meta, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := meta["type"].(string)
		switch kind {
		case "command":
			command, _ := meta["command"].(string)
			if command == "" {
				continue
			}
			handler := maps.Clone(meta)
			handler["command"] = emit.WrapPortableHook(h, target, emit.RewriteHookPath(command, target, meta))
			handlers = append(handlers, handler)
		case "http", "prompt":
			handlers = append(handlers, maps.Clone(meta))
		default:
			emit.NoteFieldNoOp(target, spec.KindHook, "type", 1, "unsupported native hook handler "+kind)
		}
	}
	return handlers
}

func HookTimeout(meta map[string]any) (any, bool) {
	native, _ := meta["x-qwen"].(map[string]any)
	if timeout, exists := native["timeout"]; exists {
		return timeout, true
	}
	if timeout, ok := emit.IntField(meta, "timeout"); ok {
		return timeout, true
	}
	return nil, false
}

func HookEvents() []string {
	return []string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "PostToolBatch", "Notification", "UserPromptSubmit", "UserPromptExpansion", "SessionStart", "Stop", "MessageDisplay", "SubagentStart", "SubagentStop", "PreCompact", "PostCompact", "SessionEnd", "SessionDelete", "PermissionRequest", "PermissionDenied", "StopFailure", "TodoCreated", "TodoCompleted", "InstructionsLoaded"}
}
