package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitTarget resolves target t and emits it under detailed recording, shared by
// the text, dry-run, and JSON sync paths so the resolve+record+emit boilerplate
// lives in one place. resolved reports whether the adapter resolved, letting
// callers tell a resolve failure (skippable) from an emit failure (fatal for
// text sync); writes holds the recorded write events, including those made
// before an emit error, and is empty in dry-run.
// The recording buffer is always stopped before returning, so no global
// recording state leaks.
func emitTarget(sess *adapters.Session, t string, b spec.Bundle, cfg *config.Config, dryRun bool) (writes []adapters.WrittenFile, resolved bool, err error) {
	adapter, err := adapters.Resolve(t)
	if err != nil {
		return nil, false, err
	}
	sess.StartDetailedRecording()
	err = emitAdapter(sess, adapter, b, cfg, dryRun)
	return sess.StopDetailedRecording(), true, err
}

// targetEmit is one target's emission result. Collecting these lets the
// serial post-emission phase process targets in stable order, so output
// stays deterministic regardless of how many workers ran the emits.
type targetEmit struct {
	target   string
	writes   []adapters.WrittenFile
	recorded []string // gitignore paths; empty unless gitignore recording is on
	resolved bool     // false when the adapter could not be resolved (skippable)
	err      error
	dur      time.Duration // wall time the target's emit took, for the verbose summary
}

func importProvenanceTargets(emits []targetEmit, sessions []*adapters.Session, main *adapters.Session) []string {
	// Skipped outputs cannot prove that a changed spec reached its tool.
	if len(main.KeptEdits()) > 0 || len(main.UnmanagedSkips()) > 0 || len(main.BackupBlockedEdits()) > 0 {
		return nil
	}
	var targets []string
	for i, e := range emits {
		if !e.resolved || e.err != nil || slices.ContainsFunc(e.writes, func(w adapters.WrittenFile) bool {
			return w.Action == "edited"
		}) {
			continue
		}
		if i < len(sessions) && sessions[i] != nil && len(sessions[i].UnmanagedSkips()) > 0 {
			continue
		}
		targets = append(targets, e.target)
	}
	return targets
}

// resolveJobs maps the --jobs flag to a worker count: 0 or negative means
// runtime.NumCPU(). The count is capped at the target count (idle workers
// buy nothing) and floored at 1.
func resolveJobs(jobs, targets int) int {
	if jobs <= 0 {
		jobs = runtime.NumCPU()
	}
	if jobs > targets {
		jobs = targets
	}
	if jobs < 1 {
		jobs = 1
	}
	return jobs
}

// provenanceBatch is a set of target indices that share one
// provenance-header setting, run together so the process-global toggle is
// constant while they emit concurrently.
type provenanceBatch struct {
	provenance bool
	indices    []int
}

// provenanceBatches partitions target indices by their provenance-header
// setting, preserving target order within each batch. Concurrent emission
// runs one batch at a time and pins the toggle to the batch's value, so no
// target sees another's setting through the shared global (see
// internal/adapters/internal/emit/header.go). The common case — every
// target on the default — is a single batch, i.e. full fan-out.
func provenanceBatches(cfg *config.Config, targets []string) []provenanceBatch {
	var on, off []int
	for i, t := range targets {
		if cfg.ProvenanceHeaderEnabled(t) {
			on = append(on, i)
		} else {
			off = append(off, i)
		}
	}
	var batches []provenanceBatch
	if len(on) > 0 {
		batches = append(batches, provenanceBatch{provenance: true, indices: on})
	}
	if len(off) > 0 {
		batches = append(batches, provenanceBatch{provenance: false, indices: off})
	}
	return batches
}

