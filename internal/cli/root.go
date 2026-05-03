// Package cli builds the cobra command tree for the agnostic-ai binary.
package cli

import (
	"fmt"
	"os"

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
	}
	root.AddCommand(
		newSyncCmd(),
		newValidateCmd(),
		newListCmd(),
		newInitCmd(),
		newDoctorCmd(),
	)
	return root
}

func newSyncCmd() *cobra.Command {
	var targets []string
	var dryRun, check bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Emit per-target configs from agnostic specs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				reports, err := collectDrift(targets)
				if err != nil {
					return err
				}
				if printDrift(reports) {
					return fmt.Errorf("drift detected")
				}
				return nil
			}
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				targets = cfg.Targets
			}
			for _, t := range targets {
				adapter, ok := adapters.Get(t)
				if !ok {
					fmt.Fprintf(os.Stderr, "! unknown target: %s\n", t)
					continue
				}
				fmt.Printf("→ emit %s\n", t)
				if err := adapter.Emit(b, cfg, dryRun); err != nil {
					return fmt.Errorf("%s: %w", t, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to emit (default: all in config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print outputs instead of writing")
	cmd.Flags().BoolVar(&check, "check", false, "Compare emitted output to disk; non-zero exit on drift")
	return cmd
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate agnostic specs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, b, err := loadProject(".")
			if err != nil {
				return err
			}
			fmt.Printf("loaded %d entries. ok.\n", len(b.All()))
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List loaded specs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, b, err := loadProject(".")
			if err != nil {
				return err
			}
			for _, e := range b.All() {
				fmt.Printf("%s\t%s\n", e.Kind, e.Name)
			}
			return nil
		},
	}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return scaffold(".")
		},
	}
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
