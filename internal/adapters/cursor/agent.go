package cursor

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitAgent writes one native Cursor subagent file at
// `.cursor/agents/<name>.md` (Cursor 2.4+). The frontmatter carries the
// documented fields (`name`, `description`, `model`, `readonly`,
// `is_background`) when the spec declares them; arbitrary `x-cursor`
// keys pass through. The body is the agent's system prompt.
func emitAgent(a spec.Entry, agentsDir string, dryRun bool) error {
	path := filepath.Join(agentsDir, a.Name+".md")
	return emit.WriteFile(path, emit.WithHeader(agentMarkdown(a), emit.FormatMarkdown), dryRun)
}

func agentMarkdown(a spec.Entry) string {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{
		"name":        a.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	for _, k := range []string{"model", "readonly", "is_background"} {
		if v, ok := resolved[k]; ok {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	exclude := append([]string(nil), keys...)
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, exclude...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(a.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}
