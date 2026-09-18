package cli

import (
	"path/filepath"
)

// importClaudeCommands copies `.claude/commands/*.md` byte-for-byte into the
// commands source dir. The agnostic-ai provenance header is stripped, but
// every frontmatter key (`argument-hint`, `model`, `disable-model-invocation`,
// `description`, `allowed-tools`, etc.) is preserved verbatim so a round-trip
// through emit produces an identical file.
func importClaudeCommands(root, dstDir string, layout claudeLayout) (int, error) {
	src := filepath.Join(root, layout.commands)
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
}
