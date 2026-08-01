package cli

import "github.com/chemaclass/agnostic-ai/internal/spec"

// hookEventsByTarget enumerates the hook events each target's
// underlying CLI consumes. Specs whose `event:` falls outside the
// union of every configured target's set are reported by `validate`.
//
// Sources of truth (kept out of source comments because they rot):
// the per-adapter docs linked from `docs/user/targets.md`. When a
// target adds a new event, append it here so validation stays useful.
var hookEventsByTarget = map[string]map[string]struct{}{
	"claude": setOf(
		"Setup",
		"PreToolUse", "PostToolUse", "PostToolUseFailure", "PostToolBatch",
		"PermissionRequest", "PermissionDenied",
		"UserPromptSubmit", "UserPromptExpansion", "InstructionsLoaded",
		"Elicitation", "ElicitationResult", "MessageDisplay",
		"SessionStart", "SessionEnd", "Stop", "StopFailure",
		"SubagentStart", "SubagentStop",
		"TaskCreated", "TaskCompleted", "TeammateIdle",
		"PreCompact", "PostCompact",
		"ConfigChange", "CwdChanged", "FileChanged",
		"WorktreeCreate", "WorktreeRemove",
		"Notification",
	),
	"codex": setOf(
		"PreToolUse", "PostToolUse",
		"PermissionRequest",
		"UserPromptSubmit",
		"SessionStart", "SessionEnd", "Stop",
		"SubagentStart", "SubagentStop",
		"PreCompact", "PostCompact",
	),
	"gemini": setOf(
		"BeforeTool", "AfterTool",
		"SessionStart", "SessionEnd",
	),
	"cursor": setOf(
		"beforeShellExecution", "afterShellExecution",
		"beforeMCPExecution", "afterMCPExecution",
		"beforeReadFile", "afterFileEdit",
		"beforeSubmitPrompt",
		"preToolUse", "postToolUse", "postToolUseFailure",
		"sessionStart", "sessionEnd",
		"subagentStart", "subagentStop",
		"preCompact", "stop",
		"afterAgentResponse", "afterAgentThought",
		"beforeTabFileRead", "afterTabFileEdit",
		"workspaceOpen",
	),
}

// matcherAcceptingEvents lists the hook events whose native CLI consumes a
// matcher field. Events outside this set ignore matchers entirely; setting
// one is a no-op the user likely did not intend.
var matcherAcceptingEvents = setOf(
	"PreToolUse", "PostToolUse", // claude, codex
	// claude: tool-name matcher on permission and failure events,
	// subagent-name matcher on Subagent*, trigger matcher on
	// PreCompact (manual|auto) and SessionStart (startup|resume|clear).
	"PermissionRequest", "PostToolUseFailure",
	"SubagentStart", "SubagentStop",
	"PreCompact", "SessionStart",
	"BeforeTool", "AfterTool", // gemini
	// cursor: tool/shell/MCP/file events filter on a regex matcher.
	"beforeShellExecution", "afterShellExecution",
	"beforeMCPExecution", "afterMCPExecution",
	"beforeReadFile", "afterFileEdit",
	"preToolUse", "postToolUse", "postToolUseFailure",
)

// targetsSupportingKind lists the targets whose adapter actually
// emits non-empty output for the given kind. Used by the orphan-kind
// validator: if a project has hook specs but no enabled target maps
// to a hook surface, the specs are dead weight.
var targetsSupportingKind = map[spec.Kind]map[string]struct{}{
	spec.KindAgent:       setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "antigravity", "junie", "kiro", "trae", "factory", "kilo"),
	spec.KindSkill:       setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "junie", "kiro", "crush", "trae", "openhands"),
	spec.KindRule:        setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "antigravity", "junie", "kiro", "crush", "trae", "jules", "goose", "augment", "qoder", "openhands", "factory", "kilo"),
	spec.KindHook:        setOf("claude", "codex", "gemini", "cursor", "zed"),
	spec.KindMCP:         setOf("claude", "codex", "gemini", "cursor", "copilot", "continue", "amp", "zed", "warp", "opencode", "antigravity", "junie", "kiro", "crush", "kilo"),
	spec.KindCommand:     setOf("claude", "codex", "gemini", "opencode", "amp", "cursor"),
	spec.KindSettings:    setOf("claude"),
	spec.KindReview:      setOf("cursor"),
	spec.KindEnvironment: setOf("cursor"),
	spec.KindIgnore:      setOf("cursor", "gemini", "aider"),
}

func setOf(items ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, s := range items {
		out[s] = struct{}{}
	}
	return out
}
