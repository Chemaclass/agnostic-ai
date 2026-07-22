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

// emitAgents writes one native OpenCode agent definition per agent spec
// at `<agentsDir>/<name>.md`. OpenCode discovers project subagents from
// `.opencode/agents/` (plural; the singular dir is legacy) with
// frontmatter `description`, `mode` (primary|subagent|all), `model`,
// `temperature`, and `permission`, followed by the system-prompt body.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		body := emit.WithHeader(agentMarkdown(a), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// agentMarkdown renders a single agent definition: allowlisted
// frontmatter plus arbitrary x-opencode passthrough, then the body.
func agentMarkdown(e spec.Entry) string {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickKeys(meta, agentFrontmatterKeys)
	keys := append([]string{}, agentFrontmatterKeys...)
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, agentFrontmatterKeys...)
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String()
}
