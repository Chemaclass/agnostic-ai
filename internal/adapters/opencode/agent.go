package opencode

import (
	"encoding/json"
	"path/filepath"
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

// emitAgents is EmitAgents with the external_directory value a
// restricted agent writes (see agentOutsideRules).
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, outside any, dryRun bool) error {
	var unmapped []spec.Entry
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, hadUnmapped := agentMarkdown(a, outside)
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
func agentMarkdown(e spec.Entry, outside any) (body string, unmapped bool) {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickKeys(meta, agentFrontmatterKeys)
	var translated map[string]any
	translated, unmapped = agentPermission(emit.StringSlice(meta["tools"]), outside)
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
// project ones. So doom_loop goes back to "ask", and external_directory
// takes outside, or "ask" when it is nil. A bare
// `mcp__<server>` allows every tool of that server as `<server>_*`.
//
// Returns nil when no name translated, so an agent whose whole list is
// untranslatable keeps its default permissions rather than being locked
// out by a bare `{"*": "deny"}` nobody asked for. unmapped reports
// whether at least one name had no OpenCode key.
func agentPermission(tools []string, outside any) (perms map[string]any, unmapped bool) {
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
	allowed[catchAllPattern] = "deny"
	allowed["doom_loop"] = "ask"
	if outside == nil {
		outside = "ask"
	}
	allowed[externalDirectoryKey] = outside
	return allowed, unmapped
}

// agentOutsideRules returns the external_directory value for a
// restricted agent. Agent rules come after the project ones, so the
// agent repeats them: "ask", OpenCode's default, then the settings'
// own rules in their order, then the store directories sync allows in
// opencode.json, unless a settings rule already names one. A settings
// deny then still wins, and a bare action collapses to a scalar.
func agentOutsideRules(settings []spec.Entry, directories []string) (any, error) {
	rules := emit.NewOrderedJSON()
	if err := rules.Set(catchAllPattern, "ask"); err != nil {
		return nil, err
	}
	project, _ := buildPermissions(settings)
	own := emit.NewOrderedJSON()
	if value, set := project[externalDirectoryKey]; set {
		var err error
		if own, err = specRules(value, settings); err != nil {
			return nil, err
		}
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
			&yaml.Node{Kind: yaml.ScalarNode, Value: pattern},
			&yaml.Node{Kind: yaml.ScalarNode, Value: action})
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
