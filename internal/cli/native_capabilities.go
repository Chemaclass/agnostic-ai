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
		"PreToolUse", "PostToolUse",
		"UserPromptSubmit",
		"SessionStart", "SessionEnd", "Stop", "SubagentStop",
		"PreCompact", "PostCompact",
		"Notification",
	),
	"codex": setOf(
		"PreToolUse", "PostToolUse",
		"UserPromptSubmit",
		"SessionStart", "SessionEnd", "Stop",
		"PreCompact", "PostCompact",
	),
	"gemini": setOf(
		"BeforeTool", "AfterTool",
		"SessionStart", "SessionEnd",
	),
}

// matcherAcceptingEvents lists the hook events whose native CLI consumes a
// matcher field. Events outside this set ignore matchers entirely; setting
// one is a no-op the user likely did not intend.
var matcherAcceptingEvents = setOf(
	"PreToolUse", "PostToolUse", // claude, codex
	"BeforeTool", "AfterTool", // gemini
)

// targetsSupportingKind lists the targets whose adapter actually
// emits non-empty output for the given kind. Used by the orphan-kind
// validator: if a project has hook specs but no enabled target maps
// to a hook surface, the specs are dead weight.
var targetsSupportingKind = map[spec.Kind]map[string]struct{}{
	spec.KindAgent:       setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "antigravity"),
	spec.KindSkill:       setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode"),
	spec.KindRule:        setOf("claude", "codex", "gemini", "cursor", "copilot", "aider", "cline", "windsurf", "continue", "amp", "zed", "warp", "opencode", "antigravity"),
	spec.KindHook:        setOf("claude", "codex", "gemini", "zed"),
	spec.KindMCP:         setOf("claude", "codex", "gemini", "cursor", "copilot", "continue", "amp", "zed", "warp", "opencode"),
	spec.KindCommand:     setOf("claude", "codex", "cursor", "gemini", "amp", "opencode"),
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
