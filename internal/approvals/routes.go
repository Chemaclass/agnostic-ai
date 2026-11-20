package approvals

import (
	"slices"
	"sort"
)

// Mechanism is how a tool carries out one loosening mode.
type Mechanism struct {
	// Name says what answers the prompt, such as a PermissionRequest hook.
	Name string `json:"name"`
	// File is the user-level file sync --global writes it to.
	File string `json:"file"`
	// Evidence is the vendor page the route rests on.
	Evidence string `json:"evidence"`
}

// Route is one target's way of answering permission prompts. A mode
// with no mechanism falls back to ask, for Reason.
type Route struct {
	Target string     `json:"target"`
	Yolo   *Mechanism `json:"yolo,omitempty"`
	Review *Mechanism `json:"review,omitempty"`
	// Reason says why a mode without a mechanism falls back to ask.
	Reason string `json:"reason,omitempty"`
	// MinVersion is the oldest tool release the route works with, when
	// the vendor states one.
	MinVersion string `json:"min_version,omitempty"`
}

// WindowsReason is why a hook route falls back to ask on Windows: the
// built-in hook has no Windows command yet.
const WindowsReason = "Windows"

// Mechanism returns the route for mode on goos, or nil when the mode
// falls back to ask there.
func (r Route) Mechanism(mode, goos string) *Mechanism {
	var m *Mechanism
	switch mode {
	case ModeYolo:
		m = r.Yolo
	case ModeReview:
		m = r.Review
	}
	if m != nil && goos == "windows" && m.Name == MechanismPermissionHook {
		return nil
	}
	return m
}

// FallbackReason is why mode falls back to ask on goos, or "" when the
// mode has a route.
func (r Route) FallbackReason(mode, goos string) string {
	if mode != ModeYolo && mode != ModeReview {
		return ""
	}
	if r.Mechanism(mode, goos) != nil {
		return ""
	}
	if goos == "windows" && (mode == ModeYolo && r.Yolo != nil || mode == ModeReview && r.Review != nil) {
		return WindowsReason
	}
	return r.Reason
}

// ModeReview lets the tool's own reviewer decide.
const ModeReview = "review"

// MechanismPermissionHook is the name of the route through a
// PermissionRequest hook that runs agnostic-ai hook approve.
const MechanismPermissionHook = "PermissionRequest hook"

const (
	reasonNotBuilt   = "not built yet"
	reasonDenyRules  = "a hook allow would skip the deny rules"
	reasonNoApprove  = "hooks cannot approve one call"
	reasonSilentOK   = "a silent hook approves"
	reasonAllowDeny  = "a policy script can only allow or deny"
	reasonNoRoute    = "hooks can only block or observe"
	reasonNoPrompt   = "no local permission prompt"
	reasonNoReviewer = "no native reviewer"
)

// routes is the one list of approval routes. Each per-target change
// flips its own row, so the table is never ahead of the code.
var routes = []Route{
	{Target: "aider", Reason: "no hook; only --yes-always"},
	{Target: "amp", Reason: "does not ask before running tools by default"},
	{Target: "antigravity", Reason: reasonNotBuilt},
	{Target: "augment", Reason: reasonAllowDeny},
	{Target: "claude", MinVersion: "2.0.45", Yolo: &Mechanism{
		Name:     MechanismPermissionHook,
		File:     "~/.claude/settings.json",
		Evidence: "https://code.claude.com/docs/en/hooks.md",
	}, Review: &Mechanism{
		Name:     "Claude Code auto mode",
		File:     "~/.claude/settings.json",
		Evidence: "https://code.claude.com/docs/en/permission-modes.md",
	}},
	{Target: "cline", Reason: reasonNoRoute},
	{Target: "codebuddy", Reason: reasonNotBuilt},
	{Target: "codex", Yolo: &Mechanism{
		Name:     MechanismPermissionHook,
		File:     "~/.codex/hooks.json",
		Evidence: "https://learn.chatgpt.com/docs/hooks.md",
	}, Review: &Mechanism{
		Name:     "Codex Auto-review",
		File:     "~/.codex/config.toml",
		Evidence: "https://learn.chatgpt.com/docs/agent-approvals-security.md",
	}},
	{Target: "continue", Reason: "no documented hook surface"},
	{Target: "copilot", Reason: reasonDenyRules},
	{Target: "crush", Reason: reasonNotBuilt},
	{Target: "cursor", Reason: reasonNotBuilt},
	{Target: "ellipsis", Reason: reasonNoPrompt},
	{Target: "factory", Reason: reasonNotBuilt},
	{Target: "gemini", Reason: reasonNoApprove},
	{Target: "goose", Reason: reasonNoRoute},
	{Target: "grok", Reason: reasonNoRoute},
	{Target: "jules", Reason: reasonNoPrompt},
	{Target: "junie", Reason: reasonSilentOK},
	{Target: "kilo", Reason: reasonNoRoute},
	{Target: "kimi", Reason: reasonNoRoute},
	{Target: "kiro", Reason: reasonNotBuilt},
	{Target: "openhands", Reason: reasonNoRoute},
	{Target: "opencode", Reason: reasonNoRoute},
	{Target: "pi", Reason: reasonNoRoute},
	{Target: "qoder", Reason: reasonNotBuilt},
	{Target: "qwen", Reason: reasonNotBuilt, Review: &Mechanism{
		Name:     "Qwen Code auto mode",
		File:     "~/.qwen/settings.json",
		Evidence: "https://github.com/QwenLM/qwen-code/blob/main/docs/users/features/approval-mode.md",
	}},
	{Target: "trae", Reason: "a hook allow still needs the person in manual mode"},
	{Target: "vibe", Reason: reasonNotBuilt},
	{Target: "warp", Reason: "no external hook"},
	{Target: "windsurf", Reason: reasonNotBuilt},
	{Target: "zed", Reason: reasonNoReviewer + "; no hook system"},
}

// Routes returns every route, sorted by target.
func Routes() []Route {
	out := slices.Clone(routes)
	sort.Slice(out, func(i, j int) bool { return out[i].Target < out[j].Target })
	return out
}

// RouteFor returns target's route. An unknown target has none and
// falls back to ask.
func RouteFor(target string) Route {
	for _, r := range routes {
		if r.Target == target {
			return r
		}
	}
	return Route{Target: target, Reason: "unknown tool"}
}
