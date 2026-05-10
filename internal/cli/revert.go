package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func newRevertCmd() *cobra.Command {
	var targets, only, except []string
	var dryRun, jsonOut bool

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Undo a previous sync. Restores .bak files when present.",
		Long: "revert reverses the effect of `sync` for every target.\n" +
			"For each file the matching adapter would emit:\n" +
			"  - if <path>.bak exists, the .bak content is restored and the .bak removed\n" +
			"  - otherwise the file is removed (it did not exist before sync)\n" +
			"Pair with `sync --backup` so the .bak trail exists. Without backups,\n" +
			"revert simply deletes the generated files.",
		Example: `  # Preview what would be reverted
  agnostic-ai revert --dry-run

  # Restore .bak files where they exist; remove generated files otherwise
  agnostic-ai revert

  # Revert only Claude and Cursor
  agnostic-ai revert --only claude,cursor

  # Revert everything except Codex
  agnostic-ai revert --except codex

  # Machine-readable output for CI dashboards and editor extensions
  agnostic-ai revert --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(only) > 0 && len(except) > 0 {
				return fmt.Errorf("--only and --except are mutually exclusive")
			}
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			base := targets
			if len(base) == 0 {
				base = cfg.Targets
			}
			effective, err := filterTargets(base, only, except)
			if err != nil {
				return err
			}

			if jsonOut {
				return runRevertJSON(cmd, effective, dryRun)
			}

			for _, t := range effective {
				adapter, err := adapters.Resolve(t)
				if err != nil {
					fmt.Fprintf(os.Stderr, "! %v\n", err)
					continue
				}
				adapters.StartCapture()
				// Pass dryRun=true so any adapter that writes outside
				// emit.WriteFile (e.g. a future os.Mkdir for an empty
				// output dir) does not side-effect during revert.
				if err := adapter.Emit(b, cfg, true); err != nil {
					adapters.StopCapture()
					return fmt.Errorf("%s: %w", t, err)
				}
				files := adapters.StopCapture()
				summaryf("← revert %s\n", t)
				for _, f := range files {
					if _, err := revertOne(f.Path, dryRun); err != nil {
						return fmt.Errorf("%s: %w", t, err)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to revert (default: all in config)")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Revert only these targets (comma-separated); mutually exclusive with --except")
	cmd.Flags().StringSliceVar(&except, "except", nil, "Revert all configured targets except these (comma-separated); mutually exclusive with --only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report intended actions without touching disk")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	registerTargetCompletion(cmd)
	return cmd
}

// revertOne restores path from path+".bak" if the backup exists, else
// removes path. Missing files at either location are ignored.
// Returns the action taken: "restore", "remove", or "skip" (file absent).
func revertOne(path string, dryRun bool) (string, error) {
	bak := path + ".bak"
	data, err := os.ReadFile(bak)
	switch {
	case err == nil:
		if dryRun {
			verbosef("    restore: %s ← %s\n", path, bak)
			return "restore", nil
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", fmt.Errorf("restore %s: %w", path, err)
		}
		if err := os.Remove(bak); err != nil {
			return "", fmt.Errorf("remove %s: %w", bak, err)
		}
		verbosef("    restored: %s\n", path)
		return "restore", nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("read backup %s: %w", bak, err)
	}

	if dryRun {
		verbosef("    remove:  %s\n", path)
		return "remove", nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "skip", nil
		}
		return "", fmt.Errorf("remove %s: %w", path, err)
	}
	verbosef("    removed:  %s\n", path)
	return "remove", nil
}

// runRevertJSON performs the revert and emits a JSON result describing each
// file restored, removed, or skipped per target.
func runRevertJSON(cmd *cobra.Command, targets []string, dryRun bool) error {
	cfg, b, err := loadProject(".")
	if err != nil {
		return err
	}

	out := jsonOutput{Version: "1", Command: "revert"}
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		adapters.StartCapture()
		if err := adapter.Emit(b, cfg, true); err != nil {
			adapters.StopCapture()
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		files := adapters.StopCapture()
		for _, f := range files {
			action, err := revertOne(f.Path, dryRun)
			if err != nil {
				out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
				continue
			}
			rec := fileRecord{Target: t, Path: f.Path, Action: action, Bytes: 0}
			if action == "skip" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
	}
	return emitJSON(cmd, out)
}
