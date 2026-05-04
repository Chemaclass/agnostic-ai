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
	}
	root.AddCommand(
		newSyncCmd(),
		newValidateCmd(),
		newListCmd(),
		newInitCmd(),
		newDoctorCmd(),
		newRevertCmd(),
	)
	return root
}

// loadProject loads config and bundle from root.
func loadProject(root string) (*config.Config, spec.Bundle, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	b, err := spec.LoadBundle(root, cfg)
	if err != nil {
		return nil, spec.Bundle{}, err
	}
	return cfg, b, nil
}
