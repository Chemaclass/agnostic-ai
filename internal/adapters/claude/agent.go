package claude

import (
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// readonlyDisallowedTools is the portable `readonly: true` in Claude Code
// terms: deny the tools that write files. Bash stays allowed, so this is
// a coarse restriction, the same as Cursor's own `readonly`.
var readonlyDisallowedTools = []any{"Write", "Edit", "NotebookEdit"}

// EmitAgents writes native agents for project or user-level sync.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, agent := range agents {
		meta, keys := agentMeta(agent)
		body := emit.WithHeader(emit.DocumentStyled(meta, keys, agent.MetaStyles, agent.Body, target), emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, agent.Name+".md"), body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// agentMeta resolves the agent frontmatter for Claude and replaces
// `readonly`, a key Claude Code does not read, with `disallowedTools` in
// the same position. A `disallowedTools` the spec sets itself wins, and
// an explicit `x-claude.disallowedTools: null` opts out of the mapping.
func agentMeta(a spec.Entry) (map[string]any, []string) {
	meta, keys := emit.ResolveMetaOrdered(a.Meta, a.MetaKeys, target)
	i := slices.Index(keys, "readonly")
	if i < 0 {
		return meta, keys
	}
	custom, _ := a.Meta[emit.XPrefix+target].(map[string]any)
	_, explicit := custom["disallowedTools"]
	_, set := meta["disallowedTools"]
	readonly := meta["readonly"] == true
	delete(meta, "readonly")
	if readonly && !explicit && !set {
		meta["disallowedTools"] = readonlyDisallowedTools
		keys[i] = "disallowedTools"
		return meta, keys
	}
	return meta, slices.Delete(keys, i, i+1)
}
