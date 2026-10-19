package claude

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// readonlyDisallowedTools is the portable `readonly: true` in Claude Code
// terms: deny the tools that write files. Bash stays allowed, so this is
// weaker than Cursor's read-only mode, which also blocks state-changing
// shell commands.
const readonlyDisallowedTools = "Write, Edit, NotebookEdit"

// agentNameRule is what Claude Code's subagent docs say it skips: a name
// that starts with "-" or contains ":", the separator of plugin-scoped
// identifiers. The 256-character cap needs no check here, since a longer
// name already exceeds the filename limit.
var agentNameRule = emit.NameRule{
	Pattern: regexp.MustCompile(`^[^-:][^:]*$`),
	Rule:    `not start with "-" and not contain ":"; Claude Code skips such agents`,
}

// EmitAgents writes native agents for project or user-level sync. Claude
// Code names an agent by its frontmatter `name`, so the rule applies to the
// resolved value, `x-claude.name` overrides included, and every agent is
// checked before the first file is written.
//
// Claude Code skips an agent file without both keys, without reporting
// it: "No name: Claude Code treats the file as documentation" and "A name
// but no description: Claude Code skips the file". So `name` defaults to
// the spec name and `description` to the name, with a note. An agent
// whose tool list resolves to nothing is left out: "When nothing in the
// tools list resolves to a tool ... Claude Code usually refuses to launch
// the subagent", and dropping the list instead would grant every tool.
// Only a list that resolves empty is caught; a misspelled tool name is
// written as given.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	type resolved struct {
		entry spec.Entry
		meta  map[string]any
		keys  []string
	}
	var all []resolved
	var named []spec.Entry
	var undescribed []string
	for _, agent := range agents {
		meta, keys := agentMeta(agent)
		if emptyToolList(meta) {
			emit.NoteCoverageGap(target, spec.KindAgent, 1, fmt.Sprintf("agent %s has an empty tool list, such as `can: []` or `can: [delete]`, and Claude Code refuses to launch it; list a tool it may use, or set `target:` to keep it off Claude Code", agent.Name))
			emit.NoteEntryOmitted(target, spec.KindAgent, agent.Name)
			continue
		}
		meta, keys, fallback := agentIdentity(meta, keys, agent.Name)
		if fallback {
			undescribed = append(undescribed, agent.Name)
		}
		all = append(all, resolved{agent, meta, keys})
		e := agent
		e.Name = meta["name"].(string)
		named = append(named, e)
	}
	if err := emit.ValidateNames(named, target, "agent", agentNameRule); err != nil {
		return err
	}
	if n := len(undescribed); n > 0 {
		subject := fmt.Sprintf("%d agents have", n)
		if n == 1 {
			subject = "1 agent has"
		}
		emit.NoteProject(fmt.Sprintf("%s: %s no description, so sync wrote each name as the description (%s); Claude Code skips an agent without one, so write when to use it",
			target, subject, strings.Join(undescribed, ", ")))
	}
	for _, r := range all {
		body := emit.WithHeader(emit.DocumentStyled(r.meta, r.keys, r.entry.MetaStyles, r.entry.Body, target), emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, r.entry.Name+".md"), body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// agentIdentity returns meta and keys with `name` set to the spec name
// and `description` to the name when either is missing or blank, and
// whether description fell back. It copies both, since an empty meta is
// the spec's own map.
func agentIdentity(meta map[string]any, keys []string, specName string) (map[string]any, []string, bool) {
	meta, keys = maps.Clone(meta), slices.Clone(keys)
	if meta == nil {
		meta = map[string]any{}
	}
	name, _ := meta["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = specName
		meta["name"] = name
	}
	if !slices.Contains(keys, "name") {
		keys = slices.Insert(keys, 0, "name")
	}
	desc, _ := meta["description"].(string)
	fallback := strings.TrimSpace(desc) == ""
	if fallback {
		meta["description"] = name
	}
	if !slices.Contains(keys, "description") {
		keys = slices.Insert(keys, slices.Index(keys, "name")+1, "description")
	}
	return meta, keys, fallback
}

// LeavesOutAgent reports whether sync leaves a, as written in its spec,
// out of Claude Code for an empty tool list. `delete` counts as no tool:
// sync drops it before the agent is written.
func LeavesOutAgent(a spec.Entry) bool {
	a, problem := a.NativeTools()
	if problem != "" {
		return false
	}
	meta, _ := agentMeta(a)
	switch v := meta["tools"].(type) {
	case []any:
		meta = maps.Clone(meta)
		meta["tools"] = slices.DeleteFunc(slices.Clone(v), func(t any) bool { return t == "Delete" })
	case string:
		meta = maps.Clone(meta)
		meta["tools"] = strings.Join(slices.DeleteFunc(spec.SplitToolList(v), func(t string) bool { return t == "Delete" }), ", ")
	}
	return emptyToolList(meta)
}

// emptyToolList reports a `tools` value that is set but lists no tool.
func emptyToolList(meta map[string]any) bool {
	raw, set := meta["tools"]
	if !set {
		return false
	}
	switch v := raw.(type) {
	case []any:
		return len(v) == 0
	case []string:
		return len(v) == 0
	case string:
		return !slices.ContainsFunc(spec.SplitToolList(v), func(t string) bool { return strings.TrimSpace(t) != "" })
	}
	return false
}

func agentMeta(a spec.Entry) (map[string]any, []string) {
	meta, keys := emit.ResolveMetaOrdered(a.Meta, a.MetaKeys, target)
	i := slices.Index(keys, "readonly")
	if i < 0 {
		return meta, keys
	}
	readonly := meta["readonly"] == true
	delete(meta, "readonly")
	keys = slices.Delete(keys, i, i+1)
	if !readonly {
		return meta, keys
	}
	custom, _ := a.Meta[emit.XPrefix+target].(map[string]any)
	_, explicit := custom["disallowedTools"]
	if _, set := meta["disallowedTools"]; set || explicit {
		return meta, keys
	}
	meta["disallowedTools"] = readonlyDisallowedTools
	return meta, slices.Insert(keys, i, "disallowedTools")
}
