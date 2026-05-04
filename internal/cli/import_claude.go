package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

const claudeOnlyConfig = `version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks

targets:
  - claude

on-unsupported: warn
`

type importCounts struct{ rules, agents, skills, hooks int }

// importFromClaude scaffolds an agnostic-ai project by reading existing
// Claude Code config (CLAUDE.md and .claude/) under root. Refuses if
// agnostic.config.yaml already exists.
func importFromClaude(root string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	if err := ensureSourceDirs(root); err != nil {
		return err
	}

	c := importCounts{}
	var err error
	if c.rules, err = importClaudeRules(root); err != nil {
		return err
	}
	if c.agents, err = importClaudeAgents(root); err != nil {
		return err
	}
	if c.skills, err = importClaudeSkills(root); err != nil {
		return err
	}
	if c.hooks, err = importClaudeHooks(root); err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, []byte(claudeOnlyConfig), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	fmt.Printf("imported %d rules, %d agents, %d skills, %d hooks\n",
		c.rules, c.agents, c.skills, c.hooks)
	return nil
}

func ensureSourceDirs(root string) error {
	for _, d := range []string{"agents", "skills", "rules", "hooks"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}
