package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// syncStateVersion identifies the on-disk schema of `.agnostic-ai/.sync-state`.
// Bumped to 2 when the per-sync output ledger (Outputs) was added.
// Readers tolerate older versions by treating missing fields as zero values.
const syncStateVersion = 2

type syncStateFile struct {
	Version        int       `json:"version,omitempty"`
	SyncedAt       time.Time `json:"synced_at"`
	FilesChanged   int       `json:"files_changed"`
	WarningsDigest string    `json:"warnings_digest,omitempty"`
	// NotesDigest fingerprints the coverage notes from the previous
	// sync so the next run can sticky-suppress an unchanged set, the
	// same way WarningsDigest gates capability warnings.
	NotesDigest string `json:"notes_digest,omitempty"`
	// Outputs lists every file path the previous sync wrote (create or
	// update or skip), relative to the project root. The next sync uses
	// it to detect orphans: any path present in the prior ledger but
	// absent from the current write set is swept via the emit
	// session's RemoveGenerated. The header-guarded sweep skips
	// user-authored files. An empty list (older state files, or first
	// sync after upgrade) disables the sweep so projects without a
	// recorded baseline never lose files.
	Outputs []string `json:"outputs,omitempty"`
}

func stateFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".agnostic-ai", ".sync-state")
}

