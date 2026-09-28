package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// driftReport summarizes per-target drift between source specs and on-disk
// emitted artifacts. Missing, Stale, and Edited carry the full captured
// content so `--fix` can reconcile without a second adapter pass. Stale
// lists files that still hold what the last sync wrote, so only the
// specs changed; Edited lists files whose bytes differ from that record,
// a hand edit the next sync overwrites. Orphaned lists
// files a prior sync wrote, no longer emits, and could not remove; no
// write fixes them, so `--fix` leaves them to the user. Blocking lists
// the removals sync makes before writing a missing file, for a file that
// stands where the file's parent directory belongs (Cline's single-file
// `.clinerules`, #1064); `--fix` replays them first. Leftover lists
// files a prior sync wrote and no longer emits that the next full sync
// removes; until then the tool still loads them. Current lists the
// files already matching what sync writes, which only a dry run reports.
type driftReport struct {
	Target   string
	Current  []adapters.CapturedFile
	Missing  []adapters.CapturedFile
	Stale    []adapters.CapturedFile
	Edited   []adapters.CapturedFile
	Orphaned []string
	Leftover []string
	Blocking []adapters.CapturedRemoval
}

func (r driftReport) hasDrift() bool {
	return len(r.Missing) > 0 || len(r.Stale) > 0 || len(r.Edited) > 0 || len(r.Orphaned) > 0 || len(r.Leftover) > 0
}

// addChanged files f, whose bytes on disk differ from what sync would
// write, as edited when they also differ from the sum the last sync
// recorded, and as stale otherwise. No recorded sum proves no edit.
func (r *driftReport) addChanged(f adapters.CapturedFile, disk []byte, sums map[string]string) {
	if sum := sums[f.Path]; sum != "" && adapters.ContentSum(string(disk)) != sum {
		r.Edited = append(r.Edited, f)
		return
	}
	r.Stale = append(r.Stale, f)
}

// changed returns the files the next sync rewrites: stale, then edited.
func (r driftReport) changed() []adapters.CapturedFile {
	return append(append([]adapters.CapturedFile{}, r.Stale...), r.Edited...)
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
// allTargetsResolve reports whether every requested target, or every
// configured one when none was requested, resolves to an adapter.
func allTargetsResolve(requested, configured []string) bool {
	if len(requested) == 0 {
		requested = configured
	}
	for _, t := range requested {
		if _, err := adapters.Resolve(t); err != nil {
			return false
		}
	}
	return true
}

func collectDrift(targets []string) ([]driftReport, error) {
	return collectDriftWithEntryPointTargets(targets, nil)
}

// collectDriftWithEntryPointTargets keeps native adapter verification scoped
// to targets while allowing shared entry points to be rendered with their
// complete configured consumer set. A nil entryPointTargets slice preserves
// the normal check/doctor behavior by using targets for both concerns.
func collectDriftWithEntryPointTargets(targets, entryPointTargets []string) ([]driftReport, error) {
	reports := make([]driftReport, 0, len(targets)+1)
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	if entryPointTargets == nil {
		entryPointTargets = targets
	}
	if err := detectCollisions(cfg, b, targets); err != nil {
		return nil, err
	}
	sess := adapters.NewSession()
	sums := readStateFile(".").OutputSums
	emitted := map[string]bool{}
	resolvedAll := coversAllConfiguredTargets(targets, cfg.Targets)
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "! %v\n", err)
			resolvedAll = false
			continue
		}
		files, err := captureAdapterFiles(sess, adapter, b, cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range files {
			emitted[f.Path] = true
		}

		rep := driftReport{Target: t}
		for _, f := range files {
			disk, err := os.ReadFile(f.Path)
			if err != nil {
				if notOnDisk(err) {
					rep.Missing = append(rep.Missing, f)
					continue
				}
				return nil, fmt.Errorf("read %s: %w", f.Path, err)
			}
			if string(disk) != f.Content {
				rep.addChanged(f, disk, sums)
				continue
			}
			rep.Current = append(rep.Current, f)
		}
		rep.Blocking = blockingRemovals(sess.CapturedRemovals(), rep.Missing)
		reports = append(reports, rep)
	}
	epRep, err := collectEntryPointDrift(cfg, b, entryPointTargets)
	if err != nil {
		return nil, err
	}
	reports = append(reports, epRep)
	// Another target's files are not in emitted, so only a check that
	// covers every configured target can tell what sync stopped writing.
	// The ledger does not record which target wrote a file, so leftovers
	// get a report of their own.
	if resolvedAll {
		if leftover := leftoverOutputs(cfg, emitted); len(leftover) > 0 {
			reports = append(reports, driftReport{Target: ledgerReport, Leftover: leftover})
		}
	}
	return reports, nil
}

