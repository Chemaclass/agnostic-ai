package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

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
		Args: cobra.MinimumNArgs(1),
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
			if len(added) == 0 {
				summaryf("%s %s already in use; edit .agnostic-ai/ and run agnostic-ai sync to change what it reads\n", tick(), strings.Join(tools, ", "))
				return nil
			}
			if err := runSyncPass(".", nil, false, false, false, false, "", 0); err != nil {
				return err
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
	if _, _, err := config.ResolveConfigPath("."); err != nil {
		if !errors.Is(err, os.ErrNotExist) && errs.CodeOf(err) != errs.CodeConfigMissing {
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
	if len(added) == 0 {
		return nil, nil
	}
	if err := config.PersistTargets(".", append(slices.Clone(cfg.Targets), added...)); err != nil {
		return nil, fmt.Errorf("add %s to targets: %w", strings.Join(added, ", "), err)
	}
	summaryf("→ added %s to targets in %s\n", strings.Join(added, ", "), config.ConfigFileName)
	if cfg, err = config.Load("."); err != nil {
		return nil, err
	}
	var sources []string
	for _, t := range added {
		if hasOwnConfig(cfg, t) {
			sources = append(sources, t)
		}
	}
	return added, importToolConfig(cfg, sources)
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

func importToolConfig(cfg *config.Config, sources []string) error {
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
		for _, a := range adapters.NativeArtifactsFor(t, cfg) {
			entries := entriesFor(mine, a.Label)
			if len(entries) == 0 {
				continue
			}
			label := fmt.Sprintf("%d %s", len(entries), countLabel(a.Label, len(entries)))
			_, _ = fmt.Fprintf(w, "    %-16s %-22s %s\n", label, a.Location, entryNames(entries))
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
