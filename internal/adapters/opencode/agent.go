package opencode

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"gopkg.in/yaml.v3"
)

// agentFrontmatterKeys names the frontmatter keys OpenCode reads from a
// markdown agent definition. Anything else in the spec is dropped so
// internal-only fields (globs, tools, ...) do not leak.
var agentFrontmatterKeys = []string{"description", "mode", "model", "temperature", "permission"}

// agentToolsUntranslatedReason explains, in the flushed coverage note,
// why some names in an agent's `tools` or `can` list did not reach its
// `permission` map.
const agentToolsUntranslatedReason = "tool name(s) outside OpenCode's permission vocabulary have no key there; set x-opencode.permission with OpenCode's own rules for those"

// EmitAgents writes one native OpenCode agent definition per agent spec
// at `<agentsDir>/<name>.md`. OpenCode discovers project subagents from
// `.opencode/agents/` (plural; the singular dir is legacy) with
// frontmatter `description`, `mode` (primary|subagent|all), `model`,
// `temperature`, and `permission`, followed by the system-prompt body.
// Names in a `tools` list with no OpenCode key fold into one coverage
// note per sync.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	return emitAgents(sess, agents, dir, nil, dryRun)
}

// agentProject holds the project permission rules a restricted agent
// repeats. OpenCode puts agent rules after the project ones and the last
// match wins, so an agent that did not repeat them would override every
// project ask and deny for the tools it lists.
type agentProject struct {
	settings   []spec.Entry
	permission map[string]any
	rules      map[string]*emit.OrderedJSON
	outside    any
}

func newAgentProject(settings []spec.Entry, directories []string) (*agentProject, error) {
	permission, _ := buildPermissions(settings)
	project := &agentProject{settings: settings, permission: permission, rules: map[string]*emit.OrderedJSON{}}
	for key, value := range permission {
		rules, err := specRules(value, settings, key)
		if err != nil {
			return nil, err
		}
		project.rules[key] = rules
	}
	outside, err := project.outsideRules(directories)
	if err != nil {
		return nil, err
	}
	project.outside = outside
	return project, nil
}

// emitAgents is EmitAgents with the project rules a restricted agent
// repeats; project may be nil.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, project *agentProject, dryRun bool) error {
	var unmapped []spec.Entry
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, hadUnmapped := agentMarkdown(a, project)
		if hadUnmapped {
			unmapped = append(unmapped, a)
		}
		body := emit.WithHeader(md, emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	emit.NoteAgentToolsNoOp(target, unmapped, agentToolsUntranslatedReason)
	return nil
}

// agentMarkdown renders a single agent definition: allowlisted
// frontmatter plus arbitrary x-opencode passthrough, then the body.
// A `tools` list becomes the `permission` map; an `x-opencode.permission`
// map wins for the keys it names, and a scalar one wins outright.
func agentMarkdown(e spec.Entry, project *agentProject) (body string, unmapped bool) {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickKeys(meta, agentFrontmatterKeys)
	var translated map[string]any
	translated, unmapped = agentPermission(emit.StringSlice(meta["tools"]), project)
	if translated != nil {
		front[permissionKey] = withNativePermission(translated, front[permissionKey])
	}
	keys := append([]string{}, agentFrontmatterKeys...)
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, agentFrontmatterKeys...)
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String(), unmapped
}

// withNativePermission lays the author's own permission over the
// translated one, one tool key at a time.
func withNativePermission(translated map[string]any, native any) any {
	if native == nil {
		return translated
	}
	nativeMap, ok := native.(map[string]any)
	if !ok {
		return native
	}
	for tool, value := range nativeMap {
		translated[tool] = value
	}
	return translated
}

