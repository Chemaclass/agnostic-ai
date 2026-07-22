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
// into session in the order they were emitted. Delete actions are
// skipped because the file no longer belongs to this sync's output
// set. Callers feed session to writeStateFile at end-of-sync so the
// next run knows the full prior output footprint.
func recordLedgerWrites(writes []adapters.WrittenFile, session *[]string) {
	for _, w := range writes {
		switch w.Action {
		case "create", "update", "skip":
			*session = append(*session, w.Path)
		}
	}
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

// sweepLedgerOrphans removes every path the previous sync wrote but
// the current sync did not. Each candidate flows through
// emit.RemoveGenerated, which leaves user-authored files (no agnostic
// provenance marker) on disk. Returns the list of paths actually
// removed (or that would be removed when dryRun is true).
//
// After each successful removal, empty parent directories are pruned
// bottom-up until a non-empty ancestor (or the project root) blocks
// the walk. The project root itself is never removed.
func sweepLedgerOrphans(sess *adapters.Session, prior, current []string, dryRun bool) ([]string, error) {
	if len(prior) == 0 {
		return nil, nil
	}
	currentSet := make(map[string]struct{}, len(current))
	for _, p := range current {
		currentSet[p] = struct{}{}
	}
	var removed []string
	prunedDirs := make(map[string]bool)
	for _, p := range prior {
		if _, kept := currentSet[p]; kept {
			continue
		}
		// Ledgered symlinks (shared-skills links) are removed as links:
		// reading through them would either hit the canonical file (and
		// delete it through the link) or dangle forever when the target
		// is already gone.
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if dryRun {
				removed = append(removed, p)
				continue
			}
			if err := os.Remove(p); err == nil {
				removed = append(removed, p)
				pruneAncestorDirs(p, prunedDirs)
			}
			continue
		}
		// RemoveGenerated is a no-op when the file is missing or
		// not agnostic-managed, so we can fold both "already gone" and
		// "user took ownership" into the same code path.
		existedBefore := fileExists(p)
		if err := sess.RemoveGenerated(p, dryRun); err != nil {
			return removed, err
		}
		if dryRun {
			if existedBefore {
				removed = append(removed, p)
			}
			continue
		}
		if existedBefore && !fileExists(p) {
			removed = append(removed, p)
			pruneAncestorDirs(p, prunedDirs)
		}
	}
	return removed, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// pruneAncestorDirs walks from path's parent upward, removing each
// directory that is empty. Stops at the first non-empty directory and
// refuses to climb above the current working directory. Memoizes
// already-visited dirs so repeated calls within one sweep do not
// re-scan shared ancestors.
func pruneAncestorDirs(path string, visited map[string]bool) {
	dir := filepath.Dir(path)
	for {
		if dir == "." || dir == "/" || dir == "" {
			return
		}
		if visited[dir] {
			return
		}
		visited[dir] = true
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
		dir = filepath.Dir(dir)
	}
}
