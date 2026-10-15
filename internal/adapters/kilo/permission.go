package kilo

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionKey is the `kilo.jsonc` key holding the per-tool map, and
// the agent-frontmatter key holding the same shape: "Permissions are
// configured under the `permission` key in `kilo.jsonc`"
// (kilo.ai/docs/getting-started/settings/auto-approving-actions), and
// "This page focuses on Markdown agent files, where permission rules
// are written as YAML frontmatter under the `permission` key"
// (kilo.ai/docs/customize/agent-permissions).
const permissionKey = "permission"

// anyPattern is Kilo's catch-all. It works as a pattern inside one
// tool's map ("`*` | Any target for that permission") and as a
// top-level key across every tool ("Top-level permission keys follow
// the same rule ... `permission: {"*": ask, bash: allow}`").
const anyPattern = "*"

// permissionUntranslatedReason explains, in the flushed coverage note,
// why some rules did not reach `kilo.jsonc`.
const permissionUntranslatedReason = "rule(s) outside Kilo Code's own permission vocabulary have no key there; set x-kilo.permission with Kilo's own rules for those"

// agentToolsUntranslatedReason explains the same thing for an agent's
// `tools` list, which reaches the same map through frontmatter.
const agentToolsUntranslatedReason = "tool name(s) outside Kilo Code's own permission vocabulary have no key there; set x-kilo.permission with Kilo's own rules for those"

// permissionRule translates one agnostic-ai permission rule into the
// Kilo key and glob pattern it belongs under, reporting false for
// anything with no faithful spelling there:
//
//	Read(glob)        -> read,     glob
//	Write(glob)       -> write,    glob
//	Edit(glob)        -> edit,     glob
//	Bash(prefix:*)    -> bash,     "prefix *"
//	Bash(cmd)         -> bash,     "cmd"
//	WebFetch(pattern) -> webfetch, pattern
//	Read/Bash/...     -> read/bash/..., "*"
//	mcp__srv__tool    -> srv_tool, "*"
//
// Claude's `:*` prefix marker becomes Kilo's trailing ` *`: "`git *` |
// `git`, `git status`, `git log --oneline`, and other `git` commands".
// Without the marker the command is exact on both sides, so it passes
// through as written; unlike Devin's prefix-only `Exec`, Kilo can spell
// an exact command, so nothing has to widen here.
//
// Claude's `WebFetch(domain:example.test)` shorthand has no Kilo form:
// Kilo matches "the tool's arguments", so a literal `domain:` prefix
// would match no URL and the rule would be inert. It drops instead.
func permissionRule(rule string) (tool, pattern string, ok bool) {
	if server, name, found := spec.SplitMCPPermissionRule(rule); found {
		return server + "_" + name, anyPattern, true
	}
	if scope, arg, found := spec.SplitPermissionRule(rule); found {
		key, known := emit.CapabilityTool(toolCapabilities, scope, true)
		if !known {
			return "", "", false
		}
		if scope == "Bash" {
			if prefix, isPrefixRule := strings.CutSuffix(arg, ":*"); isPrefixRule {
				if prefix == "" {
					return "", "", false
				}
				return key, prefix + " *", true
			}
		}
		if scope == "WebFetch" && strings.HasPrefix(arg, "domain:") {
			return "", "", false
		}
		return key, arg, true
	}
	if key, known := emit.CapabilityTool(toolCapabilities, rule, true); known {
		return key, anyPattern, true
	}
	return "", "", false
}

// settingsPermission builds the `permission` map from the portable
// allow, deny, and ask lists across every settings spec, and reports
// how many specs carried at least one rule with no Kilo spelling so the
// caller can fold them into one coverage note per sync.
//
// The three lists are walked allow, then ask, then deny, so a rule
// repeated across lists resolves to the most restrictive action rather
// than to whichever spec happened to come last.
//
// Tool keys sort alphabetically, so the top-level `*` comes first. Inside
// one tool, "the last matching rule wins" in Kilo, while Claude Code
// evaluates deny, then ask, then allow, whatever the pattern. So the
// patterns are written allow first, then ask, then deny, each group in
// alphabetical order (`*` first, as Kilo asks: "Put broad fallbacks
// first and exceptions after them"). A deny or ask then wins over any
// allow it overlaps, as it does in Claude Code.
func settingsPermission(settings []spec.Entry) (map[string]any, int) {
	dropped := map[int]bool{}
	out := map[string]any{}
	for _, list := range []string{"allow", "ask", "deny"} {
		for i, entry := range settings {
			rules, native := entryRules(entry, list)
			if native {
				continue
			}
			for _, rule := range rules {
				tool, pattern, ok := permissionRule(rule)
				if !ok {
					dropped[i] = true
					continue
				}
				for _, key := range restrictedKeys(tool, list) {
					patterns, _ := out[key].(map[string]any)
					if patterns == nil {
						patterns = map[string]any{}
						out[key] = patterns
					}
					patterns[pattern] = list
				}
			}
		}
	}
	for tool, value := range out {
		out[tool] = byAction(value.(map[string]any))
	}
	// Native maps merge last, one tool key at a time, so an author
	// writing under the kilo namespace wins over any translated rule
	// for the same tool without wiping a sibling spec's rules for
	// other tools. Same deal `x-kilo.permission` already gets on an
	// agent: that author is presumed to know Kilo's own spelling.
	for _, entry := range settings {
		native, _ := emit.SettingsCustomObject(entry, target, permissionKey)
		for tool, value := range native {
			out[tool] = value
		}
	}
	if len(out) == 0 {
		return nil, len(dropped)
	}
	return out, len(dropped)
}