// ledgerReport names the drift report for files the last sync wrote and
// no longer emits.
const ledgerReport = "ledger"

// leftoverOutputs returns the files the last sync wrote, that are not in
// emitted or an entry point, and that the next full sync's orphan sweep
// removes: still on disk, not user-owned, and still carrying the
// provenance header or the bytes sync recorded. Kept orphans are
// reported apart (recordedOrphans), and links and directories never are.
//
// With no ledger (a deleted `.sync-state`, or a fresh checkout of a repo
// that commits its generated files), state.Outputs is empty and there is
// no recorded output list to check first. The scan falls back to every
// git-tracked file, trusting the provenance header alone since there is
// no recorded sum either (#1334). Outside a git work tree, or when git is
// missing or slow, trackedFiles reports not ok and the scan finds nothing,
// the same way it already behaves with a ledger and nothing left to sweep.
func leftoverOutputs(cfg *config.Config, emitted map[string]bool) []string {
	state := readStateFile(".")
	skip := map[string]bool{}
	for _, p := range entryPointPaths(cfg, cfg.Targets) {
		skip[p] = true
	}
	for _, p := range state.Orphans {
		skip[p] = true
	}
	candidates := state.Outputs
	if len(candidates) == 0 {
		tracked, ok := trackedFiles(".")
		if !ok {
			return nil
		}
		candidates = tracked
	}
	var out []string
	for _, p := range candidates {
		if emitted[p] || skip[p] || cfg.IsUnmanaged(p) || underSymlinkedDir(p) {
			continue
		}
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		sum := state.OutputSums[p]
		if header.Has(string(data)) || sum != "" && adapters.ContentSum(string(data)) == sum {
			out = append(out, p)
		}
	}
	return out
}

// notOnDisk reports whether a read failed because no file sits at the
// path. A regular file where a parent directory belongs counts: Cline's
// single-file `.clinerules` shadows `.clinerules/<name>.md` until sync
// replaces it (#1060).
func notOnDisk(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR)
}

