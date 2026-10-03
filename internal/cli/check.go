package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
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
// write fixes them, so `doctor --fix` offers their removal. Blocking lists
// the removals sync makes before writing a missing file, for a file that
// stands where the file's parent directory belongs (Cline's single-file
// `.clinerules`, #1064); `--fix` replays them first. Leftover lists
// files a prior sync wrote and no longer emits that the next full sync
// removes; until then the tool still loads them. Current lists the
// files already matching what sync writes, which only a dry run reports.
// Unledgered marks the report of leftovers no ledger proves sync wrote
// (unledgeredReport): sync keeps them, only `--fix` removes Leftover, and
// Orphaned holds the scope documents left to the user.
type driftReport struct {
	Target     string
	Current    []adapters.CapturedFile
	Missing    []adapters.CapturedFile
	Stale      []adapters.CapturedFile
	Edited     []adapters.CapturedFile
	Orphaned   []string
	Leftover   []string
	Blocking   []adapters.CapturedRemoval
	Unledgered bool
	// Unmanaged lists hand-written config that Git tracks inside a folder
	// the managed block ignores, found by `--against`. Only the tool that
	// reads that folder sees it, so it must move under `.agnostic-ai/`.
	Unmanaged []unmanagedFinding
	// proven holds the content sum of each headerless leftover the output
	// manifest or the last commit's render proves sync wrote, the proof
	// doctor --fix removes it with.
	proven map[string]string
	// against is the Git state `--against` compared, index or HEAD, or ""
	// for the working tree.
	against string
	// sharedWith maps a path to the other targets that read it, set by
	// foldSharedDrift for printing.
	sharedWith map[string][]string
}

// leftoverFix names the command that removes the report's Leftover. Sync
// keeps a leftover no ledger proves it wrote.
func (r driftReport) leftoverFix() string {
	if r.Unledgered {
		return "doctor --fix"
	}
	return "sync"
}

