package codex

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HookCommand is one command handler sync writes for a hook spec.
type HookCommand struct {
	Command        string
	CommandWindows string
	// Timeout is the spec's timeout in seconds, zero when it sets none.
	Timeout int
}

// HookCommands returns the command handlers sync writes for h, one per
// entry of its command list.
func HookCommands(h spec.Entry) []HookCommand {
	if kind, _ := h.Meta["type"].(string); kind != "" && kind != "command" {
		return nil
	}
	windows := specCommandWindows(h)
	var out []HookCommand
	for _, raw := range hookCommands(h.Meta["command"]) {
		cmd := specCommand(h, raw)
		c := HookCommand{Command: emit.ExportHookTarget(cmd, target), CommandWindows: windows, Timeout: hookIntMeta(h.Meta, "timeout")}
		if c.CommandWindows == "" {
			c.CommandWindows = cmd
		}
		out = append(out, c)
	}
	return out
}

// specCommand is one entry of h's command as Codex runs it, before the
// target export.
func specCommand(h spec.Entry, raw string) string {
	return emit.WrapPortableHook(h, target, emit.ShellHookCommand(raw, target, h.Meta))
}

func specCommandWindows(h spec.Entry) string {
	windows, _ := h.Meta["commandWindows"].(string)
	return emit.RewriteWindowsHookRoot(emit.RewriteHookDirectories(windows, target), target)
}
