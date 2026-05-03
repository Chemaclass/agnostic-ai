package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultConfig = `version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks

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

on-unsupported: warn
`

func scaffold(root string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	dirs := []string{"agents", "skills", "rules", "hooks"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(cfgPath, []byte(defaultConfig), 0o644); err != nil {
		return err
	}
	fmt.Println("scaffold complete. edit agents/, skills/, rules/, hooks/ then run `agnostic-ai sync`.")
	return nil
}
