package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// AgnosticMainFile is the project-root "main instructions" file
// agnostic-ai mirrors during import. Sits alongside CLAUDE.md /
// AGENTS.md / GEMINI.md and holds a verbatim copy of the source CLI's
// top-level instructions.
const AgnosticMainFile = "AGNOSTIC_AI.md"

type importCounts struct{ rules, agents, skills, hooks int }

// importFromClaude reads existing Claude Code config (CLAUDE.md and
// .claude/) under root and writes specs into the configured source
// directories.
func importFromClaude(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks); err != nil {
		return err
	}

	c := importCounts{}
	var err error
	if c.rules, err = importClaudeRules(root, filepath.Join(root, src.Rules)); err != nil {
		return err
	}
	if c.agents, err = importClaudeAgents(root, filepath.Join(root, src.Agents)); err != nil {
		return err
	}
	if c.skills, err = importClaudeSkills(root, filepath.Join(root, src.Skills)); err != nil {
		return err
	}
	if c.hooks, err = importClaudeHooks(root, filepath.Join(root, src.Hooks)); err != nil {
		return err
	}
	if err := copyClaudeMainFile(root); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks\n",
		c.rules, c.agents, c.skills, c.hooks)
	return nil
}

// copyClaudeMainFile mirrors CLAUDE.md to <root>/AGNOSTIC_AI.md
// byte-for-byte so projects keep a CLI-agnostic top-level
// instructions file alongside CLAUDE.md / AGENTS.md / GEMINI.md.
// No-op when CLAUDE.md is absent.
func copyClaudeMainFile(root string) error {
	src := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	dst := filepath.Join(root, AgnosticMainFile)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// mkdirAllSources creates each non-empty source directory under root.
func mkdirAllSources(root string, dirs ...string) error {
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}
