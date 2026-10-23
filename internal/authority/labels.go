package authority

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// targetLabels names each coding tool the way people know it.
var targetLabels = map[string]string{
	"claude":      "Claude Code",
	"codex":       "Codex",
	"cursor":      "Cursor",
	"gemini":      "Gemini CLI",
	"copilot":     "GitHub Copilot",
	"aider":       "Aider",
	"cline":       "Cline",
	"windsurf":    "Windsurf",
	"continue":    "Continue",
	"zed":         "Zed",
	"opencode":    "OpenCode",
	"kilo":        "Kilo Code",
	"augment":     "Augment",
	"factory":     "Factory",
	"kiro":        "Kiro",
	"qoder":       "Qoder",
	"trae":        "Trae",
	"junie":       "Junie",
	"warp":        "Warp",
	"antigravity": "Antigravity",
	"crush":       "Crush",
	"amp":         "Amp",
	"goose":       "Goose",
	"ellipsis":    "Ellipsis",
	"pi":          "Pi coding agent",
	"qwen":        "Qwen Code (Alibaba)",
	"grok":        "Grok Build",
	"vibe":        "Mistral Vibe",
	"kimi":        "Kimi Code CLI",
	"codebuddy":   "CodeBuddy Code (Tencent)",
	"jules":       "Jules",
	"openhands":   "OpenHands",
}

// TargetLabel returns the human name of a target, or the id itself when
// no name is known.
func TargetLabel(target string) string {
	if label, ok := targetLabels[target]; ok {
		return label
	}
	return target
}

// effectLabels is the glossary entry for each effect.
var effectLabels = map[Effect]string{
	EffectAllowed:     "Permitted",
	EffectAsk:         "Permitted, approval required",
	EffectDenied:      "Blocked",
	EffectUnavailable: "Unavailable through this route",
	EffectDefault:     "No rule; the tool's default applies",
	EffectUnresolved:  "Unresolved on this tool",
}

// conditionalPhrases words a known effect for a sentence that starts
// with "Would be".
var conditionalPhrases = map[Effect]string{
	EffectAllowed:     "permitted",
	EffectAsk:         "permitted after approval",
	EffectDenied:      "blocked",
	EffectUnavailable: "unavailable through this route",
}

// routePhrases words an open route's effect inside a sentence.
var routePhrases = map[Effect]string{
	EffectAllowed: "permitted",
	EffectAsk:     "permitted with approval",
	EffectDefault: "not decided by any rule",
}

// fileActionLabels names the operations on local files.
var fileActionLabels = []actionInfo{
	{action: "read", label: "Read"},
	{action: "edit", label: "Edit"},
	{action: "write", label: "Create or overwrite"},
	{action: "delete", label: "Delete"},
}

// anyToolAction is the operation that stands for every tool of an MCP
// server that no rule names.
const anyToolAction = "*"

// humanizeTool turns an MCP tool name into a label, such as
// create_pull_request into "Create pull request".
func humanizeTool(name string) string {
	if preview.RedactText(name) != name {
		return preview.Redacted
	}
	var b strings.Builder
	prevLower := false
	for _, r := range name {
		switch {
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(' ')
			prevLower = false
			continue
		case unicode.IsUpper(r) && prevLower:
			b.WriteRune(' ')
		}
		b.WriteRune(unicode.ToLower(r))
		prevLower = unicode.IsLower(r) || unicode.IsDigit(r)
	}
	words := strings.Fields(b.String())
	if len(words) == 0 {
		return name
	}
	text := strings.Join(words, " ")
	return strings.ToUpper(text[:1]) + text[1:]
}

// slug turns text into an id fragment of lowercase letters, digits, and
// single dashes. Wildcards keep a readable trace so `src/**` and `src/*`
// stay apart.
func slug(text string) string {
	if shown := preview.RedactText(text); shown != text {
		return "redacted-" + opaqueID(text)[:16]
	}
	text = strings.NewReplacer("**", " all ", "*", " any ", "?", " one ").Replace(text)
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteRune('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "x"
	}
	return out
}

// sentence capitalizes the first letter of s.
func sentence(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// explanations is the glossary every view carries.
func explanations() Explanations {
	effects := make(map[string]string, len(effectLabels))
	for effect, label := range effectLabels {
		effects[string(effect)] = label
	}
	return Explanations{
		Effects: effects,
		Resolution: map[string]string{
			ResolutionTranslated:           "The adapter writes the winning rule in the tool's own format, and this repository documents how the tool matches that rule form.",
			ResolutionTranslatedUnverified: "The adapter writes a rule, but this repository does not document how the tool matches that rule form, so the effect stays unresolved.",
			ResolutionHook:                 "A generated hook checks the action before the tool runs it.",
			ResolutionNotExpressed:         "The specs decide this, but sync writes nothing for it on this tool, so the tool's own default applies.",
			ResolutionAgentScope:           "The subject's own spec (can, tools, readonly, or mcpServers) decides this, on a tool that writes that narrowing.",
			ResolutionNoRule:               "No rule names this action; the tool's own default decides.",
			ResolutionDisabled:             "The server's spec sets disabled: true, and the tool honors it or never receives the server.",
		},
		Lists: map[string]string{
			"allow":      "Approved without prompting.",
			"ask":        "The tool asks before running it.",
			"deny":       "Always blocked.",
			"protected":  "A protected path from a settings spec: edits ask first or are blocked, by the block's decision.",
			"can":        "The agent's neutral capability list; anything it leaves out is unavailable to the agent.",
			"tools":      "The agent's tool list in tool names; anything it leaves out is unavailable to the agent.",
			"readonly":   "readonly: true leaves out edits, writes, and deletes.",
			"mcpServers": "The MCP servers the agent may reach; any other server is unavailable to it.",
		},
		Enforcement: map[string]string{
			"native-rule":    "The tool's own configuration carries the decision.",
			"hook":           "A generated hook carries the decision.",
			"not-expressed":  "Sync writes nothing for this decision on the tool; its own defaults apply.",
			"not-applicable": "There is no decision to carry, such as when no rule applies.",
		},
		Installed: map[string]string{
			InstalledInSync:       "The generated file matches what sync would write.",
			InstalledStale:        "The generated file differs from what sync would write; run sync.",
			InstalledNotGenerated: "Sync has not written the file yet; run sync.",
			InstalledNotChecked:   "Generated files were not compared with disk.",
			installedNotApplied:   "No generated file carries this decision.",
		},
		Rights: map[string]string{
			"discover":     "Whether the subject can learn that the secret exists and what it is called.",
			"readValue":    "Whether the subject can read the secret's value.",
			"use":          "Whether the subject can act with the secret through the provider that references it.",
			"manage":       "Whether the subject can rotate or revoke the secret.",
			"allowed":      "The configuration grants this right.",
			"unavailable":  "The subject's scope or the provider's status leaves this right out.",
			"not-readable": "The configuration keeps the value out of the subject's reach.",
			"unknown":      "The configuration does not decide this right.",
		},
	}
}

// installedNotApplied marks an operation whose decision no generated
// file carries.
const installedNotApplied = "not-applicable"

// supportedActions lists what a client may offer.
func supportedActions() []SupportedAction {
	return []SupportedAction{
		{
			ID:          "propose-change",
			Method:      "POST",
			Path:        "/api/views/authority/impact",
			Description: "Compute the effect of adding or removing permission rules, without writing anything.",
			Status:      "available",
		},
		{
			ID:          "apply-change",
			Description: "Editing the spec and running sync is the only way to apply; applying through the API needs revision checks, approval, and rollback first.",
			Status:      "unsupported",
		},
	}
}

func opaqueID(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}