// byAction orders one tool's patterns allow, ask, then deny, each group
// alphabetical, so the strictest action is the last match.
func byAction(patterns map[string]any) *emit.OrderedJSON {
	out := emit.NewOrderedJSON()
	for _, action := range []string{"allow", "ask", "deny"} {
		var group []string
		for pattern, a := range patterns {
			if a == action {
				group = append(group, pattern)
			}
		}
		slices.Sort(group)
		for _, pattern := range group {
			_ = out.Set(pattern, action)
		}
	}
	return out
}

// restrictedKeys returns the Kilo keys a rule for tool lands under in
// list. Claude Code's Edit rules "apply to all built-in tools that edit
// files" (code.claude.com/docs/en/permissions), and Kilo keeps `write`
// apart from `edit` ("File tools such as `read`, `edit`, and `write`
// resolve the input path first"), so an Edit deny or ask also covers
// `write`; otherwise `edit(.env)` would leave `.env` writable. An allow
// stays on `edit` alone, which grants no more than it says.
func restrictedKeys(tool, list string) []string {
	if tool == "edit" && list != "allow" {
		return []string{"edit", "write"}
	}
	return []string{tool}
}

// entryRules returns one settings spec's rules for list, and whether
// they already hold Kilo's own vocabulary under `x-kilo.permission`.
func entryRules(entry spec.Entry, list string) (rules []string, native bool) {
	if _, ok := emit.SettingsCustomObject(entry, target, permissionKey); ok {
		return nil, true
	}
	permissions, _ := entry.Meta["permissions"].(map[string]any)
	return emit.StringSlice(permissions[list]), false
}

// agentPermission turns an agent's portable `tools` allowlist into the
// `permission` frontmatter Kilo actually reads. A tools list is an
// allowlist, so the map denies everything first and re-allows the names
// that translated: "Top-level permission keys follow the same rule",
// with the vendor's own example `permission: {"*": ask, bash: allow}`
// showing a top-level catch-all overridden by a later key.
//
// Returns nil when no name translated, so an agent whose whole list is
// untranslatable keeps its default permissions rather than being locked
// out by a bare `{"*": "deny"}` nobody asked for. unmapped reports
// whether at least one name had no Kilo key, for the coverage note.
//
// Kilo appends agent rules after the project ones and the last match
// wins (packages/opencode/src/agent/agent.ts merges `defaults, user`,
// then the agent's `value.permission`; permission/index.ts evaluates
// with `findLast`). So each key keeps the project's ask and deny rules
// after the agent's allow (see restrictToProject). The catch-all would
// also deny `external_directory` and `doom_loop`; both default to "ask"
// (kilo.ai/docs/getting-started/settings/auto-approving-actions,
// "Default behavior"), so they go back to that, still under the
// project's own rules. project is the `kilo.jsonc` permission map
// settingsPermission builds, and may be nil.
func agentPermission(tools []string, project map[string]any) (perms map[string]any, unmapped bool) {
	allowed := map[string]any{}
	for _, name := range tools {
		key, pattern, known := permissionRule(name)
		if !known {
			unmapped = true
			continue
		}
		if pattern == anyPattern {
			allowed[key] = "allow"
			continue
		}
		if allowed[key] == "allow" {
			continue
		}
		patterns, _ := allowed[key].(map[string]any)
		if patterns == nil {
			patterns = map[string]any{anyPattern: "deny"}
			allowed[key] = patterns
		}
		patterns[pattern] = "allow"
	}

	if len(allowed) == 0 {
		return nil, unmapped
	}
	allowed["doom_loop"] = "ask"
	allowed["external_directory"] = "ask"
	for key, value := range allowed {
		allowed[key] = restrictToProject(value, projectRules(project, key))
	}
	allowed[anyPattern] = "deny"
	return allowed, unmapped
}

// patternRule is one Kilo pattern and the action it maps to.
type patternRule struct{ pattern, action string }

var actionStrictness = map[string]int{"allow": 0, "ask": 1, "deny": 2}

// stricter returns whichever of two actions grants less.
func stricter(a, b string) string {
	if actionStrictness[b] > actionStrictness[a] {
		return b
	}
	return a
}