// agentPermission turns an agent's `tools` allowlist into OpenCode's
// agent `permission` map. An allowlist denies everything first and
// re-allows the names that translated; a bare `mcp__<server>` allows
// every tool of that server as `<server>_*`. The frontmatter encoder
// sorts keys, so the catch-all `*` comes before every tool key.
//
// OpenCode reads one flat rule list per agent, its built-in defaults,
// then the project's `permission`, then the agent's, and the last rule
// whose key and pattern both match wins (`Permission.merge` and
// `evaluate` in packages/opencode/src/permission/index.ts, v1.19.0).
// The agent's allows come last, so tighten repeats every default and
// project ask or deny that reaches them. The catch-all would also deny
// `doom_loop` and `external_directory`, which default to "ask", so
// both stay "ask" unless the project is stricter, and
// external_directory also keeps the store grants sync writes.
//
// Returns nil when no name translated, so an agent whose whole list is
// untranslatable keeps its default permissions rather than being locked
// out by a bare `{"*": "deny"}` nobody asked for. unmapped reports
// whether at least one name had no OpenCode key.
func agentPermission(tools []string, project *agentProject) (perms map[string]any, unmapped bool) {
	allowed := map[string]*emit.OrderedJSON{}
	grant := func(key, pattern string) {
		rules, set := allowed[key]
		if pattern == catchAllPattern || (set && ruleAction(rules, catchAllPattern) == "allow") {
			allowed[key] = actionRules(catchAllPattern, "allow")
			return
		}
		if !set {
			rules = actionRules(catchAllPattern, "deny")
			allowed[key] = rules
		}
		_ = rules.Set(pattern, "allow")
	}
	for _, name := range tools {
		if server, whole := wholeMCPServer(name); whole {
			grant(server+"_"+catchAllPattern, catchAllPattern)
			continue
		}
		key, pattern, known := translateRule(name)
		if !known {
			unmapped = true
			continue
		}
		grant(key, pattern)
	}
	if len(allowed) == 0 {
		return nil, unmapped
	}
	allowed["doom_loop"] = actionRules(catchAllPattern, "ask")
	tighten(allowed, project.restrictive())

	perms = map[string]any{catchAllPattern: "deny", externalDirectoryKey: "ask"}
	for key, rules := range allowed {
		perms[key], _ = orderedActions(rules)
	}
	if project != nil && project.outside != nil {
		perms[externalDirectoryKey] = project.outside
	}
	return perms, unmapped
}

func actionRules(pattern, action string) *emit.OrderedJSON {
	rules := emit.NewOrderedJSON()
	_ = rules.Set(pattern, action)
	return rules
}

// permissionRule is one entry of OpenCode's flat rule list.
type permissionRule struct{ key, pattern, action string }

// builtinRestrictive are the ask and deny rules among OpenCode's own
// defaults, which come before the project's: `defaults` in
// packages/opencode/src/agent/agent.ts (v1.19.0). Its `read` block
// "mirrors github.com/github/gitignore Node.gitignore pattern for .env
// files", so an agent that may read still asks before reading `.env`.
var builtinRestrictive = []permissionRule{
	{"doom_loop", catchAllPattern, "ask"},
	{externalDirectoryKey, catchAllPattern, "ask"},
	{"question", catchAllPattern, "deny"},
	{"plan_enter", catchAllPattern, "deny"},
	{"plan_exit", catchAllPattern, "deny"},
	{"read", "*.env", "ask"},
	{"read", "*.env.*", "ask"},
}

// restrictive returns the default and project ask and deny rules. Allow
// rules never reach the agent: after its catch-all deny they would
// widen its list.
func (p *agentProject) restrictive() []permissionRule {
	rules := slices.Clone(builtinRestrictive)
	if p == nil {
		return rules
	}
	for _, key := range slices.Sorted(maps.Keys(p.rules)) {
		for _, pattern := range p.rules[key].Keys() {
			if action := ruleAction(p.rules[key], pattern); action == "ask" || action == "deny" {
				rules = append(rules, permissionRule{key, pattern, action})
			}
		}
	}
	return rules
}

