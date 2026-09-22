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
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	// claudeDir is the per-project Claude Code config directory.
	claudeDir = ".claude"
	// claudeMainFile is the project-root Claude Code instructions file.
	claudeMainFile = "CLAUDE.md"
	// claudeAgentsMainFile is the shared cross-tool instructions file
	// Claude Code falls back to. Since v2.1.277 a session with no
	// CLAUDE.md at or above the working directory reads AGENTS.md and
	// .claude/AGENTS.md instead, so in that repo shape this file, not
	// CLAUDE.md, is what the user's session is actually loading.
	claudeAgentsMainFile = "AGENTS.md"
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
func importFromClaude(root string, src config.Sources, layout claudeLayout) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks, src.MCPs, src.Commands); err != nil {
		return err
	}

	c := importCounts{}
	var err error
	if c.rules, err = importClaudeRules(root, filepath.Join(root, src.Rules), layout); err != nil {
		return err
	}
	if c.agents, err = importClaudeAgents(root, filepath.Join(root, src.Agents), layout); err != nil {
		return err
	}
	if c.skills, err = importClaudeSkills(root, filepath.Join(root, src.Skills), layout); err != nil {
		return err
	}
	if c.hooks, err = importClaudeHooks(root, filepath.Join(root, src.Hooks)); err != nil {
		return err
	}
	if err := captureHookScripts(root, "claude"); err != nil {
		return err
	}
	mainResult, mainSrc, promotedNested, err := mirrorClaudeMainFile(root)
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
	if c.mcps, err = importClaudeMCPWithSettings(root, filepath.Join(root, src.MCPs), layout.dir); err != nil {
		return err
	}
	if c.commands, err = importClaudeCommands(root, filepath.Join(root, src.Commands), layout); err != nil {
		return err
	}
	overlaySeeded, err := importClaudeSettingsOverlay(root)
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks, %d mcps, %d commands\n",
		c.rules, c.agents, c.skills, c.hooks, c.mcps, c.commands)
	switch mainResult {
	case mirrorWritten:
		summaryf("  → %s seeded from %s (commit this file — sync distributes it to all targets)\n",
			agnosticMainFile, mainSrc)
	case mirrorUnchanged:
		summaryf("  → %s unchanged (%s matches its fenced view)\n", agnosticMainFile, mainSrc)
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

// nestedClaudeAgentsMainFile is the nested half of the AGENTS.md
// fallback. code.claude.com/docs/en/memory: "At session start: every
// AGENTS.md and .claude/AGENTS.md in your working directory and the
// directories above it."
var nestedClaudeAgentsMainFile = filepath.Join(claudeDir, claudeAgentsMainFile)

// claudeMainFileRung is one step of the precedence chain
// mirrorClaudeMainFile walks.
type claudeMainFileRung struct {
	// name is the project-relative path of the candidate file.
	name string
	// nested marks the .claude/CLAUDE.md rung, whose promotion tells the
	// caller not to also capture the same file as a claude-private
	// helper overlay.
	nested bool
	// sharedEntryPoint marks a rung another importer owns outright (the
	// root AGENTS.md is codex's, amp's, and warp's own main file). The
	// rung is skipped when such an importer runs in the same invocation.
	sharedEntryPoint bool
}

// claudeMainFileChain is the order `import claude` looks for the
// project's Claude Code instructions, mirroring the vendor's own read
// order: a CLAUDE.md anywhere suppresses the AGENTS.md fallback
// entirely ("By default, Claude reads AGENTS.md only when you have no
// CLAUDE.md in your working directory or above it"), and the nested
// copy of each pair is the lower-ranked one here because the root file
// is the conventional home.
var claudeMainFileChain = []claudeMainFileRung{
	{name: claudeMainFile},
	{name: nestedClaudeMainFile, nested: true},
	{name: claudeAgentsMainFile, sharedEntryPoint: true},
	{name: nestedClaudeAgentsMainFile},
}

// agentsMainFileImporters names every import source whose own top-level
// instructions file is the root AGENTS.md. When one of them runs in the
// same invocation as claude it mirrors that file itself (and codex also
// shreds it into rule specs), so claude must not claim it too.
var agentsMainFileImporters = map[string]bool{
	"codex": true, "amp": true, "warp": true,
	"crush": true, "kiro": true, "opencode": true,
}

// mirrorClaudeMainFile mirrors the project's Claude main instructions to
// <root>/.agnostic-ai/AGNOSTIC_AI.md so `sync` distributes the body to
// every target's native entry-point (AGENTS.md, GEMINI.md, ...).
//
// Source precedence is claudeMainFileChain: CLAUDE.md, then
// .claude/CLAUDE.md, then AGENTS.md, then .claude/AGENTS.md. Promoting
// a lower rung is what lets a project that keeps its instructions
// somewhere other than the root CLAUDE.md (all three are documented
// Claude Code read paths) still feed codex, gemini, and the rest.
// Without it, those targets receive only the generic pointer template.
//
// The root AGENTS.md rung is skipped when codex, amp, warp, crush,
// kiro, or opencode imports in the same invocation: that file is their
// own main file, and reading one file from two importers would slice
// the same rules twice.
//
// Returns (result, srcName, promotedNested, err): result is mirrorAbsent
// on a project with no Claude instructions at all (callers suppress the
// summary line); srcName names the file the body came from; promotedNested
// is true when the nested .claude/CLAUDE.md was used, written or kept
// unchanged, signaling the caller to skip capturing it as a
// claude-private helper overlay.
func mirrorClaudeMainFile(root string) (result mirrorResult, srcName string, promotedNested bool, err error) {
	claimed := rootAgentsMainFileClaimedByPeer()
	for _, rung := range claudeMainFileChain {
		if rung.sharedEntryPoint && claimed {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(root, rung.name)); statErr != nil {
			continue
		}
		result, err = mirrorMainFile(root, rung.name)
		return result, rung.name, rung.nested && result != mirrorAbsent, err
	}
	return mirrorAbsent, claudeMainFile, false, nil
}

// rootAgentsMainFileClaimedByPeer reports whether another source in the
// current `import` run owns the root AGENTS.md.
func rootAgentsMainFileClaimedByPeer() bool {
	for _, s := range importRunSources {
		if agentsMainFileImporters[s] {
			return true
		}
	}
	return false
}

// mirrorResult is what mirrorMainFile did with the imported entry point.
type mirrorResult int

const (
	// mirrorAbsent: the source file does not exist; nothing to report.
	mirrorAbsent mirrorResult = iota
	// mirrorWritten: AGNOSTIC_AI.md was (re)seeded from the source.
	mirrorWritten
	// mirrorUnchanged: the source is exactly the view sync renders from a
	// fenced AGNOSTIC_AI.md, which is kept as is.
	mirrorUnchanged
)

// mirrorMainFile copies <root>/<srcName> to
// <root>/.agnostic-ai/AGNOSTIC_AI.md. Returns mirrorAbsent when the
// source is absent so the caller can skip its "seeded from <src>"
// summary line, and mirrorUnchanged when a fenced source is kept. Each importer calls this with the target's own
// top-level instructions filename so the project keeps a CLI-agnostic
// copy under the managed directory. Later imports overwrite earlier
// mirrors (last-import wins).
//
// The generated appendices (inlined rules + target-overview) are
// stripped before the write: they are per-entry-point derived output,
// not part of the canonical body, and carrying them back would
// duplicate them on the next sync (and leak target-specific rule copies
// into every other target's entry-point).
//
// The agnostic-ai provenance header is stripped too, exactly as it is
// for per-rule and per-skill specs. A synced entry-point (CLAUDE.md,
// AGENTS.md, ...) carries the header on line 1; carrying it back would
// reseed the source you are meant to edit with a "Do not edit" banner
// and break import->sync byte-stability (#429).
func mirrorMainFile(root, srcName string) (mirrorResult, error) {
	src := filepath.Join(root, srcName)
	dst := filepath.Join(root, agnosticMainFile)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return mirrorAbsent, nil
	}
	if err != nil {
		return mirrorAbsent, fmt.Errorf("mirror %s: %w", srcName, err)
	}
	body := header.Strip(adapters.StripGeneratedAppendices(string(data)))

	// A fenced source renders a per-file view; when the imported entry point
	// is exactly that view there is nothing new to capture and overwriting
	// would erase every other target's ::target block.
	if existing, readErr := os.ReadFile(dst); readErr == nil && strings.Contains(string(existing), "::target") {
		if matchesRenderedView(root, srcName, header.Strip(string(existing)), body) {
			return mirrorUnchanged, nil
		}
		summaryf("  ! %s replaced a fenced %s; ::target blocks for other tools are gone. Restore them from git if needed.\n", srcName, agnosticMainFile)
	}

	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return mirrorAbsent, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, []byte(body), 0o644); err != nil {
		return mirrorAbsent, fmt.Errorf("write %s: %w", dst, err)
	}
	warnUncapturedEntryPoints(root, srcName, body)
	return mirrorWritten, nil
}

