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
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "! %v\n", err)
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
	epRep, err := collectEntryPointDrift(cfg, targets)
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
func collectEntryPointDrift(cfg *config.Config, targets []string) (driftReport, error) {
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

	rendered := header.With(body, header.FormatMarkdown)
	seen := map[string]bool{adapters.AgnosticEntryPointPath: true}
	for _, t := range targets {
		if adapters.HasLegacyRulesFile(cfg, t) {
			continue
		}
		path := adapters.EntryPointPath(cfg, t)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		disk, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				rep.Missing = append(rep.Missing, adapters.CapturedFile{Path: path, Content: rendered})
				continue
			}
			return rep, fmt.Errorf("read %s: %w", path, err)
		}
		if string(disk) != rendered {
			rep.Stale = append(rep.Stale, adapters.CapturedFile{Path: path, Content: rendered})
		}
	}
	return rep, nil
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
	var fix, backup, jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Unified diagnostic: config, CLIs, spec health, and drift.",
		Long: "doctor runs a prioritized punch list:\n" +
			"  1. Detect installed AI CLIs on PATH.\n" +
			"  2. Validate agnostic-ai.yaml config.\n" +
			"  3. Report unsupported spec kinds per target.\n" +
			"  4. Compare what sync would emit against files on disk (drift).\n" +
			"  5. Check MCP server command binaries.\n" +
			"  6. Suggest a concrete next step.\n\n" +
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

			// 4. Drift
			cmd.Println()
			cmd.Println("Sync drift:")
			reports, err := collectDrift(targets)
			if err != nil {
				return err
			}
			hasDrift := printDrift(reports)

			// 5. MCP resolution
			reportMCPCommandResolution(cmd)

			// 6. Next step
			doctorNextStep(cmd, hasDrift, true)

			if hasDrift {
				if !fix {
					return fmt.Errorf("drift detected. run `agnostic-ai sync` to reconcile, or `agnostic-ai doctor --fix`")
				}
				fixed, err := fixDrift(reports, backup)
				if err != nil {
					return err
				}
				summaryf("→ reconciled %d file(s)\n", fixed)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to check (default: all in config)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Reconcile drift by writing missing/stale files")
	cmd.Flags().BoolVar(&backup, "backup", false, "With --fix, copy each existing file to <path>.bak before overwriting")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	registerTargetCompletion(cmd)
	cmd.AddCommand(newDoctorMCPCmd())
	cmd.AddCommand(newDoctorInstallCmd())
	cmd.AddCommand(newDoctorConfigCmd())
	return cmd
}

// printDoctorJSON emits a JSON drift report for `doctor`. Mirrors the schema
// used by `sync --check --json`: missing/stale files appear in writes,
// up-to-date files appear in skipped.
func printDoctorJSON(cmd *cobra.Command, reports []driftReport) error {
	out := jsonOutput{Version: "1", Command: "doctor"}
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
