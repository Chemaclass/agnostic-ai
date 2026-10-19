package opencode

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
// Native permission maps retain source order for last-match evaluation.
// Names in a `tools` list with no OpenCode key fold into one coverage
// note per sync.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	return emitAgents(sess, agents, dir, nil, dryRun)
}

// permissionRule is one entry of OpenCode's flat rule list.
type permissionRule struct{ key, pattern, action string }

// builtinPermission is OpenCode's own `defaults`, which come before the
// project's rules (packages/opencode/src/agent/agent.ts, v1.19.0). The
// external_directory allows it adds for its temp, skill, and reference
// folders are left out: their paths differ on every machine. Its `read`
// block "mirrors github.com/github/gitignore Node.gitignore pattern for
// .env files".
var builtinPermission = []permissionRule{
	{catchAllPattern, catchAllPattern, "allow"},
	{"doom_loop", catchAllPattern, "ask"},
	{externalDirectoryKey, catchAllPattern, "ask"},
	{"question", catchAllPattern, "deny"},
	{"plan_enter", catchAllPattern, "deny"},
	{"plan_exit", catchAllPattern, "deny"},
	{"read", catchAllPattern, "allow"},
	{"read", "*.env", "ask"},
	{"read", "*.env.*", "ask"},
	{"read", "*.env.example", "allow"},
}

// agentProject keeps defaults, legacy rules, and native V2 rules in evaluation order.
type agentProject struct {
	rules  []permissionRule
	native bool
	path   string
	// servers names every MCP server the project configures.
	servers []string
}

// emitAgents is EmitAgents with the project rules a restricted agent
// repeats; project nil stands for OpenCode's defaults alone.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, project *agentProject, dryRun bool) error {
	if project == nil {
		project = &agentProject{rules: builtinPermission}
	}
	var unmapped []spec.Entry
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, hadUnmapped, err := agentMarkdown(a, project)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
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
func agentMarkdown(e spec.Entry, project *agentProject) (body string, unmapped bool, err error) {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickKeys(meta, agentFrontmatterKeys)
	var translated map[string]any
	translated, unmapped = agentPermission(emit.StringSlice(meta["tools"]), project)
	native := front[permissionKey]
	if translated != nil {
		front[permissionKey] = withNativePermission(translated, native)
		if err := project.validateAgentPermission(front[permissionKey], emit.StringSlice(meta["tools"]), native); err != nil {
			return "", unmapped, err
		}
	}
	if nativeMap, ok := native.(map[string]any); ok {
		path := []string{permissionKey}
		if custom, ok := e.Meta["x-"+target].(map[string]any); ok {
			if _, set := custom[permissionKey]; set {
				path = []string{"x-" + target, permissionKey}
			}
		}
		var generated []string
		for _, key := range slices.Sorted(maps.Keys(translated)) {
			if _, overridden := nativeMap[key]; !overridden {
				generated = append(generated, key)
			}
		}
		front[permissionKey] = agentPermissionYAML{value: front[permissionKey], entry: e, path: path, generated: generated}
	}
	keys := append([]string{}, agentFrontmatterKeys...)
	if steps, ok := (emit.AgentLimit{Portable: emit.AgentMaxTurns, Native: "steps"}).Value(e, target); ok {
		front["steps"] = steps
		keys = append(keys, "steps")
	}
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, append(slices.Clone(agentFrontmatterKeys), "steps", "maxTurns")...)
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String(), unmapped, nil
}

// Native rules follow generated defaults so their recorded order decides the last match.
type agentPermissionYAML struct {
	value     any
	entry     spec.Entry
	path      []string
	generated []string
}

func (p agentPermissionYAML) MarshalYAML() (any, error) {
	node := &yaml.Node{}
	if err := node.Encode(p.value); err != nil {
		return nil, fmt.Errorf("encode agent permission: %w", err)
	}
	orderAgentPermission(node, p.entry, p.path, p.generated)
	return node, nil
}

