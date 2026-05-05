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
		Example: `  # Emit every target listed in agnostic.config.yaml
  agnostic-ai sync

  # Emit only Claude and Cursor
  agnostic-ai sync -t claude -t cursor

  # Preview without writing
  agnostic-ai sync --dry-run

  # CI gate: non-zero exit when output drifts from specs
  agnostic-ai sync --check

  # Back up each existing file to <path>.bak before overwriting
  agnostic-ai sync --backup`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateGitignoreFlag(gitignoreFlag); err != nil {
				return err
			}
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
			gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
			if gitignoreOn {
				adapters.StartRecording()
			}
			for _, t := range targets {
				adapter, ok := adapters.Get(t)
				if !ok {
					fmt.Fprintf(os.Stderr, "! unknown target: %s\n", t)
					continue
				}
				fmt.Printf("→ emit %s\n", t)
				if err := adapter.Emit(b, cfg, dryRun); err != nil {
					if gitignoreOn {
						adapters.StopRecording()
					}
					return fmt.Errorf("%s: %w", t, err)
				}
			}
			if gitignoreOn {
				if err := updateGitignore(cfg, normalizeAndSort(adapters.StopRecording())); err != nil {
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
	cmd.Flags().StringVar(&gitignoreFlag, "gitignore", "", "Override config: 'on' or 'off' to manage the .gitignore block this run.")
	return cmd
}

// resolveGitignore picks the effective gitignore mode: the --gitignore
// flag wins when set, otherwise cfg.Gitignore.Enabled.
func resolveGitignore(cfg *config.Config, flag string) bool {
	switch flag {
	case "":
		return cfg.Gitignore.Enabled
	case "on":
		return true
	case "off":
		return false
	}
	return cfg.Gitignore.Enabled
}

// validateGitignoreFlag returns nil for "", "on", or "off"; otherwise an
// error. Used by sync to fail fast on bad input rather than silently
// falling back to config.
func validateGitignoreFlag(flag string) error {
	switch flag {
	case "", "on", "off":
		return nil
	}
	return fmt.Errorf("--gitignore: expected 'on' or 'off', got %q", flag)
}
