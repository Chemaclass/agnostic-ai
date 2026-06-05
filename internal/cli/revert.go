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
	var dryRun, jsonOut, force bool

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Undo a previous sync. Restores .bak files when present.",
		Long: "revert reverses the effect of `sync` for every target.\n" +
			"For each file the matching adapter would emit, plus the entry-point\n" +
			"files sync distributes (CLAUDE.md, AGENTS.md, GEMINI.md, ...):\n" +
			"  - if <path>.bak exists, the .bak content is restored and the .bak removed\n" +
			"  - otherwise the file is left in place to protect user-authored content\n" +
			"    that happens to share a path with an adapter-emitted file (e.g. a\n" +
			"    helper script next to SKILL.md). Pass --force to delete unbacked files.\n" +
			"Pair with `sync --backup` so the .bak trail exists for the files you want\n" +
			"restored to their pre-sync content.",
		Example: `  # Preview what would be reverted
  agnostic-ai revert --dry-run

  # Restore .bak files where they exist; leave unbacked files alone
  agnostic-ai revert

  # Also remove files that have no .bak (old default behavior)
  agnostic-ai revert --force

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
				return runRevertJSON(cmd, effective, dryRun, force)
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
				if err := adapters.EmitWithProvenance(adapter, b, cfg, true); err != nil {
					adapters.StopCapture()
					return fmt.Errorf("%s: %w", t, err)
				}
				files := adapters.StopCapture()
				summaryf("← revert %s\n", t)
				var restored, removed, preserved int
				for _, f := range files {
					action, err := revertOne(f.Path, dryRun, force)
					if err != nil {
						return fmt.Errorf("%s: %w", t, err)
					}
					switch action {
					case "restore":
						restored++
					case "remove":
						removed++
					case "preserve":
						preserved++
					}
				}
				if preserved > 0 {
					summaryf("    %d file(s) preserved (no .bak; pass --force to delete)\n", preserved)
				}
				_ = restored
				_ = removed
			}

			// sync writes the entry-point files (CLAUDE.md, AGENTS.md, ...)
			// outside adapter emission, so revert them on the same terms.
			paths := entryPointPaths(cfg, effective)
			summaryf("← revert entry-points\n")
			var preserved int
			for _, p := range paths {
				action, err := revertOne(p, dryRun, force)
				if err != nil {
					return fmt.Errorf("entry-point: %w", err)
				}
				if action == "preserve" {
					preserved++
				}
			}
			if preserved > 0 {
				summaryf("    %d file(s) preserved (no .bak; pass --force to delete)\n", preserved)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to revert (default: all in config)")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Revert only these targets (comma-separated); mutually exclusive with --except")
	cmd.Flags().StringSliceVar(&except, "except", nil, "Revert all configured targets except these (comma-separated); mutually exclusive with --only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report intended actions without touching disk")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&force, "force", false, "Also delete adapter-emitted files that lack a .bak (use with care: removes user-authored files that share a path with adapter output)")
	registerTargetCompletion(cmd)
	return cmd
}

// revertOne restores path from path+".bak" when the backup exists. When
// there is no .bak the default is to leave path alone ("preserve") so a
// user-authored file that happens to share a path with adapter output
// (e.g. a helper script propagated into a skill folder) is not silently
// deleted. Pass force=true to delete unbacked files (old behavior).
// Missing source files are reported as "skip".
//
// Returns the action taken: "restore", "remove", "preserve", or "skip".
func revertOne(path string, dryRun, force bool) (string, error) {
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

	if !force {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			return "skip", nil
		}
		verbosef("    preserve: %s (no .bak; pass --force to delete)\n", path)
		return "preserve", nil
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
// file restored, removed, preserved, or skipped per target.
func runRevertJSON(cmd *cobra.Command, targets []string, dryRun, force bool) error {
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
		if err := adapters.EmitWithProvenance(adapter, b, cfg, true); err != nil {
			adapters.StopCapture()
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		files := adapters.StopCapture()
		for _, f := range files {
			action, err := revertOne(f.Path, dryRun, force)
			if err != nil {
				out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
				continue
			}
			rec := fileRecord{Target: t, Path: f.Path, Action: action, Bytes: 0}
			if action == "skip" || action == "preserve" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
	}

	// Entry-point files are written outside adapter emission; revert them
	// too so the JSON report matches the text path.
	for _, p := range entryPointPaths(cfg, targets) {
		action, err := revertOne(p, dryRun, force)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
			continue
		}
		rec := fileRecord{Target: "agnostic-ai", Path: p, Action: action, Bytes: 0}
		if action == "skip" || action == "preserve" {
			out.Skipped = append(out.Skipped, rec)
		} else {
			out.Writes = append(out.Writes, rec)
		}
	}
	return emitJSON(cmd, out)
}
