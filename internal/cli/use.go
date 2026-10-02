package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <tool>...",
		Short: "Start using an AI tool with what the project already has, in one step.",
		Long: "Sets the project up when it has no agnostic-ai.yaml yet, importing the instructions, " +
			"skills, agents, hooks, and MCP servers of the tools it already uses. Adds each named tool " +
			"to targets, imports that tool's own existing config first, syncs, and shows what the " +
			"tool now reads.",
		Example: `  # Switching to Codex this month
  agnostic-ai use codex

  # A team on several tools
  agnostic-ai use claude codex cursor`,
		Args:      cobra.MinimumNArgs(1),
		ValidArgs: adapters.Names(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			tools, err := knownTools(args)
			if err != nil {
				return err
			}
			added, err := useTools(cmd, tools)
			if err != nil {
				return err
			}
			// Always sync, a no-op when nothing changed, so a run that
			// stopped halfway finishes on the next try.
			if err := runSyncPass(".", nil, false, false, false, false, "", 0); err != nil {
				return err
			}
			if len(added) == 0 {
				summaryf("%s %s already in use; edit .agnostic-ai/ and run agnostic-ai sync to change what it reads\n", tick(), strings.Join(tools, ", "))
				return nil
			}
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			printToolReads(logOut, cfg, b, added)
			return nil
		},
	}
}

// knownTools checks each name against the built-in targets, with a
// did-you-mean for a likely typo.
func knownTools(args []string) ([]string, error) {
	var tools []string
	for _, a := range args {
		if !slices.Contains(adapters.Names(), a) {
			if s := suggest.Name(a, adapters.Names()); s != "" {
				return nil, errs.Coded(errs.CodeSyncTargetUnknown, "unknown tool %q (did you mean %s?)", a, s)
			}
			return nil, errs.Coded(errs.CodeSyncTargetUnknown, "unknown tool %q; see https://agnostic-ai.org/docs/targets/", a)
		}
		if !slices.Contains(tools, a) {
			tools = append(tools, a)
		}
	}
	return tools, nil
}

// useTools sets the project up or extends it so it emits for tools, and
// returns the tools it added. A new project enables the tools it detects
// too and imports what they already have. An existing one imports each
// added tool's own config before the sync would write over it.
func useTools(cmd *cobra.Command, tools []string) ([]string, error) {
	path, _, err := config.ResolveConfigPath(".")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && errs.CodeOf(err) != errs.CodeConfigMissing {
			return nil, err
		}
		if err := refuseNestedProject(); err != nil {
			return nil, err
		}
		return tools, startProject(cmd, tools)
	}
	cfg, err := config.Load(".")
	if err != nil {
		return nil, err
	}
	var added []string
	for _, t := range tools {
		if !slices.Contains(cfg.Targets, t) {
			added = append(added, t)
		}
	}
	if len(added) > 0 {
		// The local file's targets win over the committed list, so a
		// tool added to the committed list would never sync.
		if localSetsTargets() {
			return nil, fmt.Errorf("%s sets targets, which win over %s; add %s there", config.LocalOverrideFileName, filepath.Base(path), strings.Join(added, ", "))
		}
		if err := config.PersistTargets(".", append(slices.Clone(cfg.Targets), added...)); err != nil {
			return nil, fmt.Errorf("add %s to targets: %w", strings.Join(added, ", "), err)
		}
		summaryf("→ added %s to targets in %s\n", strings.Join(added, ", "), filepath.Base(path))
		if cfg, err = config.Load("."); err != nil {
			return nil, err
		}
	}
	// Import is idempotent, so the named tools' own config is imported on
	// every run, and one an earlier run stopped partway through finishes.
	// Another target's hand-written instructions file would stop the
	// sync, so it is imported too.
	var sources []string
	for _, t := range cfg.Targets {
		if slices.Contains(tools, t) && hasOwnConfig(cfg, t) || uncapturedInstructions(cfg, t) {
			sources = append(sources, t)
		}
	}
	return added, importToolConfig(cfg, sources)
}

// uncapturedInstructions reports whether target's instructions file holds
// hand-written text AGNOSTIC_AI.md does not have, which sync stops on.
func uncapturedInstructions(cfg *config.Config, target string) bool {
	path := adapters.EntryPointPath(cfg, target)
	if path == "" {
		return false
	}
	captured := ""
	if data, err := os.ReadFile(adapters.AgnosticEntryPointPath); err == nil {
		captured = header.Strip(string(data))
	}
	held, err := heldInstructions(captured)
	if err != nil {
		return false
	}
	uncaptured, err := handWrittenUncaptured(path, held)
	return err == nil && uncaptured
}

// localSetsTargets reports whether agnostic-ai.local.yaml sets targets.
func localSetsTargets() bool {
	data, err := os.ReadFile(config.LocalOverrideFileName)
	if err != nil {
		return false
	}
	var local struct {
		Targets []string `yaml:"targets"`
	}
	return yaml.Unmarshal(data, &local) == nil && local.Targets != nil
}

// refuseNestedProject stops `use` from starting a second project inside
// one an enclosing directory already holds, up to the Git root, or up to
// the filesystem root outside Git.
func refuseNestedProject() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, _ := filepath.EvalSymlinks(cwd)
	top := ""
	if root, err := gitRevParse(cwd, "--show-toplevel"); err == nil {
		top, _ = filepath.EvalSymlinks(root)
	}
	for dir != top && filepath.Dir(dir) != dir {
		dir = filepath.Dir(dir)
		if _, _, err := config.ResolveConfigPath(dir); err == nil {
			return fmt.Errorf("this directory is inside the agnostic-ai project at %s; run agnostic-ai use there", dir)
		}
	}
	return nil
}

