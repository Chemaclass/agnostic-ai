package claude

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Claude's non-command handlers share filters and timeout fields, but
// never inherit command-only options such as args, shell, or async.
func hookHandlers(h spec.Entry) []claudehooks.CommandEntry {
	meta := h.Meta
	kind, _ := meta["type"].(string)
	if kind == "" {
		kind = "command"
	}
	base := claudehooks.CommandEntry{Type: kind, Timeout: hookIntMeta(meta, "timeout"), Once: hookBoolMeta(meta, "once")}
	base.StatusMessage, _ = meta["statusMessage"].(string)
	base.If, _ = meta["if"].(string)
	switch kind {
	case "command":
		base.Args = emit.StringSlice(meta["args"])
		base.Async = hookBoolMeta(meta, "async")
		base.AsyncRewake = hookBoolMeta(meta, "asyncRewake")
		base.Shell, _ = meta["shell"].(string)
		var handlers []claudehooks.CommandEntry
		for _, command := range hookCommands(meta["command"]) {
			handler := base
			handler.Command = emit.RewriteHookPath(command, target)
			handlers = append(handlers, handler)
		}
		return handlers
	case "http":
		base.URL, _ = meta["url"].(string)
		if base.URL == "" {
			return nil
		}
		base.Headers = emit.StringMap(meta["headers"])
		base.AllowedEnvVars = emit.StringSlice(meta["allowedEnvVars"])
	case "mcp_tool":
		base.Server, _ = meta["server"].(string)
		base.Tool, _ = meta["tool"].(string)
		if base.Server == "" || base.Tool == "" {
			return nil
		}
		base.Input, _ = meta["input"].(map[string]any)
	case "prompt":
		base.Prompt, _ = meta["prompt"].(string)
		if base.Prompt == "" {
			return nil
		}
		base.Model, _ = meta["model"].(string)
	default:
		emit.NoteFieldNoOp(target, spec.KindHook, "type", 1, "supported hook handlers are command, http, mcp_tool, and prompt")
		return nil
	}
	return []claudehooks.CommandEntry{base}
}
