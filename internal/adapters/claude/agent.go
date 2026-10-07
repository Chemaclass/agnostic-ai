package claude

import (
	"path/filepath"
	"regexp"
	"slices"

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
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	type resolved struct {
		meta map[string]any
		keys []string
	}
	all := make([]resolved, len(agents))
	named := make([]spec.Entry, len(agents))
	for i, agent := range agents {
		meta, keys := agentMeta(agent)
		all[i] = resolved{meta, keys}
		named[i] = agent
		if name, ok := meta["name"].(string); ok && name != "" {
			named[i].Name = name
		}
	}
	if err := emit.ValidateNames(named, target, "agent", agentNameRule); err != nil {
		return err
	}
	for i, agent := range agents {
		body := emit.WithHeader(emit.DocumentStyled(all[i].meta, all[i].keys, agent.MetaStyles, agent.Body, target), emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, agent.Name+".md"), body, dryRun); err != nil {
			return err
		}
	}
	return nil
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
