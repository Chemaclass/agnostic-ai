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

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		Long: "Creates agnostic.config.yaml plus source folders. " +
			"Default base dir is .agnostic-ai/. Pass a positional argument " +
			"to override (use \".\" for the legacy root-level layout).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := defaultBaseDir
			if len(args) == 1 {
				base = args[0]
			}
			return scaffold(".", base)
		},
	}
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
	fmt.Println("import existing AI CLI config with `agnostic-ai import <source>`.")
	return nil
}

func scaffoldHint(base string, kinds []string) string {
	list := strings.Join(kinds, ",")
	if base == "" || base == "." {
		return fmt.Sprintf("{%s}/", list)
	}
	return fmt.Sprintf("%s/{%s}/", filepath.ToSlash(base), list)
}
