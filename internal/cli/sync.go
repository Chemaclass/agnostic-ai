package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func newSyncCmd() *cobra.Command {
	var targets []string
	var dryRun, check, backup bool
	var gitignoreFlag string

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
			if backup {
				adapters.SetBackup(true)
				defer adapters.SetBackup(false)
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
			if !dryRun && resolveGitignore(cfg, gitignoreFlag) {
				paths, err := collectEmittedPaths(cfg, b, targets)
				if err != nil {
					return fmt.Errorf("gitignore: %w", err)
				}
				if err := updateGitignore(cfg, paths); err != nil {
					return fmt.Errorf("gitignore: %w", err)
				}
				fmt.Println("→ updated .gitignore")
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to emit (default: all in config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print outputs instead of writing")
	cmd.Flags().BoolVar(&check, "check", false, "Compare emitted output to disk; non-zero exit on drift")
	cmd.Flags().BoolVar(&backup, "backup", false, "Copy each existing target file to <path>.bak before overwriting")
	cmd.Flags().StringVar(&gitignoreFlag, "gitignore", "", "Override config: 'on' or 'off' to manage the .gitignore block this run")
	return cmd
}

// resolveGitignore picks the effective gitignore mode: the --gitignore
// flag wins when set, otherwise cfg.Gitignore.Enabled.
func resolveGitignore(cfg *config.Config, flag string) bool {
	switch flag {
	case "on", "true", "yes":
		return true
	case "off", "false", "no":
		return false
	}
	return cfg.Gitignore.Enabled
}
