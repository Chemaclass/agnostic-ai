package opencode

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
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

// agentProject is the flat rule list a restricted agent's own rules
// follow: OpenCode's defaults, then the project's `permission` in file
// order. OpenCode builds each agent's rules as Permission.merge(defaults,
// user, agent), merge is a plain flatten, and evaluate takes the last
// rule whose key and pattern both match (packages/opencode/src/agent/
// agent.ts and permission/index.ts, v1.19.0).
type agentProject struct {
	rules []permissionRule
}

// readProjectPermission reads the `permission` opencode.json holds after
// this sync: the content the session just planned for it, or the file
// on disk when the session left it alone.
func readProjectPermission(sess *emit.Session, path string) (*agentProject, error) {
	content, planned := sess.PlannedFile(path)
	if !planned {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		content = string(data)
	}
	project := &agentProject{rules: slices.Clone(builtinPermission)}
	if strings.TrimSpace(content) == "" {
		return project, nil
	}
	stripped, _ := emit.StripJSONC([]byte(content))
	doc := emit.NewOrderedJSON()
	if err := json.Unmarshal(stripped, doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	raw, found := doc.Get(permissionKey)
	if !found {
		return project, nil
	}
	var action string
	if json.Unmarshal(raw, &action) == nil {
		project.rules = append(project.rules, permissionRule{catchAllPattern, catchAllPattern, action})
		return project, nil
	}
	keys := emit.NewOrderedJSON()
	if err := json.Unmarshal(raw, keys); err != nil {
		return nil, fmt.Errorf("parse %s permission: %w", path, err)
	}
	for _, key := range keys.Keys() {
		value, _ := keys.Get(key)
		if json.Unmarshal(value, &action) == nil {
			project.rules = append(project.rules, permissionRule{key, catchAllPattern, action})
			continue
		}
		patterns := emit.NewOrderedJSON()
		if json.Unmarshal(value, patterns) != nil {
			continue
		}
		for _, pattern := range patterns.Keys() {
			if a := ruleAction(patterns, pattern); a != "" {
				project.rules = append(project.rules, permissionRule{key, pattern, a})
			}
		}
	}
	return project, nil
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
	grants["doom_loop"] = []string{catchAllPattern}
	grants[externalDirectoryKey] = []string{catchAllPattern}

	perms = map[string]any{catchAllPattern: "deny"}
	for _, key := range slices.Sorted(maps.Keys(grants)) {
		patterns := grants[key]
		perms[key] = project.replay(key, patterns)
		// A project rule on a narrower key, such as one MCP tool under a
		// granted server, gets its own entry after this one.
		for _, r := range project.rules {
			if !wildcardOverlap(r.key, key) || wildcardContains(r.key, key) {
				continue
			}
			for _, narrower := range globIntersection(r.key, key) {
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
func (p *agentProject) replay(key string, patterns []string) any {
	var rules []permissionRule
	for _, r := range p.rules {
		if wildcardContains(r.key, key) {
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
			if wildcardContains(r.pattern, q) {
				base, start = r.action, i+1
			}
		}
		set(q, base)
		for _, r := range rules[start:] {
			for _, part := range globIntersection(r.pattern, q) {
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
	return len(globIntersection(a, b)) > 0
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

// globIntersection returns patterns whose union matches exactly the
// inputs both a and b match, or nil when they share none. It walks both
// patterns side by side: a `*` facing a `*` matches any run both allow,
// a `*` facing a character takes that character, and two characters
// must agree.
func globIntersection(a, b string) []string {
	// One pattern inside the other is the intersection as written.
	if wildcardContains(b, a) {
		return []string{a}
	}
	if wildcardContains(a, b) {
		return []string{b}
	}
	var parts []string
	for _, altA := range wildcardAlternatives(a) {
		for _, altB := range wildcardAlternatives(b) {
			for _, part := range globMeet(altA, altB) {
				// OpenCode would also match the bare command before a
				// trailing " *"; "**" keeps it out when it is not shared.
				if prefix, found := strings.CutSuffix(part, " *"); found && !(wildcardContains(a, prefix) && wildcardContains(b, prefix)) {
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
			return other != part && wildcardContains(other, part) && !wildcardContains(part, other)
		})
	})
}

// globMeet intersects two plain globs, with no trailing-space rule.
func globMeet(a, b string) []string {
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
		case b[j] == '?' || a[i] == b[j]:
			add(string(a[i]), meet(i+1, j+1))
		}
		memo[key] = out
		return out
	}
	return meet(0, 0)
}