// matchesRenderedView reports whether body is the view sync renders for
// srcName from the fenced source. It renders with the project's config,
// so the readers are the enabled targets with their outputs.<t>.file
// overrides, exactly as sync computes them. Without a loadable config
// there is no view to trust, so it reports false.
func matchesRenderedView(root, srcName, source, body string) bool {
	cfg, err := config.Load(root)
	if err != nil {
		return false
	}
	files, err := renderEntryPointFiles(cfg, spec.Bundle{}, cfg.Targets, source)
	if err != nil {
		return false
	}
	want := filepath.ToSlash(srcName)
	for _, f := range files {
		if filepath.ToSlash(f.Path) != want {
			continue
		}
		view := header.Strip(adapters.StripGeneratedAppendices(f.Content))
		return strings.TrimRight(view, "\n") == strings.TrimRight(body, "\n")
	}
	return false
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

// copyMarkdownTree copies every *.md file under srcDir into dstDir,
// preserving the relative subdirectory path. Scoped rules now emit under
// .claude/rules/<scope>/<name>.md, so a flat top-level copy would drop the
// nested files; walking the tree round-trips them back into
// <dstDir>/<scope>/<name>.md (#411, claude target). The agnostic-ai
// provenance header is stripped per file. Caller ensures srcDir exists.
func copyMarkdownTree(srcDir, dstDir string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("%s: %w", path, walkErr)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := copyMarkdownFile(path, filepath.Join(dstDir, rel)); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
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
	if err := importWriteSpecMarkdown(dst, []byte(out), 0o644); err != nil {
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
