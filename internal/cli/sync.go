package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

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
