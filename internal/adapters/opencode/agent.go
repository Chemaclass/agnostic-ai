package opencode

import (
	"path/filepath"
	"strings"

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
// Names in a `tools` list with no OpenCode key fold into one coverage
// note per sync.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	var unmapped []spec.Entry
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, hadUnmapped := agentMarkdown(a)
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
func agentMarkdown(e spec.Entry) (body string, unmapped bool) {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickKeys(meta, agentFrontmatterKeys)
	var translated map[string]any
	translated, unmapped = agentPermission(emit.StringSlice(meta["tools"]))
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
// Returns nil when no name translated, so an agent whose whole list is
// untranslatable keeps its default permissions rather than being locked
// out by a bare `{"*": "deny"}` nobody asked for. unmapped reports
// whether at least one name had no OpenCode key.
func agentPermission(tools []string) (perms map[string]any, unmapped bool) {
	allowed := map[string]any{}
	for _, name := range tools {
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
	return allowed, unmapped
}
