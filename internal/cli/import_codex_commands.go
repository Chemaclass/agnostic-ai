package cli

import (
	"path/filepath"
)

// codexCommandsDir is the per-project directory Codex reads slash prompts
// from. Each `*.md` file there becomes one command spec.
const codexCommandsDir = ".codex/prompts"

// importCodexCommands copies `.codex/prompts/*.md` byte-for-byte into the
// commands source dir. The agnostic-ai provenance header is stripped, but
// every frontmatter key is preserved verbatim so a round-trip through emit
// produces an identical file.
func importCodexCommands(root, dstDir string) (int, error) {
	src := filepath.Join(root, codexCommandsDir)
	if !dirExists(src) {
		return 0, nil
	}
	return copyMarkdownDir(src, dstDir)
}
