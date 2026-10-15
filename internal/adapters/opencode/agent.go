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
// re-allows the names that translated. OpenCode lets "the last matching
// rule" win and the frontmatter encoder sorts keys, so the catch-all
// `*` comes before every tool key and every pattern it overrides.
//
// The catch-all would also deny `external_directory` and `doom_loop`,
// which OpenCode asks about by default, and agent rules override the
// project ones. So doom_loop goes back to "ask", external_directory
// takes the project's outside rules, or "ask", and each listed tool
// keeps the project's ask and deny rules after its allow. A bare
// `mcp__<server>` allows every tool of that server as `<server>_*`.
//
// Returns nil when no name translated, so an agent whose whole list is
// untranslatable keeps its default permissions rather than being locked
// out by a bare `{"*": "deny"}` nobody asked for. unmapped reports
// whether at least one name had no OpenCode key.
func agentPermission(tools []string, project *agentProject) (perms map[string]any, unmapped bool) {
	allowed := map[string]any{}
	for _, name := range tools {
		if server, whole := wholeMCPServer(name); whole {
			allowed[server+"_"+catchAllPattern] = "allow"
			continue
		}
		key, pattern, known := translateRule(name)
		if !known {
			unmapped = true
			continue
		}
		if pattern == catchAllPattern {
			allowed[key] = "allow"
			continue
		}
		if allowed[key] == "allow" {
			continue
		}
		patterns, _ := allowed[key].(map[string]any)
		if patterns == nil {
			patterns = map[string]any{catchAllPattern: "deny"}
			allowed[key] = patterns
		}
		patterns[pattern] = "allow"
	}
	if len(allowed) == 0 {
		return nil, unmapped
	}
	for key, value := range allowed {
		allowed[key] = project.restrict(key, value)
	}
	allowed[catchAllPattern] = "deny"
	allowed["doom_loop"] = project.restrict("doom_loop", "ask")
	allowed[externalDirectoryKey] = "ask"
	if project != nil && project.outside != nil {
		allowed[externalDirectoryKey] = project.outside
	}
	return allowed, unmapped
}

// restrict makes the agent's value for key no looser than the project's.
// The project's catch-all action for key applies in place: to the
// agent's catch-all, or to each pattern a scoped value allows, keeping
// the stricter action, so the agent's own `"*": deny` stays first. The
// project's scoped deny rules follow, so they still win, and so do its
// ask rules on a pattern inside one the agent allows; an ask elsewhere
// would loosen the agent's deny. Its allow rules stay out: they would
// widen the agent's list.
func (p *agentProject) restrict(key string, value any) any {
	if p == nil {
		return value
	}
	own, set := p.rules[key]
	if !set {
		own = emit.NewOrderedJSON()
	}
	catchAll := p.catchAll(key)
	rules := emit.NewOrderedJSON()
	var agentAllows []string
	patterns, scoped := value.(map[string]any)
	if scoped {
		_ = rules.Set(catchAllPattern, "deny")
		for _, pattern := range slices.Sorted(maps.Keys(patterns)) {
			if pattern != catchAllPattern {
				_ = rules.Set(pattern, stricter("allow", catchAll))
				agentAllows = append(agentAllows, pattern)
			}
		}
	} else {
		action, _ := value.(string)
		_ = rules.Set(catchAllPattern, stricter(action, catchAll))
	}
	for _, pattern := range own.Keys() {
		action := ruleAction(own, pattern)
		if pattern == catchAllPattern || action == "allow" || action == "" {
			continue
		}
		if action == "ask" && scoped && !slices.ContainsFunc(agentAllows, func(allowed string) bool {
			return wildcardMatch(allowed, pattern)
		}) {
			continue
		}
		rules.Delete(pattern)
		_ = rules.Set(pattern, action)
	}
	ordered, _ := orderedActions(rules)
	return ordered
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

// wildcardMatch reports whether s matches an OpenCode pattern, where `*`
// is zero or more of any character, `?` exactly one, and the rest is
// literal.
func wildcardMatch(pattern, s string) bool {
	p, i, star, mark := 0, 0, -1, 0
	for i < len(s) {
		switch {
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == s[i]):
			p++
			i++
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, i
			p++
		case star >= 0:
			p = star + 1
			mark++
			i = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
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