// restrictToProject lays the project's ask and deny rules for one key
// over the agent's value so the agent is never looser than the project
// or than its own list. A project catch-all applies in place, raising
// each agent rule to at least its action, so an agent scoped to `git *`
// keeps its leading `"*": deny`. A project deny goes after the agent's
// rules and wins as the last match; a project ask does the same only
// inside what the agent allows (see askWithinAllowed). Project allows
// stay out: after the agent's deny they would widen its list.
func restrictToProject(value any, project []patternRule) any {
	if len(project) == 0 {
		return value
	}
	var rules []patternRule
	switch v := value.(type) {
	case string:
		rules = []patternRule{{anyPattern, v}}
	case map[string]any:
		rules = []patternRule{{anyPattern, v[anyPattern].(string)}}
		for _, pattern := range slices.Sorted(maps.Keys(v)) {
			if pattern != anyPattern {
				rules = append(rules, patternRule{pattern, v[pattern].(string)})
			}
		}
	default:
		return value
	}
	for _, rule := range project {
		if rule.action == "allow" {
			continue
		}
		if rule.pattern == anyPattern {
			for i := range rules {
				rules[i].action = stricter(rules[i].action, rule.action)
			}
			continue
		}
		if rule.action == "ask" && !askWithinAllowed(rules, rule.pattern) {
			continue
		}
		rules = slices.DeleteFunc(rules, func(r patternRule) bool { return r.pattern == rule.pattern })
		rules = append(rules, rule)
	}
	if len(rules) == 1 && rules[0].pattern == anyPattern {
		return rules[0].action
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, rule := range rules {
		node.Content = append(node.Content, yamlString(rule.pattern), yamlString(rule.action))
	}
	return node
}

// yamlString is a string scalar the encoder quotes when a YAML 1.1
// reader would take it for something else: `~` and `null` already get
// quotes from the !!str tag, `yes` or `on` (booleans in YAML 1.1) do not.
func yamlString(value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	switch strings.ToLower(value) {
	case "y", "n", "yes", "no", "on", "off":
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

// askWithinAllowed reports whether a project ask on pattern belongs
// after the agent's rules: only when an allowed agent pattern contains
// it, since after the agent's catch-all deny an ask anywhere else would
// loosen that deny. An allowed pattern that may overlap the ask without
// containing it, such as `git status *` under `git *`, becomes "ask"
// in place instead.
func askWithinAllowed(rules []patternRule, pattern string) bool {
	within := false
	for i, rule := range rules {
		if rule.action != "allow" {
			continue
		}
		if wildcardMatch(pattern, rule.pattern) {
			within = true
			continue
		}
		if mayOverlap(rule.pattern, pattern) {
			rules[i].action = "ask"
		}
	}
	return within
}

// mayOverlap reports whether some input could match both patterns.
// Every match starts with the pattern's text before its first wildcard,
// so two patterns whose literal prefixes disagree are disjoint.
func mayOverlap(a, b string) bool {
	pa, pb := literalPrefix(a), literalPrefix(b)
	return strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa)
}

// literalPrefix is the text before the first wildcard, without the
// space Kilo's optional trailing ` *` would need.
func literalPrefix(pattern string) string {
	if i := strings.IndexAny(pattern, "*?"); i >= 0 {
		pattern = pattern[:i]
	}
	return strings.TrimRight(pattern, " ")
}

// wildcardMatch mirrors Kilo's Wildcard.match
// (packages/opencode/src/util/wildcard.ts): `*` matches any run, `?`
// one character, and a trailing ` *` also matches the bare command.
func wildcardMatch(input, pattern string) bool {
	input = strings.ReplaceAll(input, "\\", "/")
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	var expr strings.Builder
	body, optionalTail := strings.CutSuffix(pattern, " *")
	for _, r := range body {
		switch r {
		case '*':
			expr.WriteString(".*")
		case '?':
			expr.WriteString(".")
		default:
			expr.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if optionalTail {
		expr.WriteString("( .*)?")
	}
	re, err := regexp.Compile("(?s)^" + expr.String() + "$")
	return err == nil && re.MatchString(input)
}

// projectRules lists the project's rules for key in evaluation order,
// falling back to the project's top-level catch-all when key has none.
func projectRules(project map[string]any, key string) []patternRule {
	value, set := project[key]
	if !set {
		value, set = project[anyPattern]
	}
	if !set {
		return nil
	}
	switch v := value.(type) {
	case string:
		return []patternRule{{anyPattern, v}}
	case *emit.OrderedJSON:
		var rules []patternRule
		for _, pattern := range v.Keys() {
			raw, _ := v.Get(pattern)
			var action string
			if json.Unmarshal(raw, &action) == nil {
				rules = append(rules, patternRule{pattern, action})
			}
		}
		return rules
	case map[string]any:
		var rules []patternRule
		for _, pattern := range slices.Sorted(maps.Keys(v)) {
			if action, ok := v[pattern].(string); ok {
				rules = append(rules, patternRule{pattern, action})
			}
		}
		return rules
	}
	return nil
}
