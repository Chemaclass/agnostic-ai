package cli

import (
	"bytes"
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
	// lineEndings marks the changed files that differ from what sync
	// writes only by CRLF line endings. Sync cannot settle them while Git
	// converts the files on checkout.
	lineEndings    map[string]bool
	releasedMerged map[string]bool
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
	if onlyLineEndingsDiffer(disk, f.Content) {
		if r.lineEndings == nil {
			r.lineEndings = map[string]bool{}
		}
		r.lineEndings[f.Path] = true
		r.Stale = append(r.Stale, f)
		return
	}
	if sum := sums[f.Path]; sum != "" && !adapters.MatchesSum(disk, sum) {
		r.Edited = append(r.Edited, f)
		return
	}
	r.Stale = append(r.Stale, f)
}

// onlyLineEndingsDiffer reports whether disk is want with CRLF line
// endings, as Git writes a file under core.autocrlf or an eol rule.
func onlyLineEndingsDiffer(disk []byte, want string) bool {
	return bytes.Contains(disk, []byte("\r\n")) && !strings.Contains(want, "\r") &&
		string(bytes.ReplaceAll(disk, []byte("\r\n"), []byte("\n"))) == want
}

// lineEndingAdvice is the fix for files that differ only by line endings
// in the state ref names, "" for the working tree. There, running sync
// alone does not hold: the next checkout converts the files again. The
// --against export keeps the stored line endings, so there the stored
// file or an eol rule holds the CRLF.
func lineEndingAdvice(ref string) string {
	if ref == "" {
		return "Git checked them out with CRLF line endings; run git config core.autocrlf false, or add a .gitattributes rule such as `* text eol=lf`, then run agnostic-ai sync"
	}
	_, step := againstPlace(ref)
	return "Git stores them with CRLF line endings, or an eol rule in .gitattributes converts them; drop any such rule, then " + step
}

// lineEndingCount counts the changed files in reports that differ only
// by line endings, and names the state they were compared in.
func lineEndingCount(reports []driftReport) (n int, ref string) {
	for _, r := range reports {
		for _, f := range r.changed() {
			if r.lineEndings[f.Path] {
				n++
				ref = r.against
			}
		}
	}
	return n, ref
}

// splitMerged separates the files that sync merges into from the ones it
// writes whole.
func splitMerged(files []adapters.CapturedFile) (plain, merged []adapters.CapturedFile) {
	for _, f := range files {
		if f.Merged {
			merged = append(merged, f)
		} else {
			plain = append(plain, f)
		}
	}
	return plain, merged
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

// driftQuiet silences the warnings collectDrift prints itself.
var driftQuiet bool

// driftWarnf prints a collectDrift warning to the current stderr.
func driftWarnf(format string, args ...any) {
	if !driftQuiet {
		fmt.Fprintf(os.Stderr, format, args...)
	}
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
	return collectDriftWithLedger(targets, nil, "", readLedger("."))
}

// collectDriftWithLedger loads the project and collects its drift against
// the state file the caller read. It keeps native adapter verification
// scoped to targets while allowing shared entry points to be rendered with
// their complete configured consumer set. A nil entryPointTargets slice
// preserves the normal check/doctor behavior by using targets for both
// concerns.
func collectDriftWithLedger(targets, entryPointTargets []string, gitignoreFlag string, ledger ledgerSnapshot) ([]driftReport, error) {
	cfg, b, err := loadProject(".")
	if err != nil {
		return nil, err
	}
	return collectLoadedDriftWith(ledger, cfg, b, targets, entryPointTargets, gitignoreFlag)
}

// collectLoadedDrift is collectDriftWithLedger for a project the
// caller already loaded. It leaves cfg unchanged.
func collectLoadedDrift(loaded *config.Config, b spec.Bundle, targets []string, gitignoreFlag string) ([]driftReport, error) {
	return collectLoadedDriftWith(readLedger("."), loaded, b, targets, nil, gitignoreFlag)
}

func collectLoadedDriftWith(ledger ledgerSnapshot, loaded *config.Config, b spec.Bundle, targets, entryPointTargets []string, gitignoreFlag string) ([]driftReport, error) {
	reports := make([]driftReport, 0, len(targets))
	view := *loaded
	cfg := &view
	cfg.Gitignore.Enabled = resolveGitignore(cfg, gitignoreFlag)
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	if entryPointTargets == nil {
		entryPointTargets = targets
	}
	if err := stopOnSpecTypos(b, append(slices.Clone(cfg.Targets), targets...)); err != nil {
		return nil, err
	}
	state := ledger.state
	release := holdCaptureState(state)
	defer release()
	readerCfg := cfg.WithAdditionalTargets(append(append([]string{}, targets...), entryPointTargets...)...)
	renders := newTargetRenders(readerCfg, b)
	found, err := findRenderedCollisions(renders, readerCfg, targets, ".", state.Backups)
	if err != nil {
		return nil, err
	}
	winners := found.winners
	sess := adapters.NewSession()
	sess.SetCollisionWinners(winners)
	runFiles := found.runFiles
	if !slices.Equal(entryPointTargets, targets) {
		runFiles, _ = renderEntryPointFiles(readerCfg, b, entryPointTargets, "")
	}
	replaced := replacedRulesFiles(found.shared, runFiles)
	sums := state.OutputSums
	emitted := map[string]bool{}
	outputSums := map[string]string{}
	resolvedAll := coversAllConfiguredTargets(targets, cfg.Targets)
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			driftWarnf("! %v\n", err)
			resolvedAll = false
			continue
		}
		render := renders.capture(adapter, readerCfg)
		if len(winners) > 0 {
			// A winner replaces a contested file's bytes as the adapter
			// writes it, and an adapter can read back what it wrote, so
			// only a render under the winners shows what sync writes.
			render.files, render.err = captureAdapterFiles(sess, adapter, b, readerCfg)
			render.removals = sess.CapturedRemovals()
		}
		if render.err != nil {
			return nil, fmt.Errorf("%s: %w", t, render.err)
		}
		files := slices.DeleteFunc(slices.Clone(render.files), func(f adapters.CapturedFile) bool {
			if replaced[t] != filepath.Clean(f.Path) || !sameFileOnDisk(f.Path, replaced[t]) {
				return false
			}
			emitted[f.Path] = true
			return true
		})
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
		rep.Blocking = blockingRemovals(render.removals, rep.Missing)
		reports = append(reports, rep)
	}
	epRep, err := collectEntryPointDriftIn(readerCfg, b, entryPointTargets, state)
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
	generated, unloaded, renderErr := orphanGeneratedPaths(cfg, renders, reports)
	for _, target := range unloaded {
		// A requested target that failed to resolve was reported above.
		if !slices.Contains(targets, target) {
			driftWarnf("! could not load %s to check orphans; orphans it may still generate stay listed\n", target)
		}
	}
	if renderErr != nil {
		driftWarnf("! could not render entry points to check orphans, so orphans they may still generate stay listed: %v\n", renderErr)
	}
	for i := range reports {
		reports[i].Orphaned = slices.DeleteFunc(reports[i].Orphaned, func(path string) bool {
			return slices.ContainsFunc(generated, func(generatedPath string) bool { return samePath(generatedPath, path) })
		})
	}
	release()
	// Another target's files are not in emitted, so only a check that
	// covers every configured target can tell what sync stopped writing.
	// The ledger does not record which target wrote a file, so leftovers
	// get a report of their own.
	if resolvedAll {
		for _, rep := range leftoverReportsIn(cfg, emitted, ledger) {
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
	return leftoverReportsIn(cfg, emitted, readLedger("."))
}

func leftoverReportsIn(cfg *config.Config, emitted map[string]bool, ledger ledgerSnapshot) []driftReport {
	state := ledger.state
	stranded := strandedOutput(cfg, emitted, state)
	missing := ledger.missing()
	unledgered := unledgeredReport(cfg, emitted, state, missing, stranded)
	if missing {
		return []driftReport{unledgered}
	}
	return []driftReport{ledgerLeftovers(state, stranded), unledgered}
}

// ledgerLeftovers is the ledger report: each stranded output the last
// sync recorded that the orphan sweep can prove it wrote.
func ledgerLeftovers(state syncStateFile, stranded func(string) bool) driftReport {
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
		if merged || header.Has(string(data)) || sum != "" && adapters.MatchesSum(data, sum) {
			rep.Leftover = append(rep.Leftover, p)
		}
	}
	return rep
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
	err := emitAdapter(sess, adapter, b, cfg, false)
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
	return collectEntryPointDriftIn(cfg, b, targets, readStateFile("."))
}