// emitTargetsConcurrent emits every target on its own emit.Session with
// bounded parallelism (jobs workers), returning the per-target results in
// the SAME order as targets so downstream processing is deterministic
// regardless of --jobs. Each target owns its Session, so the per-target
// detailed-recording, gitignore-recording, and transaction buffers never
// cross-talk; the returned sessions (target order) carry the transaction
// logs the caller commits or rolls back.
//
// Targets run in provenance-homogeneous batches so the one emit-time
// process global adapters still share — the provenance-header toggle — is
// constant for every concurrently-running target.
//
// With failFast set, the first emit error cancels the group and is
// returned wrapped with its target (`<target>: <err>`); resolve failures
// (unknown target) stay non-fatal and ride along in the result. Without
// it, every target runs to completion and all errors ride along in the
// results — the JSON path, which reports per-target errors rather than
// aborting the whole sync. edits sets how each session treats a hand edit.
func emitTargetsConcurrent(targets []string, b spec.Bundle, cfg *config.Config, dryRun, backup, gitignoreOn, failFast bool, jobs int, edits editGuard, winners map[string]string) ([]targetEmit, []*adapters.Session, error) {
	cfg = cfg.WithAdditionalTargets(targets...)
	results := make([]targetEmit, len(targets))
	sessions := make([]*adapters.Session, len(targets))
	for i, t := range targets {
		results[i] = targetEmit{target: t}
	}
	workers := resolveJobs(jobs, len(targets))

	// Pin/restore the shared provenance toggle around the whole phase so
	// the serial post-emission writers (entry points) see the prior value.
	orig := adapters.ProvenanceEnabled()
	defer adapters.SetProvenanceEnabled(orig)

	var firstErr error
	for _, batch := range provenanceBatches(cfg, targets) {
		adapters.SetProvenanceEnabled(batch.provenance)
		g, ctx := errgroup.WithContext(context.Background())
		g.SetLimit(workers)
		for _, idx := range batch.indices {
			idx := idx
			g.Go(func() error {
				if failFast && ctx.Err() != nil {
					return nil // group already failing; do not start new work
				}
				sess := adapters.NewSession()
				sess.SetProjectRoot(".")
				sessions[idx] = sess
				if backup {
					sess.SetBackup(true)
				}
				edits.apply(sess)
				sess.SetCollisionWinners(winners)
				if !dryRun {
					sess.StartTransaction()
				}
				if gitignoreOn {
					sess.StartRecording()
				}
				emitStart := time.Now()
				writes, resolved, err := emitTarget(sess, targets[idx], b, cfg, dryRun)
				emitDur := time.Since(emitStart)
				var recorded []string
				if gitignoreOn {
					recorded = sess.StopRecording()
				}
				results[idx] = targetEmit{target: targets[idx], writes: writes, recorded: recorded, resolved: resolved, err: err, dur: emitDur}
				if failFast && err != nil && resolved {
					return fmt.Errorf("%s: %w", targets[idx], err)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if failFast {
				break
			}
		}
	}
	return results, sessions, firstErr
}

// rollbackSessions undoes the writes recorded across every session, latest
// phase first: sessions are appended in write order (targets, then the
// serial entry-point session), so reverse iteration restores a path an
// entry point overwrote before the target-level create is undone. Errors
// are joined.
func rollbackSessions(sessions []*adapters.Session) error {
	var errs []error
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i] == nil {
			continue
		}
		if err := sessions[i].Rollback(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// undoSweep rolls back the orphan sweep logged on sess, puts back the
// ignore files in prior, and returns err, so a sync that fails after the
// sweep keeps every orphan its unchanged ledger still names, and the
// ignores that list them.
func undoSweep(sess *adapters.Session, prior []priorFile, err error) error {
	if rbErr := errors.Join(sess.Rollback(), restoreFiles(prior)); rbErr != nil {
		fmt.Fprintf(os.Stderr, "! rollback: %v\n", rbErr)
	}
	return err
}

// commitSessions releases the transaction log on every session. Order is
// irrelevant: Commit only clears state.
func commitSessions(sessions []*adapters.Session) {
	for _, s := range sessions {
		if s != nil {
			s.Commit()
		}
	}
}

// normalizeSharedWriteAttribution makes the create/update/skip action for a
// path several targets write deterministic regardless of --jobs. The emit
// layer's per-path lock already guarantees exactly one target creates or
// updates a shared path and the rest skip it, but WHICH target won the write
// depends on goroutine scheduling. Serial emission always credits the
// create/update to the first target in target order and skips the rest, so
// reassign the non-skip action to the earliest target (emits is in stable
// target order) and force every later occurrence of the path to "skip".
//
// The sharing targets wrote byte-identical content — a genuine content
// conflict on one path is caught earlier by collision detection — so this
// relabels bookkeeping only; it never changes bytes on disk, the total
// created+updated count, or the ledger's path set.
func normalizeSharedWriteAttribution(emits []targetEmit) {
	// winning[path] is the non-skip action some target recorded for path.
	// A path absent here was skipped by every target, so its attribution is
	// already order-independent and left untouched.
	winning := map[string]string{}
	for i := range emits {
		for _, w := range emits[i].writes {
			if w.Action != "skip" {
				winning[w.Path] = w.Action
			}
		}
	}
	claimed := map[string]bool{}
	for i := range emits {
		for j := range emits[i].writes {
			w := &emits[i].writes[j]
			act, ok := winning[w.Path]
			if !ok {
				continue
			}
			if claimed[w.Path] {
				w.Action = "skip"
				continue
			}
			w.Action = act
			claimed[w.Path] = true
		}
	}
}
