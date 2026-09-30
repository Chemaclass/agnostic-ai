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

// RewriteHookPath translates the project root and sibling hook directories.
func RewriteHookPath(cmd, target string, metadata ...map[string]any) string {
	if cmd == "" || target == "" {
		return cmd
	}
	cmd = RewriteHookRoot(cmd, target, metadata...)
	replacement := "." + target + "/hooks/"
	for _, prefix := range hookSiblingPrefixes {
		if prefix == replacement {
			continue
		}
		cmd = strings.ReplaceAll(cmd, prefix, replacement)
	}
	return cmd
}