// readStateFile loads the sync-state cache. A missing or corrupt file
// yields the zero value on purpose: the state is a self-healing cache the
// next sync rewrites, so an unreadable one means "no prior state" rather
// than a fatal error. Mirrors lastSyncTimestamp, which swallows the same
// read for the same reason.
func readStateFile(projectRoot string) syncStateFile {
	var s syncStateFile
	data, err := os.ReadFile(stateFilePath(projectRoot))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

func writeStateFile(projectRoot string, filesChanged int, warningsDigest, notesDigest string, outputs []string) error {
	p := stateFilePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	data, err := json.Marshal(syncStateFile{
		Version:        syncStateVersion,
		SyncedAt:       time.Now().UTC(),
		FilesChanged:   filesChanged,
		WarningsDigest: warningsDigest,
		NotesDigest:    notesDigest,
		Outputs:        outputs,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// emitTarget resolves target t and emits it under detailed recording, shared by
// the text, dry-run, and JSON sync paths so the resolve+record+emit boilerplate
// lives in one place. resolved reports whether the adapter resolved, letting
// callers tell a resolve failure (skippable) from an emit failure (fatal for
// text sync); writes holds the recorded write events and is empty in dry-run.
// The recording buffer is always stopped before returning, so no global
// recording state leaks.
func emitTarget(sess *adapters.Session, t string, b spec.Bundle, cfg *config.Config, dryRun bool) (writes []adapters.WrittenFile, resolved bool, err error) {
	adapter, err := adapters.Resolve(t)
	if err != nil {
		return nil, false, err
	}
	sess.StartDetailedRecording()
	if err := adapters.EmitWithProvenance(sess, adapter, b, cfg, dryRun); err != nil {
		sess.StopDetailedRecording()
		return nil, true, err
	}
	return sess.StopDetailedRecording(), true, nil
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
// aborting the whole sync.
func emitTargetsConcurrent(targets []string, b spec.Bundle, cfg *config.Config, dryRun, backup, gitignoreOn, failFast bool, jobs int) ([]targetEmit, []*adapters.Session, error) {
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
				sessions[idx] = sess
				if backup {
					sess.SetBackup(true)
				}
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

func runSyncOnce(root string, targets []string, dryRun, backup bool, gitignoreFlag string, jobs int) (retErr error) {
	start := time.Now()
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
	cfg, b, err := loadProject(root)
	if err != nil {
		return err
	}
	effectiveTargets := targets
	if len(effectiveTargets) == 0 {
		effectiveTargets = cfg.Targets
	}
	if err := detectCollisions(cfg, b, effectiveTargets); err != nil {
		return err
	}
	shared, err := planSharedSkills(cfg, b, effectiveTargets)
	if err != nil {
		return err
	}
	prev := readStateFile(root)

	// mainSess drives the serial post-emission writes (entry points,
	// shared-skill links, orphan sweep). Each concurrent target owns its
	// own session; sessions collects every session with a rollback log in
	// write order, so the deferred rollback undoes the latest phase first.
	mainSess := adapters.NewSession()
	if backup {
		mainSess.SetBackup(true)
	}
	var sessions []*adapters.Session
	if !dryRun {
		mainSess.StartTransaction()
		defer func() {
			if retErr != nil {
				fmt.Fprintf(os.Stderr, "! sync failed; rolling back partial writes\n")
				if rbErr := rollbackSessions(sessions); rbErr != nil {
					fmt.Fprintf(os.Stderr, "! rollback: %v\n", rbErr)
				}
			} else {
				commitSessions(sessions)
			}
		}()
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)

	shared.reconcile(prev.Outputs, dryRun)

	// Emit every target concurrently (bounded by jobs) on its own session,
	// collecting per-target results in stable order. The first emit error
	// cancels the group and trips the rollback above.
	emits, targetSessions, emitErr := emitTargetsConcurrent(effectiveTargets, b, cfg, dryRun, backup, gitignoreOn, true, jobs)
	for _, s := range targetSessions {
		if s != nil {
			sessions = append(sessions, s)
		}
	}
	sessions = append(sessions, mainSess) // written last (entry points), rolled back first
	if emitErr != nil {
		return emitErr
	}
	normalizeSharedWriteAttribution(emits)

	verbose := verbosity >= levelVerbose
	filesChanged := 0
	var ledgerSession []string
	var gitignoreEntries []string
	for _, e := range emits {
		if e.err != nil && !e.resolved {
			fmt.Fprintf(os.Stderr, "! %v\n", e.err)
			continue
		}
		gitignoreEntries = append(gitignoreEntries, e.recorded...)
		if dryRun {
			continue
		}
		recordLedgerWrites(e.writes, &ledgerSession)
		created, updated, skipped := classifyDetailedWrites(e.writes)
		filesChanged += created + updated
		if verbose {
			verbosef("→ %s: %d created, %d updated, %d unchanged in %dms\n", e.target, created, updated, skipped, e.dur.Milliseconds())
		}
	}

	// Entry points and shared-skill links write serially through mainSess,
	// after every target, so their ordering, dedupe, and gitignore capture
	// stay stable regardless of --jobs.
	if gitignoreOn {
		mainSess.StartRecording()
	}
	mainSess.StartDetailedRecording()
	if err := writeAgnosticEntryPoints(mainSess, cfg, b, effectiveTargets, dryRun); err != nil {
		mainSess.StopDetailedRecording()
		if gitignoreOn {
			mainSess.StopRecording()
		}
		return err
	}
	entryWrites := mainSess.StopDetailedRecording()
	if gitignoreOn {
		gitignoreEntries = append(gitignoreEntries, mainSess.StopRecording()...)
	}
	if !dryRun {
		recordLedgerWrites(entryWrites, &ledgerSession)
		created, updated, _ := classifyDetailedWrites(entryWrites)
		filesChanged += created + updated
		// resolveAgnosticBody reads AGNOSTIC_AI.md from disk on
		// subsequent syncs and skips the re-write, so detailed
		// recording never captures the path. Add it explicitly so
		// the ledger does not treat the source body as an orphan.
		if _, err := os.Stat(adapters.AgnosticEntryPointPath); err == nil {
			ledgerSession = append(ledgerSession, adapters.AgnosticEntryPointPath)
		}
	}
	applied := shared.apply(mainSess, dryRun)
	ledgerSession = adjustLedgerForLinks(ledgerSession, applied)
	if gitignoreOn {
		for _, l := range applied {
			gitignoreEntries = append(gitignoreEntries, l.path)
		}
		gitignoreEntries = append(gitignoreEntries, gitignoreHintsForTargets(cfg, effectiveTargets)...)
		block := buildManagedBlock(cfg, gitignoreEntries)
		if err := updateGitignore(root, cfg, block); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
		summaryf("→ updated .gitignore\n")
	}

	// Concurrent emission appends capability warnings / coverage notes in
	// completion order; re-sort them to the target sequence so the flushed
	// output below is byte-identical regardless of --jobs.
	adapters.OrderBufferedDropsByTarget(effectiveTargets)

	digest := adapters.CapabilityWarningsDigest()
	notesDigest := adapters.CoverageNotesDigest()
	warningsUnchanged := digest != "" && digest == prev.WarningsDigest
	notesUnchanged := notesDigest != "" && notesDigest == prev.NotesDigest
	// Render the per-target summary only when at least one of the buffers
	// actually changed, so it honors the same unchanged-since-last-sync
	// suppression as the kind-grouped flushes below instead of re-printing
	// every run. Must run before the flushes clear the buffers.
	dropsChanged := !warningsUnchanged || !notesUnchanged
	if cfg.Sync.DroppedSummary && verbosity >= levelDefault && dropsChanged {
		adapters.RenderDroppedSummary(logOut)
	}
	if warningsUnchanged {
		n := adapters.PendingCapabilityWarningsCount()
		summaryf("  (%d capability warning%s unchanged since last sync; delete %s to re-show)\n",
			n, plural(n), stateFilePath(root))
		adapters.ResetCapabilityWarnings()
	} else {
		adapters.FlushCapabilityWarnings()
	}
	if notesUnchanged {
		n := adapters.PendingCoverageNotesCount()
		summaryf("  (%d coverage note%s unchanged since last sync; delete %s to re-show)\n",
			n, plural(n), stateFilePath(root))
		adapters.ResetCoverageNotes()
	} else {
		adapters.FlushCoverageNotes()
	}
	ledger := finalizeLedger(ledgerSession)
	ledger = reconcilePartialLedger(ledger, prev.Outputs, coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
	removed, sweepErr := sweepLedgerOrphans(mainSess, prev.Outputs, ledger, dryRun)
	if sweepErr != nil {
		fmt.Fprintf(os.Stderr, "! orphan sweep: %v\n", sweepErr)
	}
	if len(removed) > 0 {
		verb := "removed"
		if dryRun {
			verb = "would remove"
		}
		summaryf("  %s %d orphan file%s from prior sync\n", verb, len(removed), plural(len(removed)))
		if verbose {
			for _, p := range removed {
				verbosef("  - %s\n", p)
			}
		}
	}
	if !dryRun {
		if err := writeStateFile(root, filesChanged, digest, notesDigest, ledger); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}
	printSyncSummary(len(effectiveTargets), filesChanged, time.Since(start), dryRun)
	return nil
}

func classifyDetailedWrites(files []adapters.WrittenFile) (created, updated, skipped int) {
	for _, f := range files {
		switch f.Action {
		case "create":
			created++
		case "update":
			updated++
		case "skip":
			skipped++
		}
	}
	return
}

func printSyncSummary(targets, files int, elapsed time.Duration, dryRun bool) {
	verb := "synced"
	if dryRun {
		verb = "would sync"
	}
	summaryf("%s %s %d target%s · %d file%s · %s\n",
		tick(), verb, targets, plural(targets), files, plural(files), shortDuration(elapsed))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// appendFileRecords sorts each write event into out.Writes or out.Skipped, tagged by target.
func appendFileRecords(out *jsonOutput, target string, writes []adapters.WrittenFile) {
	for _, f := range writes {
		rec := fileRecord{Target: target, Path: f.Path, Action: f.Action, Bytes: f.Bytes}
		if f.Action == "skip" {
			out.Skipped = append(out.Skipped, rec)
		} else {
			out.Writes = append(out.Writes, rec)
		}
	}
}

// runSyncJSON runs a real sync pass and emits a JSON result describing each
// file written, updated, or skipped per target.
func runSyncJSON(cmd *cobra.Command, root string, targets []string, dryRun, backup bool, gitignoreFlag string, jobs int) error {
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
	defer adapters.ResetCapabilityWarnings()
	defer adapters.ResetCoverageNotes()
	cfg, b, err := loadProject(root)
	if err != nil {
		return err
	}
	effectiveTargets := targets
	if len(effectiveTargets) == 0 {
		effectiveTargets = cfg.Targets
	}
	if err := detectCollisions(cfg, b, effectiveTargets); err != nil {
		return err
	}
	shared, err := planSharedSkills(cfg, b, effectiveTargets)
	if err != nil {
		return err
	}
	prev := readStateFile(root)

	// mainSess handles the serial entry-point and shared-link writes; each
	// target emits on its own session. The JSON path is not transactional:
	// it reports per-target errors in the result rather than rolling back.
	mainSess := adapters.NewSession()
	if backup {
		mainSess.SetBackup(true)
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
	shared.reconcile(prev.Outputs, dryRun)

	out := jsonOutput{Version: "1", Command: "sync"}
	var ledgerSession []string
	var gitignoreEntries []string

	// Emit every target concurrently; failFast is off so all per-target
	// errors ride along in the results and are reported in target order.
	emits, _, _ := emitTargetsConcurrent(effectiveTargets, b, cfg, dryRun, backup, gitignoreOn, false, jobs)
	normalizeSharedWriteAttribution(emits)
	for _, e := range emits {
		if e.err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: e.target, Message: e.err.Error()})
			continue
		}
		gitignoreEntries = append(gitignoreEntries, e.recorded...)
		recordLedgerWrites(e.writes, &ledgerSession)
		appendFileRecords(&out, e.target, e.writes)
	}

	if gitignoreOn {
		mainSess.StartRecording()
	}
	mainSess.StartDetailedRecording()
	if err := writeAgnosticEntryPoints(mainSess, cfg, b, effectiveTargets, dryRun); err != nil {
		mainSess.StopDetailedRecording()
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
	} else {
		entryWrites := mainSess.StopDetailedRecording()
		recordLedgerWrites(entryWrites, &ledgerSession)
		appendFileRecords(&out, "agnostic-ai", entryWrites)
		// See runSyncOnce: AGNOSTIC_AI.md is read on subsequent
		// syncs without going through emit, so register it for the
		// ledger by hand.
		if _, err := os.Stat(adapters.AgnosticEntryPointPath); err == nil {
			ledgerSession = append(ledgerSession, adapters.AgnosticEntryPointPath)
		}
	}
	if gitignoreOn {
		gitignoreEntries = append(gitignoreEntries, mainSess.StopRecording()...)
	}

	applied := shared.apply(mainSess, dryRun)
	ledgerSession = adjustLedgerForLinks(ledgerSession, applied)
	for _, l := range applied {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: l.path, Action: "link"})
	}
	if gitignoreOn {
		for _, l := range applied {
			gitignoreEntries = append(gitignoreEntries, l.path)
		}
		gitignoreEntries = append(gitignoreEntries, gitignoreHintsForTargets(cfg, effectiveTargets)...)
		block := buildManagedBlock(cfg, gitignoreEntries)
		if err := updateGitignore(root, cfg, block); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
	}
	ledger := finalizeLedger(ledgerSession)
	ledger = reconcilePartialLedger(ledger, prev.Outputs, coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
	removed, sweepErr := sweepLedgerOrphans(mainSess, prev.Outputs, ledger, dryRun)
	if sweepErr != nil {
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: sweepErr.Error()})
	}
	for _, p := range removed {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: "delete"})
	}
	if !dryRun {
		// JSON path does not print warnings or notes, so preserve the
		// previous digests so the next non-JSON run can still
		// sticky-suppress.
		if err := writeStateFile(root, len(out.Writes), prev.WarningsDigest, prev.NotesDigest, ledger); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}
	return emitJSON(cmd, out)
}

// printSyncPlan prints a human-readable per-target summary of what sync
// would change: how many files are missing (added) or stale (changed).
func printSyncPlan(cmd *cobra.Command, reports []driftReport) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, r := range reports {
		if !r.hasDrift() {
			_, _ = fmt.Fprintf(w, "[%s]\tno changes\n", r.Target)
			continue
		}
		_, _ = fmt.Fprintf(w, "[%s]\tadded: %d\tchanged: %d\n", r.Target, len(r.Missing), len(r.Stale))
	}
	_ = w.Flush()
}

