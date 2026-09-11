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
		"ConfigChange", "CwdChanged", "DirectoryAdded", "FileChanged",
		"WorktreeCreate", "WorktreeRemove",
		"Notification",
		"PreModelSwitch", "PostModelSwitch",
	),
	"codex": setOf(
		"PreToolUse", "PostToolUse",
		"PermissionRequest",
		"UserPromptSubmit",
		"SessionStart", "SessionEnd", "Stop",
		"SubagentStart", "SubagentStop",
		"PreCompact", "PostCompact",
		"Interrupt",
	),
	"gemini": setOf(
		"BeforeTool", "AfterTool",
		"BeforeAgent", "AfterAgent",
		"Notification",
		"SessionStart", "SessionEnd",
		"PreCompress",
		"BeforeModel", "AfterModel",
		"BeforeToolSelection",
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
	"kiro": setOf(
		"SessionStart", "Stop",
		"PreToolUse", "PostToolUse",
		"PreTaskExec", "PostTaskExec",
		"UserPromptSubmit",
		"PostFileCreate", "PostFileSave", "PostFileDelete",
		"AgentSpawn",
	),
	// The six in the vendor's own Hook Types table. OpenHands names
	// them in snake_case natively and documents these PascalCase keys as
	// equally supported, for sharing hook scripts with Claude Code, so
	// the set is spelled the way this repo emits it
	// (docs.openhands.dev/openhands/usage/customization/hooks).
	"openhands": setOf(
		"PreToolUse", "PostToolUse",
		"UserPromptSubmit", "Stop",
		"SessionStart", "SessionEnd",
	),
	// The eight events docs.devin.ai/cli/extensibility/hooks/overview's
	// own Hook Events table lists for Devin CLI's `.devin/hooks.v1.json`.
	"windsurf": setOf(
		"PreToolUse", "PostToolUse",
		"PermissionRequest", "UserPromptSubmit",
		"Stop", "PostCompaction",
		"SessionStart", "SessionEnd",
	),
	// The 23 events in docs.qoder.com/cli/hooks' own Event Reference
	// Overview table (verified 2026-09-10, up from 6 at an earlier
	// audit). PascalCase, and a strict superset match against Claude
	// Code's own vocabulary above (#629).
	"qoder": setOf(
		"SessionStart", "SessionEnd",
		"UserPromptSubmit",
		"PreToolUse", "PostToolUse", "PostToolUseFailure",
		"PermissionRequest", "PermissionDenied",
		"Stop", "StopFailure",
		"SubagentStart", "SubagentStop",
		"PreCompact", "PostCompact",
		"Notification",
		"InstructionsLoaded", "ConfigChange",
		"CwdChanged", "FileChanged",
		"WorktreeCreate", "WorktreeRemove",
		"Elicitation", "ElicitationResult",
	),
	// The five events docs.augmentcode.com/cli/hooks' own "Hook Events"
	// section documents, re-verified fresh 2026-09-10 (superseding an
	// earlier audit comment that read six off a stale fetch): the
	// vendor's `hook_event_name` field also lists `Notification` as a
	// possible value, but it has no configuration section of its own,
	// so it is not a sixth registrable event (#629).
	"augment": setOf(
		"PreToolUse", "PostToolUse",
		"Stop", "SessionStart", "SessionEnd",
	),
	// The six events docs.trae.ai/ide/automate-actions-with-hooks' own
	// "Hook events" table lists for TraeCode's `.trae/hooks.json`
	// (target-audit 2026-09-11, #729).
	"trae": setOf(
		"SessionStart", "UserPromptSubmit",
		"PreToolUse", "PostToolUse",
		"Stop", "Notification",
	),
	// "Crush currently supports just one hook, PreToolUse, with plans
	// to support the full gamut" (docs/hooks/README.md, re-verified
	// 2026-09-10 against both that doc and the vendor's schema.json,
	// whose $defs.HookConfig carries no per-event variant; #629). The
	// same page lists five legal spellings of that one event: "Event
	// names are case insensitive and snake-caseable, so PreToolUse,
	// pretooluse, PRETOOLUSE, pre_tool_use, and PRE_TOOL_USE all
	// work" (verified 2026-09-11; #731). The emitter applies the rule
	// itself rather than this list, so a sixth spelling such as
	// preToolUse still reaches crush.json, only without a validate
	// entry to vouch for it.
	"crush": setOf(
		"PreToolUse", "pretooluse", "PRETOOLUSE",
		"pre_tool_use", "PRE_TOOL_USE",
	),
	// The nine events docs.factory.ai/harness/hooks' own Event
	// reference table lists for Droid CLI's `.factory/hooks.json`
	// (verified 2026-09-11; #629).
	"factory": setOf(
		"PreToolUse", "PostToolUse",
		"UserPromptSubmit", "Notification",
		"Stop", "SubagentStop",
		"PreCompact", "SessionStart", "SessionEnd",
	),
	// docs.github.com/en/copilot/reference/hooks-reference's own "Hook
	// events" table lists 14 events today (verified 2026-09-10, up from
	// 13 recorded when #629 was filed). Each is a literal, independently
	// valid JSON key in both the PascalCase "VS Code compatible" form
	// this repo's other hook emitters already share (Claude Code,
	// Codex, OpenHands, Windsurf, Qoder) and Copilot's own camelCase
	// form; this adapter passes `event:` through verbatim (#629), so
	// both spellings are listed. `userPromptTransformed` has no
	// PascalCase pairing documented on that page, so only its camelCase
	// spelling is valid.
	"copilot": setOf(
		"SessionStart", "sessionStart",
		"SessionEnd", "sessionEnd",
		"UserPromptSubmit", "userPromptSubmitted",
		"userPromptTransformed",
		"PreToolUse", "preToolUse",
		"PostToolUse", "postToolUse",
		"PostToolUseFailure", "postToolUseFailure",
		"Stop", "agentStop",
		"SubagentStart", "subagentStart",
		"SubagentStop", "subagentStop",
		"ErrorOccurred", "errorOccurred",
		"PreCompact", "preCompact",
		"Notification", "notification",
		"PermissionRequest", "permissionRequest",
	),
}

