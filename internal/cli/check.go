package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// driftReport summarizes per-target drift between source specs and on-disk
// emitted artifacts.
type driftReport struct {
	Target  string
	Missing []string
	Stale   []string
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
					rep.Missing = append(rep.Missing, f.Path)
					continue
				}
				return nil, fmt.Errorf("read %s: %w", f.Path, err)
			}
			if string(disk) != f.Content {
				rep.Stale = append(rep.Stale, f.Path)
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
			fmt.Printf("✓ %s: in sync\n", r.Target)
			continue
		}
		any = true
		fmt.Printf("✗ %s: drift\n", r.Target)
		for _, p := range r.Missing {
			fmt.Printf("    missing: %s\n", p)
		}
		for _, p := range r.Stale {
			fmt.Printf("    stale:   %s\n", p)
		}
	}
	return any
}

func newDoctorCmd() *cobra.Command {
	var targets []string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose drift between specs and emitted target files.",
		Long: "doctor compares what `sync` would emit against the files on disk.\n" +
			"Reports missing outputs and hand-edits. Exits non-zero on any drift.\n" +
			"Use as a CI gate or after rebases.",
		RunE: func(cmd *cobra.Command, args []string) error {
			reports, err := collectDrift(targets)
			if err != nil {
				return err
			}
			if printDrift(reports) {
				return fmt.Errorf("drift detected. run `agnostic-ai sync` to reconcile")
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to check (default: all in config)")
	return cmd
}
