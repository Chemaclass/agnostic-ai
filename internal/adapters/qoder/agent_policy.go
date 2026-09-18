package qoder

import (
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionModes is Qoder's own value space for a subagent's approval
// boundary: "`default`, `acceptEdits`, `bypassPermissions`, `dontAsk`,
// `auto`, `plan`". Claude Code documents a seventh, `manual`, as an
// alias for `default`, so a portable spec can carry a value Qoder never
// defined.
var permissionModes = map[string]bool{
	"default": true, "acceptEdits": true, "bypassPermissions": true,
	"dontAsk": true, "auto": true, "plan": true,
}

// agentHookEvents is the narrower set Qoder documents for a subagent,
// against the 27 its project hook file accepts: "Supported events
// include `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `Stop`,
// `SubagentStart`, `SubagentStop`, and `Notification`".
var agentHookEvents = map[string]bool{
	"PreToolUse": true, "PostToolUse": true, "PostToolUseFailure": true,
	"Stop": true, "SubagentStart": true, "SubagentStop": true, "Notification": true,
}

// noteAgentPolicyGaps reports the parts of a portable agent policy Qoder
// cannot honor. Both fields are emitted whole, because Qoder renders its
// own `.qoder/agents/<name>.md`, but a value outside its documented set
// reaches a file the tool parses and then ignores, which is the quiet
// half of a safety boundary (target-audit 2026-09-18, #825, #826).
func noteAgentPolicyGaps(agents []spec.Entry) {
	modes, events := 0, 0
	for _, agent := range agents {
		resolved := emit.ResolveMeta(agent.Meta, target)
		if mode, _ := resolved["permissionMode"].(string); mode != "" && !permissionModes[mode] {
			modes++
		}
		if unknown := unknownAgentHookEvents(resolved["hooks"]); len(unknown) > 0 {
			events++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "permissionMode", modes,
		"Qoder documents `default`, `acceptEdits`, `bypassPermissions`, `dontAsk`, `auto`, and `plan`; another value parses and then inherits the parent session mode")
	emit.NoteFieldNoOp(target, spec.KindAgent, "hooks", events,
		"a subagent runs a narrower event set than the project hook file: PreToolUse, PostToolUse, PostToolUseFailure, Stop, SubagentStart, SubagentStop, Notification")
}

// unknownAgentHookEvents returns the event keys a subagent hook map
// declares that Qoder does not run at agent scope, sorted so a note
// reads the same across runs.
func unknownAgentHookEvents(hooks any) []string {
	byEvent, ok := hooks.(map[string]any)
	if !ok {
		return nil
	}
	var unknown []string
	for event := range byEvent {
		if !agentHookEvents[event] {
			unknown = append(unknown, event)
		}
	}
	sort.Strings(unknown)
	return unknown
}