func orderAgentPermission(node *yaml.Node, entry spec.Entry, path, prefix []string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	order := append(slices.Clone(prefix), entry.KeyOrder(path...)...)
	position := 0
	for _, key := range order {
		for i := position; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value != key {
				continue
			}
			k, value := node.Content[i], node.Content[i+1]
			copy(node.Content[position+2:i+2], node.Content[position:i])
			node.Content[position], node.Content[position+1] = k, value
			position += 2
			break
		}
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		childPath := append(slices.Clone(path), node.Content[i].Value)
		orderAgentPermission(node.Content[i+1], entry, childPath, nil)
	}
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
// agent `permission` map. A bare `mcp__<server>` allows every tool of
// that server as `<server>_*`.
//
// The agent's rules come after the project's, so the map decides every
// input on its own: `*` denies what the list leaves out, and inside what
// it allows, replay repeats the project's decision. doom_loop and
// external_directory are no tools the list restricts, so they replay
// the project's decision whole. The frontmatter encoder sorts keys, so
// the catch-all `*` comes before every tool key.
//
// Returns nil when no name translated, so an agent whose whole list is
// untranslatable keeps its default permissions rather than being locked
// out by a bare `{"*": "deny"}` nobody asked for. unmapped reports
// whether at least one name had no OpenCode key.
func agentPermission(tools []string, project *agentProject) (perms map[string]any, unmapped bool) {
	if project == nil {
		project = &agentProject{rules: builtinPermission}
	}
	grants := map[string][]string{}
	grant := func(key, pattern string) {
		if slices.Contains(grants[key], catchAllPattern) {
			return
		}
		if pattern == catchAllPattern {
			grants[key] = []string{catchAllPattern}
			return
		}
		if !slices.Contains(grants[key], pattern) {
			grants[key] = append(grants[key], pattern)
		}
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
	if len(grants) == 0 {
		return nil, unmapped
	}
	perms = map[string]any{catchAllPattern: "deny"}
	// `<server>_*` also matches the tools of a server whose name starts
	// with `<server>_`, so each one the list leaves out is denied after it.
	for _, other := range project.servers {
		for key := range grants {
			server, whole := strings.CutSuffix(key, "_"+catchAllPattern)
			if !whole || !strings.HasPrefix(other, server+"_") {
				continue
			}
			if _, listed := grants[other+"_"+catchAllPattern]; !listed {
				perms[other+"_"+catchAllPattern] = "deny"
			}
		}
	}
	grants["doom_loop"] = []string{catchAllPattern}
	grants[externalDirectoryKey] = []string{catchAllPattern}

	for _, key := range slices.Sorted(maps.Keys(grants)) {
		patterns := grants[key]
		perms[key] = project.replay(key, patterns)
		// A project rule on a narrower key, such as one MCP tool under a
		// granted server, gets its own entry after this one.
		for _, r := range project.rules {
			fold := r.action != "allow"
			if !wildcardOverlap(r.key, project.actionKey(key), fold) || wildcardContains(r.key, project.actionKey(key), fold) {
				continue
			}
			for _, narrower := range globIntersection(r.key, project.actionKey(key), fold) {
				if _, set := perms[narrower]; set || narrower <= key {
					continue
				}
				perms[narrower] = project.replay(narrower, patterns)
			}
		}
	}
	return perms, unmapped
}

// replay returns key's value that decides each input inside patterns as
// the project does, and denies the rest. For each pattern it starts from
// the last rule covering the whole pattern, then repeats in order every
// later rule that reaches into it, cut down to the part inside it.
//
// OpenCode ignores case on Windows only, and the output must not depend
// on the OS sync runs on. So an ask or deny rule reaches every pattern
// it matches ignoring case, and an allow rule only those it matches
// exactly. Where only case differs, the agent is stricter than the
// project on one OS, never looser on either.
func (p *agentProject) replay(key string, patterns []string) any {
	key = p.actionKey(key)
	var rules []permissionRule
	for _, r := range p.rules {
		if wildcardContains(r.key, key, r.action != "allow") {
			rules = append(rules, r)
		}
	}
	out := emit.NewOrderedJSON()
	if !slices.Contains(patterns, catchAllPattern) {
		_ = out.Set(catchAllPattern, "deny")
	}
	set := func(pattern, action string) {
		out.Delete(pattern)
		_ = out.Set(pattern, action)
	}
	for _, q := range slices.Sorted(slices.Values(patterns)) {
		base, start := "ask", 0
		for i, r := range rules {
			if wildcardContains(r.pattern, q, r.action != "allow") {
				base, start = r.action, i+1
			}
		}
		set(q, base)
		for _, r := range rules[start:] {
			for _, part := range globIntersection(r.pattern, q, r.action != "allow") {
				set(part, r.action)
			}
		}
	}
	value, _ := orderedActions(out)
	return value
}

func ruleAction(rules *emit.OrderedJSON, pattern string) string {
	raw, set := rules.Get(pattern)
	var action string
	if !set || json.Unmarshal(raw, &action) != nil {
		return ""
	}
	return action
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

// wildcardByteEqual compares two literal characters as OpenCode does: a
// backslash is a slash on every platform ("replaceAll("\\", "/")"), and
// with fold set, letters match in either case, as on Windows ("win32" ?
// "si" : "s"; packages/opencode/src/util/wildcard.ts, v1.19.0).
func wildcardByteEqual(a, b byte, fold bool) bool {
	normalize := func(c byte) byte {
		if c == '\\' {
			return '/'
		}
		if fold && 'A' <= c && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}
	return normalize(a) == normalize(b)
}

// wildcardContains reports whether every input b matches also matches
// a, ignoring case when fold is set. It may answer false for a true
// containment, never the reverse.
func wildcardContains(a, b string, fold bool) bool {
	for _, altB := range wildcardAlternatives(b) {
		if !slices.ContainsFunc(wildcardAlternatives(a), func(altA string) bool {
			return globCovers(altA, altB, fold)
		}) {
			return false
		}
	}
	return true
}

// wildcardOverlap reports whether some input matches both patterns.
func wildcardOverlap(a, b string, fold bool) bool {
	return len(globIntersection(a, b, fold)) > 0
}

// globCovers reports whether pattern a covers pattern s: only a `*` in a
// covers a `*` in s, and a `?` in s needs a `?` or `*` in a.
func globCovers(a, s string, fold bool) bool {
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
		case s[j] == '*':
			result = false
		case s[j] == '?':
			result = a[i] == '?' && covers(i+1, j+1)
		default:
			result = (a[i] == '?' || wildcardByteEqual(a[i], s[j], fold)) && covers(i+1, j+1)
		}
		memo[key] = result
		return result
	}
	return covers(0, 0)
}

