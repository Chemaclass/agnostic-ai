package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "agnostic-ai",
		Short: "Define AI agents, skills, rules, hooks once. Transpile per AI CLI.",
	}
	root.Version = version

	root.AddCommand(newSyncCmd())
	root.AddCommand(newValidateCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newInitCmd())
	return root
}

func newSyncCmd() *cobra.Command {
	var targets []string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Emit per-target configs from agnostic spec.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(".")
			if err != nil {
				return err
			}
			s, err := spec.LoadAll(".", cfg)
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
				if err := adapter.Emit(s, cfg, dryRun); err != nil {
					return fmt.Errorf("%s: %w", t, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to emit (default: all in config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print outputs instead of writing")
	return cmd
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate agnostic specs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(".")
			if err != nil {
				return err
			}
			s, err := spec.LoadAll(".", cfg)
			if err != nil {
				return err
			}
			fmt.Printf("loaded %d entries. ok.\n", len(s))
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List loaded specs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(".")
			if err != nil {
				return err
			}
			s, err := spec.LoadAll(".", cfg)
			if err != nil {
				return err
			}
			for _, e := range s {
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