// printSyncCheckJSON emits a JSON result for `sync --check`. Files that need
// to be written appear in writes (action: "missing" or "stale"); files that
// are already in sync appear in skipped (action: "ok").
func printSyncCheckJSON(cmd *cobra.Command, reports []driftReport) error {
	out := jsonOutput{Version: "1", Command: "sync --check"}
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
		return errDriftDetected()
	}
	return nil
}

// checkFormatHuman is the default `--check` report: a per-target table.
// checkFormatGitHub emits GitHub Actions annotations. JSON output stays
// behind the separate `--json` flag, which takes precedence over --format.
const (
	checkFormatHuman  = "human"
	checkFormatGitHub = "github"
)

// validateCheckFormat rejects an unknown `--format` value so a typo fails
// fast instead of silently falling back to the human table.
func validateCheckFormat(format string) error {
	switch format {
	case checkFormatHuman, checkFormatGitHub:
		return nil
	}
	return fmt.Errorf("--format: expected %q or %q, got %q", checkFormatHuman, checkFormatGitHub, format)
}

// errDriftDetected is the failure a drifting `sync --check` returns. It names
// `agnostic-ai doctor` so a bare CI line still points at the full diagnosis;
// the reconcile command prints separately as a stderr hint.
func errDriftDetected() error {
	return fmt.Errorf("drift detected; run `agnostic-ai doctor` for a full diagnosis")
}

