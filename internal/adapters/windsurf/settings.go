package windsurf

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionsKey is the project-config key holding the three rule lists.
const permissionsKey = "permissions"

// modelUserOnlyReason explains why a portable `model` never reaches
// `.devin/config.json`: Devin marks the whole `agent` block user only,
// and its project-config section lists the three keys that are not.
const modelUserOnlyReason = "Devin marks the agent.model block user only; a project config takes permissions, read_config_from, and hooks"

// rulesUntranslatedReason explains, in the flushed coverage note, why
// some rules did not reach the file. Devin's Exec is a prefix matcher
// with no exact-command form, so an exact `Bash(git status)` would
// widen into `Exec(git status)` and auto-approve `git status --short`
// too; that is the fail-open direction on allow and ask, so the rule
// drops there. On deny the same widening blocks more than was asked
// for, which is fail-safe, so a deny rule keeps it.
const rulesUntranslatedReason = "rule(s) outside Devin's Read/Write/Exec/Fetch/mcp__ vocabulary have no faithful spelling there; set x-windsurf.permissions with Devin's own rules for those"

// emitConfig merges the portable allow, deny, and ask lists into
// `.devin/config.json` (default; override via outputs.windsurf.conf-file)
// under `permissions`, translated onto Devin's own rule vocabulary.
//
// Routes through MergeJSONFileNested so the rest of that file survives
// a sync: `read_config_from` and `hooks`, the only other keys Devin
// accepts in a project config, plus any sibling inside `permissions`
// itself that a newer CLI writes there. Only `permissions` is ever set.
//
// No file is written when no rule translated, so a project with only a
// portable `model` never gains a `.devin/config.json` at all.
func emitConfig(sess *emit.Session, settings []spec.Entry, path string, dryRun bool) error {
	emit.NoteFieldNoOp(target, spec.KindSettings, "model", specsWithModel(settings), modelUserOnlyReason)
	permissions, dropped := devinPermissions(settings)
	emit.NoteFieldNoOp(target, spec.KindSettings, permissionsKey, dropped, rulesUntranslatedReason)
	if permissions == nil {
		return nil
	}
	keys := map[string]any{permissionsKey: permissions}
	return sess.MergeJSONFileNested(path, keys, []string{permissionsKey}, dryRun)
}

func specsWithModel(settings []spec.Entry) int {
	n := 0
	for _, entry := range settings {
		if model, _ := entry.Meta["model"].(string); model != "" {
			n++
		}
	}
	return n
}

// devinPermissions merges the three lists across settings specs in
// source order, translating each rule onto Devin's vocabulary and
// deduplicating on the translated spelling, since Write and Edit
// collapse onto the same rule. It also reports how many specs carried
// at least one rule with no faithful Devin spelling, so the caller can
// fold them into one coverage note per sync.
func devinPermissions(settings []spec.Entry) (map[string]any, int) {
	dropped := map[int]bool{}
	out := map[string]any{}
	for _, list := range []string{"allow", "deny", "ask"} {
		seen := map[string]bool{}
		var values []string
		for i, entry := range settings {
			rules, native := entryRules(entry, list)
			for _, rule := range rules {
				mapped := rule
				if !native {
					translated, ok := devinPermissionRule(rule, list)
					if !ok {
						dropped[i] = true
						continue
					}
					mapped = translated
				}
				if mapped == "" || seen[mapped] {
					continue
				}
				seen[mapped] = true
				values = append(values, mapped)
			}
		}
		if len(values) > 0 {
			out[list] = values
		}
	}
	if len(out) == 0 {
		return nil, len(dropped)
	}
	return out, len(dropped)
}

