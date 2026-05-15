package cli

import (
	"path/filepath"
)

// claudeCommandsDir is the per-project directory Claude Code reads slash
// commands from. Each `*.md` file there becomes one command spec.
const claudeCommandsDir = ".claude/commands"

// importClaudeCommands copies `.claude/commands/*.md` byte-for-byte into the
// commands source dir. The agnostic-ai provenance header is stripped, but
// every frontmatter key (`argument-hint`, `model`, `disable-model-invocation`,
// `description`, `allowed-tools`, etc.) is preserved verbatim so a round-trip
// through emit produces an identical file.
func importClaudeCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, claudeCommandsDir)
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
}
