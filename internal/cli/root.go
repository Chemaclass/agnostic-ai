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
		Example: `  # Start a new project and emit configs for every target
  agnostic-ai init
  agnostic-ai sync

  # Migrate an existing project from another tool
  agnostic-ai init
  agnostic-ai import claude

  # CI gate: fail when emitted files drift from specs
  agnostic-ai sync --check`,
	}

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
		switch {
		case quiet:
			verbosity = levelQuiet
			adapters.SetWarner(io.Discard)
		default:
			verbosity = v
		}
		return nil
	}

	root.AddCommand(
		newSyncCmd(),
		newVerifyCmd(),
		newValidateCmd(),
		newLintCmd(),
		newListCmd(),
		newInitCmd(),
		newImportCmd(),
		newDoctorCmd(),
		newInstallHookCmd(),
		newHookCmd(),
		newRevertCmd(),
		newCleanupCmd(),
		newPacksCmd(),
		newStatusCmd(),
		newNewCmd(),
		newRenderCmd(),
		newExplainCmd(),
		newCompareCmd(),
		newWhyCmd(),
		newGraphCmd(),
		newLSPCmd(),
		newUpgradeCmd(),
	)
	root.InitDefaultCompletionCmd()
	profileEachRun(root, &profilePath)
	return root
}

// profileEachRun starts and stops the CPU profile inside every RunE, so
// a run that fails, such as sync --check on drift, still leaves a complete
// profile. Cobra skips PersistentPostRunE on an error, and its flag checks
// run between the pre-run hooks and RunE.
func profileEachRun(cmd *cobra.Command, path *string) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
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
	cfg, sources, err := config.LoadWithSources(root)
	if unknown, ok := errors.AsType[*config.UnknownKeysError](err); ok {
		if rerr := requireVersion(unknown.Source, unknown.Requires); rerr != nil {
			return nil, spec.Bundle{}, rerr
		}
	}
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	if err := requireVersion(strings.Join(sources, " + "), cfg.Requires); err != nil {
		return nil, spec.Bundle{}, err
	}
	if err := validateConfigTargets(cfg, strings.Join(sources, " + ")); err != nil {
		return nil, spec.Bundle{}, err
	}
	if err := validateCoverageTargets(cfg); err != nil {
		return nil, spec.Bundle{}, err
	}
	if err := validateTierTargets(cfg.Models, strings.Join(sources, " + ")); err != nil {
		return nil, spec.Bundle{}, err
	}
	if len(sources) > 1 {
		verbosef("→ merged %d config layers: %s\n",
			len(sources), strings.Join(sources, ", "))
	}
	b, err := spec.LoadLayered(resolveLayers(root, cfg))
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	b.ApplyModelTiers(cfg.Models)
	return cfg, b, nil
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