// entryRules returns one settings spec's rules for list, and whether
// they already hold Devin's own vocabulary. An `x-windsurf.permissions`
// object wins outright over the portable one, the same deal
// `x-windsurf.allowed-tools` gets on an agent: an author writing under
// the windsurf namespace is presumed to know Devin's spelling.
func entryRules(entry spec.Entry, list string) (rules []string, native bool) {
	if custom, _ := emit.CustomTargetMeta(entry.Meta, target); custom != nil {
		if permissions, ok := custom[permissionsKey].(map[string]any); ok {
			return emit.StringSlice(permissions[list]), true
		}
	}
	permissions, _ := entry.Meta[permissionsKey].(map[string]any)
	return emit.StringSlice(permissions[list]), false
}

// devinPermissionTool maps agnostic-ai's Claude-style tool identifiers
// onto the bare tool names Devin's `permissions` lists accept. It is
// keyed separately from the subagent map in agent.go: the two surfaces
// draw their vocabularies from different pages and have already moved
// apart once (#951).
//
// `/cli/reference/permissions` enumerates five names, "**Available
// tool names:** `read`, `edit`, `grep`, `glob`, `exec`", and lags its
// own changelog. The CLI changelog's v3000.10.21 entry (2026-09-10)
// adds a sixth under `### Fixed`: "`web_search` can now be used as a
// tool name in `permissions.deny` / `permissions.ask` /
// `permissions.allow`; previously it was rejected and web searches
// were always auto-approved."
//
// `webfetch` stays out. It is a real tool name, but only in the
// lifecycle-hooks tool table and a user-tier `disabled_tools` example,
// and neither governs `permissions`. `web_search` itself is the proof:
// it shipped as a tool in May 2026 and `permissions` rejected it until
// September. Only a sentence about `permissions` licenses an entry
// here.
var devinPermissionTool = map[string]string{
	"Read":      "read",
	"Grep":      "grep",
	"Glob":      "glob",
	"Bash":      "exec",
	"Write":     "edit",
	"Edit":      "edit",
	"WebSearch": "web_search",
}

// devinPermissionRule translates one agnostic-ai permission rule onto
// Devin's own vocabulary, reporting false for anything with no faithful
// spelling there rather than guessing one
// (docs.devin.ai/cli/reference/permissions):
//
//	Read(glob)        -> Read(glob), identical syntax and glob semantics
//	Write(glob)       -> Write(glob)
//	Edit(glob)        -> Write(glob), Devin's one file-mutation scope
//	Bash(prefix:*)    -> Exec(prefix), both prefix matchers
//	WebFetch(pattern) -> Fetch(pattern), same `domain:` shorthand
//	Read/Bash/...     -> read/exec/..., the bare names in devinPermissionTool
//	WebSearch         -> web_search
//	mcp__server__tool -> unchanged, the spelling Devin documents too
//
// An exact `Bash(cmd)` turns on which list holds it, because Devin has
// no exact-command form: Exec matches any command starting with the
// prefix, so the translation always widens. On allow and ask that
// approves commands the author never listed, so the rule drops. On deny
// it blocks more than was asked for, which is the safe direction, and
// Devin's changelog for v3000.10.31 states a command deny outranks a
// broader allow or ask, so deny takes `Exec(cmd)` over nothing at all.
func devinPermissionRule(rule, list string) (string, bool) {
	if strings.HasPrefix(rule, spec.MCPToolPrefix) {
		return rule, true
	}
	if scope, arg, ok := spec.SplitPermissionRule(rule); ok {
		switch scope {
		case "Read":
			return "Read(" + arg + ")", true
		case "Write", "Edit":
			return "Write(" + arg + ")", true
		case "WebFetch":
			return "Fetch(" + arg + ")", true
		case "Bash":
			prefix, isPrefixRule := strings.CutSuffix(arg, ":*")
			if isPrefixRule {
				if prefix == "" {
					return "", false
				}
				return "Exec(" + prefix + ")", true
			}
			if list != "deny" {
				return "", false
			}
			return "Exec(" + arg + ")", true
		}
		return "", false
	}
	if name, ok := devinPermissionTool[rule]; ok {
		return name, true
	}
	return "", false
}
