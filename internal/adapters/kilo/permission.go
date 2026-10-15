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
				patterns, _ := out[tool].(map[string]any)
				if patterns == nil {
					patterns = map[string]any{}
					out[tool] = patterns
				}
				patterns[pattern] = list
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
// "Default behavior"), so the agent asks for them at most. Each key is
// then built by agentKeyRules against Kilo's defaults and project, the
// final `kilo.jsonc` permission rules in file order (nil when there are
// none).
//
// Kilo matches a permission key as a wildcard too, so a whole MCP server
// `github_*` would also cover `github_ent_*`, the tools of another
// configured server (servers); that one gets its own deny unless the
// agent lists it whole. A project rule on a narrower key inside an
// allowed one, such as `github_create_issue`, gets an agent key of its
// own, built the same way, since the broader agent key would override it.
func agentPermission(tools []string, project []flatRule, servers []string) (perms map[string]any, unmapped bool) {
	allowed := map[string][]patternRule{}
	for _, name := range tools {
		key, pattern, known := permissionRule(name)
		if !known {
			unmapped = true
			continue
		}
		rules := allowed[key]
		if len(rules) == 1 && rules[0].pattern == anyPattern {
			continue
		}
		if pattern == anyPattern {
			rules = nil
		}
		if !slices.Contains(rules, patternRule{pattern, "allow"}) {
			rules = append(rules, patternRule{pattern, "allow"})
		}
		allowed[key] = rules
	}

	if len(allowed) == 0 {
		return nil, unmapped
	}
	allowed["doom_loop"] = []patternRule{{anyPattern, "ask"}}
	allowed["external_directory"] = []patternRule{{anyPattern, "ask"}}
	base := append(kiloDefaults(), project...)
	perms = map[string]any{anyPattern: "deny"}
	var denied []string
	for key := range allowed {
		server, whole := strings.CutSuffix(key, "_"+anyPattern)
		if !whole {
			continue
		}
		for _, other := range servers {
			sibling := other + "_" + anyPattern
			if other != server && strings.HasPrefix(other, server+"_") && allowed[sibling] == nil && !slices.Contains(denied, sibling) {
				denied = append(denied, sibling)
				perms[sibling] = "deny"
			}
		}
	}
	for _, rule := range base {
		for _, key := range slices.Sorted(maps.Keys(allowed)) {
			if !strings.ContainsAny(key, "*?") || allowed[rule.key] != nil || patternContains(rule.key, key) || !mayOverlap(key, rule.key) {
				continue
			}
			if slices.ContainsFunc(denied, func(d string) bool { return patternContains(d, rule.key) }) {
				continue
			}
			if patternContains(key, rule.key) {
				allowed[rule.key] = slices.Clone(allowed[key])
				continue
			}
			perms[rule.key] = "deny"
		}
	}
	for key, rules := range allowed {
		slices.SortFunc(rules, func(a, b patternRule) int { return strings.Compare(a.pattern, b.pattern) })
		perms[key] = agentKeyRules(key, rules, base)
	}
	return perms, unmapped
}

// flatRule is one entry of the single list Kilo evaluates: a permission
// key pattern, a target pattern, and an action.
type flatRule struct{ key, pattern, action string }

// patternRule is one target pattern of an agent key and its action.
type patternRule struct{ pattern, action string }

// kiloDefaults are the rules Kilo puts before every config, in order:
// `baseDefaults` in packages/opencode/src/agent/agent.ts, then
// `KiloAgent.prepare(...).defaultsPatch` in
// packages/opencode/src/kilocode/agent/index.ts. Left out: the
// directories Kilo itself allows outside the project, which depend on
// the machine, so the agent asks there; and the rules only an editor
// client or the experimental board turns on.
func kiloDefaults() []flatRule {
	rules := []flatRule{
		{anyPattern, anyPattern, "allow"},
		{"doom_loop", anyPattern, "ask"},
		{"external_directory", anyPattern, "ask"},
		{"suggest", anyPattern, "deny"},
		{"question", anyPattern, "deny"},
		{"plan_enter", anyPattern, "deny"},
		{"plan_exit", anyPattern, "deny"},
		{"repo_clone", anyPattern, "deny"},
		{"repo_overview", anyPattern, "deny"},
		{"read", anyPattern, "allow"},
		{"read", "*.env", "ask"},
		{"read", "*.env.*", "ask"},
		{"read", "*.env.example", "allow"},
		{"bash", anyPattern, "ask"},
	}
	for _, command := range kiloAllowedCommands {
		rules = append(rules, flatRule{"bash", command + " *", "allow"})
	}
	return append(rules,
		flatRule{"recall", anyPattern, "ask"},
		flatRule{"kilo_memory_recall", anyPattern, "ask"},
		flatRule{"kilo_memory_save", anyPattern, "ask"},
	)
}

