package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// recordLedgerWrites appends every create/update/skip path from writes
// into session in the order they were emitted, and records each path's
// content sum in written (an empty sum still marks the path as written
// this run). A hand edit --keep-edits left in place stays with the sum
// the last sync recorded, so it still reads as edited. Delete actions
// are skipped because the file no longer belongs to this sync's output
// set. Callers feed session and written to
// writeStateFile at end-of-sync so the next run knows the full prior
// output footprint and can prove ownership of header-less files.
func recordLedgerWrites(writes []adapters.WrittenFile, session *[]string, written map[string]string) {
	for _, w := range writes {
		switch w.Action {
		case "create", "update", "skip", "edited":
			*session = append(*session, w.Path)
			written[w.Path] = w.Sum
		}
	}
}

// mergedOutput is what the ledger keeps about a JSON file sync merges
// into: the key paths it set there, and whether sync created the file.
type mergedOutput struct {
	Keys    [][]string `json:"keys,omitempty"`
	Created bool       `json:"created,omitempty"`
}

// recordMergedWrites adds the merged JSON writes in writes to merged.
// Targets that write one shared file each add the keys they set.
func recordMergedWrites(writes []adapters.WrittenFile, merged map[string]mergedOutput) {
	for _, w := range writes {
		if !w.Merged {
			continue
		}
		switch w.Action {
		case "create", "update", "skip", "edited":
			m := merged[w.Path]
			m.Keys = mergedKeyPaths(append(m.Keys, w.Keys...))
			m.Created = m.Created || w.Action == "create"
			merged[w.Path] = m
		}
	}
}

func mergedKeyPaths(keys [][]string) [][]string {
	slices.SortFunc(keys, slices.Compare[[]string])
	return slices.CompactFunc(keys, slices.Equal[[]string])
}

