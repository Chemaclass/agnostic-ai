package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	count, err := copyMarkdownDir(src, dstDir)
	if err != nil {
		return count, err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return count, fmt.Errorf("read %s: %w", src, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if err := moveClaudeOnlyKeysInFile(filepath.Join(dstDir, e.Name())); err != nil {
			return count, err
		}
	}
	return count, nil
}
