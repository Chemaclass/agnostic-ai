// Package cli builds the cobra command tree for the agnostic-ai binary.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// envProfile is the env-var fallback for --profile. When set (and --profile
// is empty), the command writes a runtime/pprof CPU profile of the run to
// this path. Off by default; the flag wins when both are set.
const envProfile = "AGNOSTIC_AI_PROFILE"

// NewRootCmd builds the root command tree.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "agnostic-ai",
		Short:         "Define AI agents, skills, rules, hooks once. Transpile per AI CLI.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		Example: `  # Set up a project for the tools you use
  echo "claude,codex" | agnostic-ai init

  # Preview changes, then generate tool configs
  agnostic-ai sync --plan
  agnostic-ai sync

  # Start troubleshooting, then follow the suggested next step
  agnostic-ai doctor

  # Migrate an existing project from another tool
  agnostic-ai init
  agnostic-ai import claude

  # CI gate: fail when emitted files drift from specs
  agnostic-ai sync --check`,
	}
	// Cobra's cmd.Print* writes to stderr until an output is set, which hid
	// every report from a pipe.
	root.SetOut(os.Stdout)

	var quiet bool
	root.PersistentFlags().CountP("verbose", "v", "Increase output verbosity")
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")

	var profilePath string
	root.PersistentFlags().StringVar(&profilePath, "profile", "",
		"Write a runtime/pprof CPU profile of the run to this file (or set AGNOSTIC_AI_PROFILE); off by default")

	// Cobra auto binds -v to --version when Version is set so
	// So here taking -v back for verbosity by clearing the shorthand on the version flag.
	root.InitDefaultVersionFlag()
	if vf := root.Flags().Lookup("version"); vf != nil {
		vf.Shorthand = ""
	}

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		v, _ := cmd.Flags().GetCount("verbose")
		if quiet && v > 0 {
			return fmt.Errorf("--quiet and --verbose are mutually exclusive")
		}
		requiresWarnOut, requiresWarned = cmd.ErrOrStderr(), map[string]bool{}
		gitquery.Reset()
		switch {
		case quiet:
			verbosity = levelQuiet
			adapters.SetWarner(io.Discard)
		default:
			verbosity = v
		}
		return nil
	}

	addCommandGroup(root, "setup", "Setup:",
		newInitCmd(),
		newImportCmd(),
		newNewCmd(),
		newUseCmd(),
	)
	addCommandGroup(root, "generation", "Daily generation:",
		newSyncCmd(),
		newRenderCmd(),
	)
	addCommandGroup(root, "diagnosis", "Diagnosis and inspection:",
		newDoctorCmd(),
		newValidateCmd(),
		newLintCmd(),
		newStatusCmd(),
		newVerifyCmd(),
		newListCmd(),
		newExplainCmd(),
		newCompareCmd(),
		newWhyCmd(),
		newGraphCmd(),
		newViewCmd(),
		newServeCmd(),
		newMCPCmd(),
	)
	addCommandGroup(root, "maintenance", "Maintenance:",
		newInstallHookCmd(),
		newProjectCmd(),
		newHookCmd(),
		newRevertCmd(),
		newCleanupCmd(),
		newPacksCmd(),
		newLSPCmd(),
		newUpgradeCmd(),
		newMigrateCmd(),
		newMemoryCmd(),
		newHandoffCmd(),
	)
	root.ValidArgsFunction = completeCommandAliases
	root.SetHelpCommandGroupID("maintenance")
	root.SetCompletionCommandGroupID("maintenance")
	root.InitDefaultCompletionCmd()
	profileEachRun(root, &profilePath)
	guidedUpgradeEachRun(root)
	return root
}

// completeCommandAliases offers the aliases of visible commands, which
// cobra leaves out of completion.
func completeCommandAliases(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var aliases []cobra.Completion
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		for _, alias := range sub.Aliases {
			if strings.HasPrefix(alias, toComplete) {
				aliases = append(aliases, cobra.CompletionWithDesc(alias, sub.Short))
			}
		}
	}
	return aliases, cobra.ShellCompDirectiveNoFileComp
}

func addCommandGroup(root *cobra.Command, id, title string, commands ...*cobra.Command) {
	root.AddGroup(&cobra.Group{ID: id, Title: title})
	for _, cmd := range commands {
		cmd.GroupID = id
	}
	root.AddCommand(commands...)
}

// profileEachRun starts and stops the CPU profile inside every RunE, so
// a run that fails, such as sync --check on drift, still leaves a complete
// profile. Cobra skips PersistentPostRunE on an error, and its flag checks
// run between the pre-run hooks and RunE.
func profileEachRun(cmd *cobra.Command, path *string) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) (runErr error) {
			defer func() { runErr = errors.Join(runErr, cleanupBuiltinLayers()) }()
			f, err := startCPUProfile(*path)
			if err != nil {
				return err
			}
			err = run(c, args)
			if stopErr := stopCPUProfile(f); stopErr != nil {
				return errors.Join(err, stopErr)
			}
			return err
		}
	}
	for _, sub := range cmd.Commands() {
		profileEachRun(sub, path)
	}
}

// loadProject loads config and project-scoped specs. Packs have lower
// precedence than project specs, while .agnostic-ai/local has higher
// precedence. User-level specs are installed only by sync --global.
// It stops when the running binary misses the config's requires, before
// specs load, so a spec only a newer release reads cannot fail ahead of
// the upgrade hint.
func loadProject(root string) (*config.Config, spec.Bundle, error) {
	return loadProjectWithLoader(root, spec.LoadLayered)
}

func loadProjectWithLoader(root string, load func([]spec.Layer) (spec.Bundle, error)) (*config.Config, spec.Bundle, error) {
	cfg, sources, err := loadValidConfig(root)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	if len(sources) > 1 {
		verbosef("→ merged %d config layers: %s\n",
			len(sources), strings.Join(sources, ", "))
	}
	layers, err := resolveLayers(root, cfg)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	b, err := load(layers)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	b.ApplyModelTiers(cfg.Models)
	return cfg, b, nil
}

// loadValidConfig loads the project config with its layer sources and
// rejects what sync cannot honor, so doctor config and every command
// that loads the project agree on what a valid config is.
func loadValidConfig(root string) (*config.Config, []string, error) {
	cfg, sources, err := config.LoadWithSources(root)
	if unknown, ok := errors.AsType[*config.UnknownKeysError](err); ok {
		if rerr := requireVersion(unknown.Source, unknown.Requires); rerr != nil {
			return nil, nil, rerr
		}
	}
	if err != nil {
		return nil, nil, err
	}
	source := strings.Join(sources, " + ")
	if err := requireVersion(source, cfg.Requires); err != nil {
		return nil, nil, err
	}
	if err := validateConfigTargets(cfg, source); err != nil {
		return nil, nil, err
	}
	if err := validateBuiltinNames(cfg.Builtins, source); err != nil {
		return nil, nil, err
	}
	if err := validateAgentsOutput(cfg, source); err != nil {
		return nil, nil, err
	}
	if err := validateCoverageTargets(cfg); err != nil {
		return nil, nil, err
	}
	if err := validateTierTargets(cfg.Models, source); err != nil {
		return nil, nil, err
	}
	return cfg, sources, nil
}

// startCPUProfile begins a runtime/pprof CPU profile for the current run. The
// path comes from the --profile flag, or AGNOSTIC_AI_PROFILE when the flag is
// empty. An empty path leaves profiling off and returns a nil file. Hand the
// returned file to stopCPUProfile to flush and close it. Profiling is
// stdlib-only and opt-in; an existing file at path is truncated.
func startCPUProfile(path string) (*os.File, error) {
	if path == "" {
		path = os.Getenv(envProfile)
	}
	if path == "" {
		return nil, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("start cpu profile: %w", err)
	}
	return f, nil
}

// stopCPUProfile stops the CPU profile and closes its file. A nil file means
// profiling was off, so it is a no-op. Stopping flushes the pprof payload, so
// the file is a complete profile only after this returns.
func stopCPUProfile(f *os.File) error {
	if f == nil {
		return nil
	}
	pprof.StopCPUProfile()
	if err := f.Close(); err != nil {
		return fmt.Errorf("%s: %w", f.Name(), err)
	}
	return nil
}