func collectEntryPointDriftIn(cfg *config.Config, b spec.Bundle, targets []string, state syncStateFile) (driftReport, error) {
	rep := driftReport{Target: "agnostic-ai"}
	sums := state.OutputSums

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
	// An orphan proven generated again is a leftover the sweep removes.
	rep.Orphaned = slices.DeleteFunc(recordedOrphans(cfg, state.Orphans), func(path string) bool {
		return provenGenerated(path, state.OutputSums[path]) || slices.ContainsFunc(generated, func(generatedPath string) bool { return samePath(generatedPath, path) })
	})
	return rep, nil
}

// recordedOrphans returns the orphans the last sync kept (see
// syncStateFile.Orphans) that are still on disk and not user-owned. The
// state file is the only record of them: once the source spec is gone,
// no adapter render mentions the path again (#785).
func recordedOrphans(cfg *config.Config, orphans []string) []string {
	var out []string
	for _, p := range orphans {
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
func orphanGeneratedPaths(cfg *config.Config, renders *targetRenders, reports []driftReport) (generated, unloaded []string, renderErr error) {
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
	for _, target := range remaining {
		adapter, err := adapters.Resolve(target)
		if err != nil {
			unloaded = append(unloaded, target)
			continue
		}
		render := renders.captureQuiet(adapter, cfg)
		if render.err != nil {
			unloaded = append(unloaded, target)
			continue
		}
		for _, file := range render.files {
			generated = append(generated, file.Path)
		}
	}
	entryPoints, err := collectEntryPointDrift(cfg, renders.b, cfg.Targets)
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
				if r.releasedMerged[f.Path] {
					summaryf("      - %s (sync removes unchanged managed keys and keeps your keys and edits)\n", r.label(f.Path))
				} else {
					summaryf("      - %s\n", r.label(f.Path))
				}
			}
		}
		plain, merged := splitMerged(r.Edited)
		if len(plain) > 0 {
			summaryf("    %d file(s) edited locally since last sync (sync saves each as <path>.bak, then writes the spec version; move the edits into .agnostic-ai/):\n", len(plain))
			for _, f := range plain {
				summaryf("      - %s\n", r.label(f.Path))
			}
		}
		if len(merged) > 0 {
			summaryf("    %d merged file(s) edited since last sync (sync keeps the keys it did not write and resets the ones it writes, with no .bak; move those edits into .agnostic-ai/):\n", len(merged))
			for _, f := range merged {
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

// sameFileOnDisk reports whether a and b name one file. A `..` after a
// linked folder leads elsewhere, so lexical cleaning cannot tell.
func sameFileOnDisk(a, b string) bool {
	if a == b {
		return true
	}
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}
