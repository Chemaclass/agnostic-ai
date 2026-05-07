package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// driftReport summarizes per-target drift between source specs and on-disk
// emitted artifacts. Missing and Stale carry the full captured content so
// `--fix` can reconcile without a second adapter pass.
type driftReport struct {
	Target  string
	Missing []adapters.CapturedFile
	Stale   []adapters.CapturedFile
}

func (r driftReport) hasDrift() bool {
	return len(r.Missing) > 0 || len(r.Stale) > 0
}

// collectDrift runs each target adapter in capture mode and compares each
// would-be file against disk. No files are written.
func collectDrift(targets []string) ([]driftReport, error) {
	reports := make([]driftReport, 0, len(targets))
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, err
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
		adapters.StartCapture()
		if err := adapter.Emit(b, cfg, false); err != nil {
			adapters.StopCapture()
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		files := adapters.StopCapture()

		rep := driftReport{Target: t}
		for _, f := range files {
			disk, err := os.ReadFile(f.Path)
			if err != nil {
				if os.IsNotExist(err) {
					rep.Missing = append(rep.Missing, f)
					continue
				}
				return nil, fmt.Errorf("read %s: %w", f.Path, err)
			}
			if string(disk) != f.Content {
				rep.Stale = append(rep.Stale, f)
			}
		}
		reports = append(reports, rep)
	}
	return reports, nil
}

// printDrift prints a per-target summary. Returns true if any drift exists.
func printDrift(reports []driftReport) bool {
	any := false
	for _, r := range reports {
		if !r.hasDrift() {
			verbosef("✓ %s: in sync\n", r.Target)
			continue
		}
		any = true
		summaryf("✗ %s: drift\n", r.Target)
		for _, f := range r.Missing {
			verbosef("    missing: %s\n", f.Path)
		}
		for _, f := range r.Stale {
			verbosef("    stale:   %s\n", f.Path)
		}
	}
	return any
}

func newDoctorCmd() *cobra.Command {
	var targets []string
	var fix, backup bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose drift between specs and emitted target files.",
		Long: "doctor compares what `sync` would emit against the files on disk.\n" +
			"Reports missing outputs and hand-edits. Exits non-zero on any drift.\n" +
			"Use as a CI gate or after rebases.\n\n" +
			"With --fix, doctor reconciles drift by writing the missing or stale\n" +
			"files. Pair with --backup to copy each existing file to <path>.bak\n" +
			"before overwriting hand-edits.",
		Example: `  # Diagnose drift (CI gate)
  agnostic-ai doctor

  # Reconcile drift in place, keeping a .bak of each hand-edited file
  agnostic-ai doctor --fix --backup`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reports, err := collectDrift(targets)
			if err != nil {
				return err
			}
			if !printDrift(reports) {
				return nil
			}
			if !fix {
				return fmt.Errorf("drift detected. run `agnostic-ai sync` to reconcile, or `agnostic-ai doctor --fix`")
			}
			fixed, err := fixDrift(reports, backup)
			if err != nil {
				return err
			}
			summaryf("→ reconciled %d file(s)\n", fixed)
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to check (default: all in config)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Reconcile drift by writing missing/stale files")
	cmd.Flags().BoolVar(&backup, "backup", false, "With --fix, copy each existing file to <path>.bak before overwriting")
	registerTargetCompletion(cmd)
	return cmd
}

// fixDrift writes the captured content for every missing or stale file in
// reports. Files in sync are left untouched. Returns the number of files
// written.
func fixDrift(reports []driftReport, backup bool) (int, error) {
	if backup {
		adapters.SetBackup(true)
		defer adapters.SetBackup(false)
	}
	written := 0
	for _, r := range reports {
		if !r.hasDrift() {
			continue
		}
		for _, f := range append(append([]adapters.CapturedFile{}, r.Missing...), r.Stale...) {
			if err := adapters.WriteFile(f.Path, f.Content, false); err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
}
