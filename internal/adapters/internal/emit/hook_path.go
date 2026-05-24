// Package emit hook_path normalizes hook command paths so a spec
// authored against one tool's hooks directory still runs when emitted
// to a different target.
//
// Hook scripts conventionally live at `.<tool>/hooks/<basename>` (Claude
// uses `.claude/hooks/`, Codex `.codex/hooks/`, Gemini `.gemini/hooks/`).
// When the source-of-truth spec was imported from one tool it captures
// that tool's prefix verbatim; without rewriting, a sync to a sibling
// target would emit a settings file referencing a path that does not
// exist under that target.
//
// `RewriteHookPath` rewrites the leading `.<other-tool>/hooks/` segment
// to `.<target>/hooks/` whenever it sees a sibling-tool prefix. Paths
// that do not start with one of the recognized prefixes pass through
// unchanged so absolute paths, project-relative scripts (`scripts/x.sh`),
// and arbitrary commands (`gofmt`, `bash -c '…'`) keep their author's
// intent.
package emit

import "strings"

// hookSiblingPrefixes enumerates the per-tool hook directories the
// rewriter recognizes. Anything outside this list is treated as user
// content and left untouched.
var hookSiblingPrefixes = []string{
	".claude/hooks/",
	".codex/hooks/",
	".gemini/hooks/",
}

// RewriteHookPath returns cmd with every recognized sibling-tool hook
// directory substring replaced by `.<target>/hooks/`. Scans the whole
// command so paths wrapped in shell expansions (`"$(git rev-parse
// --show-toplevel)/.codex/hooks/x.sh"`), absolute paths, or quoted
// strings all rewrite. Same-target substrings stay put (no-op).
//
// Substring match has a small false-positive risk if the literal text
// `.codex/hooks/` appears in a hook command for non-path reasons; the
// substring is specific enough that this is acceptable, and the user
// can pin a hook to one tool with the `target:` frontmatter field.
func RewriteHookPath(cmd, target string) string {
	if cmd == "" || target == "" {
		return cmd
	}
	replacement := "." + target + "/hooks/"
	for _, prefix := range hookSiblingPrefixes {
		if prefix == replacement {
			continue
		}
		cmd = strings.ReplaceAll(cmd, prefix, replacement)
	}
	return cmd
}
