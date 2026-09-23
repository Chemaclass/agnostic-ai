package factory

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// The three keys Droid CLI evaluates every shell command against.
// Each is a `string[]` of "Shell command patterns ... (accumulated
// across levels)"
// (docs.factory.ai/enterprise/hierarchical-settings-and-org-control).
const (
	allowlistKey = "commandAllowlist"
	denylistKey  = "commandDenylist"
	blocklistKey = "commandBlocklist"
)

// commandListKey maps each portable list onto the Factory key with the
// same behavior. The names do not line up, and taking them at face
// value is the bug this table exists to prevent.
//
// Factory's denylist prompts and can then be approved: "Commands in
// this array always require confirmation ... A denied command can
// still be run if you explicitly approve it" (/droid-cli/settings),
// stated again on /enterprise/hierarchical-settings-and-org-control
// ("always require confirmation ... use `commandBlocklist` for a hard
// block") and /enterprise/llm-safety-and-agent-controls ("A denylisted
// command can still run if the user approves it"). That is the
// portable `ask` tier, not `deny`.
//
// The hard stop is the blocklist: "Commands in this array can never
// run. Unlike the denylist, there is no prompt and no way to approve
// them: the block applies even under full autonomy, auto-run, or
// `--skip-permissions-unsafe`." That is the portable `deny`.
//
// Precedence needs no work here, because Factory's matches this
// project's own resolution order: "Commands that appear in both the
// allowlist and denylist default to the denylist behavior. The
// blocklist always takes precedence over both" (target-audit
// 2026-09-20, #948).
var commandListKey = map[string]string{
	"allow": allowlistKey,
	"ask":   denylistKey,
	"deny":  blocklistKey,
}

// commandListOrder fixes the order the lists are built in, since Go
// map iteration is random and two syncs of the same specs must produce
// the same file.
var commandListOrder = []string{"allow", "ask", "deny"}

// permissionsNonShellReason explains, in the flushed coverage note,
// which rules stop here. The three command lists are shell-command
// patterns, so a rule scoping a path, a URL, or an MCP tool has no
// spelling among them, and neither does a bare tool name, for which
// the vendor publishes no catch-all pattern. Factory's `sandbox` block
// does gate file and network access, but by kernel enforcement rather
// than approval, so it is not a substitute; reach it through
// `x-factory` when that is what is wanted.
const permissionsNonShellReason = "Factory's commandAllowlist, commandDenylist, and commandBlocklist take shell-command patterns, so a rule scoping a path, URL, or MCP tool has no spelling there; set x-factory keys for those"

// emitSettings merges the portable default model and permission policy
// into the project-tier `<git-root>/.factory/settings.json` (override
// via outputs.factory.conf-file). Factory documents the project tier on
// its hierarchical-settings page, not on the CLI settings page whose
// "Where settings live" table lists the user tier alone: "Settings are
// authored in `.factory/` folders, using the same schema at every
// level", with the levels table rowing "**Project** | `<git-root>/.factory/`".
// The skills page corroborates it: "the **Project** tab writes to
// `<project>/.factory/settings.json`".
//
// MergeJSONFile, not a whole-document write: this file is shared with
// every other key Droid CLI or a maintainer puts there, `disabledSkills`
// among them. Only `model`, the three command lists, and the keys an
// author writes under `x-factory` are ever set. A command list is set
// only when at least one rule translates into it, so a hand-written
// list survives a sync that has nothing to put there.
//
// That `x-factory` block is how a Factory-only key reaches this file.
// `sandbox` is the case that earned it: kernel-enforced isolation whose
// `denyWrite` overrides `allowWrite`, with no `ask` tier and an egress
// filter no portable field matches (#949).
//
// An author writing one of the three command lists there adds to it:
// MergeSettingsCustomKeys unions two lists rather than replacing one
// with the other, so a pattern the portable policy translated into
// `commandBlocklist`, the tier with no approval path, cannot fall out
// of the file because a Factory-only pattern was added beside it
// (#966).
// reasoningEffortLevels are the reasoningEffort values Droid CLI's
// settings reference lists; each model accepts a subset.
var reasoningEffortLevels = []string{"none", "dynamic", "off", "minimal", "low", "medium", "high", "xhigh", "max"}

// SettingsEffortLevels returns the values reasoningEffort accepts.
func (Adapter) SettingsEffortLevels() []string { return reasoningEffortLevels }

func emitSettings(sess *emit.Session, settings []spec.Entry, path string, dryRun bool) error {
	keys := map[string]any{}
	if model := emit.LastSettingsModel(settings); model != "" {
		keys["model"] = model
	}
	if level := emit.SettingsEffortLevel(settings, target, reasoningEffortLevels); level != "" {
		keys["reasoningEffort"] = level
	}
	lists, dropped := buildCommandLists(settings)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", dropped, permissionsNonShellReason)
	for key, patterns := range lists {
		keys[key] = patterns
	}
	emit.MergeSettingsCustomKeys(keys, settings, target)
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFile(path, keys, dryRun)
}

// buildCommandLists renders the portable allow, ask, and deny lists as
// Factory's three command lists, and reports how many settings specs
// carried at least one rule with no spelling there.
//
// Specs layer in source order and a repeated pattern emits once,
// matching how the same lists already merge for every other target
// with this surface. A list with no pattern is left out entirely
// rather than written empty, so an empty array never overwrites what
// a maintainer put in the file by hand.
func buildCommandLists(settings []spec.Entry) (map[string][]string, int) {
	dropped := map[int]bool{}
	out := map[string][]string{}
	for _, list := range commandListOrder {
		seen := map[string]bool{}
		var patterns []string
		for i, entry := range settings {
			permissions, _ := entry.Meta["permissions"].(map[string]any)
			for _, rule := range emit.StringSlice(permissions[list]) {
				pattern, ok := commandPattern(rule)
				if !ok {
					dropped[i] = true
					continue
				}
				if seen[pattern] {
					continue
				}
				seen[pattern] = true
				patterns = append(patterns, pattern)
			}
		}
		if len(patterns) > 0 {
			out[commandListKey[list]] = patterns
		}
	}
	return out, len(dropped)
}

// commandPattern translates one agnostic-ai permission rule into a
// Factory shell-command pattern, reporting false for anything with no
// form there rather than guessing one:
//
//	Bash(npm:*)   -> "npm *", the prefix-glob spelling
//	Bash(curl)    -> "curl", the bare spelling
//
// Both come from vendor examples: `"commandAllowlist": ["ls", "pwd",
// "dir"]` on /droid-cli/settings, and `"commandAllowlist": ["npm *",
// "pnpm *", "make *"]` on /enterprise/llm-safety-and-agent-controls.
//
// Everything else has no pattern form. A path- or URL-scoped rule and
// an MCP tool rule name no shell command at all. A bare `Bash` names
// every command, and Factory publishes no catch-all pattern for these
// lists, so it is not widened into one.
func commandPattern(rule string) (string, bool) {
	scope, arg, ok := spec.SplitPermissionRule(rule)
	if !ok || scope != "Bash" {
		return "", false
	}
	if prefix, isPrefixRule := strings.CutSuffix(arg, ":*"); isPrefixRule {
		if prefix == "" {
			return "", false
		}
		return prefix + " *", true
	}
	return arg, true
}
