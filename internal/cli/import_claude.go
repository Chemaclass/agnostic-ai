package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
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

type importCounts struct{ rules, agents, skills, hooks, mcps, commands int }

// importFromClaude reads existing Claude Code config (CLAUDE.md and
// .claude/) under root and writes specs into the configured source
// directories.
func importFromClaude(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks, src.MCPs, src.Commands); err != nil {
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
	if err := captureHookScripts(root, "claude"); err != nil {
		return err
	}
	mainSeeded, mainSrc, promotedNested, err := mirrorClaudeMainFile(root)
	if err != nil {
		return err
	}
	// When the nested .claude/CLAUDE.md is promoted to the shared body,
	// do not also capture it as a claude-private helper overlay: that
	// would restore a duplicate copy under .claude/ on the next sync.
	var helperExclude []string
	if promotedNested {
		helperExclude = append(helperExclude, claudeMainFile)
	}
	helpers, err := captureHelperFiles(root, "claude", helperExclude...)
	if err != nil {
		return err
	}
	if c.mcps, err = importClaudeMCP(root, filepath.Join(root, src.MCPs)); err != nil {
		return err
	}
	if c.commands, err = importClaudeCommands(root, filepath.Join(root, src.Commands)); err != nil {
		return err
	}
	overlaySeeded, err := importClaudeSettingsOverlay(root)
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks, %d mcps, %d commands\n",
		c.rules, c.agents, c.skills, c.hooks, c.mcps, c.commands)
	if mainSeeded {
		summaryf("  → %s seeded from %s (commit this file — sync distributes it to all targets)\n",
			agnosticMainFile, mainSrc)
	}
	if overlaySeeded {
		summaryf("  → %s seeded from %s/settings.json (carries non-hook settings across re-syncs)\n",
			claudeOverlayRelPath(), claudeDir)
	}
	for _, h := range helpers {
		summaryf("  → %s seeded from %s/%s\n",
			filepath.Join(agnosticOverlayDir, "claude", h), claudeDir, h)
	}
	printImportNextSteps(root, "claude")
	return nil
}

// nestedClaudeMainFile is the project-nested Claude Code instructions
// file. Per the Claude Code docs a project CLAUDE.md may live at either
// `./CLAUDE.md` or `./.claude/CLAUDE.md`; both are auto-loaded project
// memory. agnostic-ai treats the nested form as the project's main
// instructions when no root CLAUDE.md exists.
var nestedClaudeMainFile = filepath.Join(claudeDir, claudeMainFile)

// mirrorClaudeMainFile mirrors the project's Claude main instructions to
// <root>/.agnostic-ai/AGNOSTIC_AI.md so `sync` distributes the body to
// every target's native entry-point (AGENTS.md, GEMINI.md, ...).
//
// Source precedence: the project-root CLAUDE.md wins; when it is absent
// the nested .claude/CLAUDE.md is promoted to the shared body. Promoting
// the nested file is what lets a project that keeps its instructions
// under .claude/ (a documented Claude Code location) still feed codex,
// gemini, and the rest — without it, those targets receive only the
// generic pointer template.
//
// Returns (wrote, srcName, promotedNested, err): wrote is false on a
// project with no Claude instructions at all (callers suppress the
// summary line); srcName names the file the body came from; promotedNested
// is true when the nested file was used, signaling the caller to skip
// capturing it as a claude-private helper overlay.
func mirrorClaudeMainFile(root string) (wrote bool, srcName string, promotedNested bool, err error) {
	if _, statErr := os.Stat(filepath.Join(root, claudeMainFile)); statErr == nil {
		wrote, err = mirrorMainFile(root, claudeMainFile)
		return wrote, claudeMainFile, false, err
	}
	wrote, err = mirrorMainFile(root, nestedClaudeMainFile)
	return wrote, nestedClaudeMainFile, wrote, err
}

// mirrorMainFile copies <root>/<srcName> to
// <root>/.agnostic-ai/AGNOSTIC_AI.md. Returns (false, nil) when the
// source is absent so the caller can skip its "seeded from <src>"
// summary line. Each importer calls this with the target's own
// top-level instructions filename so the project keeps a CLI-agnostic
// copy under the managed directory. Later imports overwrite earlier
// mirrors (last-import wins).
//
// The generated target-overview appendix (sync.target-overview) is
// stripped before the write: it is per-entry-point derived output, not
// part of the canonical body, and carrying it back would duplicate it
// on the next sync.
func mirrorMainFile(root, srcName string) (bool, error) {
	src := filepath.Join(root, srcName)
	dst := filepath.Join(root, agnosticMainFile)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mirror %s: %w", srcName, err)
	}
	body := adapters.StripTargetOverview(string(data))
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, []byte(body), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", dst, err)
	}
	return true, nil
}

// copyMarkdownDir copies every top-level *.md file from srcDir into
// dstDir. Subdirectories and non-.md entries are skipped. The
// agnostic-ai provenance header (when present) is stripped so a
// roundtrip does not carry it back into source specs. Caller must
// ensure srcDir exists.
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
		if err := copyMarkdownFile(src, dst); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// copyMarkdownFile reads src, strips the agnostic-ai provenance header
// when present, and writes the result to dst. Missing src is a no-op
// so callers can probe optional paths without pre-checking existence.
func copyMarkdownFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", src, err)
	}
	out := header.Strip(string(data))
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, []byte(out), 0o644); err != nil {
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
		if err := importMkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}
