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

// sweepLedgerOrphans removes every path the previous sync wrote but
// the current sync did not. Each candidate flows through
// emit.RemoveGenerated, which leaves user-authored files (no agnostic
// provenance marker) on disk. Returns the list of paths actually
// removed (or that would be removed when dryRun is true).
//
// After each successful removal, empty parent directories are pruned
// bottom-up until a non-empty ancestor (or the project root) blocks
// the walk. The project root itself is never removed.
func sweepLedgerOrphans(prior, current []string, dryRun bool) ([]string, error) {
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
		// emit.RemoveGenerated is a no-op when the file is missing or
		// not agnostic-managed, so we can fold both "already gone" and
		// "user took ownership" into the same code path.
		existedBefore := fileExists(p)
		if err := adapters.RemoveGenerated(p, dryRun); err != nil {
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