// reportCheckDrift renders a `sync --check` result in the requested text
// format and returns a non-zero error when any target drifts. The report
// goes to stdout; the fix hint goes to stderr, so a machine reading stdout (a
// captured log, a github matcher) sees only the drift lines. Presentation
// only: it never writes files or changes what counts as drift.
func reportCheckDrift(cmd *cobra.Command, reports []driftReport, format string, diff bool) error {
	var drift bool
	switch format {
	case checkFormatGitHub:
		drift = printDriftGitHub(cmd, reports)
	default:
		drift = printDrift(reports)
	}
	if diff {
		printDriftDiffs(cmd, reports)
	}
	if !drift {
		return nil
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "to reconcile, run: agnostic-ai sync")
	return errDriftDetected()
}

// printDriftGitHub emits one GitHub Actions error annotation per drifted file
// so drift surfaces inline on the pull request. Missing files carry no line;
// stale files point at the first changed line. Returns true if any target
// drifted, so an in-sync run emits nothing and stays silent.
func printDriftGitHub(cmd *cobra.Command, reports []driftReport) bool {
	out := cmd.OutOrStdout()
	drift := false
	for _, r := range reports {
		for _, f := range r.Missing {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s is missing; run agnostic-ai sync to generate it\n",
				githubProp(f.Path), githubData(filepath.ToSlash(f.Path)))
		}
		for _, f := range r.Stale {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s,line=%d::%s drifted from specs; run agnostic-ai sync to reconcile\n",
				githubProp(f.Path), firstChangedLine(f.Path, f.Content), githubData(filepath.ToSlash(f.Path)))
		}
	}
	return drift
}

