package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func newSyncCmd() *cobra.Command {
	var targets, only, except []string
	var dryRun, check, plan, backup, keepEdits, untrack, watch, watchPoll, jsonOut, allTargets, diff, global bool
	var gitignoreFlag, format, against string
	var jobs int

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

  # Show a unified diff of every drifted file
  agnostic-ai sync --check --diff

  # Emit GitHub Actions annotations so drift surfaces inline on the PR
  agnostic-ai sync --check --format=github

  # Back up each existing file to <path>.bak before overwriting
  agnostic-ai sync --backup

  # From a post-checkout hook: leave hand-edited outputs in place and name them
  agnostic-ai sync --keep-edits

  # Structured per-target diff (added/changed counts) without writing
  agnostic-ai sync --plan

  # Machine-readable output for CI dashboards and editor extensions
  agnostic-ai sync --json

  # After moving generated files into the managed .gitignore block, stop tracking them
  agnostic-ai sync --untrack`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if keepEdits && (check || plan || watch || global) {
				return errs.Coded(errs.CodeFlagConflict, "--keep-edits cannot be combined with --check, --plan, --watch, or --global")
			}
			if err := validateAgainst(against, check, plan, watch, global); err != nil {
				return err
			}
			if untrack && (check || plan || dryRun || watch || global) {
				return errs.Coded(errs.CodeFlagConflict, "--untrack cannot be combined with --check, --plan, --dry-run, --watch, or --global")
			}
			if global {
				return runGlobalSync(cmd, globalSyncOptions{targets: targets, only: only, except: except, dryRun: dryRun, check: check, backup: backup, plan: plan, watch: watch, watchPoll: watchPoll, jsonOut: jsonOut, allTargets: allTargets, diff: diff, format: format, gitignore: gitignoreFlag, jobs: jobs})
			}
			if err := validateGitignoreFlag(gitignoreFlag); err != nil {
				return err
			}
			if err := validateCheckFormat(format); err != nil {
				return err
			}
			if len(only) > 0 && len(except) > 0 {
				return errs.Coded(errs.CodeFlagConflict, "--only and --except are mutually exclusive")
			}
			if watch && check {
				return errs.Coded(errs.CodeFlagConflict, "--watch and --check are incompatible")
			}
			if jsonOut && (watch || diff) {
				return errs.Coded(errs.CodeFlagConflict, "--json cannot be combined with --watch or --diff")
			}
			if diff && !check && !dryRun {
				return errs.Coded(errs.CodeFlagConflict, "--diff requires --check (or --dry-run); add --check to preview drift without writing")
			}
			if err := refuseGlobalHome(".", globalHomeSyncRemedy); err != nil {
				return err
			}

			var tree *againstTree
			if against != "" {
				entered, err := enterAgainstTree(against)
				if err != nil {
					return err
				}
				tree = entered
				defer tree.leave()
			}
			cfg, _, err := loadProject(".")
			if err != nil {
				return err
			}
			// A config target may name an external adapter a teammate has
			// not installed, so emit only warns about it. A name typed on
			// the command line is a request, and a typo must not pass as
			// "up to date".
			for _, t := range targets {
				if _, err := adapters.Resolve(t); err != nil {
					return err
				}
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
				adapters.ResetCoverageNotes()
				if !backup {
					if err := checkHandWrittenInstructions(effective); err != nil {
						return err
					}
				}
				reports, err := collectDrift(effective)
				if err != nil {
					return err
				}
				notesErr := checkCoverageNotes(cfg, effective)
				// With --check the plan still gates, so a CI step that
				// asks for the short report does not pass on drift.
				var driftErr error
				if check && slices.ContainsFunc(reports, driftReport.hasDrift) {
					driftErr = errDriftDetected()
				}
				if jsonOut {
					return errors.Join(printSyncPlanJSON(cmd, "sync --plan", reports, false, notesErr), driftErr)
				}
				printSyncPlan(cmd, reports)
				return errors.Join(notesErr, driftErr)
			}
			if check {
				adapters.ResetCoverageNotes()
				// --against compares what Git holds, not these files.
				if tree == nil && !backup {
					if err := checkHandWrittenInstructions(effective); err != nil {
						return err
					}
				}
				reports, err := collectDrift(effective)
				if err != nil {
					return err
				}
				notesErr := checkCoverageNotes(cfg, effective)
				if tree != nil {
					filtered := len(targets) > 0 || len(only) > 0 || len(except) > 0
					dropped, note, err := tree.droppedOutputs(filtered, reports)
					if err != nil {
						return err
					}
					if note != "" && verbosity >= levelDefault {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", note)
					}
					if reports, err = tree.trackedDrift(reports, dropped); err != nil {
						return err
					}
					markAgainst(reports, against)
					unmanaged, err := tree.trackedUnmanaged(cfg)
					if err != nil {
						return err
					}
					if len(unmanaged) > 0 {
						reports = append(reports, driftReport{Target: "unmanaged", Unmanaged: unmanaged})
					}
				}
				if jsonOut {
					return printSyncCheckJSON(cmd, reports, notesErr)
				}
				err = reportCheckDrift(cmd, reports, format, diff)
				if err != nil && tree != nil && regeneratedDrift(reports) {
					err = errors.New(againstHint(against))
				}
				return errors.Join(err, notesErr)
			}
			if watchPoll && !watch {
				return fmt.Errorf("--watch-poll requires --watch")
			}
			if watch {
				ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
				defer stop()
				return watchSync(ctx, 200*time.Millisecond, ".", effective, dryRun, backup, gitignoreFlag, watchPoll, jobs)
			}
			if jsonOut && dryRun {
				adapters.ResetCoverageNotes()
				if !backup {
					if err := checkHandWrittenInstructions(effective); err != nil {
						return err
					}
				}
				reports, err := collectDrift(effective)
				if err != nil {
					return err
				}
				return printSyncPlanJSON(cmd, "sync --dry-run", reports, true, checkCoverageNotes(cfg, effective))
			}
			if jsonOut {
				return runSyncJSON(cmd, ".", effective, backup, keepEdits, untrack, gitignoreFlag, jobs)
			}
			return runSyncPass(".", effective, dryRun, backup, keepEdits, untrack, gitignoreFlag, jobs)
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to emit (default: all in config)")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Emit only these targets (comma-separated); mutually exclusive with --except")
	cmd.Flags().StringSliceVar(&except, "except", nil, "Emit all configured targets except these (comma-separated); mutually exclusive with --only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print outputs instead of writing")
	cmd.Flags().BoolVar(&check, "check", false, "Compare emitted output to disk; non-zero exit on drift")
	cmd.Flags().BoolVar(&diff, "diff", false, "Requires --check (or --dry-run); prints a unified diff per drifted file (default: counts only, so CI logs stay lean)")
	cmd.Flags().StringVar(&format, "format", checkFormatHuman, "With --check, drift report format: 'human' or 'github' (GitHub Actions ::error annotations)")
	cmd.Flags().StringVar(&against, "against", "", "With --check, compare what Git holds instead of the working tree: 'index' (staged, for pre-commit hooks) or 'HEAD' (the last commit, for CI). Only outputs Git tracks are compared.")
	cmd.Flags().BoolVar(&plan, "plan", false, "Show per-target added/changed counts without writing")
	cmd.Flags().BoolVar(&backup, "backup", false, "Copy each existing target file to <path>.bak before overwriting (consumed by `agnostic-ai revert`; clear leftover .bak with `agnostic-ai cleanup --backups`)")
	cmd.Flags().BoolVar(&keepEdits, "keep-edits", false, "Leave each output edited since the last sync in place and list it, writing the rest (for post-checkout and post-merge hooks)")
	cmd.Flags().BoolVar(&untrack, "untrack", false, "Remove a generated path from git's index when it is also gitignored (git rm --cached, working tree untouched). Not with --check, --plan, --dry-run, --watch, or --global.")
	cmd.Flags().StringVar(&gitignoreFlag, "gitignore", "", "Override config: 'on' or 'off' to manage the .gitignore block this run.")
	cmd.Flags().BoolVar(&watch, "watch", false, "Re-emit on spec changes (Ctrl+C to exit)")
	cmd.Flags().BoolVar(&watchPoll, "watch-poll", false, "Force polling instead of fsnotify (use on filesystems where fsnotify is unreliable, e.g. some network mounts)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&allTargets, "all", false, "Sync every configured target without prompting (skip the first-sync target picker)")
	cmd.Flags().IntVar(&jobs, "jobs", 0, "Number of targets to emit in parallel (0 = one per CPU; 1 = serial). Output is identical regardless.")
	cmd.Flags().BoolVar(&global, "global", false, "Sync user-level instructions, rules, hooks, skills, and native agents")
	registerTargetCompletion(cmd)
	return cmd
}