// matcherAcceptingEvents lists the hook events whose native CLI consumes a
// matcher field. Events outside this set ignore matchers entirely; setting
// one is a no-op the user likely did not intend.
//
// One union keyed by event name, with no target dimension: an event one
// target filters on silences the warning for every target spelling it the
// same way. That direction is deliberate. A missed warning costs nothing,
// while a wrong one tells the user to delete config that works.
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
	// The rest of cursor.com/docs/hooks' own "Available matchers by
	// hook" table, verified 2026-09-11 (#734): subagentStop filters by
	// subagent type, and the last four match one fixed value each
	// (UserPromptSubmit, Stop, AgentResponse, AgentThought).
	// subagentStart is already listed above, added for copilot.
	"subagentStop", "beforeSubmitPrompt",
	"stop", "afterAgentResponse", "afterAgentThought",
	// copilot: matcher-accepting events per
	// docs.github.com/en/copilot/reference/hooks-reference's own
	// matcher-filtering table (notification, permissionRequest,
	// postToolUse, preCompact, preToolUse, subagentStart), added in
	// both the PascalCase and camelCase spellings this adapter's
	// verbatim pass-through accepts (#629).
	"Notification", "notification",
	"permissionRequest", "preCompact", "subagentStart",
)

// targetsSupportingKind lists the targets whose adapter actually
// emits non-empty output for the given kind. Used by the orphan-kind
// validator: if a project has hook specs but no enabled target maps
// to a hook surface, the specs are dead weight.
var targetsSupportingKind = map[spec.Kind]map[string]struct{}{
	spec.KindAgent:       setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "antigravity", "junie", "kiro", "trae", "augment", "factory", "kilo", "qoder"),
	spec.KindSkill:       setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "antigravity", "junie", "kiro", "crush", "trae", "augment", "openhands", "kilo", "qoder", "factory", "goose"),
	spec.KindRule:        setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "antigravity", "junie", "kiro", "crush", "trae", "jules", "goose", "augment", "qoder", "openhands", "factory", "kilo"),
	spec.KindHook:        setOf("claude", "codex", "gemini", "cursor", "zed", "kiro", "openhands", "windsurf", "qoder", "augment", "crush", "copilot", "factory", "trae"),
	spec.KindMCP:         setOf("claude", "codex", "gemini", "cursor", "copilot", "continue", "amp", "zed", "warp", "opencode", "antigravity", "junie", "kiro", "crush", "kilo", "factory", "qoder", "openhands", "trae", "windsurf", "augment"),
	spec.KindCommand:     setOf("claude", "codex", "gemini", "opencode", "cursor", "trae", "junie", "kilo", "qoder"),
	spec.KindSettings:    setOf("claude"),
	spec.KindReview:      setOf("cursor"),
	spec.KindEnvironment: setOf("cursor", "openhands"),
	spec.KindIgnore:      setOf("cursor", "gemini", "aider", "windsurf", "kiro", "trae", "junie"),
}

func setOf(items ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, s := range items {
		out[s] = struct{}{}
	}
	return out
}