// ledgerMerged returns the merged-file records to store for ledger. A
// file merged this run takes this run's keys and stays created if an
// earlier sync created it. One not written this run keeps its prior
// record.
func ledgerMerged(ledger []string, merged map[string]mergedOutput, written map[string]string, prior map[string]mergedOutput) map[string]mergedOutput {
	out := map[string]mergedOutput{}
	for _, p := range ledger {
		m, ok := merged[p]
		if ok {
			m.Created = m.Created || prior[p].Created
		} else if _, rewritten := written[p]; !rewritten {
			m, ok = prior[p]
		}
		if ok {
			out[p] = m
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ledgerSums returns the content sums to store for ledger. A path written
// this run takes its fresh sum. A path not written this run (another
// target's on a partial sync, or a kept orphan) carries its prior sum so
// ownership proof is not lost.
func ledgerSums(ledger []string, written, prior map[string]string) map[string]string {
	out := map[string]string{}
	for _, p := range ledger {
		sum, ok := written[p]
		if !ok {
			sum = prior[p]
		}
		if sum != "" {
			out[p] = sum
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ledgerOrphans returns the orphan list to store. A full run records what
// the sweep kept. A partial run sweeps nothing, so it carries the prior
// orphans forward, minus any path this run wrote again.
func ledgerOrphans(kept, prior []string, written map[string]string, coversAll bool) []string {
	if coversAll {
		return kept
	}
	var out []string
	for _, p := range prior {
		if _, ok := written[p]; !ok {
			out = append(out, p)
		}
	}
	return out
}

// finalizeLedger returns the deterministic, deduplicated path set used
// as the new sync ledger. Callers pass the in-order session list
// collected via recordLedgerWrites.
func finalizeLedger(session []string) []string {
	seen := make(map[string]struct{}, len(session))
	out := make([]string, 0, len(session))
	for _, p := range session {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// coversAllConfiguredTargets reports whether emitted includes every
// target in configured. A sync that emits only a subset (via --only or
// --except) must not sweep the un-emitted targets' files as orphans.
func coversAllConfiguredTargets(emitted, configured []string) bool {
	set := make(map[string]struct{}, len(emitted))
	for _, t := range emitted {
		set[t] = struct{}{}
	}
	for _, t := range configured {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}

// reconcilePartialLedger guards the orphan sweep against partial syncs.
// When a run emits only a subset of the configured targets, the files
// owned by the un-emitted targets are absent from the current ledger and
// sweepLedgerOrphans would delete them. Folding the prior ledger into the
// current set keeps those files on disk and turns the sweep into a no-op
// for paths this run did not own. A later full sync (one that covers
// every configured target) reconciles any genuine orphans.
//
// On a full run the ledger is returned unchanged so removed targets and
// deleted specs are still swept.
func reconcilePartialLedger(ledger, priorOutputs []string, coversAll bool) []string {
	if coversAll {
		return ledger
	}
	merged := make([]string, 0, len(ledger)+len(priorOutputs))
	merged = append(merged, ledger...)
	merged = append(merged, priorOutputs...)
	return finalizeLedger(merged)
}

// sweepAndFinalizeLedger builds this sync's ledger from the recorded
// writes, sweeps prior outputs the run no longer emits, and returns the
// footprint to persist. Paths the sweep kept stay in the ledger with
// their prior sums so the next sync retries them and check can report
// them. Shared by the text and JSON sync paths.
//
// A dry run records no writes, so the current output set is unknown and
// every prior output would look orphaned. It sweeps nothing and returns
// an empty ledger, which callers never persist on a dry run.
func sweepAndFinalizeLedger(sess *adapters.Session, prev syncStateFile, session []string, written map[string]string, merged map[string]mergedOutput, emitted, configured []string, dryRun bool) (ledger syncLedger, kept, removed, stripped []string, err error) {
	if dryRun {
		return syncLedger{}, nil, nil, nil, nil
	}
	coversAll := coversAllConfiguredTargets(emitted, configured)
	outputs := reconcilePartialLedger(finalizeLedger(session), prev.Outputs, coversAll)
	released, removed, stripped, kept, err := releaseMergedOrphans(sess, prev, outputs)
	if err == nil {
		var sweptRemoved, sweptKept []string
		sweptRemoved, sweptKept, err = sweepLedgerOrphans(sess, removeMatching(prev.Outputs, released), prev.OutputSums, outputs)
		removed = append(removed, sweptRemoved...)
		kept = append(kept, sweptKept...)
	}
	orphans := ledgerOrphans(kept, prev.Orphans, written, coversAll)
	if err != nil {
		for _, path := range prev.Outputs {
			if !slices.Contains(released, path) && !slices.Contains(removed, path) && !sess.IsUnmanaged(path) {
				outputs = append(outputs, path)
			}
		}
		for _, path := range ledgerOrphans(nil, prev.Orphans, written, false) {
			if !slices.Contains(removed, path) && !sess.IsUnmanaged(path) {
				orphans = append(orphans, path)
			}
		}
	}
	outputs = finalizeLedger(append(outputs, kept...))
	return syncLedger{
		outputs: outputs,
		sums:    ledgerSums(outputs, written, prev.OutputSums),
		orphans: finalizeLedger(orphans),
		merged:  ledgerMerged(outputs, merged, written, prev.Merged),
	}, kept, removed, stripped, err
}

// releaseMergedOrphans hands back the merged JSON files the last sync
// wrote and this one does not. It takes out the keys sync set there and
// keeps the rest, which a whole-file sweep would delete with them
// (#1541). released lists every path it dealt with, so the sweep leaves
// them alone. A file it cannot read is kept, and anything but a regular
// file is left to the sweep.
func releaseMergedOrphans(sess *adapters.Session, prev syncStateFile, current []string) (released, removed, stripped, kept []string, err error) {
	pruned := make(map[string]bool)
	for _, p := range prev.Outputs {
		m, ok := prev.Merged[p]
		if !ok || slices.Contains(current, p) || underSymlinkedDir(p) {
			continue
		}
		if fi, statErr := os.Lstat(p); statErr != nil || !fi.Mode().IsRegular() {
			continue
		}
		if sess.KeepsEdits() && editedSince(p, prev.OutputSums[p]) {
			continue
		}
		result, err := sess.ReleaseMergedJSON(p, m.Keys, m.Created, false)
		if err != nil {
			return released, removed, stripped, kept, err
		}
		released = append(released, p)
		switch result {
		case adapters.MergedRemoved:
			removed = append(removed, p)
			pruneAncestorDirs(p, pruned)
		case adapters.MergedStripped:
			stripped = append(stripped, p)
		case adapters.MergedKept:
			if !sess.IsUnmanaged(p) {
				kept = append(kept, p)
			}
		}
	}
	return released, removed, stripped, kept, nil
}

// keepUnledgered records in ledger the leftovers no ledger proves sync
// wrote (unledgeredReport) and returns them for the caller to report.
// Without the record, the ledger this sync writes would hide them from
// every later check (#1354). A run that did not emit every configured
// target cannot tell another target's file from a leftover. With a prior
// ledger it carries that ledger's list forward, minus what it wrote.
// With none, it records every candidate it did not write and names none:
// a later check or full sync drops the ones a full render emits.
func keepUnledgered(cfg *config.Config, prev syncStateFile, ledger *syncLedger, written map[string]string, complete bool) driftReport {
	if !complete && !ledgerMissing(".") {
		for _, p := range prev.Unledgered {
			if _, ok := written[p]; !ok {
				ledger.unledgered = append(ledger.unledgered, p)
			}
		}
		return driftReport{}
	}
	emitted := make(map[string]bool, len(ledger.outputs))
	for _, p := range ledger.outputs {
		emitted[p] = true
	}
	rep := unledgeredReport(cfg, emitted, prev, strandedOutput(cfg, emitted, prev))
	ledger.unledgered = finalizeLedger(append(append([]string{}, rep.Leftover...), rep.Orphaned...))
	if !complete {
		return driftReport{Target: unledgeredReportTarget}
	}
	return rep
}

// sweepLedgerOrphans removes every path the previous sync wrote but
// the current sync did not. Each candidate flows through the emit
// session's RemoveOwned: a file with the provenance header, or one whose
// bytes still match the sum recorded in priorSums, is removed. Returns
// the paths removed, and the paths kept because ownership could not be proven (a
// header-less file edited since, or one synced before sums existed).
// Kept paths are still on disk and not user-owned; callers carry them
// in the ledger and report them so the leftover never goes silent.
//
// After each successful removal, empty parent directories are pruned
// bottom-up until a non-empty ancestor (or the project root) blocks
// the walk. The project root itself is never removed.
func sweepLedgerOrphans(sess *adapters.Session, prior []string, priorSums map[string]string, current []string) (removed, kept []string, err error) {
	if len(prior) == 0 {
		return nil, nil, nil
	}
	currentSet := make(map[string]struct{}, len(current))
	for _, p := range current {
		currentSet[p] = struct{}{}
	}
	prunedDirs := make(map[string]bool)
	for _, p := range prior {
		if _, inCurrent := currentSet[p]; inCurrent {
			continue
		}
		// Ledgered symlinks (shared-skills links) are removed as links:
		// reading through them would either hit the canonical file (and
		// delete it through the link) or dangle forever when the target
		// is already gone.
		fi, err := os.Lstat(p)
		// A ledgered link that is now a real directory (shared-skills
		// turned off, or the folder holds a user-owned file) is not an
		// orphan: its files carry their own ledger entries.
		if err == nil && fi.IsDir() {
			continue
		}
		if err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if ok, err := sess.RemoveLink(p, false); err == nil && ok {
				removed = append(removed, p)
				pruneAncestorDirs(p, prunedDirs)
			}
			continue
		}
		// A file whose folder became a shared-skills link is not an
		// orphan: the link replaced it and carries its own ledger entry,
		// and removing through it would delete the canonical copy.
		if underSymlinkedDir(p) {
			continue
		}
		if sess.KeepsEdits() && editedSince(p, priorSums[p]) && !sess.IsUnmanaged(p) {
			kept = append(kept, p)
			continue
		}
		ok, err := sess.RemoveOwned(p, priorSums[p], false)
		if err != nil {
			return removed, kept, err
		}
		switch {
		case ok:
			removed = append(removed, p)
			pruneAncestorDirs(p, prunedDirs)
		case fileExists(p) && !sess.IsUnmanaged(p):
			kept = append(kept, p)
		}
	}
	return removed, kept, nil
}

// editedSince reports whether the file at path no longer holds the bytes
// whose sum the last sync recorded. No recorded sum proves no edit.
func editedSince(path, sum string) bool {
	if sum == "" {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && adapters.ContentSum(string(data)) != sum
}

// underSymlinkedDir reports whether any directory between path and the
// working directory is a symlink.
func underSymlinkedDir(path string) bool {
	for dir := filepath.Dir(path); dir != "." && dir != "/" && dir != ""; dir = filepath.Dir(dir) {
		if fi, err := os.Lstat(dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// pruneAncestorDirs walks from path's parent upward, removing each
// directory that is empty. Stops at the first non-empty directory and
// refuses to climb above the current working directory. Memoizes only
// the directories it removed: a directory found non-empty may empty out
// once the sweep removes its last sibling, so it must be scanned again
// (a skill folder with bundled references, #785).
func pruneAncestorDirs(path string, removed map[string]bool) {
	dir := filepath.Dir(path)
	for {
		if dir == "." || dir == "/" || dir == "" {
			return
		}
		if removed[dir] {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				dir = filepath.Dir(dir)
				continue
			}
			return
		}
		if len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		removed[dir] = true
		dir = filepath.Dir(dir)
	}
}
