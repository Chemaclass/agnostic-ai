package cli

import (
	"fmt"
	"os"
	"path/filepath"

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
		RunE: func(cmd *cobra.Command, args []string) error {
			reports, err := collectDrift(targets)
			if err != nil {
				return err
			}
			drift := printDrift(reports)
			if !drift {
				return nil
			}
			if !fix {
				return fmt.Errorf("drift detected. run `agnostic-ai sync` to reconcile, or `agnostic-ai doctor --fix`")
			}
			fixed, err := fixDrift(reports, backup)
			if err != nil {
				return err
			}
			fmt.Printf("→ reconciled %d file(s)\n", fixed)
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to check (default: all in config)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Reconcile drift by writing missing/stale files")
	cmd.Flags().BoolVar(&backup, "backup", false, "With --fix, copy each existing file to <path>.bak before overwriting")
	return cmd
}

// fixDrift writes only the files reported as missing or stale, sourced
// from a fresh capture pass. Files in sync are left untouched. Returns
// the number of files written.
func fixDrift(reports []driftReport, backup bool) (int, error) {
	wanted := make(map[string]struct{})
	targetsToFix := make(map[string]struct{})
	for _, r := range reports {
		if !r.hasDrift() {
			continue
		}
		targetsToFix[r.Target] = struct{}{}
		for _, p := range r.Missing {
			wanted[p] = struct{}{}
		}
		for _, p := range r.Stale {
			wanted[p] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return 0, nil
	}

	cfg, b, err := loadProject(".")
	if err != nil {
		return 0, err
	}

	written := 0
	for t := range targetsToFix {
		adapter, ok := adapters.Get(t)
		if !ok {
			continue
		}
		adapters.StartCapture()
		if err := adapter.Emit(b, cfg, false); err != nil {
			adapters.StopCapture()
			return written, fmt.Errorf("%s: %w", t, err)
		}
		files := adapters.StopCapture()
		for _, f := range files {
			if _, want := wanted[f.Path]; !want {
				continue
			}
			if err := writeFixed(f.Path, f.Content, backup); err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
}

// writeFixed writes a single drift-fix file, honoring backup.
func writeFixed(path, content string, backup bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if backup {
		if existing, err := os.ReadFile(path); err == nil {
			if err := os.WriteFile(path+".bak", existing, 0o644); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
