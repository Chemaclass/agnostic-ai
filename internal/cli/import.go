package cli

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// rulesDirImporters maps a source name to the rules directory the
// importer walks. Claude, Codex, and Cursor have richer importers and
// route directly; cline, windsurf, junie, trae, and qoder also have
// dedicated importers (import_cline.go, import_windsurf.go,
// import_junie.go, import_trae.go, import_qoder.go) because each
// additionally reconstructs something beyond flat rule files: skills
// from a native `SKILL.md` folder tree, and commands from a native
// commands directory (trae, qoder); qoder also reconstructs native
// agents from `.qoder/agents/`.
var rulesDirImporters = map[string]string{}

// importSourceNames lists every source runImport dispatches. Both the
// help text and multi-source validation read it, so the two cannot
// disagree: when they were separate lists, `import antigravity codex`
// failed with an error that named antigravity as supported (#905).
var importSourceNames = []string{
	"aider", "amp", "antigravity", "augment", "claude", "cline", "codex", "continue",
	"copilot", "crush", "cursor", "factory", "gemini", "goose", "junie", "kilo", "kiro",
	"opencode", "openhands", "qoder", "trae", "warp", "windsurf", "zed",
}

// importSources lists every source the import command accepts, used in
// help text and error messages.
func importSources() string {
	names := append([]string(nil), importSourceNames...)
	for k := range rulesDirImporters {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func newImportCmd() *cobra.Command {
	var dryRun, diff bool
	cmd := &cobra.Command{
		Use:   "import <source>...",
		Short: "Import existing config from one or more AI CLIs into this project's source directories.",
		Long: "Reads agnostic-ai.yaml to resolve source paths, then translates " +
			"existing AI CLI configurations into agnostic specs. Sources: " + importSources() + ". " +
			"Pass multiple sources to import from each in order; `.agnostic-ai/AGNOSTIC_AI.md` " +
			"reflects the last source's top-level instructions file (last-wins). " +
			"`--dry-run` runs the import in a temporary copy of the project (without .git) and " +
			"lists the files it would write; the project stays untouched. Add `--diff` to show " +
			"each proposed change, which sources wrote it, and where sources disagree.",
		Example: `  # Migrate an existing Claude Code project
  agnostic-ai init
  agnostic-ai import claude

  # Migrate from Cursor (.cursor/rules/*.mdc -> rules/*.md)
  agnostic-ai import cursor

  # Import from multiple CLIs in one shot (AGNOSTIC_AI.md = AGENTS.md, last-wins)
  agnostic-ai import claude codex

  # Import from every detected AI CLI in one shot
  agnostic-ai import all

  # Preview what would be imported without writing
  agnostic-ai import claude --dry-run

  # Review the proposed content and sources that compete for one file
  agnostic-ai import claude codex --dry-run --diff`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if diff && !dryRun {
				return errs.Coded(errs.CodeFlagConflict, "--diff requires --dry-run")
			}
			if diff {
				return previewImport(args)
			}
			if dryRun {
				return dryRunImport(args)
			}
			return runImportArgs(args)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report which spec files would be written without touching disk.")
	cmd.Flags().BoolVar(&diff, "diff", false, "With --dry-run, show created, changed, and unchanged destinations, a unified diff per change, and sources that propose different content for one destination.")
	return cmd
}

// runImportArgs loads the project config from the working directory and
// imports every named source in order.
func runImportArgs(args []string) error {
	cfg, err := config.Load(".")
	if err != nil {
		return fmt.Errorf("load config: %w (run `agnostic-ai init` first)", err)
	}
	if len(args) == 1 {
		return runImport(".", args[0], cfg)
	}
	return runImportMany(".", args, cfg)
}

// runImport dispatches to the per-source importer.
func runImport(root, source string, cfg *config.Config) error {
	if importRecording != nil && source != "all" {
		importRecording.beginSource(source)
	}
	src := cfg.Sources
	switch source {
	case "all":
		return importAll(root, cfg)
	case "claude":
		return importFromClaude(root, src, claudeLayoutFor(cfg))
	case "codex":
		return importFromCodexWithOpts(root, src, importCodexOpts{
			Shred:     cfg.Import.Codex.Shred,
			RulesFile: codexRulesFileFromCfg(cfg),
		})
	case "cursor":
		return importFromCursor(root, src)
	case "cline":
		return importFromCline(root, src)
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
	case "antigravity":
		return importFromAntigravity(root, src, cfg)
	case "augment":
		return importFromAugment(root, src)
	case "continue":
		return importFromContinue(root, src)
	case "kiro":
		return importFromKiro(root, src)
	case "kilo":
		return importKiloIgnore(root, src)
	case "crush":
		return importFromCrush(root, src)
	case "windsurf":
		return importFromWindsurf(root, src, cfg)
	case "trae":
		return importFromTrae(root, src)
	case "junie":
		return importFromJunie(root, src)
	case "qoder":
		return importFromQoder(root, src)
	case "goose":
		return importFromGoose(root, src)
	case "openhands":
		return importFromOpenhands(root, src)
	case "factory":
		return importFromFactory(root, src)
	}
	if srcDir, ok := rulesDirImporters[source]; ok {
		return importFromRulesDir(root, source, srcDir, src)
	}
	return errs.Coded(errs.CodeImportFileUnknown,
		"unknown source: %q (supported: %s, all)", source, importSources())
}

// runImportMany imports from each named source in order. `.agnostic-ai/
// AGNOSTIC_AI.md` ends up mirroring the last source's top-level
// instructions file (last-wins). Sources are validated up-front so a
// typo on arg N fails before any writes happen.
func runImportMany(root string, sources []string, cfg *config.Config) error {
	for _, s := range sources {
		if s == "all" {
			return errs.Coded(errs.CodeImportFileUnknown,
				"`all` cannot be combined with other sources")
		}
		if !isKnownImportSource(s) {
			return errs.Coded(errs.CodeImportFileUnknown,
				"unknown source: %q (supported: %s, all)", s, importSources())
		}
	}
	setImportRunSources(sources)
	defer setImportRunSources(nil)
	var failed []string
	for _, s := range sources {
		_, _ = fmt.Fprintf(os.Stdout, "→ importing from %s\n", s)
		if err := runImport(root, s, cfg); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "! %s: %v\n", s, err)
			failed = append(failed, s)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("import failed for: %s", strings.Join(failed, ", "))
	}
	return nil
}

// isKnownImportSource reports whether source is dispatched by runImport.
func isKnownImportSource(source string) bool {
	if slices.Contains(importSourceNames, source) {
		return true
	}
	_, ok := rulesDirImporters[source]
	return ok
}

// detectImportSources splits the targets detected under root into the
// ones runImport dispatches and the ones it has no importer for.
// Detection covers every target because `init` and the sync picker use
// it too, so every import path filters through here: `import all` once
// tried openhands and factory and failed on each (#1052).
func detectImportSources(root string) (importable, unsupported []string) {
	for _, t := range detectExistingTargets(root) {
		if isKnownImportSource(t) {
			importable = append(importable, t)
		} else {
			unsupported = append(unsupported, t)
		}
	}
	return importable, unsupported
}

// importAll detects every AI CLI present in root and imports from each.
func importAll(root string, cfg *config.Config) error {
	detected, unsupported := detectImportSources(root)
	for _, t := range unsupported {
		_, _ = fmt.Fprintf(os.Stdout, "- skipping %s: detected, but there is no importer for it\n", t)
	}
	if len(detected) == 0 {
		fmt.Println("no importable AI CLI configs detected")
		return nil
	}
	setImportRunSources(detected)
	defer setImportRunSources(nil)
	importAllSkippedEntryFiles = map[string]bool{}
	defer func() { importAllSkippedEntryFiles = nil }()
	var errs []string
	for _, t := range detected {
		_, _ = fmt.Fprintf(os.Stdout, "→ importing from %s\n", t)
		if err := runImport(root, t, cfg); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "! %s: %v\n", t, err)
			errs = append(errs, t)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("import failed for: %s", strings.Join(errs, ", "))
	}
	return nil
}
