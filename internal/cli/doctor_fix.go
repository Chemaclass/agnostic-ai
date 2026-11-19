package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

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
	// Save hand edits as <path>.bak, as sync does, so --fix loses none.
	// Merged files keep the user's keys by merging, so sync backs none of
	// them up; they go through a session without the guard.
	prior := readStateFile(".")
	editGuardFor(false, prior).apply(sess)
	merged := adapters.NewSession()
	if backup {
		merged.SetBackup(true)
		defer merged.SetBackup(false)
	}
	fixed := map[string]string{}
	releasedMerged := map[string]*mergedOutput{}
	fixedMerged := map[string]mergedOutput{}
	defer func() {
		if err := errors.Join(recordOutputSums(".", fixed), recordBackups(".", syncBackups(prior.Backups, append(sess.Backups(), merged.Backups()...))), recordMergedFixes(".", fixedMerged, fixed), recordMergedRelease(".", releasedMerged)); err != nil {
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
			writer := sess
			if f.Merged {
				writer = merged
			}
			if err := writer.WriteFile(f.Path, f.Content, false); err != nil {
				return written, err
			}
			if slices.Contains(sess.BackupBlockedEdits(), f.Path) {
				continue
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
	if blocked := sess.BackupBlockedEdits(); len(blocked) > 0 {
		for _, p := range blocked {
			fmt.Fprintf(os.Stderr, "%s kept a hand edit to %s: %s.bak already holds an earlier one; move both into .agnostic-ai/, then delete the .bak\n", bang(), p, p)
		}
		return written, fmt.Errorf("kept %d hand-edited file(s) whose .bak already exists", len(blocked))
	}
	return written, nil
}

// recordBackups stores the backups in the state file, so import can tell
// a backup doctor --fix made from a skill's own asset, as for sync.
func recordBackups(projectRoot string, backups map[string]string) error {
	if len(backups) == 0 {
		return nil
	}
	p := stateFilePath(projectRoot)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	s, err := decodeLedger(projectRoot, p, data)
	if newer := new(newerLedgerError); errors.As(err, &newer) {
		return err
	}
	if err != nil {
		return nil
	}
	s.Backups = backups
	return replaceStateFile(projectRoot, s)
}
