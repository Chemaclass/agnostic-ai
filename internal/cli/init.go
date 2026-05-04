package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch from {
			case "":
				return scaffold(".")
			case "claude":
				return importFromClaude(".")
			case "codex":
				return importFromCodex(".")
			case "cursor":
				return importFromCursor(".")
			case "cline":
				return importFromCline(".")
			case "windsurf":
				return importFromWindsurf(".")
			case "continue":
				return importFromContinue(".")
			default:
				return fmt.Errorf("unknown source for --from: %q (supported: claude, codex, cursor, cline, windsurf, continue)", from)
			}
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Import existing config from a source (supported: claude, codex, cursor, cline, windsurf, continue)")
	return cmd
}

const defaultConfig = `version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks
  mcps: mcps

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
`

func scaffold(root string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	dirs := []string{"agents", "skills", "rules", "hooks", "mcps"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(cfgPath, []byte(defaultConfig), 0o644); err != nil {
		return err
	}
	fmt.Println("scaffold complete. edit agents/, skills/, rules/, hooks/, mcps/ then run `agnostic-ai sync`.")
	return nil
}
