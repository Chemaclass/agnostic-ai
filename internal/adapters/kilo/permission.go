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
// The catch-all would also deny `external_directory` and `doom_loop`;
// both default to "ask" (kilo.ai/docs/getting-started/settings/auto-approving-actions,
// "Default behavior"), so they go back to that. Every rule the agent
// writes then passes through restrictToBase, so neither Kilo's defaults
// nor the project's ask and deny rules lose to it. project is the
// `kilo.jsonc` permission map settingsPermission builds, and may be nil.
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
	base := withoutShadowed(append(kiloDefaults(), flatRules(project)...))
	for key, value := range allowed {
		allowed[key] = restrictToBase(key, value, base)
	}
	allowed[anyPattern] = "deny"
	return allowed, unmapped
}

// flatRule is one entry of the single list Kilo evaluates: a permission
// key pattern, a target pattern, and an action.
type flatRule struct{ key, pattern, action string }

// patternRule is one target pattern of an agent key and its action.
type patternRule struct{ pattern, action string }

// kiloDefaults are the ask and deny rules Kilo puts before every config
// (packages/opencode/src/agent/agent.ts `baseDefaults`, then
// `KiloAgent.prepare(...).defaultsPatch` in src/kilocode/agent/index.ts).
// Kilo's allows are left out: they never restrict an agent.
func kiloDefaults() []flatRule {
	return []flatRule{
		{"doom_loop", anyPattern, "ask"},
		{"external_directory", anyPattern, "ask"},
		{"suggest", anyPattern, "deny"},
		{"question", anyPattern, "deny"},
		{"plan_enter", anyPattern, "deny"},
		{"plan_exit", anyPattern, "deny"},
		{"repo_clone", anyPattern, "deny"},
		{"repo_overview", anyPattern, "deny"},
		{"read", "*.env", "ask"},
		{"read", "*.env.*", "ask"},
		{"read", "*.env.example", "allow"},
		{"recall", anyPattern, "ask"},
		{"kilo_memory_recall", anyPattern, "ask"},
		{"kilo_memory_save", anyPattern, "ask"},
	}
}

// flatRules lists the project's rules in the order `kilo.jsonc` holds
// them: tool keys sorted, as the JSON encoder writes a map, and each
// key's patterns in their written order.
func flatRules(project map[string]any) []flatRule {
	var rules []flatRule
	for _, key := range slices.Sorted(maps.Keys(project)) {
		switch v := project[key].(type) {
		case string:
			rules = append(rules, flatRule{key, anyPattern, v})
		case *emit.OrderedJSON:
			for _, pattern := range v.Keys() {
				raw, _ := v.Get(pattern)
				var action string
				if json.Unmarshal(raw, &action) == nil {
					rules = append(rules, flatRule{key, pattern, action})
				}
			}
		case map[string]any:
			for _, pattern := range slices.Sorted(maps.Keys(v)) {
				if action, ok := v[pattern].(string); ok {
					rules = append(rules, flatRule{key, pattern, action})
				}
			}
		}
	}
	return rules
}

// withoutShadowed drops each rule a later one covers entirely, so a
// project rule that overrides one of Kilo's defaults also overrides it
// for the agent.
func withoutShadowed(rules []flatRule) []flatRule {
	var kept []flatRule
	for i, rule := range rules {
		shadowed := slices.ContainsFunc(rules[i+1:], func(later flatRule) bool {
			return patternContains(later.key, rule.key) && patternContains(later.pattern, rule.pattern)
		})
		if !shadowed {
			kept = append(kept, rule)
		}
	}
	return kept
}

