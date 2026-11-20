package cli

import (
	"path/filepath"
)

// importClaudeCommands copies `.claude/commands/*.md` into the commands
// source dir. The agnostic-ai provenance header is stripped and every
// frontmatter key (`argument-hint`, `model`, `disable-model-invocation`,
// `description`, etc.) is kept as written, except the Claude-only keys
// moveClaudeOnlyKeys puts under `x-claude:`. Sync flattens those back, so
// a round-trip through emit produces an identical file.
func importClaudeCommands(root, dstDir string, layout claudeLayout) (int, error) {
	src := filepath.Join(root, layout.commands)
	if !dirExists(src) {
		return 0, nil
	}
	written, err := copyMarkdownDirTo(src, dstDir)
	if err != nil {
		return len(written), err
	}
	for _, path := range written {
		if err := moveClaudeOnlyKeysInFile(path); err != nil {
			return len(written), err
		}
	}
	return len(written), nil
}
