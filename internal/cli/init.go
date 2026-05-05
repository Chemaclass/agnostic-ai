package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// defaultBaseDir is the default parent directory for scaffolded source
// folders (agents, skills, rules, hooks, mcps).
const defaultBaseDir = ".agnostic-ai"

// rulesDirImporters maps a --from source name to the rules directory the
// importer walks. Claude, Codex, and Cursor have richer importers and
// route directly.
var rulesDirImporters = map[string]string{
	"cline":    ".clinerules",
	"windsurf": filepath.Join(".windsurf", "rules"),
	"continue": filepath.Join(".continue", "rules"),
}

func newInitCmd() *cobra.Command {
	var from string
	var dir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch from {
			case "":
				return scaffold(".", dir)
			case "claude":
				return importFromClaude(".")
			case "codex":
				return importFromCodex(".")
			case "cursor":
				return importFromCursor(".")
			}
			if srcDir, ok := rulesDirImporters[from]; ok {
				return importFromRulesDir(".", from, srcDir)
			}
			return fmt.Errorf("unknown source for --from: %q (supported: claude, codex, cursor, cline, windsurf, continue)", from)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Import existing config from a source (supported: claude, codex, cursor, cline, windsurf, continue)")
	cmd.Flags().StringVar(&dir, "dir", defaultBaseDir, "Base directory for scaffolded source folders. Use \".\" to write at project root.")
	return cmd
}

// renderDefaultConfig builds agnostic.config.yaml with source paths nested
// under base. base="." writes paths at the project root.
func renderDefaultConfig(base string) string {
	prefix := ""
	if base != "" && base != "." {
		prefix = filepath.ToSlash(base) + "/"
	}
	return fmt.Sprintf(`version: 1

sources:
  agents: %sagents
  skills: %sskills
  rules: %srules
  hooks: %shooks
  mcps: %smcps

targets:
  - claude
  - codex
  - gemini
  - cursor
  - copilot
  - aider
  - cline
  - windsurf
  - continue
  - amp
  - zed
  - warp
  - opencode

on-unsupported: warn
`, prefix, prefix, prefix, prefix, prefix)
}

func scaffold(root, base string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	if base == "" {
		base = defaultBaseDir
	}
	kinds := []string{"agents", "skills", "rules", "hooks", "mcps"}
	for _, k := range kinds {
		if err := os.MkdirAll(filepath.Join(root, base, k), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(cfgPath, []byte(renderDefaultConfig(base)), 0o644); err != nil {
		return err
	}
	fmt.Printf("scaffold complete. edit %s then run `agnostic-ai sync`.\n", scaffoldHint(base, kinds))
	return nil
}

func scaffoldHint(base string, kinds []string) string {
	list := strings.Join(kinds, ",")
	if base == "" || base == "." {
		return fmt.Sprintf("{%s}/", list)
	}
	return fmt.Sprintf("%s/{%s}/", filepath.ToSlash(base), list)
}
