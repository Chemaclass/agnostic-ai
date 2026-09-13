package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// recordLedgerWrites appends every create/update/skip path from writes
// into session in the order they were emitted, and records each path's
// content sum in written (an empty sum still marks the path as written
// this run). Delete actions are skipped because the file no longer belongs to
// this sync's output set. Callers feed session and written to
// writeStateFile at end-of-sync so the next run knows the full prior
// output footprint and can prove ownership of header-less files.
func recordLedgerWrites(writes []adapters.WrittenFile, session *[]string, written map[string]string) {
	for _, w := range writes {
		switch w.Action {
		case "create", "update", "skip":
			*session = append(*session, w.Path)
			written[w.Path] = w.Sum
		}
	}
}

// ledgerSums returns the content sums to store for ledger. A path written
// this run takes its fresh sum, even when empty (a header file needs
// none). A path not written this run (another target's on a partial sync,
// or a kept orphan) carries its prior sum so ownership proof is not lost.
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
func sweepAndFinalizeLedger(sess *adapters.Session, prev syncStateFile, session []string, written map[string]string, emitted, configured []string, dryRun bool) (ledger syncLedger, kept, removed []string, err error) {
	if dryRun {
		return syncLedger{}, nil, nil, nil
	}
	coversAll := coversAllConfiguredTargets(emitted, configured)
	outputs := reconcilePartialLedger(finalizeLedger(session), prev.Outputs, coversAll)
	removed, kept, err = sweepLedgerOrphans(sess, prev.Outputs, prev.OutputSums, outputs)
	outputs = finalizeLedger(append(outputs, kept...))
	return syncLedger{
		outputs: outputs,
		sums:    ledgerSums(outputs, written, prev.OutputSums),
		orphans: ledgerOrphans(kept, prev.Orphans, written, coversAll),
	}, kept, removed, err
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
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(p); err == nil {
				removed = append(removed, p)
				pruneAncestorDirs(p, prunedDirs)
			}
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
