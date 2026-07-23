// Package cli builds the cobra command tree for the agnostic-ai binary.
package cli

import (
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

	// profilePath drives opt-in CPU profiling of the whole run. profileFile
	// is captured by the pre/post hooks so the file opened before the command
	// runs is flushed and closed after it finishes.
	var profilePath string
	var profileFile *os.File
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
		switch {
		case quiet:
			verbosity = levelQuiet
			adapters.SetWarner(io.Discard)
		default:
			verbosity = v
		}
		f, err := startCPUProfile(profilePath)
		if err != nil {
			return err
		}
		profileFile = f
		return nil
	}
	root.PersistentPostRunE = func(_ *cobra.Command, _ []string) error {
		return stopCPUProfile(profileFile)
	}

	root.AddCommand(
		newSyncCmd(),
		newValidateCmd(),
		newLintCmd(),
		newListCmd(),
		newInitCmd(),
		newImportCmd(),
		newDoctorCmd(),
		newInstallHookCmd(),
		newRevertCmd(),
		newCleanupCmd(),
		newPacksCmd(),
		newStatusCmd(),
		newNewCmd(),
		newRenderCmd(),
		newExplainCmd(),
		newWhyCmd(),
		newGraphCmd(),
		newLSPCmd(),
		newUpgradeCmd(),
	)
	root.InitDefaultCompletionCmd()
	return root
}

// loadProject loads config and a layered bundle from root. Layer
// precedence (low to high): user-global (~/.agnostic-ai or
// $AGNOSTIC_AI_HOME) → project (cfg.Sources) → project-user
// (.agnostic-ai.local). Optional layers load only when their root
// exists.
func loadProject(root string) (*config.Config, spec.Bundle, error) {
	cfg, sources, err := config.LoadWithSources(root)
	if err != nil {
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
