package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// rulesDirImporters maps a source name to the rules directory the
// importer walks. Claude, Codex, and Cursor have richer importers and
// route directly; cline, windsurf, junie, and trae also have dedicated
// importers (import_cline.go, import_windsurf.go, import_junie.go,
// import_trae.go) because each additionally reconstructs skills from its
// native `SKILL.md` folder tree (trae also reconstructs commands).
var rulesDirImporters = map[string]string{
	"qoder": filepath.Join(".qoder", "rules"),
}

// importSources lists every source the import command accepts, used in
// help text and error messages.
func importSources() string {
	names := []string{
		"aider", "amp", "antigravity", "claude", "cline", "codex", "continue",
		"copilot", "crush", "cursor", "gemini", "junie", "kiro", "opencode",
		"trae", "warp", "windsurf", "zed",
	}
	for k := range rulesDirImporters {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func newImportCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import <source>...",
		Short: "Import existing config from one or more AI CLIs into this project's source directories.",
		Long: "Reads agnostic-ai.yaml to resolve source paths, then translates " +
			"existing AI CLI configurations into agnostic specs. Sources: " + importSources() + ". " +
			"Pass multiple sources to import from each in order; `.agnostic-ai/AGNOSTIC_AI.md` " +
			"reflects the last source's top-level instructions file (last-wins).",
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
  agnostic-ai import claude --dry-run`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(".")
			if err != nil {
				return fmt.Errorf("load config: %w (run `agnostic-ai init` first)", err)
			}
			importDryRun = dryRun
			defer func() { importDryRun = false }()
			if dryRun {
				resetImportDryRunPaths()
				defer reportImportDryRun()
			}
			if len(args) == 1 {
				return runImport(".", args[0], cfg)
			}
			return runImportMany(".", args, cfg)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report which spec files would be written without touching disk.")
	return cmd
}

// runImport dispatches to the per-source importer.
func runImport(root, source string, cfg *config.Config) error {
	src := cfg.Sources
	switch source {
	case "all":
		return importAll(root, cfg)
	case "claude":
		return importFromClaude(root, src)
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
	case "continue":
		return importFromContinue(root, src)
	case "kiro":
		return importFromKiro(root, src)
	case "crush":
		return importFromCrush(root, src)
	case "windsurf":
		return importFromWindsurf(root, src)
	case "trae":
		return importFromTrae(root, src)
	case "junie":
		return importFromJunie(root, src)
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
	switch source {
	case "claude", "codex", "cursor", "cline", "aider", "amp", "warp",
		"gemini", "copilot", "opencode", "zed", "windsurf", "kiro", "crush",
		"trae", "junie":
		return true
	}
	_, ok := rulesDirImporters[source]
	return ok
}

// importAll detects every AI CLI present in root and imports from each.
func importAll(root string, cfg *config.Config) error {
	detected := detectExistingTargets(root)
	if len(detected) == 0 {
		fmt.Println("no known AI CLI configs detected")
		return nil
	}
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
