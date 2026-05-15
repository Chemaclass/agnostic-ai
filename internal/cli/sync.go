package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func newSyncCmd() *cobra.Command {
	var targets, only, except []string
	var dryRun, check, plan, backup, watch, watchPoll, jsonOut, allTargets bool
	var gitignoreFlag, autoSyncFlag string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Emit per-target configs from agnostic specs.",
		Example: `  # Emit every target listed in agnostic-ai.yaml
  agnostic-ai sync

  # Emit only Claude and Cursor
  agnostic-ai sync --only claude,cursor

  # Emit everything except Codex
  agnostic-ai sync --except codex

  # Preview without writing
  agnostic-ai sync --dry-run

  # CI gate: non-zero exit when output drifts from specs
  agnostic-ai sync --check

  # Back up each existing file to <path>.bak before overwriting
  agnostic-ai sync --backup

  # Structured per-target diff (added/changed counts) without writing
  agnostic-ai sync --plan

  # Machine-readable output for CI dashboards and editor extensions
  agnostic-ai sync --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateGitignoreFlag(gitignoreFlag); err != nil {
				return err
			}
			if len(only) > 0 && len(except) > 0 {
				return errs.Coded(errs.CodeFlagConflict, "--only and --except are mutually exclusive")
			}
			if watch && check {
				return errs.Coded(errs.CodeFlagConflict, "--watch and --check are incompatible")
			}

			cfg, _, err := loadProject(".")
			if err != nil {
				return err
			}
			base := targets
			if len(base) == 0 {
				base = cfg.Targets
			}
			if !check && !dryRun && !allTargets && len(targets) == 0 && len(only) == 0 && len(except) == 0 {
				if shouldPromptTargetSelection(".", cfg) {
					picked, err := firstSyncTargetSelection(".", cmd.InOrStdin(), cmd.OutOrStdout())
					if err != nil {
						return err
					}
					if len(picked) > 0 {
						base = picked
					}
				}
			}
			effective, err := filterTargets(base, only, except)
			if err != nil {
				return err
			}

			if plan {
				reports, err := collectDrift(effective)
				if err != nil {
					return err
				}
				printSyncPlan(cmd, reports)
				return nil
			}
			if check {
				reports, err := collectDrift(effective)
				if err != nil {
					return err
				}
				if jsonOut {
					return printSyncCheckJSON(cmd, reports)
				}
				if printDrift(reports) {
					return fmt.Errorf("drift detected")
				}
				return nil
			}
			if !dryRun {
				if err := handleAutoSync(".", autoSyncFlag, cmd.InOrStdin(), cmd.OutOrStdout()); err != nil {
					return err
				}
			}
			if watchPoll && !watch {
				return fmt.Errorf("--watch-poll requires --watch")
			}
			if watch {
				ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
				defer stop()
				return watchSync(ctx, 200*time.Millisecond, ".", effective, dryRun, backup, gitignoreFlag, watchPoll)
			}
			if jsonOut {
				return runSyncJSON(cmd, ".", effective, dryRun, backup, gitignoreFlag)
			}
			return runSyncOnce(".", effective, dryRun, backup, gitignoreFlag)
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to emit (default: all in config)")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Emit only these targets (comma-separated); mutually exclusive with --except")
	cmd.Flags().StringSliceVar(&except, "except", nil, "Emit all configured targets except these (comma-separated); mutually exclusive with --only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print outputs instead of writing")
	cmd.Flags().BoolVar(&check, "check", false, "Compare emitted output to disk; non-zero exit on drift")
	cmd.Flags().BoolVar(&plan, "plan", false, "Show per-target added/changed counts without writing")
	cmd.Flags().BoolVar(&backup, "backup", false, "Copy each existing target file to <path>.bak before overwriting (consumed by `agnostic-ai revert`; clear leftover .bak with `agnostic-ai cleanup --backups`)")
	cmd.Flags().StringVar(&gitignoreFlag, "gitignore", "", "Override config: 'on' or 'off' to manage the .gitignore block this run.")
	cmd.Flags().BoolVar(&watch, "watch", false, "Re-emit on spec changes (Ctrl+C to exit)")
	cmd.Flags().BoolVar(&watchPoll, "watch-poll", false, "Force polling instead of fsnotify (use on filesystems where fsnotify is unreliable, e.g. some network mounts)")
	cmd.Flags().StringVar(&autoSyncFlag, "auto-sync", "", "Enable agent-managed auto-sync: 'yes' or 'no'")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&allTargets, "all", false, "Sync every configured target without prompting (skip the first-sync target picker)")
	registerTargetCompletion(cmd)
	return cmd
}