// tighten makes every agent allow no looser than the restrictive rules
// before it. For a rule (K, P, A) and an agent allow (k, q) whose keys
// and patterns overlap:
//
//   - when K and P cover k and q, q takes the stricter action in place;
//   - when k and q cover K and P, the rule is repeated after q, so it
//     wins for its own inputs only; under another key it must sort
//     after k, since the encoder writes keys in order;
//   - otherwise q takes the stricter action in place, stricter than the
//     project for the inputs outside the rule, never looser.
func tighten(allowed map[string]*emit.OrderedJSON, restrictive []permissionRule) {
	type grantRef struct{ key, pattern string }
	var grants, appended []grantRef
	for _, key := range slices.Sorted(maps.Keys(allowed)) {
		for _, pattern := range allowed[key].Keys() {
			if ruleAction(allowed[key], pattern) != "deny" {
				grants = append(grants, grantRef{key, pattern})
			}
		}
	}
	for _, r := range restrictive {
		for _, g := range grants {
			if !wildcardOverlap(r.key, g.key) || !wildcardOverlap(r.pattern, g.pattern) {
				continue
			}
			rules := allowed[g.key]
			covered := wildcardContains(r.key, g.key) && wildcardContains(r.pattern, g.pattern)
			inside := wildcardContains(g.key, r.key) && wildcardContains(g.pattern, r.pattern)
			if inside && !covered && r.key >= g.key {
				target, set := allowed[r.key]
				if !set {
					target = emit.NewOrderedJSON()
					allowed[r.key] = target
				}
				action := stricter(ruleAction(target, r.pattern), r.action)
				target.Delete(r.pattern)
				_ = target.Set(r.pattern, action)
				appended = append(appended, grantRef{r.key, r.pattern})
				continue
			}
			_ = rules.Set(g.pattern, stricter(ruleAction(rules, g.pattern), r.action))
		}
	}
	// A repeated rule sits after the allows, so once a later rule makes
	// an allow it overlaps stricter, the repeated rule must follow suit.
	for changed := true; changed; {
		changed = false
		for _, g := range grants {
			action := ruleAction(allowed[g.key], g.pattern)
			for _, a := range appended {
				rules := allowed[a.key]
				if permissionSeverity[ruleAction(rules, a.pattern)] >= permissionSeverity[action] ||
					!wildcardOverlap(a.key, g.key) || !wildcardOverlap(a.pattern, g.pattern) {
					continue
				}
				_ = rules.Set(a.pattern, action)
				changed = true
			}
		}
	}
}

// catchAll returns the project's action for every input of key: the
// key's own `*` rule, or else the top-level `*` that OpenCode matches
// against every key. It returns "" when the project sets neither.
func (p *agentProject) catchAll(key string) string {
	if own, set := p.rules[key]; set {
		if action := ruleAction(own, catchAllPattern); action != "" {
			return action
		}
	}
	if global, set := p.rules[catchAllPattern]; set {
		return ruleAction(global, catchAllPattern)
	}
	return ""
}

func ruleAction(rules *emit.OrderedJSON, pattern string) string {
	raw, set := rules.Get(pattern)
	var action string
	if !set || json.Unmarshal(raw, &action) != nil {
		return ""
	}
	return action
}

// wildcardAlternatives spells out OpenCode's one special case: "If
// pattern ends with " *" (space + wildcard), make the trailing part
// optional", so `git *` also matches a bare `git`
// (packages/opencode/src/util/wildcard.ts, v1.19.0).
func wildcardAlternatives(pattern string) []string {
	if prefix, found := strings.CutSuffix(pattern, " *"); found {
		return []string{pattern, prefix}
	}
	return []string{pattern}
}

// wildcardMatch reports whether s matches an OpenCode pattern, where `*`
// is zero or more of any character, `?` exactly one, and the rest is
// literal.
func wildcardMatch(pattern, s string) bool {
	return slices.ContainsFunc(wildcardAlternatives(pattern), func(alt string) bool {
		return globCovers(alt, s, false)
	})
}

// wildcardContains reports whether every input b matches also matches
// a. It may answer false for a true containment, never the reverse.
func wildcardContains(a, b string) bool {
	for _, altB := range wildcardAlternatives(b) {
		if !slices.ContainsFunc(wildcardAlternatives(a), func(altA string) bool {
			return globCovers(altA, altB, true)
		}) {
			return false
		}
	}
	return true
}

