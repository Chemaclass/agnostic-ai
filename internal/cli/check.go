package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// driftReport summarizes per-target drift between source specs and on-disk
// emitted artifacts. Missing and Stale carry the full captured content so
// `--fix` can reconcile without a second adapter pass. Orphaned lists
// files a prior sync wrote, no longer emits, and could not remove; no
// write fixes them, so `--fix` leaves them to the user.
type driftReport struct {
	Target   string
	Missing  []adapters.CapturedFile
	Stale    []adapters.CapturedFile
	Orphaned []string
}

func (r driftReport) hasDrift() bool {
	return len(r.Missing) > 0 || len(r.Stale) > 0 || len(r.Orphaned) > 0
}

// orphanedCount totals the orphaned files across reports.
func orphanedCount(reports []driftReport) int {
	n := 0
	for _, r := range reports {
		n += len(r.Orphaned)
	}
	return n
}

// collectDrift runs each target adapter in capture mode and compares each
// would-be file against disk. Also checks entry-point files (CLAUDE.md,
// AGENTS.md, AGNOSTIC_AI.md). No files are written.
func collectDrift(targets []string) ([]driftReport, error) {
	reports := make([]driftReport, 0, len(targets)+1)
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	if err := detectCollisions(cfg, b, targets); err != nil {
		return nil, err
	}
	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "! %v\n", err)
			continue
		}
		sess.StartCapture()
		if err := adapters.EmitWithProvenance(sess, adapter, b, cfg, false); err != nil {
			sess.StopCapture()
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		files := sess.StopCapture()

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
	epRep, err := collectEntryPointDrift(cfg, b, targets)
	if err != nil {
		return nil, err
	}
	reports = append(reports, epRep)
	return reports, nil
}

// collectEntryPointDrift checks whether AGNOSTIC_AI.md and every enabled
// target's native entry-point file (CLAUDE.md, AGENTS.md, etc.) match what
// sync would write. The body source is AGNOSTIC_AI.md when it exists;
// otherwise the template body is used.
func collectEntryPointDrift(cfg *config.Config, b spec.Bundle, targets []string) (driftReport, error) {
	rep := driftReport{Target: "agnostic-ai"}

	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	var body string
	if err == nil {
		body = header.Strip(string(data))
	} else if errors.Is(err, fs.ErrNotExist) {
		body = adapters.EntryPointBody(cfg)
		rendered := header.With(body, header.FormatMarkdown)
		rep.Missing = append(rep.Missing, adapters.CapturedFile{
			Path:    adapters.AgnosticEntryPointPath,
			Content: rendered,
		})
	} else {
		return rep, fmt.Errorf("%s: %w", adapters.AgnosticEntryPointPath, err)
	}

	files, err := renderEntryPointFiles(cfg, b, targets, body)
	if err != nil {
		return rep, err
	}
	for _, f := range files {
		if cfg.IsUnmanaged(f.Path) {
			continue // user-owned: never drift
		}
		disk, err := os.ReadFile(f.Path)
		if err != nil {
			if os.IsNotExist(err) {
				rep.Missing = append(rep.Missing, adapters.CapturedFile{Path: f.Path, Content: f.Content})
				continue
			}
			return rep, fmt.Errorf("read %s: %w", f.Path, err)
		}
		if string(disk) != f.Content {
			rep.Stale = append(rep.Stale, adapters.CapturedFile{Path: f.Path, Content: f.Content})
		}
	}
	rep.Orphaned = recordedOrphans(cfg)
	return rep, nil
}

