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
//
// Returns whether the spec declared a `tools` list, which Cursor has no
// frontmatter field for. The caller folds that into one coverage note per
// sync so the dropped restriction is never silent.
func emitAgent(sess *emit.Session, a spec.Entry, agentsDir string, dryRun bool) (bool, error) {
	path := filepath.Join(agentsDir, a.Name+".md")
	md, hadTools := agentMarkdown(a)
	return hadTools, sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun)
}

// agentMarkdown renders one subagent file and reports whether the spec
// declared `tools`. Cursor's documented subagent frontmatter is `name`,
// `description`, `model`, `readonly`, and `is_background`; there is no
// tools field, so an allowlist cannot restrict a Cursor subagent and is
// deliberately not written. `readonly: true` is the coarse equivalent
// Cursor does document.
func agentMarkdown(a spec.Entry) (string, bool) {
	resolved := emit.ResolveMeta(a.Meta, target)
	hadTools := len(emit.StringSlice(resolved["tools"])) > 0
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
	// `tools` joins the exclude list so an x-cursor escape-hatch attempt
	// cannot reintroduce a key Cursor does not read.
	exclude := append(append([]string(nil), keys...), "tools")
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, exclude...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(a.Body)
	if body == "" {
		return front + "\n", hadTools
	}
	return front + "\n" + body + "\n", hadTools
}