// diffBodyMax caps the changed lines shown per file so a large rewrite does
// not flood CI logs. Past it, a summary line reports the remainder.
const diffBodyMax = 200

// printDriftDiffs prints a unified diff for every drifted file: the on-disk
// content against what sync would write. Missing files show a concise create
// line rather than a full body. Only drifted files produce output, so an
// in-sync run stays silent. Presentation only; nothing is written.
func printDriftDiffs(cmd *cobra.Command, reports []driftReport) {
	out := cmd.OutOrStdout()
	for _, r := range reports {
		for _, f := range r.Missing {
			_, _ = fmt.Fprintf(out, "would create %s (%d bytes)\n", filepath.ToSlash(f.Path), len(f.Content))
		}
		for _, f := range r.Stale {
			disk, err := os.ReadFile(f.Path)
			if err != nil {
				_, _ = fmt.Fprintf(out, "%s: %v\n", f.Path, err)
				continue
			}
			_, _ = fmt.Fprint(out, unifiedDiff(f.Path, string(disk), f.Content, diffBodyMax))
		}
	}
}

// unifiedDiff renders a minimal unified diff between the on-disk content
// (have) and what sync would write (want), keyed by path. It trims the shared
// leading and trailing lines and prints the differing middle as one hunk:
// on-disk lines with `-`, would-write lines with `+`. Output past maxLines is
// dropped with a summary so CI logs stay lean. A display helper over line
// slices, not a general diff engine.
func unifiedDiff(path, have, want string, maxLines int) string {
	haveLines := splitLines(have)
	wantLines := splitLines(want)

	p := commonPrefix(haveLines, wantLines)
	s := 0
	for s < len(haveLines)-p && s < len(wantLines)-p &&
		haveLines[len(haveLines)-1-s] == wantLines[len(wantLines)-1-s] {
		s++
	}
	removed := haveLines[p : len(haveLines)-s]
	added := wantLines[p : len(wantLines)-s]

	var b strings.Builder
	slash := filepath.ToSlash(path)
	fmt.Fprintf(&b, "--- %s (on disk)\n", slash)
	fmt.Fprintf(&b, "+++ %s (agnostic-ai sync)\n", slash)
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", p+1, len(removed), p+1, len(added))

	shown := 0
	body := len(removed) + len(added)
	write := func(mark string, lines []string) {
		for _, ln := range lines {
			if shown >= maxLines {
				return
			}
			fmt.Fprintf(&b, "%s%s\n", mark, ln)
			shown++
		}
	}
	write("-", removed)
	write("+", added)
	if body > maxLines {
		fmt.Fprintf(&b, "... %d more changed line(s) truncated\n", body-maxLines)
	}
	return b.String()
}

