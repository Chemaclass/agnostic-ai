package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// rulesDirImporters maps a source name to the rules directory the
// importer walks. Claude, Codex, and Cursor have richer importers and
// route directly.
var rulesDirImporters = map[string]string{
	"cline":    ".clinerules",
	"windsurf": filepath.Join(".windsurf", "rules"),
	"continue": filepath.Join(".continue", "rules"),
}

// importSources lists every source the import command accepts, used in
// help text and error messages.
func importSources() string {
	names := []string{
		"aider", "amp", "claude", "codex", "copilot",
		"cursor", "gemini", "opencode", "warp", "zed",
	}
	for k := range rulesDirImporters {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <source>",
		Short: "Import existing config from another AI CLI into this project's source directories.",
		Long: "Reads agnostic-ai.yaml to resolve source paths, then translates an " +
			"existing AI CLI configuration into agnostic specs. Sources: " + importSources() + ".",
		Example: `  # Migrate an existing Claude Code project
  agnostic-ai init
  agnostic-ai import claude

  # Migrate from Cursor (.cursor/rules/*.mdc -> rules/*.md)
  agnostic-ai import cursor`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			cfg, err := config.Load(".")
			if err != nil {
				return fmt.Errorf("load config: %w (run `agnostic-ai init` first)", err)
			}
			return runImport(".", source, cfg.Sources)
		},
	}
	return cmd
}

// runImport dispatches to the per-source importer.
func runImport(root, source string, src config.Sources) error {
	switch source {
	case "claude":
		return importFromClaude(root, src)
	case "codex":
		return importFromCodex(root, src)
	case "cursor":
		return importFromCursor(root, src)
	case "aider":
		return importFromAider(root, src)
	case "amp":
		return importFromAmp(root, src)
	case "warp":
		return importFromWarp(root, src)
	case "gemini":
		return importFromGemini(root, src)
	case "copilot":
		return importFromCopilot(root, src)
	case "opencode":
		return importFromOpencode(root, src)
	case "zed":
		return importFromZed(root, src)
	}
	if srcDir, ok := rulesDirImporters[source]; ok {
		return importFromRulesDir(root, source, srcDir, src)
	}
	return fmt.Errorf("unknown source: %q (supported: %s)", source, importSources())
}