// wildcardOverlap reports whether some input matches both patterns.
func wildcardOverlap(a, b string) bool {
	for _, altA := range wildcardAlternatives(a) {
		for _, altB := range wildcardAlternatives(b) {
			if globsIntersect(altA, altB) {
				return true
			}
		}
	}
	return false
}

// globCovers reports whether pattern a matches s. With glob set, s is a
// pattern too, and a must cover each of its wildcards: only a `*` in a
// covers a `*` in s, and a `?` in s needs a `?` or `*` in a.
func globCovers(a, s string, glob bool) bool {
	memo := map[[2]int]bool{}
	var covers func(i, j int) bool
	covers = func(i, j int) bool {
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v
		}
		var result bool
		switch {
		case i == len(a):
			result = j == len(s)
		case a[i] == '*':
			result = covers(i+1, j) || (j < len(s) && covers(i, j+1))
		case j == len(s):
			result = false
		case glob && s[j] == '*':
			result = false
		case glob && s[j] == '?':
			result = a[i] == '?' && covers(i+1, j+1)
		default:
			result = (a[i] == '?' || a[i] == s[j]) && covers(i+1, j+1)
		}
		memo[key] = result
		return result
	}
	return covers(0, 0)
}

// globsIntersect reports whether some string matches both globs.
func globsIntersect(a, b string) bool {
	memo := map[[2]int]bool{}
	var meet func(i, j int) bool
	meet = func(i, j int) bool {
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v
		}
		var result bool
		switch {
		case i < len(a) && a[i] == '*':
			result = meet(i+1, j) || (j < len(b) && meet(i, j+1))
		case j < len(b) && b[j] == '*':
			result = meet(i, j+1) || (i < len(a) && meet(i+1, j))
		case i == len(a) || j == len(b):
			result = i == len(a) && j == len(b)
		default:
			result = (a[i] == '?' || b[j] == '?' || a[i] == b[j]) && meet(i+1, j+1)
		}
		memo[key] = result
		return result
	}
	return meet(0, 0)
}

// stricter returns the more restrictive of two actions.
func stricter(a, b string) string {
	if permissionSeverity[b] > permissionSeverity[a] {
		return b
	}
	return a
}

// outsideRules returns the external_directory value for a
// restricted agent. Agent rules come after the project ones, so the
// agent repeats them: "ask", OpenCode's default, or the project's
// catch-all when that is deny, then the settings' own rules in their
// order, then the store directories sync allows in opencode.json,
// unless a settings rule already names one. A settings deny then still
// wins, and a bare action collapses to a scalar.
func (p *agentProject) outsideRules(directories []string) (any, error) {
	rules := emit.NewOrderedJSON()
	if err := rules.Set(catchAllPattern, stricter("ask", p.catchAll(externalDirectoryKey))); err != nil {
		return nil, err
	}
	own, set := p.rules[externalDirectoryKey]
	if !set {
		own = emit.NewOrderedJSON()
	}
	for _, pattern := range own.Keys() {
		raw, _ := own.Get(pattern)
		rules.Delete(pattern)
		rules.SetRaw(pattern, raw)
	}
	for _, dir := range directories {
		if _, set := own.Get(dir); set {
			continue
		}
		rules.Delete(dir)
		if err := rules.Set(dir, "allow"); err != nil {
			return nil, err
		}
	}
	return orderedActions(rules)
}

// orderedActions renders pattern rules as a YAML mapping in their order,
// which the frontmatter encoder would otherwise sort, or as the bare
// action when only the catch-all is left.
func orderedActions(rules *emit.OrderedJSON) (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, pattern := range rules.Keys() {
		raw, _ := rules.Get(pattern)
		var action string
		if err := json.Unmarshal(raw, &action); err != nil {
			return nil, err
		}
		if pattern == catchAllPattern && rules.Len() == 1 {
			return action, nil
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: pattern},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: action})
	}
	return node, nil
}

// wholeMCPServer reports the server a bare `mcp__<server>` names.
func wholeMCPServer(name string) (string, bool) {
	server, found := strings.CutPrefix(name, spec.MCPToolPrefix)
	if !found || server == "" || strings.Contains(server, "__") {
		return "", false
	}
	return server, true
}
