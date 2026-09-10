package kilo

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// commandFrontmatterKeys names the only frontmatter keys the new Kilo
// Code extension's slash-command loader reads from a workflow file,
// per the vendor's own field table (packages/kilo-docs/pages/
// customize/workflows.md, mirrored on GitHub since kilo.ai's rendered
// docs defeat fetching): `description`, `agent`, `model`, `variant`,
// `subtask`. That table is near-identical to OpenCode's own command
// frontmatter (internal/adapters/opencode), which this adapter already
// filters and emits the same way; `variant` (a model's reasoning-effort
// override, e.g. `low` or `high`) is the one extra key Kilo Code
// documents that OpenCode does not. Anything else in the spec is
// dropped so internal-only fields (globs, tools, ...) do not leak.
var commandFrontmatterKeys = []string{"description", "agent", "model", "variant", "subtask"}

// emitCommands writes one `<dir>/<name>.md` command file per spec.
// Kilo Code takes the workflow name from the filename ("just the
// filename without `.md` extension"), the same convention this
// adapter's agents already follow, so `name` is never written.
func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, c := range commands {
		path := filepath.Join(dir, c.Name+".md")
		body := emit.WithHeader(commandFile(c), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// commandFile renders a single command markdown file: filtered
// frontmatter (description plus any of agent/model/variant/subtask the
// spec sets) followed by the spec body as the workflow's step-by-step
// instructions.
func commandFile(e spec.Entry) string {
	meta := emit.ResolveMeta(e.Meta, target)
	front := pickCommandKeys(meta)
	keys := append([]string{}, commandFrontmatterKeys...)
	// Pass through arbitrary x-kilo keys beyond the documented set so an
	// author can declare command metadata Kilo Code adds later without
	// waiting on the allowlist above.
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, commandFrontmatterKeys...)
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String()
}

// pickCommandKeys returns the subset of meta whose keys are in
// commandFrontmatterKeys. Empty string values are dropped so an unset
// field stays absent rather than emitting a blank key.
func pickCommandKeys(meta map[string]any) map[string]any {
	out := make(map[string]any, len(commandFrontmatterKeys))
	for _, k := range commandFrontmatterKeys {
		v, ok := meta[k]
		if !ok {
			continue
		}
		if s, isStr := v.(string); isStr && s == "" {
			continue
		}
		out[k] = v
	}
	return out
}