// kiloAllowedCommands are the commands Kilo's default `bash` map allows
// (`readable` and `bash` in packages/opencode/src/kilocode/agent/index.ts).
var kiloAllowedCommands = []string{
	"cat", "head", "tail", "less", "ls", "tree", "pwd", "echo", "wc", "which", "type", "file", "diff", "du", "df",
	"date", "uname", "whoami", "printenv", "man", "grep", "rg", "ag", "sort", "uniq", "cut", "tr", "jq",
	"touch", "mkdir", "cp", "mv", "tsc", "tsgo", "tar", "unzip", "gzip", "gunzip",
}

// permissionValue returns the `permission` value of a kilo.jsonc
// document, nil when it has none. It reports false when the document
// does not parse.
func permissionValue(data []byte) (json.RawMessage, bool) {
	stripped, _ := emit.StripJSONC(data)
	doc := emit.NewOrderedJSON()
	if json.Unmarshal(stripped, doc) != nil {
		return nil, false
	}
	raw, _ := doc.Get(permissionKey)
	return raw, true
}

// withKeys lays the keys of a planned permission object over the one on
// disk the way the kilo.jsonc merge does: each replaces its whole value
// in place, and a new key goes last. A planned bare action, or nothing on
// disk to lay it over, takes over.
func withKeys(onDisk, planned json.RawMessage) json.RawMessage {
	base, over := emit.NewOrderedJSON(), emit.NewOrderedJSON()
	if onDisk == nil || json.Unmarshal(onDisk, base) != nil || json.Unmarshal(planned, over) != nil {
		return planned
	}
	for _, key := range over.Keys() {
		value, _ := over.Get(key)
		base.SetRaw(key, value)
	}
	raw, _ := json.Marshal(base)
	return raw
}

// mergeDeep merges over onto base as remeda's mergeDeep does: two
// objects merge key by key, a key keeping its first position, and any
// other pair takes over.
func mergeDeep(base, over json.RawMessage) json.RawMessage {
	if over == nil {
		return base
	}
	left, right := emit.NewOrderedJSON(), emit.NewOrderedJSON()
	if base == nil || json.Unmarshal(base, left) != nil || json.Unmarshal(over, right) != nil {
		return over
	}
	for _, key := range right.Keys() {
		value, _ := right.Get(key)
		if prior, found := left.Get(key); found {
			value = mergeDeep(prior, value)
		}
		left.SetRaw(key, value)
	}
	raw, _ := json.Marshal(left)
	return raw
}

// permissionRules flattens a permission value into Kilo's rule list in
// its order: a bare action covers every key, and each tool key's action
// or patterns follow in turn.
func permissionRules(permission json.RawMessage) []flatRule {
	if permission == nil {
		return nil
	}
	var action string
	if json.Unmarshal(permission, &action) == nil {
		return []flatRule{{anyPattern, anyPattern, action}}
	}
	object := emit.NewOrderedJSON()
	if json.Unmarshal(permission, object) != nil {
		return nil
	}
	var rules []flatRule
	for _, key := range object.Keys() {
		value, _ := object.Get(key)
		if json.Unmarshal(value, &action) == nil {
			rules = append(rules, flatRule{key, anyPattern, action})
			continue
		}
		patterns := emit.NewOrderedJSON()
		if json.Unmarshal(value, patterns) == nil {
			rules = append(rules, orderedRules(key, patterns)...)
		}
	}
	return rules
}

