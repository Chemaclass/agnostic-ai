package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

type syncStateFile struct {
	SyncedAt     time.Time `json:"synced_at"`
	FilesChanged int       `json:"files_changed"`
}

func stateFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".agnostic-ai", ".sync-state")
}

func writeStateFile(projectRoot string, filesChanged int) error {
	p := stateFilePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	data, err := json.Marshal(syncStateFile{
		SyncedAt:     time.Now().UTC(),
		FilesChanged: filesChanged,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func newSyncCmd() *cobra.Command {
	var targets, only, except []string
	var dryRun, check, backup, watch, watchPoll, jsonOut, allTargets bool
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

  # Machine-readable output for CI dashboards and editor extensions
  agnostic-ai sync --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateGitignoreFlag(gitignoreFlag); err != nil {
				return err
			}
			if len(only) > 0 && len(except) > 0 {
				return fmt.Errorf("--only and --except are mutually exclusive")
			}
			if watch && check {
				return fmt.Errorf("--watch and --check are incompatible")
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
	cmd.Flags().BoolVar(&backup, "backup", false, "Copy each existing target file to <path>.bak before overwriting")
	cmd.Flags().StringVar(&gitignoreFlag, "gitignore", "", "Override config: 'on' or 'off' to manage the .gitignore block this run.")
	cmd.Flags().BoolVar(&watch, "watch", false, "Re-emit on spec changes (Ctrl+C to exit)")
	cmd.Flags().BoolVar(&watchPoll, "watch-poll", false, "Force polling instead of fsnotify (use on filesystems where fsnotify is unreliable, e.g. some network mounts)")
	cmd.Flags().StringVar(&autoSyncFlag, "auto-sync", "", "Enable agent-managed auto-sync: 'yes' or 'no'")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&allTargets, "all", false, "Sync every configured target without prompting (skip the first-sync target picker)")
	registerTargetCompletion(cmd)
	return cmd
}

func runSyncOnce(root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
	cfg, b, err := loadProject(root)
	if err != nil {
		return err
	}
	effectiveTargets := targets
	if len(effectiveTargets) == 0 {
		effectiveTargets = cfg.Targets
	}
	if err := detectCollisions(cfg, b, effectiveTargets); err != nil {
		return err
	}
	if backup {
		adapters.SetBackup(true)
		defer adapters.SetBackup(false)
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
	if gitignoreOn {
		adapters.StartRecording()
	}
	if !dryRun {
		adapters.StartCounting()
	}
	for _, t := range effectiveTargets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "! %v\n", err)
			continue
		}
		verbosef("→ emit %s\n", t)
		if err := adapter.Emit(b, cfg, dryRun); err != nil {
			if gitignoreOn {
				adapters.StopRecording()
			}
			if !dryRun {
				adapters.StopCounting()
			}
			return fmt.Errorf("%s: %w", t, err)
		}
	}
	filesChanged := 0
	if !dryRun {
		filesChanged = adapters.StopCounting()
	}
	if gitignoreOn {
		if err := updateGitignore(cfg, normalizeAndSort(adapters.StopRecording())); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
		summaryf("→ updated .gitignore\n")
	}
	if !dryRun {
		if err := writeStateFile(root, filesChanged); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}
	return nil
}

// runSyncJSON runs a real sync pass and emits a JSON result describing each
// file written, updated, or skipped per target.
func runSyncJSON(cmd *cobra.Command, root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
	cfg, b, err := loadProject(root)
	if err != nil {
		return err
	}
	effectiveTargets := targets
	if len(effectiveTargets) == 0 {
		effectiveTargets = cfg.Targets
	}
	if err := detectCollisions(cfg, b, effectiveTargets); err != nil {
		return err
	}
	if backup {
		adapters.SetBackup(true)
		defer adapters.SetBackup(false)
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
	if gitignoreOn {
		adapters.StartRecording()
	}

	out := jsonOutput{Version: "1", Command: "sync"}
	for _, t := range effectiveTargets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		adapters.StartDetailedRecording()
		if err := adapter.Emit(b, cfg, dryRun); err != nil {
			adapters.StopDetailedRecording()
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		for _, f := range adapters.StopDetailedRecording() {
			rec := fileRecord{Target: t, Path: f.Path, Action: f.Action, Bytes: f.Bytes}
			if f.Action == "skip" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
	}

	if gitignoreOn {
		if err := updateGitignore(cfg, normalizeAndSort(adapters.StopRecording())); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
	}
	if !dryRun {
		if err := writeStateFile(root, len(out.Writes)); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}
	return emitJSON(cmd, out)
}

// printSyncCheckJSON emits a JSON result for `sync --check`. Files that need
// to be written appear in writes (action: "missing" or "stale"); files that
// are already in sync appear in skipped (action: "ok").
func printSyncCheckJSON(cmd *cobra.Command, reports []driftReport) error {
	out := jsonOutput{Version: "1", Command: "sync --check"}
	for _, r := range reports {
		for _, f := range r.Missing {
			out.Writes = append(out.Writes, fileRecord{
				Target: r.Target,
				Path:   f.Path,
				Action: "missing",
				Bytes:  len(f.Content),
			})
		}
		for _, f := range r.Stale {
			out.Writes = append(out.Writes, fileRecord{
				Target: r.Target,
				Path:   f.Path,
				Action: "stale",
				Bytes:  len(f.Content),
			})
		}
	}
	hasDrift := len(out.Writes) > 0
	if err := emitJSON(cmd, out); err != nil {
		return err
	}
	if hasDrift {
		return fmt.Errorf("drift detected")
	}
	return nil
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