// recordedOrphans returns the orphans the last sync kept (see
// syncStateFile.Orphans) that are still on disk and not user-owned. The
// state file is the only record of them: once the source spec is gone,
// no adapter render mentions the path again (#785).
func recordedOrphans(cfg *config.Config) []string {
	var out []string
	for _, p := range readStateFile(".").Orphans {
		if cfg.IsUnmanaged(p) {
			continue
		}
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

// printDrift prints a per-target summary. Splits drift into two named
// buckets so users can tell apart:
//
//   - missing: generated file does not exist yet (next sync creates it)
//   - stale:   generated file on disk differs from what sync would emit
//     (almost always a local hand-edit; next sync clobbers it)
//
// Returns true if any drift exists.
func printDrift(reports []driftReport) bool {
	any := false
	for _, r := range reports {
		if !r.hasDrift() {
			verbosef("%s %s: in sync\n", tick(), r.Target)
			continue
		}
		any = true
		summaryf("%s %s: drift\n", cross(), r.Target)
		if len(r.Missing) > 0 {
			summaryf("    %d file(s) missing (run `agnostic-ai sync` to create):\n", len(r.Missing))
			for _, f := range r.Missing {
				summaryf("      - %s\n", f.Path)
			}
		}
		if len(r.Stale) > 0 {
			summaryf("    %d file(s) edited locally since last sync (sync will overwrite — move edits into .agnostic-ai/ first):\n", len(r.Stale))
			for _, f := range r.Stale {
				summaryf("      - %s\n", f.Path)
			}
		}
		if len(r.Orphaned) > 0 {
			summaryf("    %d orphaned file(s) no longer generated but edited since sync (delete them, or list them under sync.unmanaged):\n", len(r.Orphaned))
			for _, p := range r.Orphaned {
				summaryf("      - %s\n", p)
			}
		}
	}
	return any
}

func newDoctorCmd() *cobra.Command {
	var targets []string
	var fix, backup, jsonOut, checkGlobs bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Unified diagnostic: config, CLIs, spec health, and drift.",
		Long: "doctor runs a prioritized punch list:\n" +
			"  1. Detect installed AI CLIs on PATH.\n" +
			"  2. Validate agnostic-ai.yaml config.\n" +
			"  3. Report unsupported spec kinds per target.\n" +
			"  4. Report agentic config on disk not single-sourced from .agnostic-ai/.\n" +
			"  5. Compare what sync would emit against files on disk (drift).\n" +
			"  6. Check MCP server command binaries.\n" +
			"  7. Suggest a concrete next step.\n\n" +
			"Exits non-zero on any drift. Subcommands run individual checks.",
		Example: `  # Full diagnostic (CI gate)
  agnostic-ai doctor

  # Reconcile drift in place, keeping a .bak of each hand-edited file
  agnostic-ai doctor --fix --backup

  # Machine-readable drift report for CI dashboards
  agnostic-ai doctor --json

  # Check only MCP command resolution
  agnostic-ai doctor mcp

  # Check only installed AI CLIs
  agnostic-ai doctor install`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// JSON mode: emit only the drift report, skip human-readable sections.
			if jsonOut {
				reports, err := collectDrift(targets)
				if err != nil {
					return err
				}
				return printDoctorJSON(cmd, reports)
			}

			configOK := doctorConfigOK()

			// 1. Installed CLIs
			reportInstalledCLIs(cmd)

			// 2. Config check
			cmd.Println()
			cmd.Println("Config:")
			if !configOK {
				cmd.Println("  ✗ agnostic-ai.yaml not found. Run: agnostic-ai init")
				doctorNextStep(cmd, false, false)
				return fmt.Errorf("no config found")
			}
			cfg, _, err := loadProject(".")
			if err != nil {
				cmd.Printf("  ✗ %v\n", err)
				doctorNextStep(cmd, false, false)
				return err
			}
			cmd.Printf("  ✓ agnostic-ai.yaml valid (version %d, %d target(s))\n", cfg.Version, len(cfg.Targets))

			// 3. Unsupported kinds
			reportUnsupportedKinds(cmd, cfg)

			// 3b. Config present on disk but not single-sourced, then
			// the paths the user owns through sync.unmanaged.
			reportUnmanagedConfig(cmd, ".", cfg)
			reportUserOwned(cmd, cfg)

			// 4. Drift
			cmd.Println()
			cmd.Println("Sync drift:")
			reports, err := collectDrift(targets)
			if err != nil {
				return err
			}
			hasDrift := printDrift(reports)

			// 4b. Optional: globs that match nothing in the working tree.
			unloadableRules := 0
			if checkGlobs {
				cmd.Println()
				cmd.Println("Glob coverage:")
				n, err := reportUnmatchedGlobs(cmd, ".")
				if err != nil {
					return err
				}
				unloadableRules = n
			}

			// 5. MCP resolution
			reportMCPCommandResolution(cmd)

			// 5b. Hook script body divergence across per-tool stashes.
			scriptDrift, err := reportDivergentHookScripts(cmd, ".")
			if err != nil {
				return err
			}
			hasDrift = hasDrift || scriptDrift

			// 6. Next step
			doctorNextStep(cmd, hasDrift, true)

			// A rule whose globs match nothing never loads, so it
			// silently does not exist. Reported before, but exit 0 meant
			// no CI step could gate on it (#617).
			if unloadableRules > 0 {
				return fmt.Errorf("%d rule(s) have a glob matching no files and will never load", unloadableRules)
			}
			if hasDrift {
				if !fix {
					return fmt.Errorf("drift detected. run `agnostic-ai sync` to reconcile, or `agnostic-ai doctor --fix`")
				}
				fixed, err := fixDrift(reports, backup)
				if err != nil {
					return err
				}
				summaryf("→ reconciled %d file(s)\n", fixed)
				if n := orphanedCount(reports); n > 0 {
					return fmt.Errorf("%d orphaned file(s) need manual removal", n)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to check (default: all in config)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Reconcile drift by writing missing/stale files")
	cmd.Flags().BoolVar(&backup, "backup", false, "With --fix, copy each existing file to <path>.bak before overwriting")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&checkGlobs, "check-globs", false, "Flag rules whose `globs:` pattern matches no files in the working tree")
	registerTargetCompletion(cmd)
	cmd.AddCommand(newDoctorMCPCmd())
	cmd.AddCommand(newDoctorInstallCmd())
	cmd.AddCommand(newDoctorConfigCmd())
	return cmd
}

// printDoctorJSON emits a JSON drift report for `doctor`. Mirrors the schema
// used by `sync --check --json`: missing, stale, and orphaned files appear
// in writes.
func printDoctorJSON(cmd *cobra.Command, reports []driftReport) error {
	out := jsonOutput{Version: "1", Command: "doctor", Writes: driftRecords(reports)}
	hasDrift := len(out.Writes) > 0
	if err := emitJSON(cmd, out); err != nil {
		return err
	}
	if hasDrift {
		return fmt.Errorf("drift detected")
	}
	return nil
}

// driftRecords flattens reports into the JSON write records shared by
// `sync --check --json` and `doctor --json`: one per missing, stale, or
// orphaned file.
func driftRecords(reports []driftReport) []fileRecord {
	var records []fileRecord
	for _, r := range reports {
		for _, f := range r.Missing {
			records = append(records, fileRecord{Target: r.Target, Path: f.Path, Action: "missing", Bytes: len(f.Content)})
		}
		for _, f := range r.Stale {
			records = append(records, fileRecord{Target: r.Target, Path: f.Path, Action: "stale", Bytes: len(f.Content)})
		}
		for _, p := range r.Orphaned {
			records = append(records, fileRecord{Target: r.Target, Path: p, Action: "orphan"})
		}
	}
	return records
}

// fixDrift writes the captured content for every missing or stale file in
// reports. Files in sync are left untouched. Returns the number of files
// written.
func fixDrift(reports []driftReport, backup bool) (int, error) {
	sess := adapters.NewSession()
	if backup {
		sess.SetBackup(true)
		defer sess.SetBackup(false)
	}
	written := 0
	for _, r := range reports {
		if !r.hasDrift() {
			continue
		}
		for _, f := range append(append([]adapters.CapturedFile{}, r.Missing...), r.Stale...) {
			if err := sess.WriteFile(f.Path, f.Content, false); err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
}
