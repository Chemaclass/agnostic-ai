package claude

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hookEventHandlerTypes lists the handler types each event runs, from
// code.claude.com/docs/en/hooks. Agent handlers are never written.
var hookEventHandlerTypes = func() map[string][]string {
	groups := []struct {
		types  []string
		events []string
	}{
		{[]string{"command", "http", "mcp_tool", "prompt"}, []string{
			"PermissionDenied", "PermissionRequest", "PostToolBatch", "PostToolUse", "PostToolUseFailure", "PreToolUse",
			"Stop", "SubagentStop", "TaskCompleted", "TaskCreated", "TeammateIdle", "UserPromptExpansion", "UserPromptSubmit",
		}},
		{[]string{"command", "http", "mcp_tool"}, []string{
			"ConfigChange", "CwdChanged", "DirectoryAdded", "Elicitation", "ElicitationResult", "FileChanged",
			"InstructionsLoaded", "MessageDisplay", "Notification", "PostCompact", "PostModelSwitch", "PreCompact",
			"PreModelSwitch", "SessionEnd", "StopFailure", "SubagentStart", "WorktreeCreate", "WorktreeRemove",
		}},
		{[]string{"command", "mcp_tool"}, []string{"SessionStart", "Setup"}},
	}
	out := map[string][]string{}
	for _, g := range groups {
		for _, event := range g.events {
			out[event] = g.types
		}
	}
	return out
}()

// hookIfEvents are the tool events where Claude Code evaluates `if`;
// on any other event a hook that sets it never runs.
var hookIfEvents = map[string]bool{
	"PreToolUse": true, "PostToolUse": true, "PostToolUseFailure": true, "PermissionRequest": true, "PermissionDenied": true,
}

// NoteHookEventNoOps reports hooks on a known event that cannot run
// their handler type or `if` filter. The hook is still written.
func NoteHookEventNoOps(hooks []spec.Entry) {
	var prompt, http, ifs int
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		types, known := hookEventHandlerTypes[event]
		if !known {
			continue
		}
		kind, _ := h.Meta["type"].(string)
		if kind == "" {
			kind = "command"
		}
		if !slices.Contains(types, kind) {
			switch kind {
			case "prompt":
				prompt++
			case "http":
				http++
			}
		}
		if cond, _ := h.Meta["if"].(string); cond != "" && !hookIfEvents[event] {
			ifs++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "type", prompt,
		"Claude Code runs prompt hooks only on tool, permission, prompt, Stop, SubagentStop, Task, and TeammateIdle events; use a command, http, or mcp_tool hook on this event")
	emit.NoteFieldNoOp(target, spec.KindHook, "type", http,
		"Claude Code runs http hooks on every event except SessionStart and Setup; use a command or mcp_tool hook there")
	emit.NoteFieldNoOp(target, spec.KindHook, "if", ifs,
		"Claude Code evaluates `if` only on PreToolUse, PostToolUse, PostToolUseFailure, PermissionRequest, and PermissionDenied, and never runs a hook that sets it on another event; remove `if` or move the hook to a tool event")
}