// startProject writes agnostic-ai.yaml for the detected tools plus
// tools, then imports everything the project already has.
func startProject(cmd *cobra.Command, tools []string) error {
	detected := detectExistingTargets(".")
	targets := slices.Clone(detected)
	for _, t := range tools {
		if !slices.Contains(targets, t) {
			targets = append(targets, t)
		}
	}
	gitignoreOn, err := promptGitignoreEnable(cmd.InOrStdin())
	if err != nil {
		return err
	}
	if err := scaffoldSilently(scaffoldOptions{
		Root:             ".",
		Targets:          targets,
		GitignoreEnabled: gitignoreOn,
		Version:          cmd.Root().Version,
	}); err != nil {
		return err
	}
	summaryf("→ created %s with targets: %s\n", config.ConfigFileName, strings.Join(targets, ", "))
	cfg, err := config.Load(".")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	var sources []string
	for _, t := range targets {
		if hasOwnConfig(cfg, t) {
			sources = append(sources, t)
		}
	}
	return importToolConfig(cfg, sources)
}

// hasOwnConfig reports whether the project already holds config for
// target that sync would otherwise write over: a marker init detects, or
// its instructions file written by hand.
func hasOwnConfig(cfg *config.Config, target string) bool {
	if slices.Contains(detectExistingTargets("."), target) {
		return true
	}
	path := adapters.EntryPointPath(cfg, target)
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) != "" && !header.Has(string(data))
}

// importToolConfig imports each tool that has an importer. A tool
// without one, such as jules, reads the root AGENTS.md, which is folded
// in for every run.
func importToolConfig(cfg *config.Config, tools []string) (err error) {
	var sources []string
	for _, t := range tools {
		if _, rulesDir := rulesDirImporters[t]; rulesDir || slices.Contains(importSourceNames, t) {
			sources = append(sources, t)
		}
	}
	// A hand-written root AGENTS.md no source above reads, such as a
	// Codex setup with no .codex/ folder, is folded in as `import all`
	// does; text already held is left alone.
	defer func() {
		if err == nil {
			_, err = foldRootAgentsMainFile(".")
		}
	}()
	if len(sources) == 0 {
		return nil
	}
	importNextStepsOff = true
	defer func() { importNextStepsOff = false }()
	return withImportTree(".", func() error {
		return withLocalImportGuard(".", cfg, func() error {
			if len(sources) == 1 {
				return runImport(".", sources[0], cfg)
			}
			return runImportMany(".", sources, cfg)
		})
	})
}

// printToolReads shows what each tool now reads from .agnostic-ai/: its
// instructions file, then each kind of spec it got, with where it lives.
func printToolReads(w io.Writer, cfg *config.Config, b spec.Bundle, tools []string) {
	for _, t := range tools {
		mine := b.For(t)
		_, _ = fmt.Fprintf(w, "%s %s now reads, from .agnostic-ai/:\n", tick(), t)
		if path := adapters.EntryPointPath(cfg, t); path != "" {
			_, _ = fmt.Fprintf(w, "    %-16s %s\n", "instructions", path)
		}
		// Where the tool keeps each kind, when its adapter says so.
		where := map[string]string{}
		for _, a := range adapters.NativeArtifactsFor(t, cfg) {
			if _, seen := where[strings.ToLower(a.Label)]; !seen {
				where[strings.ToLower(a.Label)] = a.Location
			}
		}
		for _, kind := range []string{"Rules", "Skills", "Agents", "Commands", "Hooks", "MCP servers"} {
			entries := entriesFor(mine, kind)
			if len(entries) == 0 {
				continue
			}
			label := fmt.Sprintf("%d %s", len(entries), countLabel(kind, len(entries)))
			_, _ = fmt.Fprintf(w, "    %-16s %-22s %s\n", label, where[strings.ToLower(kind)], entryNames(entries))
		}
	}
	_, _ = fmt.Fprintln(w, "  edit .agnostic-ai/ and run agnostic-ai sync to change what every tool reads")
}

// countLabel lowercases a native artifact label for a count, keeping an
// acronym such as MCP, and drops the plural for one.
func countLabel(label string, n int) string {
	if !strings.HasPrefix(label, "MCP") {
		label = strings.ToLower(label)
	}
	if n == 1 {
		label = strings.TrimSuffix(label, "s")
	}
	return label
}

// entriesFor returns the specs a native artifact label lists.
func entriesFor(b spec.Bundle, label string) []spec.Entry {
	switch strings.ToLower(label) {
	case "skills":
		return b.Skills
	case "agents":
		return b.Agents
	case "rules":
		return b.Rules
	case "commands":
		return b.Commands
	case "hooks":
		return b.Hooks
	case "mcp servers":
		return b.MCPs
	}
	return nil
}

func entryNames(entries []spec.Entry) string {
	const shown = 3
	names := make([]string, 0, shown)
	for i, e := range entries {
		if i == shown {
			return strings.Join(names, ", ") + fmt.Sprintf(", +%d", len(entries)-shown)
		}
		names = append(names, e.Name)
	}
	return strings.Join(names, ", ")
}