func (r driftReport) hasDrift() bool {
	return len(r.Missing) > 0 || len(r.Stale) > 0 || len(r.Edited) > 0 || len(r.Orphaned) > 0 || len(r.Leftover) > 0 || len(r.Unmanaged) > 0
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

// manualOnlyDrift returns the scope documents that are all the drift there
// is, and whether that holds. Neither sync nor doctor --fix removes a
// document no ledger proves sync wrote, so advice to run them cannot settle it.
func manualOnlyDrift(reports []driftReport) ([]string, bool) {
	var files []string
	for _, r := range reports {
		if !r.hasDrift() {
			continue
		}
		if !r.Unledgered || len(r.Missing)+len(r.Stale)+len(r.Edited)+len(r.Leftover) > 0 {
			return nil, false
		}
		files = append(files, r.Orphaned...)
	}
	return files, len(files) > 0
}

// manualRemovalAdvice names the unledgered scope documents and what to do
// with them, in the wording `sync --check` uses. It follows "delete".
func manualRemovalAdvice(files []string) string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.ToSlash(f)
	}
	pronoun := "them"
	if len(names) == 1 {
		pronoun = "it"
	}
	return strings.Join(names, ", ") + " by hand if stale, or list " + pronoun + " under sync.unmanaged"
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
	if err := stopOnSpecTypos(b, append(slices.Clone(cfg.Targets), targets...)); err != nil {
		return nil, err
	}
	readerCfg := cfg.WithAdditionalTargets(append(append([]string{}, targets...), entryPointTargets...)...)
	if err := detectCollisions(readerCfg, b, targets); err != nil {
		return nil, err
	}
	sess := adapters.NewSession()
	sums := readStateFile(".").OutputSums
	emitted := map[string]bool{}
	outputSums := map[string]string{}
	resolvedAll := coversAllConfiguredTargets(targets, cfg.Targets)
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "! %v\n", err)
			resolvedAll = false
			continue
		}
		files, err := captureAdapterFiles(sess, adapter, b, readerCfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range files {
			emitted[f.Path] = true
			outputSums[f.Path] = adapters.ContentSum(f.Content)
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
	epRep, err := collectEntryPointDrift(readerCfg, b, entryPointTargets)
	if err != nil {
		return nil, err
	}
	if resolvedAll && cfg.Sync.OutputManifest {
		for _, list := range [][]adapters.CapturedFile{epRep.Current, epRep.Missing, epRep.Stale, epRep.Edited} {
			for _, f := range list {
				outputSums[f.Path] = adapters.ContentSum(f.Content)
			}
		}
		if err := checkOutputManifest(cfg, &epRep, outputSums); err != nil {
			return nil, err
		}
		emitted[outputManifestPath] = true
	}
	reports = append(reports, epRep)
	generated, unloaded, renderErr := orphanGeneratedPaths(cfg, b, reports)
	for _, target := range unloaded {
		// A requested target that failed to resolve was reported above.
		if !slices.Contains(targets, target) {
			fmt.Fprintf(os.Stderr, "! could not load %s to check orphans; orphans it may still generate stay listed\n", target)
		}
	}
	if renderErr != nil {
		fmt.Fprintf(os.Stderr, "! could not render entry points to check orphans, so orphans they may still generate stay listed: %v\n", renderErr)
	}
	for i := range reports {
		reports[i].Orphaned = slices.DeleteFunc(reports[i].Orphaned, func(path string) bool {
			return slices.ContainsFunc(generated, func(generatedPath string) bool { return samePath(generatedPath, path) })
		})
	}
	// Another target's files are not in emitted, so only a check that
	// covers every configured target can tell what sync stopped writing.
	// The ledger does not record which target wrote a file, so leftovers
	// get a report of their own.
	if resolvedAll {
		for _, rep := range leftoverReports(cfg, emitted) {
			if rep.hasDrift() {
				reports = append(reports, rep)
			}
		}
	}
	return reports, nil
}

// ledgerReport names the drift report for files the last sync wrote and
// no longer emits.
const ledgerReport = "ledger"

// unledgeredReportTarget names the drift report for leftovers no ledger
// proves sync wrote, which sync keeps (unledgeredReport).
const unledgeredReportTarget = "unledgered"

// leftoverReports lists the files sync no longer emits that are still
// on disk. The ledger report holds the files the last sync wrote that
// the next full sync's orphan sweep removes: not user-owned, and still
// carrying the provenance header or the bytes sync recorded. The
// unledgered report holds the ones no ledger proves sync wrote, which
// sync keeps (unledgeredReport). With no `.sync-state` only the second
// applies. Kept orphans are reported apart (recordedOrphans), and links
// and directories never are.
func leftoverReports(cfg *config.Config, emitted map[string]bool) []driftReport {
	state := readStateFile(".")
	stranded := strandedOutput(cfg, emitted, state)
	unledgered := unledgeredReport(cfg, emitted, state, stranded)
	if ledgerMissing(".") {
		return []driftReport{unledgered}
	}
	rep := driftReport{Target: ledgerReport}
	for _, p := range state.Outputs {
		if !stranded(p) {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		sum := state.OutputSums[p]
		_, merged := state.Merged[p]
		if merged || header.Has(string(data)) || sum != "" && adapters.ContentSum(string(data)) == sum {
			rep.Leftover = append(rep.Leftover, p)
		}
	}
	return []driftReport{rep, unledgered}
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
		body = adapters.EntryPointBody()
		rep.Missing = append(rep.Missing, adapters.CapturedFile{
			Path:    adapters.AgnosticEntryPointPath,
			Content: body,
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
	generated := driftGeneratedPaths([]driftReport{rep})
	rep.Orphaned = slices.DeleteFunc(recordedOrphans(cfg), func(path string) bool {
		return slices.ContainsFunc(generated, func(generatedPath string) bool { return samePath(generatedPath, path) })
	})
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

// driftGeneratedPaths returns every path the current specs produce
// across every report: what a sync would write, changed or not. Missing,
// Stale, Edited, and Current all count; Leftover and Orphaned do not,
// since those are no longer produced.
func driftGeneratedPaths(reports []driftReport) []string {
	var out []string
	for _, r := range reports {
		for _, f := range r.Current {
			out = append(out, f.Path)
		}
		for _, f := range r.Missing {
			out = append(out, f.Path)
		}
		for _, f := range r.Stale {
			out = append(out, f.Path)
		}
		for _, f := range r.Edited {
			out = append(out, f.Path)
		}
	}
	return out
}

// A partial check cannot classify a ledger orphan until every configured producer is captured.
// A producer that does not resolve or fails to capture is named in unloaded and skipped without a warning.
// renderErr says why the entry points of every configured target failed to render, which leaves their paths out of generated.
func orphanGeneratedPaths(cfg *config.Config, b spec.Bundle, reports []driftReport) (generated, unloaded []string, renderErr error) {
	generated = driftGeneratedPaths(reports)
	if cfg.Sync.OutputManifest {
		generated = append(generated, outputManifestPath)
	}
	if orphanedCount(reports) == 0 {
		return generated, nil, nil
	}
	var checked []string
	for _, report := range reports {
		checked = append(checked, report.Target)
	}
	var remaining []string
	for _, target := range cfg.Targets {
		if !slices.Contains(checked, target) {
			remaining = append(remaining, target)
		}
	}
	if len(remaining) == 0 {
		return generated, nil, nil
	}
	// Rendering unselected targets only finds their paths; their drops
	// must not reach the selected targets' warnings and notes.
	defer adapters.SetAsideNotes()()
	sess := adapters.NewSession()
	for _, target := range remaining {
		adapter, err := adapters.Resolve(target)
		if err != nil {
			unloaded = append(unloaded, target)
			continue
		}
		files, err := captureAdapterFiles(sess, adapter, b, cfg)
		if err != nil {
			unloaded = append(unloaded, target)
			continue
		}
		for _, file := range files {
			generated = append(generated, file.Path)
		}
	}
	entryPoints, err := collectEntryPointDrift(cfg, b, cfg.Targets)
	if err != nil {
		return generated, unloaded, err
	}
	return append(generated, driftGeneratedPaths([]driftReport{entryPoints})...), unloaded, nil
}

// reportTrackedIgnored lists generated paths git both tracks and
// ignores: a file the repo committed before it moved into the managed
// .gitignore block, so the ignore has no effect (#1330). Informational
// like the "files to commit" hint in `sync`'s summary: it never fails
// doctor. `sync --untrack` removes the paths from the index.
func reportTrackedIgnored(cmd *cobra.Command, reports []driftReport) {
	paths := gitTrackedAndIgnored(".", driftGeneratedPaths(reports))
	if len(paths) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Tracked despite ignored:")
	cmd.Printf("  ! %d file(s) committed before they moved into the managed .gitignore block:\n", len(paths))
	cmd.Printf("      git rm --cached %s\n", strings.Join(paths, " "))
	cmd.Println("    Or run: agnostic-ai sync --untrack")
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
	for _, r := range foldSharedDrift(reports) {
		if !r.hasDrift() {
			verbosef("%s %s: in sync\n", tick(), r.Target)
			continue
		}
		any = true
		summaryf("%s %s: drift\n", cross(), r.Target)
		if r.against != "" {
			where, step := againstPlace(r.against)
			if n := len(r.Missing) + len(r.Stale); n > 0 {
				summaryf("    %d file(s) %s do not match the specs there (%s):\n", n, where, step)
				for _, f := range append(append([]adapters.CapturedFile(nil), r.Missing...), r.Stale...) {
					summaryf("      - %s\n", r.label(f.Path))
				}
			}
		} else if len(r.Missing) > 0 {
			summaryf("    %d file(s) missing (run `agnostic-ai sync` to create):\n", len(r.Missing))
			for _, f := range r.Missing {
				summaryf("      - %s\n", r.label(f.Path))
			}
		}
		if len(r.Stale) > 0 && r.against == "" {
			summaryf("    %d file(s) out of date (run `agnostic-ai sync` to update):\n", len(r.Stale))
			for _, f := range r.Stale {
				summaryf("      - %s\n", r.label(f.Path))
			}
		}
		if len(r.Edited) > 0 {
			summaryf("    %d file(s) edited locally since last sync (sync saves each as <path>.bak, then writes the spec version; move the edits into .agnostic-ai/):\n", len(r.Edited))
			for _, f := range r.Edited {
				summaryf("      - %s\n", r.label(f.Path))
			}
		}
		if len(r.Orphaned) > 0 && r.Unledgered {
			summaryf("    %d file(s) that look generated, with no ledger to prove sync wrote them (delete them by hand if stale, or list them under sync.unmanaged):\n", len(r.Orphaned))
			for _, p := range r.Orphaned {
				summaryf("      - %s\n", r.label(p))
			}
		} else if len(r.Orphaned) > 0 {
			summaryf("    %d orphaned file(s) no longer generated whose ownership could not be proven (run `agnostic-ai doctor --fix` to choose removal, or list them under sync.unmanaged):\n", len(r.Orphaned))
			for _, p := range r.Orphaned {
				summaryf("      - %s\n", r.label(p))
			}
		}
		if len(r.Leftover) > 0 {
			summaryf("    %d file(s) no longer generated and still loaded (run `agnostic-ai %s` to remove):\n", len(r.Leftover), r.leftoverFix())
			for _, p := range r.Leftover {
				summaryf("      - %s\n", r.label(p))
			}
		}
		if len(r.Unmanaged) > 0 {
			summaryf("    %d hand-written file(s) tracked in a generated folder, read by one tool only (adopt into .agnostic-ai/, then untrack):\n", len(r.Unmanaged))
			for _, f := range r.Unmanaged {
				summaryf("      - %s  (agnostic-ai import %s)\n", r.label(f.Path), f.Target)
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
			"  3. Report unsupported spec kinds per target, then spec health: the\n" +
			"     findings `agnostic-ai lint` reports.\n" +
			"  4. Report agentic config on disk not single-sourced from .agnostic-ai/,\n" +
			"     and project skills or agents that share a name with a global one.\n" +
			"  5. Compare what sync would emit against files on disk (drift), and flag\n" +
			"     a generated file still tracked despite being ignored.\n" +
			"     --check-globs and --check-references add opt-in checks here.\n" +
			"  6. Check MCP server command binaries.\n" +
			"  7. Check persisted Codex hook trust.\n" +
			"  8. Check existing packaging ignore files against generated paths.\n" +
			"  9. Suggest a concrete next step.\n\n" +
			"Exits non-zero on any drift, lint error, or inactive Codex hook; lint warnings show without\n" +
			"failing. Subcommands run individual checks.",
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
				scope, err := loadCheckScope(false)
				if err != nil {
					return err
				}
				lint, accepted, err := lintScopeReport(scope)
				if err != nil {
					return err
				}
				return printDoctorJSON(cmd, reports, refs, checkRefs, lint, accepted, collectCodexHookTrust(scope.cfg, targets), collectPackagingIgnoreFindings(reports))
			}

			configOK := doctorConfigOK()

			// 1. Installed CLIs
			reportInstalledCLIs(cmd)

			// 2. Config check
			cmd.Println()
			cmd.Println("Config:")
			if !configOK {
				cmd.Println("  ✗ agnostic-ai.yaml not found. Run: agnostic-ai init")
				doctorNextStep(cmd, false, false, false, nil, 0, 0, 0, errDoctorNoConfig)
				return errDoctorNoConfig
			}
			scope, err := loadCheckScope(false)
			if err != nil {
				cmd.Printf("  ✗ %v\n", err)
				doctorNextStep(cmd, false, false, false, nil, 0, 0, 0, err)
				return err
			}
			cfg := scope.cfg
			cmd.Printf("  ✓ agnostic-ai.yaml valid (version %d, %d target(s))\n", cfg.Version, len(cfg.Targets))

			// 3. Unsupported kinds, then what `lint` reports.
			reportUnsupportedKinds(cmd, cfg)
			lint, err := reportSpecHealth(cmd, scope)
			if err != nil {
				return err
			}

			// 3b. Config present on disk but not single-sourced, then
			// the paths the user owns through sync.unmanaged.
			unmanaged := reportUnmanagedConfig(cmd, ".", cfg)
			reportUserOwned(cmd, cfg)
			reportLegacyDefaultInstructions(cmd)
			reportGlobalNameClashes(cmd, scope.bundle, cfg.Targets)

			// 4. Drift
			cmd.Println()
			cmd.Println("Sync drift:")
			// A copy beside a scoped AGENTS.md stops the drift check, so
			// --fix removes it before that check runs.
			_, bundle, err := loadProject(".")
			if err != nil {
				return err
			}
			copies := nestedClaudeCopies(cfg, bundle)
			reportNestedClaudeCopies(cmd, copies)
			removedCopies := 0
			if fix {
				if removedCopies, err = removeNestedClaudeCopies(copies, backup); err != nil {
					return err
				}
			}
			reports, err := collectDrift(targets)
			if err != nil {
				return err
			}
			hasDrift := printDrift(reports)
			copiesOnly := !hasDrift && len(copies) > 0
			// A target that did not resolve was warned about and skipped,
			// so its files were never compared.
			if !hasDrift && len(copies) == 0 && allTargetsResolve(targets, cfg.Targets) {
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

			// 4d. Generated files git tracks despite ignoring.
			reportTrackedIgnored(cmd, reports)

			// 5. MCP resolution
			reportMCPCommandResolution(cmd)

			// 5b. Hook script body divergence across per-tool stashes.
			scriptDrift, err := reportDivergentHookScripts(cmd, ".")
			if err != nil {
				return err
			}
			copiesOnly = copiesOnly && !scriptDrift
			hasDrift = hasDrift || scriptDrift || len(copies) > 0

			packaging := collectPackagingIgnoreFindings(reports)
			reportPackagingIgnoreFindings(cmd, packaging)
			hookTrust := collectCodexHookTrust(cfg, targets)
			reportCodexHookTrust(cmd, hookTrust)

			// 6. Next step
			manualFiles, manualOnly := manualOnlyDrift(reports)
			// Hook script divergence is drift a scope document does not explain.
			manualOnly = manualOnly && !scriptDrift && len(copies) == 0
			doctorNextStep(cmd, hasDrift, manualOnly, copiesOnly, manualFiles, len(lint), len(hookTrust), unmanaged, nil, len(packaging))

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
					if manualOnly {
						return fmt.Errorf("drift detected. delete %s", manualRemovalAdvice(manualFiles))
					}
					if copiesOnly {
						return fmt.Errorf("drift detected. run `agnostic-ai doctor --fix`")
					}
					return fmt.Errorf("drift detected. run `agnostic-ai sync` to reconcile, or `agnostic-ai doctor --fix`")
				}
				fixed, err := fixDrift(reports, backup)
				if err != nil {
					return err
				}
				removedOrphans, err := offerOrphanRemoval(cfg, reports, backup, orphanRemovalPrompt(cmd))
				if err != nil {
					return err
				}
				summaryf("→ reconciled %d file(s)\n", fixed+removedCopies+removedOrphans)
				hookTrust = collectCodexHookTrust(cfg, targets)
				reportCodexHookTrust(cmd, hookTrust)
				if n := orphanedCount(reports); n > 0 {
					return fmt.Errorf("%d orphaned file(s) need manual removal", n)
				}
			}
			if err := codexHookTrustErr(hookTrust); err != nil {
				return err
			}
			return lintErrorsErr(lint)
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to check (default: all in config)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Reconcile drift and offer removal of kept orphans in a terminal")
	cmd.Flags().BoolVar(&backup, "backup", false, "With --fix, copy each existing file to <path>.bak before overwriting or confirmed orphan removal")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&checkGlobs, "check-globs", false, "Flag rules whose `globs:` pattern matches no files in the working tree")
	cmd.Flags().BoolVar(&checkRefs, "check-references", false, "Flag relative Markdown links in emitted skills whose file is missing on disk")
	registerTargetCompletion(cmd)
	cmd.AddCommand(newDoctorMCPCmd())
	cmd.AddCommand(newDoctorInstallCmd())
	cmd.AddCommand(newDoctorConfigCmd())
	return cmd
}

// doctorJSONOutput adds lint, hook trust, packaging warnings, and opt-in references.
type doctorJSONOutput struct {
	jsonOutput
	Lint            []lintFinding            `json:"lint"`
	References      *[]referenceFinding      `json:"references,omitempty"`
	HookTrust       []codex.HookTrustFinding `json:"hook_trust"`
	PackagingIgnore []packagingIgnoreFinding `json:"packaging_ignore"`
	// CoverageAccepted counts the coverage notes coverage.accept matches.
	CoverageAccepted int `json:"coverage_accepted"`
}

// printDoctorJSON emits a JSON drift report for `doctor`. Mirrors the schema
// used by `sync --check --json`: missing, stale, and orphaned files appear
// in writes. Lint, hook trust, and packaging findings have their own lists. With checkRefs, broken skill
// references appear in references.
func printDoctorJSON(cmd *cobra.Command, reports []driftReport, refs []referenceFinding, checkRefs bool, lint []lintFinding, coverageAccepted int, hookTrust []codex.HookTrustFinding, packaging []packagingIgnoreFinding) error {
	if lint == nil {
		lint = []lintFinding{}
	}
	out := doctorJSONOutput{jsonOutput: jsonOutput{Version: "1", Command: "doctor", Writes: driftRecords(reports)}.forOutput(), Lint: slashLintPaths(lint), HookTrust: slashHookTrustPaths(hookTrust), PackagingIgnore: slashPackagingPaths(packaging), CoverageAccepted: coverageAccepted}
	if out.HookTrust == nil {
		out.HookTrust = []codex.HookTrustFinding{}
	}
	if out.PackagingIgnore == nil {
		out.PackagingIgnore = []packagingIgnoreFinding{}
	}
	if checkRefs {
		if refs == nil {
			refs = []referenceFinding{}
		}
		slashed := slashReferencePaths(refs)
		out.References = &slashed
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
	if err := codexHookTrustErr(hookTrust); err != nil {
		return err
	}
	return lintErrorsErr(lint)
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
		for _, f := range r.Unmanaged {
			records = append(records, fileRecord{Target: f.Target, Path: f.Path, Action: "unmanaged"})
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
	releasedMerged := map[string]*mergedOutput{}
	fixedMerged := map[string]mergedOutput{}
	defer func() {
		if err := errors.Join(recordOutputSums(".", fixed), recordMergedFixes(".", fixedMerged, fixed), recordMergedRelease(".", releasedMerged)); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}()
	written := 0
	for i := range reports {
		r := reports[i]
		if !r.hasDrift() {
			continue
		}
		for _, rm := range r.Blocking {
			if _, err := sess.RemoveOwned(rm.Path, rm.Sum, false); err != nil {
				return written, err
			}
		}
		for _, f := range append(append([]adapters.CapturedFile{}, r.Missing...), r.changed()...) {
			created := !fileExists(f.Path)
			if err := sess.WriteFile(f.Path, f.Content, false); err != nil {
				return written, err
			}
			fixed[f.Path] = adapters.ContentSum(f.Content)
			if f.Merged {
				fixedMerged[f.Path] = mergedOutput{Keys: f.Keys, Released: f.Released, Created: created, wroteMerge: true}
			} else {
				fixedMerged[f.Path] = mergedOutput{wroteWhole: true}
			}
			written++
		}
		if len(r.Leftover) == 0 {
			continue
		}
		// The same ownership guard as the orphan sweep in sync. With no
		// ledger, a mention of the marker is no proof: RemoveOwned would
		// take one, so the header must still open the file.
		state := readStateFile(".")
		sums := state.OutputSums
		unledgered := r.Unledgered || ledgerMissing(".")
		pruned := map[string]bool{}
		for _, p := range r.Leftover {
			m, merged := state.Merged[p]
			// A merged file sync cannot fully release stays, and the run
			// reports it, so doctor does not pass while check still fails.
			if !unledgered && (m.Unrecorded || !merged && unrecordedMergedCandidate(state, p)) {
				reports[i].Orphaned = append(reports[i].Orphaned, p)
				continue
			}
			if merged && !unledgered {
				result, edited, err := sess.ReleaseMerged(p, m.Keys, m.Created, false, false)
				if err != nil {
					return written, err
				}
				switch result {
				case adapters.MergedRemoved:
					pruneAncestorDirs(p, pruned)
					written++
					releasedMerged[p] = nil
				case adapters.MergedStripped:
					written++
					releasedMerged[p] = nil
				case adapters.MergedUnchanged:
					releasedMerged[p] = nil
				case adapters.MergedEdited:
					written++
					releasedMerged[p] = &mergedOutput{Keys: edited, Created: m.Created}
					reports[i].Orphaned = append(reports[i].Orphaned, p)
				case adapters.MergedKept:
					reports[i].Orphaned = append(reports[i].Orphaned, p)
				}
				continue
			}
			sum := sums[p]
			if unledgered && !ownedWithoutLedger(p) {
				if sum = r.proven[p]; sum == "" {
					continue
				}
			}
			removed, err := sess.RemoveOwned(p, sum, false)
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
