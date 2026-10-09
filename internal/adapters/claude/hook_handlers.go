package claude

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
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
	base.OnFailure = onFailure(meta)
	switch kind {
	case "command":
		base.Args = emit.StringSlice(meta["args"])
		base.Async = hookBoolMeta(meta, "async")
		base.AsyncRewake = hookBoolMeta(meta, "asyncRewake")
		base.Shell, _ = meta["shell"].(string)
		var handlers []claudehooks.CommandEntry
		for _, command := range hookCommands(meta["command"]) {
			handler := base
			handler.Command = emit.RewriteHookPath(command, target, meta)
			// The wrapper runs a command line, so args fold in.
			if h.WrapsCommand(target) {
				handler.Command, handler.Args = emit.WrapPortableHook(h, target, emit.ShellHookCommand(command, target, meta)), nil
			}
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
		base.ContinueOnBlock = hookBoolMeta(meta, "continueOnBlock")
	default:
		emit.NoteFieldNoOp(target, spec.KindHook, "type", 1, "supported hook handlers are command, http, mcp_tool, and prompt")
		return nil
	}
	return []claudehooks.CommandEntry{base}
}

// CommandHandlers returns the command handlers sync writes for h.
func CommandHandlers(h spec.Entry) []claudehooks.CommandEntry {
	if kind, _ := h.Meta["type"].(string); kind != "" && kind != "command" {
		return nil
	}
	return hookHandlers(h)
}

// SettingsFilePath is the settings file sync writes hooks to.
func SettingsFilePath(cfg *config.Config) string {
	return filepath.Join(emit.OutputDir(cfg, target, defaultDir), "settings.json")
}

// onFailure is "block" when the spec sets failClosed, Claude Code's way to
// block the action when a command or HTTP hook cannot start, times out,
// or exits unexpectedly (2.1.295). Older imports set failClosed on MCP-tool
// and prompt handlers too, so every handler type writes it. Otherwise
// x-claude.onFailure is written as is; unset keeps the fail-open default.
func onFailure(meta map[string]any) string {
	if hookBoolMeta(meta, "failClosed") {
		return "block"
	}
	native, _ := emit.CustomTargetMeta(meta, target)
	value, _ := native["onFailure"].(string)
	return value
}