// firstChangedLine returns the 1-based line where the on-disk file at path
// first differs from want, or 1 when the file is unreadable. Only used to aim
// a github annotation; never load-bearing.
func firstChangedLine(path, want string) int {
	disk, err := os.ReadFile(path)
	if err != nil {
		return 1
	}
	return commonPrefix(splitLines(string(disk)), splitLines(want)) + 1
}

// commonPrefix returns the count of leading lines a and b share.
func commonPrefix(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// githubData escapes a value for the message body of a github workflow
// command. `%` is escaped first so later escapes are not re-escaped.
func githubData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// githubProp escapes a value used as a github workflow command property
// (file=...). Properties additionally escape `:` and `,`.
func githubProp(s string) string {
	s = githubData(filepath.ToSlash(s))
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}

// resolveGitignore picks the effective gitignore mode: the --gitignore
// flag wins when set, otherwise cfg.Gitignore.Enabled.
func resolveGitignore(cfg *config.Config, flag string) bool {
	switch flag {
	case "":
		return cfg.Gitignore.Enabled
	case "on":
		return true
	case "off":
		return false
	}
	return cfg.Gitignore.Enabled
}

// validateGitignoreFlag returns nil for "", "on", or "off"; otherwise an
// error. Used by sync to fail fast on bad input rather than silently
// falling back to config.
func validateGitignoreFlag(flag string) error {
	switch flag {
	case "", "on", "off":
		return nil
	}
	return fmt.Errorf("--gitignore: expected 'on' or 'off', got %q", flag)
}