func orderedRules(key string, patterns *emit.OrderedJSON) []flatRule {
	var rules []flatRule
	for _, pattern := range patterns.Keys() {
		raw, _ := patterns.Get(pattern)
		var action string
		if json.Unmarshal(raw, &action) == nil {
			rules = append(rules, flatRule{key, pattern, action})
		}
	}
	return rules
}

// agentKeyRules writes one agent key so that, for every input, Kilo
// decides the stricter of the base list and the agent's own rules.
// Kilo evaluates Kilo's defaults, the project, and then the agent as one
// list, and the last rule whose key and pattern match wins
// (packages/opencode/src/agent/agent.ts merges `defaults, user` and then
// the agent's `value.permission`; permission/index.ts `evaluate` uses
// `findLast` with Wildcard.match on both). Agent rules come last, so
// they must repeat the base decisions inside each pattern they allow:
//
//   - the agent's pattern first, at "ask", Kilo's answer when no rule
//     matches;
//   - then each base rule for this key, in order: one that covers the
//     pattern sets it, one inside it is copied, and a partial overlap is
//     copied as the exact intersection (see intersection);
//   - an overlap with no exact spelling becomes a deny on the base
//     pattern, stricter than needed but never looser.
//
// A scoped key starts with `"*": deny`, so nothing outside its patterns
// opens up. Every action is capped at the agent's own.
func agentKeyRules(key string, agent []patternRule, base []flatRule) any {
	var out []patternRule
	put := func(pattern, action string) {
		out = slices.DeleteFunc(out, func(r patternRule) bool { return r.pattern == pattern })
		out = append(out, patternRule{pattern, action})
	}
	if len(agent) > 1 || agent[0].pattern != anyPattern {
		put(anyPattern, "deny")
	}
	for _, own := range agent {
		put(own.pattern, stricter("ask", own.action))
		for _, rule := range base {
			restricts := rule.action != "allow"
			if !keyCovers(rule.key, key, restricts) {
				continue
			}
			action := stricter(rule.action, own.action)
			relation := patternRelation(own.pattern, rule.pattern)
			if restricts && relation != patternRelation(strings.ToLower(own.pattern), strings.ToLower(rule.pattern)) {
				// Kilo ignores case on Windows only (Wildcard.match adds
				// the "i" flag on win32), and there this rule reaches
				// further, so it holds as a deny on its own pattern.
				put(rule.pattern, "deny")
				continue
			}
			switch relation {
			case covers:
				put(own.pattern, action)
			case inside:
				put(rule.pattern, action)
			case disjoint:
			default:
				parts, exact := intersection(own.pattern, rule.pattern)
				if !exact {
					if action != "allow" {
						put(rule.pattern, "deny")
					}
					continue
				}
				for _, part := range parts {
					put(part, action)
				}
			}
		}
	}
	out = withoutShadowed(out)
	if !slices.ContainsFunc(out, func(r patternRule) bool { return r.action != out[0].action }) {
		return out[0].action
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, rule := range out {
		node.Content = append(node.Content, yamlString(rule.pattern), yamlString(rule.action))
	}
	return node
}

// How a base pattern relates to an agent pattern.
const (
	covers = iota
	inside
	disjoint
	partial
)

// patternRelation reports whether rule covers own, lies inside it, is
// disjoint from it, or overlaps it partly.
func patternRelation(own, rule string) int {
	switch {
	case patternContains(rule, own):
		return covers
	case patternContains(own, rule):
		return inside
	case !mayOverlap(own, rule):
		return disjoint
	}
	return partial
}

// keyCovers reports whether a base rule's key pattern covers an agent
// key. An ask or deny (restricts) also covers it ignoring case, as Kilo
// does on Windows, so it is never dropped there; an allow must match
// exactly.
func keyCovers(ruleKey, key string, restricts bool) bool {
	return patternContains(ruleKey, key) || restricts && patternContains(strings.ToLower(ruleKey), strings.ToLower(key))
}

// withoutShadowed drops each rule a later one covers entirely, since it
// can never be the last match.
func withoutShadowed(rules []patternRule) []patternRule {
	var kept []patternRule
	for i, rule := range rules {
		if !slices.ContainsFunc(rules[i+1:], func(later patternRule) bool { return patternContains(later.pattern, rule.pattern) }) {
			kept = append(kept, rule)
		}
	}
	return kept
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