// globIntersection returns patterns that match the inputs both a and b
// match, written in b's case, or nil when they share none. It walks both
// patterns side by side: a `*` facing a `*` matches any run both allow,
// a `*` facing a character takes that character, and two characters
// must agree. With fold set, characters agree ignoring case, and the
// result can reach inputs of b that a matches only ignoring case.
func globIntersection(a, b string, fold bool) []string {
	// One pattern exactly inside the other is the intersection as written.
	if wildcardContains(b, a, false) {
		return []string{a}
	}
	if wildcardContains(a, b, fold) {
		return []string{b}
	}
	var parts []string
	for _, altA := range wildcardAlternatives(a) {
		for _, altB := range wildcardAlternatives(b) {
			for _, part := range globMeet(altA, altB, fold) {
				// OpenCode would also match the bare command before a
				// trailing " *"; "**" keeps it out when it is not shared.
				if prefix, found := strings.CutSuffix(part, " *"); found && (!wildcardContains(a, prefix, fold) || !wildcardContains(b, prefix, false)) {
					part = prefix + " **"
				}
				if !slices.Contains(parts, part) {
					parts = append(parts, part)
				}
			}
		}
	}
	return slices.DeleteFunc(parts, func(part string) bool {
		return slices.ContainsFunc(parts, func(other string) bool {
			return other != part && wildcardContains(other, part, false) && !wildcardContains(part, other, false)
		})
	})
}

// globMeet intersects two plain globs, with no trailing-space rule.
func globMeet(a, b string, fold bool) []string {
	memo := map[[2]int][]string{}
	var meet func(i, j int) []string
	meet = func(i, j int) []string {
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v
		}
		var out []string
		add := func(prefix string, rest []string) {
			for _, r := range rest {
				s := prefix + r
				for strings.Contains(s, "**") {
					s = strings.ReplaceAll(s, "**", "*")
				}
				if !slices.Contains(out, s) {
					out = append(out, s)
				}
			}
		}
		aStar := i < len(a) && a[i] == '*'
		bStar := j < len(b) && b[j] == '*'
		switch {
		case i == len(a) && j == len(b):
			out = []string{""}
		case aStar && bStar:
			add("*", meet(i+1, j))
			add("*", meet(i, j+1))
		case aStar:
			add("", meet(i+1, j))
			if j < len(b) {
				add(string(b[j]), meet(i, j+1))
			}
		case bStar:
			add("", meet(i, j+1))
			if i < len(a) {
				add(string(a[i]), meet(i+1, j))
			}
		case i == len(a) || j == len(b):
		case a[i] == '?':
			add(string(b[j]), meet(i+1, j+1))
		case b[j] == '?':
			add(string(a[i]), meet(i+1, j+1))
		case wildcardByteEqual(a[i], b[j], fold):
			add(string(b[j]), meet(i+1, j+1))
		}
		memo[key] = out
		return out
	}
	return meet(0, 0)
}
