package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// claudeDir is the per-project Claude Code config directory.
	claudeDir = ".claude"
	// claudeMainFile is the project-root Claude Code instructions file.
	claudeMainFile = "CLAUDE.md"
	// agnosticMainFile is the CLI-agnostic instructions file. Lives
	// under the managed .agnostic-ai/ directory and holds a verbatim
	// copy of the source CLI's top-level instructions (CLAUDE.md,
	// AGENTS.md, GEMINI.md, etc.). Keeping it out of the project root
	// avoids cluttering hand-authored files.
	agnosticMainFile = ".agnostic-ai/AGNOSTIC_AI.md"
)

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
	if err := importClaudeSettingsOverlay(root); err != nil {
		return err
	}
	if err := mirrorClaudeMainFile(root); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks\n",
		c.rules, c.agents, c.skills, c.hooks)
	return nil
}

// mirrorClaudeMainFile mirrors <root>/CLAUDE.md to
// <root>/.agnostic-ai/AGNOSTIC_AI.md.
func mirrorClaudeMainFile(root string) error {
	return mirrorMainFile(root, claudeMainFile)
}

// mirrorMainFile copies <root>/<srcName> byte-for-byte to
// <root>/.agnostic-ai/AGNOSTIC_AI.md. No-op when the source is absent.
// Each importer calls this with the target's own top-level
// instructions filename so the project keeps a CLI-agnostic copy
// under the managed directory. Later imports overwrite earlier
// mirrors (last-import wins).
func mirrorMainFile(root, srcName string) error {
	src := filepath.Join(root, srcName)
	dst := filepath.Join(root, agnosticMainFile)
	if err := copyFileIfExists(src, dst); err != nil {
		return fmt.Errorf("mirror %s: %w", srcName, err)
	}
	return nil
}

// copyFileIfExists copies src to dst byte-for-byte. Returns nil when
// src is absent; surfaces every other read/write error.
func copyFileIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// copyMarkdownDir copies every top-level *.md file from srcDir into
// dstDir byte-for-byte. Subdirectories and non-.md entries are
// skipped. Caller must ensure srcDir exists.
func copyMarkdownDir(srcDir, dstDir string) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", srcDir, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())
		if err := copyFileIfExists(src, dst); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
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
