// Package cli builds the cobra command tree for the agnostic-ai binary.
package cli

import (
	"github.com/spf13/cobra"

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
	root.AddCommand(
		newSyncCmd(),
		newValidateCmd(),
		newListCmd(),
		newInitCmd(),
		newImportCmd(),
		newDoctorCmd(),
		newRevertCmd(),
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
	cfg, err := config.Load(root)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	b, err := spec.LoadLayered(resolveLayers(root, cfg))
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	return cfg, b, nil
}