// restrictToBase keeps each ask and deny rule of the defaults and the
// project in force for one agent key. Kilo evaluates defaults, project,
// then agent as one list, and the last match wins
// (packages/opencode/src/agent/agent.ts merges `defaults, user` and then
// the agent's `value.permission`; permission/index.ts evaluates with
// `findLast`, matching the key and the pattern as wildcards). So for
// every base rule whose key matches and every agent rule that is not a
// deny:
//
//   - a base pattern that contains the agent's raises it in place;
//   - one inside the agent's goes after the agent rules;
//   - a partial overlap goes after them as the exact intersection when
//     that has a spelling (see intersection), or raises the agent's
//     pattern in place, stricter than needed but never looser.
//
// Base rules that overlap nothing the agent allows are left out: the
// agent's catch-all deny already covers them. Base allows never copy, as
// after the agent's deny they would widen its list. The result is never
// looser than the project, Kilo's defaults, or the agent's own list.
func restrictToBase(key string, value any, base []flatRule) any {
	rules := agentPatternRules(value)
	if rules == nil {
		return value
	}
	var after []patternRule
	changed := false
	add := func(rule patternRule) {
		after = slices.DeleteFunc(after, func(r patternRule) bool { return r.pattern == rule.pattern })
		after = append(after, rule)
		changed = true
	}
	for _, b := range base {
		if b.action == "allow" || !wildcardMatch(key, b.key) {
			continue
		}
		for i, r := range rules {
			if r.action == "deny" || stricter(r.action, b.action) == r.action {
				continue
			}
			switch {
			case patternContains(b.pattern, r.pattern):
				rules[i].action = b.action
				changed = true
			case patternContains(r.pattern, b.pattern):
				add(patternRule{b.pattern, b.action})
			case !mayOverlap(r.pattern, b.pattern):
			default:
				if parts, ok := intersection(r.pattern, b.pattern); ok {
					for _, part := range parts {
						add(patternRule{part, b.action})
					}
					continue
				}
				rules[i].action = b.action
				changed = true
			}
		}
	}
	if !changed {
		return value
	}
	rules = append(rules, after...)
	if len(rules) == 1 && rules[0].pattern == anyPattern {
		return rules[0].action
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, rule := range rules {
		node.Content = append(node.Content, yamlString(rule.pattern), yamlString(rule.action))
	}
	return node
}

// agentPatternRules lists one agent key's rules in the order the
// frontmatter writes them: the catch-all, then the sorted patterns.
func agentPatternRules(value any) []patternRule {
	switch v := value.(type) {
	case string:
		return []patternRule{{anyPattern, v}}
	case map[string]any:
		rules := []patternRule{{anyPattern, v[anyPattern].(string)}}
		for _, pattern := range slices.Sorted(maps.Keys(v)) {
			if pattern != anyPattern {
				rules = append(rules, patternRule{pattern, v[pattern].(string)})
			}
		}
		return rules
	}
	return nil
}

var actionStrictness = map[string]int{"allow": 0, "ask": 1, "deny": 2}

// stricter returns whichever of two actions grants less.
func stricter(a, b string) string {
	if actionStrictness[b] > actionStrictness[a] {
		return b
	}
	return a
}

// patternContains reports whether every input inner matches also
// matches outer. It reads inner as text: outer's wildcards absorb
// inner's, which holds for `*`, while a `?` in outer could not absorb a
// run, so that case reports false. A trailing ` *` in inner also
// matches the bare command, so outer has to match that too.
func patternContains(outer, inner string) bool {
	if strings.Trim(outer, "*") == "" {
		return true
	}
	if strings.Contains(outer, "?") && strings.ContainsAny(inner, "*?") {
		return false
	}
	if bare, optional := strings.CutSuffix(inner, " *"); optional && !wildcardMatch(bare, outer) {
		return false
	}
	return wildcardMatch(inner, outer)
}

// mayOverlap reports whether some input could match both patterns.
// Every match starts with the text before a pattern's first wildcard
// and ends with the text after its last, so patterns that disagree on
// either are disjoint.
func mayOverlap(a, b string) bool {
	pa, pb := literalPrefix(a), literalPrefix(b)
	sa, sb := literalSuffix(a), literalSuffix(b)
	return (strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa)) &&
		(strings.HasSuffix(sa, sb) || strings.HasSuffix(sb, sa))
}

// literalPrefix is the text before the first wildcard, without the
// space Kilo's optional trailing ` *` would need.
func literalPrefix(pattern string) string {
	if i := strings.IndexAny(pattern, "*?"); i >= 0 {
		pattern = pattern[:i]
	}
	return strings.TrimRight(pattern, " ")
}

// literalSuffix is the text after the last wildcard.
func literalSuffix(pattern string) string {
	if i := strings.LastIndexAny(pattern, "*?"); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}

// intersection spells the inputs matching both an agent pattern `L*`
// (literal L, wildcards only at the end) and a base pattern `*M`, as
// patterns that each lie inside both: `L*M`, plus `L[:n-k]M` wherever
// M's match can start k characters inside L, plus the bare command when
// the agent's trailing ` *` makes it match and the base pattern takes
// it. It reports false when it cannot be sure the list is complete.
func intersection(agent, base string) ([]string, bool) {
	stars := strings.TrimRight(agent, "*")
	if strings.ContainsAny(stars, "*?") || stars == agent || !strings.HasPrefix(base, "*") {
		return nil, false
	}
	lit, rest := stars, strings.TrimLeft(base, "*")
	head := rest
	if i := strings.IndexAny(rest, "*?"); i >= 0 {
		head = rest[:i]
	}
	parts := []string{lit + "*" + rest}
	for k := 1; k <= len(lit); k++ {
		tail := lit[len(lit)-k:]
		switch {
		case strings.HasPrefix(head, tail):
			parts = append(parts, lit[:len(lit)-k]+rest)
		case strings.HasPrefix(tail, head):
			return nil, false
		}
	}
	if bare, optional := strings.CutSuffix(agent, " *"); optional && wildcardMatch(bare, base) {
		parts = append(parts, bare)
	}
	return parts, true
}

// wildcardMatch mirrors Kilo's Wildcard.match
// (packages/opencode/src/util/wildcard.ts): `*` matches any run, `?`
// one character, and a trailing ` *` also matches the bare command
// ("This allows "ls *" to match both "ls" and "ls -la"").
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