// blockingRemovals keeps the removals whose path is a parent directory
// of a missing file. Other removals are not drift, so `--fix` never
// deletes a file it did not report.
func blockingRemovals(removals []adapters.CapturedRemoval, missing []adapters.CapturedFile) []adapters.CapturedRemoval {
	var out []adapters.CapturedRemoval
	for _, r := range removals {
		prefix := filepath.Clean(r.Path) + string(filepath.Separator)
		for _, f := range missing {
			if strings.HasPrefix(filepath.Clean(f.Path), prefix) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// captureAdapterFiles renders one target into memory without touching disk.
// Drift checks and verification fingerprints share this path so they always
// identify the same native output bytes.
func captureAdapterFiles(sess *adapters.Session, adapter adapters.Adapter, b spec.Bundle, cfg *config.Config) ([]adapters.CapturedFile, error) {
	sess.StartCapture()
	err := adapters.EmitWithProvenance(sess, adapter, b, cfg, false)
	files := sess.StopCapture()
	if err != nil {
		return nil, err
	}
	return files, nil
}

// collectEntryPointDrift checks whether AGNOSTIC_AI.md and every enabled
// target's native entry-point file (CLAUDE.md, AGENTS.md, etc.) match what
// sync would write. The body source is AGNOSTIC_AI.md when it exists;
// otherwise the template body is used.
func collectEntryPointDrift(cfg *config.Config, b spec.Bundle, targets []string) (driftReport, error) {
	rep := driftReport{Target: "agnostic-ai"}
	sums := readStateFile(".").OutputSums

	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	var body string
	if err == nil {
		body = header.Strip(string(data))
		rep.Current = append(rep.Current, adapters.CapturedFile{Path: adapters.AgnosticEntryPointPath, Content: string(data)})
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
		file := adapters.CapturedFile{Path: f.Path, Content: f.Content}
		if string(disk) != f.Content {
			rep.addChanged(file, disk, sums)
			continue
		}
		rep.Current = append(rep.Current, file)
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

// printDrift prints a per-target summary, one bucket per kind of drift:
//
//   - missing: generated file does not exist yet (next sync creates it)
//   - stale:   the specs changed since the last sync (next sync updates it)
//   - edited:  the file changed since the last sync wrote it (next sync
//     overwrites the hand edit)
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
				summaryf("      - %s\n", filepath.ToSlash(f.Path))
			}
		}
		if len(r.Stale) > 0 {
			summaryf("    %d file(s) out of date (run `agnostic-ai sync` to update):\n", len(r.Stale))
			for _, f := range r.Stale {
				summaryf("      - %s\n", filepath.ToSlash(f.Path))
			}
		}
		if len(r.Edited) > 0 {
			summaryf("    %d file(s) edited locally since last sync (sync will overwrite them; move the edits into .agnostic-ai/ first):\n", len(r.Edited))
			for _, f := range r.Edited {
				summaryf("      - %s\n", filepath.ToSlash(f.Path))
			}
		}
		if len(r.Orphaned) > 0 {
			summaryf("    %d orphaned file(s) no longer generated but edited since sync (delete them, or list them under sync.unmanaged):\n", len(r.Orphaned))
			for _, p := range r.Orphaned {
				summaryf("      - %s\n", filepath.ToSlash(p))
			}
		}
		if len(r.Leftover) > 0 {
			summaryf("    %d file(s) no longer generated and still loaded (run `agnostic-ai sync` to remove):\n", len(r.Leftover))
			for _, p := range r.Leftover {
				summaryf("      - %s\n", filepath.ToSlash(p))
			}
		}
	}
	return any
}

func newDoctorCmd() *cobra.Command {
	var targets []string
	var fix, backup, jsonOut, checkGlobs, checkRefs bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Unified diagnostic: config, CLIs, spec health, and drift.",
		Long: "doctor runs a prioritized punch list:\n" +
			"  1. Detect installed AI CLIs on PATH.\n" +
			"  2. Validate agnostic-ai.yaml config.\n" +
			"  3. Report unsupported spec kinds per target.\n" +
			"  4. Report agentic config on disk not single-sourced from .agnostic-ai/.\n" +
			"  5. Compare what sync would emit against files on disk (drift).\n" +
			"     --check-globs and --check-references add opt-in checks here.\n" +
			"  6. Check MCP server command binaries.\n" +
			"  7. Suggest a concrete next step.\n\n" +
			"Exits non-zero on any drift. Subcommands run individual checks.",
		Example: `  # Full diagnostic (CI gate)
  agnostic-ai doctor

  # Reconcile drift in place, keeping a .bak of each hand-edited file
  agnostic-ai doctor --fix --backup

  # Machine-readable drift report for CI dashboards
  agnostic-ai doctor --json

  # Fail when a generated skill links to a missing local file
  agnostic-ai doctor --check-references

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
				var refs []referenceFinding
				if checkRefs {
					if refs, _, err = collectReferenceFindings(targets); err != nil {
						return err
					}
				}
				return printDoctorJSON(cmd, reports, refs, checkRefs)
			}

			configOK := doctorConfigOK()

			// 1. Installed CLIs
			reportInstalledCLIs(cmd)

			// 2. Config check
			cmd.Println()
			cmd.Println("Config:")
			if !configOK {
				cmd.Println("  ✗ agnostic-ai.yaml not found. Run: agnostic-ai init")
				doctorNextStep(cmd, false, errDoctorNoConfig)
				return errDoctorNoConfig
			}
			cfg, _, err := loadProject(".")
			if err != nil {
				cmd.Printf("  ✗ %v\n", err)
				doctorNextStep(cmd, false, err)
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
			// A target that did not resolve was warned about and skipped,
			// so its files were never compared.
			if !hasDrift && allTargetsResolve(targets, cfg.Targets) {
				cmd.Println("  ✓ generated files in sync")
			}

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

			// 4c. Optional: relative links in emitted skill documents
			// whose destination is missing on disk.
			brokenRefs := 0
			if checkRefs {
				n, err := reportBrokenReferences(cmd, targets)
				if err != nil {
					return err
				}
				brokenRefs = n
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
			doctorNextStep(cmd, hasDrift, nil)

			// A rule whose globs match nothing never loads, so it
			// silently does not exist. Reported before, but exit 0 meant
			// no CI step could gate on it (#617).
			if unloadableRules > 0 {
				return fmt.Errorf("%d rule(s) have a glob matching no files and will never load", unloadableRules)
			}
			if brokenRefs > 0 {
				return fmt.Errorf("%d broken skill reference(s): a relative link points to a file missing from the emitted skill", brokenRefs)
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
	cmd.Flags().BoolVar(&checkRefs, "check-references", false, "Flag relative Markdown links in emitted skills whose file is missing on disk")
	registerTargetCompletion(cmd)
	cmd.AddCommand(newDoctorMCPCmd())
	cmd.AddCommand(newDoctorInstallCmd())
	cmd.AddCommand(newDoctorConfigCmd())
	return cmd
}

// doctorJSONOutput extends the shared JSON schema with the opt-in
// reference findings. The key is present only under --check-references,
// so consumers of the plain drift report see the same document as before.
type doctorJSONOutput struct {
	jsonOutput
	References *[]referenceFinding `json:"references,omitempty"`
}

// printDoctorJSON emits a JSON drift report for `doctor`. Mirrors the schema
// used by `sync --check --json`: missing, stale, and orphaned files appear
// in writes. With checkRefs, broken skill references appear in references.
func printDoctorJSON(cmd *cobra.Command, reports []driftReport, refs []referenceFinding, checkRefs bool) error {
	out := doctorJSONOutput{jsonOutput: jsonOutput{Version: "1", Command: "doctor", Writes: driftRecords(reports)}.withEmptyLists()}
	if checkRefs {
		if refs == nil {
			refs = []referenceFinding{}
		}
		out.References = &refs
	}
	if err := writeIndentedJSON(cmd, out); err != nil {
		return err
	}
	if len(out.Writes) > 0 {
		return fmt.Errorf("drift detected")
	}
	if len(refs) > 0 {
		return fmt.Errorf("%d broken skill reference(s)", len(refs))
	}
	return nil
}

// driftRecords flattens reports into the JSON write records shared by
// `sync --check --json` and `doctor --json`: one per missing, stale,
// edited, orphaned, or leftover file.
func driftRecords(reports []driftReport) []fileRecord {
	var records []fileRecord
	for _, r := range reports {
		for _, f := range r.Missing {
			records = append(records, fileRecord{Target: r.Target, Path: f.Path, Action: "missing", Bytes: len(f.Content)})
		}
		for _, f := range r.Stale {
			records = append(records, fileRecord{Target: r.Target, Path: f.Path, Action: "stale", Bytes: len(f.Content)})
		}
		for _, f := range r.Edited {
			records = append(records, fileRecord{Target: r.Target, Path: f.Path, Action: "edited", Bytes: len(f.Content)})
		}
		for _, p := range r.Orphaned {
			records = append(records, fileRecord{Target: r.Target, Path: p, Action: "orphan"})
		}
		for _, p := range r.Leftover {
			records = append(records, fileRecord{Target: r.Target, Path: p, Action: "leftover"})
		}
	}
	return records
}

// fixDrift writes the captured content for every missing, stale, or
// edited file in reports, after the removals that stand in their way, and
// removes the leftovers the next sync would sweep. Files in sync are left
// untouched. The sums of the files it writes go into the ledger, as a
// sync records them. Returns the number of files written or removed.
func fixDrift(reports []driftReport, backup bool) (int, error) {
	sess := adapters.NewSession()
	if backup {
		sess.SetBackup(true)
		defer sess.SetBackup(false)
	}
	fixed := map[string]string{}
	defer func() {
		if err := recordOutputSums(".", fixed); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}()
	written := 0
	for _, r := range reports {
		if !r.hasDrift() {
			continue
		}
		for _, rm := range r.Blocking {
			if _, err := sess.RemoveOwned(rm.Path, rm.Sum, false); err != nil {
				return written, err
			}
		}
		for _, f := range append(append([]adapters.CapturedFile{}, r.Missing...), r.changed()...) {
			if err := sess.WriteFile(f.Path, f.Content, false); err != nil {
				return written, err
			}
			fixed[f.Path] = adapters.ContentSum(f.Content)
			written++
		}
		if len(r.Leftover) == 0 {
			continue
		}
		// The same ownership guard as the orphan sweep in sync.
		sums := readStateFile(".").OutputSums
		pruned := map[string]bool{}
		for _, p := range r.Leftover {
			removed, err := sess.RemoveOwned(p, sums[p], false)
			if err != nil {
				return written, err
			}
			if removed {
				pruneAncestorDirs(p, pruned)
				written++
			}
		}
	}
	return written, nil
}
