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

// RewriteHookPath returns cmd with a recognized sibling-tool hook
// directory prefix replaced by `.<target>/hooks/`. When cmd does not
// start with any recognized prefix it is returned unchanged so non-hook
// commands stay verbatim.
func RewriteHookPath(cmd, target string) string {
	if cmd == "" || target == "" {
		return cmd
	}
	for _, prefix := range hookSiblingPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return "." + target + "/hooks/" + cmd[len(prefix):]
		}
		quoted := `"` + prefix
		if strings.HasPrefix(cmd, quoted) {
			return `"` + "." + target + "/hooks/" + cmd[len(quoted):]
		}
	}
	return cmd
}
