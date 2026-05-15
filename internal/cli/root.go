// Package cli builds the cobra command tree for the agnostic-ai binary.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

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
		return nil
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
		newPacksCmd(),
		newStatusCmd(),
		newNewCmd(),
		newRenderCmd(),
		newExplainCmd(),
		newWhyCmd(),
		newGraphCmd(),
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
