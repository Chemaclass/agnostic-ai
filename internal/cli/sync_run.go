package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
// Bumped to 2 when the per-sync output ledger (Outputs) was added, and to 3
// when OutputSums and Orphans were added, to 4 when SpecSums was added,
// to 5 when Unledgered was added, to 6 when Merged was added, and to 7
// when Merged covered Aider's YAML config.
// Readers tolerate older versions by treating missing fields as zero values.
const syncStateVersion = mergedYAMLLedgerVersion

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
	// OutputSums maps each ledgered path to the sha256 of the bytes sync
	// wrote. The sweep removes a file without the provenance header only
	// while its bytes still match, so a hand-edited leftover survives
	// (#785), and `sync --check` reports a file whose bytes no longer
	// match as edited rather than out of date (#1270). Older state may
	// lack the sum of a file with the header; check reads such a file
	// as out of date.
	OutputSums map[string]string `json:"output_sums,omitempty"`
	// Orphans lists ledgered paths the last sync no longer emits but
	// could not remove (edited since, or no recorded sum). They stay in
	// Outputs so the next sync retries them, and `sync --check` and
	// `doctor` report them as drift while they remain on disk.
	Orphans []string `json:"orphans,omitempty"`
	// Unledgered lists leftovers a sync run without a prior ledger found
	// (unledgeredReport). No record proves sync wrote them, so they stay
	// out of Outputs and no sync removes them; `sync --check` and
	// `doctor` report them while they remain (#1354).
	Unledgered []string `json:"unledgered,omitempty"`
	// Merged records the ledgered JSON files sync merges into beside
	// keys it did not write. Once sync stops writing one, it takes out
	// only these keys instead of deleting the file (#1541).
	Merged map[string]mergedOutput `json:"merged,omitempty"`
	// SpecSums fingerprints each source spec and the merged config, so
	// the next sync can name which sources changed since this one.
	SpecSums map[string]string `json:"spec_sums,omitempty"`
	// SpecFileSums maps each spec file, slash-form and relative to the
	// project, to the sha256 of its raw bytes when sync rendered it or
	// import wrote it, so import can tell a spec a tool already reads
	// from one it would replace unseen (#1620).
	SpecFileSums map[string]specFileSum `json:"spec_file_sums,omitempty"`
	// ModelAliases records the id each vendor model alias resolved to,
	// by target, so the next sync can say when an upgrade moved one.
	ModelAliases map[string]map[string]string `json:"model_aliases,omitempty"`
	// Backups maps each `<path>.bak` sync made to the sum of its bytes, so
	// import can tell it from a skill's own asset while it is unchanged.
	Backups map[string]string `json:"backups,omitempty"`
	// Listed names the targets a sync has shown what they read. A project
	// whose first sync covered only some targets lists the others once
	// later; a project synced before this field lists none.
	// Written even when empty, so an empty list stays apart from a
	// ledger older than the field.
	Listed []string `json:"listed"`
	// PendingImports lists the tools `use` added to targets before their
	// own config was imported. A sync waits until `use` finishes them, so
	// it never writes over native config nothing has imported.
	PendingImports []string `json:"pending_imports,omitempty"`
}

// showRepeatedDrops lets -v print capability warnings and coverage notes
// that match the previous sync. Watch mode clears it after its first pass.
var showRepeatedDrops = true

// syncLedger is the output footprint one sync persists to the state file.
type syncLedger struct {
	outputs []string
	sums    map[string]string
	orphans []string
	// unledgered lists the leftovers no ledger proves sync wrote.
	unledgered []string
	merged     map[string]mergedOutput
	// specSums is not part of the output footprint, but it is written
	// beside it so the next sync can diff sources against this one.
	specSums     map[string]string
	specFileSums map[string]specFileSum
	modelAliases map[string]map[string]string
	backups      map[string]string
	listed       []string
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

// readStateFileStrict is readStateFile that reports a state file that
// exists but cannot be read or parsed, for a caller about to write it.
func readStateFileStrict(projectRoot string) (syncStateFile, error) {
	var s syncStateFile
	p := stateFilePath(projectRoot)
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read %s: %w", p, err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse %s: %w", p, err)
	}
	return s, nil
}

func writeStateFile(projectRoot string, filesChanged int, warningsDigest, notesDigest string, ledger syncLedger) error {
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
		Outputs:        ledger.outputs,
		OutputSums:     ledger.sums,
		Orphans:        ledger.orphans,
		Unledgered:     ledger.unledgered,
		Merged:         ledger.merged,
		SpecSums:       ledger.specSums,
		SpecFileSums:   ledger.specFileSums,
		ModelAliases:   ledger.modelAliases,
		Backups:        ledger.backups,
		Listed:         ledger.listed,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// recordOutputSums merges sums of files written outside a sync, such as
// by `doctor --fix`, into the state file, so the next check does not
// read them as hand edits. With no readable state file there is no
// ledger to update; the next sync writes one.
func recordOutputSums(projectRoot string, sums map[string]string) error {
	if len(sums) == 0 {
		return nil
	}
	p := stateFilePath(projectRoot)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var s syncStateFile
	if json.Unmarshal(data, &s) != nil {
		return nil
	}
	if s.OutputSums == nil {
		s.OutputSums = map[string]string{}
	}
	maps.Copy(s.OutputSums, sums)
	if data, err = json.Marshal(s); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

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
	err = adapters.EmitWithProvenance(sess, adapter, b, cfg, dryRun)
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
func emitTargetsConcurrent(targets []string, b spec.Bundle, cfg *config.Config, dryRun, backup, gitignoreOn, failFast bool, jobs int, edits editGuard) ([]targetEmit, []*adapters.Session, error) {
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
				sessions[idx] = sess
				if backup {
					sess.SetBackup(true)
				}
				edits.apply(sess)
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

func runSyncOnce(root string, targets []string, dryRun, backup bool, gitignoreFlag string, jobs int) error {
	return runSyncPass(root, targets, dryRun, backup, false, false, gitignoreFlag, jobs)
}

// runSyncPass is one sync pass. With keepEdits, an output edited since the
// last sync is left in place and named, and the rest is written. With
// untrack, a generated path git both tracks and ignores is removed from
// the index (not the working tree) after the sweep.
func runSyncPass(root string, targets []string, dryRun, backup, keepEdits, untrack bool, gitignoreFlag string, jobs int) (retErr error) {
	start := time.Now()
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
	adapters.TakeResolvedAliases()
	cfg, b, err := loadProject(root)
	if err != nil {
		return err
	}
	effectiveTargets := targets
	if len(effectiveTargets) == 0 {
		effectiveTargets = cfg.Targets
	}
	if err := stopOnSpecTypos(b, append(slices.Clone(cfg.Targets), effectiveTargets...)); err != nil {
		return err
	}
	if err := stopOnPendingImports(root, cfg); err != nil {
		return err
	}
	if err := keepHandWrittenInstructions(cfg, b, effectiveTargets, readStateFile(root).Outputs, backup, keepEdits); err != nil {
		return err
	}
	if err := detectCollisions(cfg, b, effectiveTargets); err != nil {
		return err
	}
	// A mistyped key parses and emits, so without this line nothing in a
	// sync says the rule or agent lost its setting. Pack specs are not the
	// user's to edit; lint still reports them.
	var own []spec.Entry
	for _, e := range b.All() {
		if !strings.HasPrefix(e.Layer, "pack:") {
			own = append(own, e)
		}
	}
	for _, f := range lintNearMissKeys(own, cfg.Targets) {
		summaryf("%s %s: %s\n", bang(), filepath.ToSlash(f.Path), f.Message)
	}
	// Targets read a malformed globs as none, so the rule loads in every
	// session. A scoped rule already fails its scope check instead.
	var unscoped []spec.Entry
	for _, e := range own {
		if e.Kind == spec.KindRule && e.EffectiveScope() == "" {
			unscoped = append(unscoped, e)
		}
	}
	// An agent may name a user or plugin skill sync cannot see, so a
	// likely typo of a project skill only warns.
	for _, is := range agentSkillTypos(ownSpecs(b)) {
		summaryf("%s %s: %s\n", bang(), is.Path, is.Message)
	}
	for _, f := range lintMalformedGlobs(unscoped) {
		summaryf("%s %s: %s (%s); the rule loads in every session\n", bang(), filepath.ToSlash(f.Path), f.Message, f.Code)
	}
	for _, c := range globalNameClashes(b, effectiveTargets) {
		if c.silencedBy(cfg.Sync) == "" {
			summaryf("%s %s\n", bang(), c)
		}
	}
	shared, err := planSharedSkills(cfg, b, effectiveTargets)
	if err != nil {
		return err
	}
	prev := readStateFile(root)
	edits := editGuardFor(keepEdits, prev)

	// mainSess drives the serial post-emission writes (entry points,
	// shared-skill links, orphan sweep). Each concurrent target owns its
	// own session; sessions collects every session with a rollback log in
	// write order, so the deferred rollback undoes the latest phase first.
	mainSess := adapters.NewSession()
	mainSess.SetUnmanaged(cfg.Sync.Unmanaged)
	if backup {
		mainSess.SetBackup(true)
	}
	edits.apply(mainSess)
	var sessions []*adapters.Session
	// No session logs the ignore files, so the rollback restores them.
	var ignoreFiles []priorFile
	// writesCompleted marks the point past which a returned error (the
	// --untrack index cleanup, below) is a real, already-written sync
	// reported as failed, never grounds to undo the writes themselves.
	var writesCompleted bool
	if !dryRun {
		mainSess.StartTransaction()
		defer func() {
			if retErr != nil && !writesCompleted {
				fmt.Fprintf(os.Stderr, "! sync failed; rolling back partial writes\n")
				if rbErr := errors.Join(rollbackSessions(sessions), restoreFiles(ignoreFiles)); rbErr != nil {
					fmt.Fprintf(os.Stderr, "! rollback: %v\n", rbErr)
				}
			} else {
				commitSessions(sessions)
			}
		}()
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)

	reconciled, err := shared.reconcile(prev.Outputs, dryRun)
	sessions = append(sessions, reconciled) // written first (link removals), rolled back last
	if err != nil {
		return err
	}

	// Emit every target concurrently (bounded by jobs) on its own session,
	// collecting per-target results in stable order. The first emit error
	// cancels the group and trips the rollback above.
	emits, targetSessions, emitErr := emitTargetsConcurrent(effectiveTargets, b, cfg, dryRun, backup, gitignoreOn, true, jobs, edits)
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
	accepted, notesErr := applyCoverageAccept(cfg, effectiveTargets)
	if notesErr != nil {
		flushFailedCoverageNotes()
		return notesErr
	}

	verbose := verbosity >= levelVerbose
	var report syncReport
	var ledgerSession []string
	ledgerWritten := map[string]string{}
	ledgerMergedWrites := map[string]mergedOutput{}
	var gitignoreEntries []string
	complete := true
	var emitted []string
	for _, e := range emits {
		if e.err != nil && !e.resolved {
			fmt.Fprintf(os.Stderr, "! %v\n", e.err)
			complete = false
			continue
		}
		emitted = append(emitted, e.target)
		gitignoreEntries = append(gitignoreEntries, e.recorded...)
		if dryRun {
			continue
		}
		recordLedgerWrites(e.writes, &ledgerSession, ledgerWritten)
		recordMergedWrites(e.writes, ledgerMergedWrites)
		report.addWrites(e.target, e.writes)
		if verbose {
			created, updated, skipped := classifyDetailedWrites(e.writes)
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
		recordLedgerWrites(entryWrites, &ledgerSession, ledgerWritten)
		report.addWrites("", entryWrites)
		manifestWrites, err := writeOutputManifest(mainSess, cfg, ledgerWritten, complete && coversAllConfiguredTargets(effectiveTargets, cfg.Targets), dryRun)
		if err != nil {
			return err
		}
		recordLedgerWrites(manifestWrites, &ledgerSession, ledgerWritten)
		report.addWrites("", manifestWrites)
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
	ledger, kept, removed, stripped, sweepErr := sweepAndFinalizeLedger(mainSess, prev, ledgerSession, ledgerWritten, ledgerMergedWrites, effectiveTargets, cfg.Targets, dryRun)
	if gitignoreOn {
		for _, l := range applied {
			gitignoreEntries = append(gitignoreEntries, l.path)
		}

		retained := ledger.orphans
		if sweepErr != nil || !coversAllConfiguredTargets(effectiveTargets, cfg.Targets) {
			retained = ledger.outputs
		}
		for _, path := range retained {
			if !cfg.IsUnmanaged(path) {
				gitignoreEntries = append(gitignoreEntries, path)
			}
		}
		block, err := syncManagedBlock(root, cfg, b, effectiveTargets, gitignoreEntries)
		if err != nil {
			return err
		}
		ignoreFiles = priorIgnoreFiles(root, cfg)
		rel, changed, err := writeGitignoreBlock(root, cfg, block)
		if err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
		if changed {
			report.updated = append(report.updated, filepath.ToSlash(rel))
		}
		if changed, err := writeWorktreeInclude(root, cfg, block); err != nil {
			return fmt.Errorf("worktreeinclude: %w", err)
		} else if changed {
			report.updated = append(report.updated, worktreeIncludeFile)
		}
	}

	// Concurrent emission appends capability warnings / coverage notes in
	// completion order; re-sort them to the target sequence so the flushed
	// output below is byte-identical regardless of --jobs.
	adapters.OrderBufferedDropsByTarget(effectiveTargets)

	digest := adapters.CapabilityWarningsDigest()
	notesDigest := adapters.CoverageNotesDigest()
	reshow := verbose && showRepeatedDrops
	warningsUnchanged := !reshow && digest != "" && digest == prev.WarningsDigest
	notesUnchanged := !reshow && notesDigest != "" && notesDigest == prev.NotesDigest
	// Render the per-target summary only when at least one of the buffers
	// actually changed, so it honors the same unchanged-since-last-sync
	// suppression as the kind-grouped flushes below instead of re-printing
	// every run. An empty buffer counts as unchanged when it was empty last
	// time too. Must run before the flushes clear the buffers.
	dropsChanged := reshow || digest != prev.WarningsDigest || notesDigest != prev.NotesDigest
	if cfg.Sync.DroppedSummary && verbosity >= levelDefault && dropsChanged {
		adapters.RenderDroppedSummary(logOut)
	}
	var hidden []string
	if warningsUnchanged {
		n := adapters.PendingCapabilityWarningsCount()
		hidden = append(hidden, fmt.Sprintf("%d capability warning%s", n, plural(n)))
		adapters.ResetCapabilityWarnings()
	} else {
		adapters.FlushCapabilityWarnings()
	}
	if notesUnchanged {
		n := adapters.PendingCoverageNotesCount()
		hidden = append(hidden, fmt.Sprintf("%d coverage note%s", n, plural(n)))
		adapters.DiscardCoverageNotes()
	} else {
		adapters.FlushCoverageNotes()
	}
	if len(hidden) > 0 {
		summaryf("  (%s unchanged since last sync; -v shows them)\n", strings.Join(hidden, " and "))
	}
	if verbose {
		adapters.PrintAcceptedNotes(accepted)
	}
	if sweepErr != nil {
		fmt.Fprintf(os.Stderr, "! orphan sweep: %v\n", sweepErr)
	}
	for _, p := range removed {
		report.removed = append(report.removed, filepath.ToSlash(p))
	}
	for _, p := range stripped {
		report.updated = append(report.updated, filepath.ToSlash(p))
	}
	for _, p := range kept {
		reason := keptOrphanReason(ledger.merged, prev.OutputSums, p)
		keptf("  ~ kept orphan %s (%s; run `agnostic-ai doctor --fix` to choose removal, or list it under sync.unmanaged)\n", p, reason)
	}
	if !dryRun {
		unledgered := keepUnledgered(cfg, prev, &ledger, ledgerWritten, complete && coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
		for _, p := range unledgered.Leftover {
			keptf("  ~ kept leftover %s (no ledger proves sync wrote it; run `agnostic-ai doctor --fix` to remove it)\n", filepath.ToSlash(p))
		}
		for _, p := range unledgered.Orphaned {
			keptf("  ~ kept leftover %s (looks generated, with no ledger to prove sync wrote it; delete it by hand if stale, or list it under sync.unmanaged)\n", filepath.ToSlash(p))
		}
	}
	for _, p := range sessionPaths(sessions, (*adapters.Session).KeptEdits) {
		keptf("  ~ kept %s (edited since the last sync; move the edit into .agnostic-ai/, then run `agnostic-ai sync`)\n", p)
	}
	for _, p := range sessionPaths(sessions, (*adapters.Session).OverwroteEdits) {
		keptf("%s overwrote a hand edit to %s (saved as %s.bak); move the edit into .agnostic-ai/\n", bang(), p, p)
	}
	for _, p := range sessionPaths(sessions, (*adapters.Session).BackupBlockedEdits) {
		keptf("%s kept a hand edit to %s: %s.bak already holds an earlier one; move both into .agnostic-ai/, then delete the .bak\n", bang(), p, p)
	}
	// After the sweep, so refused orphan removals are reported too.
	for _, p := range unmanagedSkips(sessions) {
		summaryf("  ~ skip (unmanaged) %s\n", p)
	}
	if dryRun {
		summaryf("%s would sync %d target%s · %s\n", tick(), len(effectiveTargets), plural(len(effectiveTargets)), shortDuration(time.Since(start)))
		return nil
	}
	// Every write this sync makes is done; an --untrack failure below
	// reports as a failed run but never undoes them (see writesCompleted
	// above).
	writesCompleted = true
	sums := specSums(cfg, b)
	report.specs = diffSpecSums(prev.SpecSums, sums)
	// A partial run leaves other targets on older specs; keep the old
	// baseline so the run that updates them still names the change.
	ledger.specSums = prev.SpecSums
	if coversAllConfiguredTargets(effectiveTargets, cfg.Targets) {
		ledger.specSums = sums
	}
	ledger.specFileSums = syncedSpecFileSums(root, prev.SpecFileSums, b, importProvenanceTargets(emits, targetSessions, mainSess), cfg)
	trackedIgnored := gitTrackedAndIgnored(root, trackedIgnoreCandidates(cfg, ledger.outputs))
	var untrackErr error
	if untrack && len(trackedIgnored) > 0 {
		removed, err := gitRmCached(root, trackedIgnored)
		report.untracked = removed
		if len(removed) > 0 && verbosity < levelDefault {
			keptf("  %s %s\n", bang(), untrackPullAdvice)
		}
		trackedIgnored = removeMatching(trackedIgnored, removed)
		if err != nil {
			untrackErr = fmt.Errorf("untrack: %w", err)
		}
	}
	report.trackedIgnored = trackedIgnored
	if verbosity >= levelDefault {
		report.pending = removeMatching(gitPending(root, report.changedPaths()), report.trackedIgnored)
	}
	// --quiet prints no notes, so it records only new aliases and leaves a
	// moved one for the next sync that does to report.
	resolvedAliases := adapters.TakeResolvedAliases()
	if verbosity >= levelDefault {
		for _, note := range movedAliasNotes(prev.ModelAliases, resolvedAliases) {
			summaryf("  note: %s\n", note)
		}
		ledger.modelAliases = nextModelAliases(prev.ModelAliases, resolvedAliases, emitted)
	} else {
		ledger.modelAliases = addedModelAliases(prev.ModelAliases, resolvedAliases, emitted)
	}
	ledger.backups = syncBackups(prev.Backups, sessionPaths(sessions, (*adapters.Session).Backups))
	toList := unlistedTargets(prev, intersect(effectiveTargets, emitted))
	ledger.listed = carriedListed(prev)
	if verbosity >= levelDefault && len(toList) > 0 {
		ledger.listed = append(slices.Clone(ledger.listed), toList...)
	}
	if err := writeStateFile(root, report.filesChanged(), digest, notesDigest, ledger); err != nil {
		fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
	}
	if verbosity >= levelDefault {
		report.render(logOut, len(effectiveTargets), time.Since(start), verbose)
		// The first sync is where the setup pays off, so it shows what
		// each tool now reads from the one source.
		if len(toList) > 0 {
			printToolReads(logOut, cfg, b, toList)
		}
	}
	return untrackErr
}

// unlistedTargets returns the targets whose first sync has not shown
// what they read yet: all of them on a project's first sync, then the ones
// a partial first sync left out.
func unlistedTargets(prev syncStateFile, targets []string) []string {
	if len(prev.Outputs) > 0 && prev.Listed == nil {
		return nil
	}
	var out []string
	for _, t := range targets {
		if !slices.Contains(prev.Listed, t) {
			out = append(out, t)
		}
	}
	return out
}

// carriedListed is the listed set a sync keeps when it shows nothing: an
// older ledger stays older, and any other starts as an empty list.
func carriedListed(prev syncStateFile) []string {
	if prev.Listed == nil && len(prev.Outputs) > 0 {
		return nil
	}
	if prev.Listed == nil {
		return []string{}
	}
	return prev.Listed
}

// unmanagedSkips merges the user-owned paths every session refused to
// touch, deduplicated and sorted. A shared path reaches several target
// sessions, so the per-session lists overlap. Nil sessions (a target that
// never started under fail-fast) are skipped.
func unmanagedSkips(sessions []*adapters.Session) []string {
	seen := map[string]struct{}{}
	for _, s := range sessions {
		if s == nil {
			continue
		}
		for _, p := range s.UnmanagedSkips() {
			seen[filepath.ToSlash(p)] = struct{}{}
		}
	}
	return sortedKeys(seen)
}

// editGuard is how a sync treats an output edited by hand since the
// last sync: --keep-edits leaves it in place, and otherwise sync keeps it
// as `<path>.bak` before writing over it.
type editGuard struct {
	keep bool
	sums map[string]string
}

// editGuardFor compares outputs against the sums the last sync recorded.
// Under --keep-edits a path the ledger has no sum for is compared with
// the version Git committed (#1397); with no ledger at all, the default
// backup has no proof of an edit and stays off.
func editGuardFor(keepEdits bool, prev syncStateFile) editGuard {
	if keepEdits && prev.OutputSums == nil {
		return editGuard{keep: true, sums: map[string]string{}}
	}
	return editGuard{keep: keepEdits, sums: prev.OutputSums}
}

func (g editGuard) apply(sess *adapters.Session) {
	switch {
	case g.keep:
		sess.KeepEditsSince(g.sums)
		sess.SetCommittedSum(committedSum)
	case g.sums != nil:
		sess.BackUpEditsSince(g.sums)
		sess.SetCommittedSum(committedSum)
	}
}

// committedSum returns the content sum of the version of path at HEAD, or
// "" when path is a link, untracked, or outside a git work tree.
func committedSum(path string) string {
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	blob, ok := runGitWithin(".", 10*time.Second, "show", "HEAD:./"+filepath.ToSlash(path))
	if !ok {
		return ""
	}
	return adapters.ContentSum(blob)
}

// syncBackups returns the backups sync made that still hold what it
// wrote, plus the ones this run made, each with the sum of its bytes.
func syncBackups(prior map[string]string, made []string) map[string]string {
	out := map[string]string{}
	for b, sum := range prior {
		if fileSum(b) == sum {
			out[b] = sum
		}
	}
	for _, b := range made {
		if sum := fileSum(b); sum != "" {
			out[b] = sum
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fileSum is the content sum of the regular file at path, "" when there
// is none.
func fileSum(path string) string {
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return adapters.ContentSum(string(data))
}

// sessionPaths collects the paths paths returns for each session,
// deduplicated and sorted, since targets sharing a path each report it.
func sessionPaths(sessions []*adapters.Session, paths func(*adapters.Session) []string) []string {
	seen := map[string]struct{}{}
	for _, s := range sessions {
		if s == nil {
			continue
		}
		for _, p := range paths(s) {
			seen[filepath.ToSlash(p)] = struct{}{}
		}
	}
	return sortedKeys(seen)
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
		rec := fileRecord{Target: target, Path: f.Path, Action: f.Action, Bytes: f.Bytes, Backup: f.Backup}
		if f.Action == "skip" || f.Action == "edited" {
			out.Skipped = append(out.Skipped, rec)
		} else {
			out.Writes = append(out.Writes, rec)
		}
	}
}

// runSyncJSON runs a real sync pass and emits a JSON result describing each
// file written, updated, or skipped per target.
func runSyncJSON(cmd *cobra.Command, root string, targets []string, backup, keepEdits, untrack bool, gitignoreFlag string, jobs int) error {
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
	if err := stopOnSpecTypos(b, append(slices.Clone(cfg.Targets), effectiveTargets...)); err != nil {
		return err
	}
	if err := stopOnPendingImports(root, cfg); err != nil {
		return err
	}
	if err := keepHandWrittenInstructions(cfg, b, effectiveTargets, readStateFile(root).Outputs, backup, keepEdits); err != nil {
		return err
	}
	if err := detectCollisions(cfg, b, effectiveTargets); err != nil {
		return err
	}
	shared, err := planSharedSkills(cfg, b, effectiveTargets)
	if err != nil {
		return err
	}
	prev := readStateFile(root)
	edits := editGuardFor(keepEdits, prev)

	// mainSess handles the serial entry-point and shared-link writes; each
	// target emits on its own session. The JSON path does not roll back
	// writes, since targets share paths such as AGENTS.md: it reports a
	// failed target's error and the writes it made before it. Only the
	// orphan sweep and the ignore files are undone, when an ignore file
	// fails after the sweep, and a .gitignore written by then keeps
	// ignoring the outputs that stay.
	mainSess := adapters.NewSession()
	mainSess.SetUnmanaged(cfg.Sync.Unmanaged)
	if backup {
		mainSess.SetBackup(true)
	}
	edits.apply(mainSess)
	gitignoreOn := resolveGitignore(cfg, gitignoreFlag)
	reconciled, err := shared.reconcile(prev.Outputs, false)
	if err != nil {
		return err
	}

	out := jsonOutput{Version: "1", Command: "sync"}
	var ledgerSession []string
	ledgerWritten := map[string]string{}
	ledgerMergedWrites := map[string]mergedOutput{}
	var gitignoreEntries []string

	// Emit every target concurrently; failFast is off so all per-target
	// errors ride along in the results and are reported in target order.
	emits, sessions, _ := emitTargetsConcurrent(effectiveTargets, b, cfg, false, backup, gitignoreOn, false, jobs, edits)
	sessions = append(sessions, mainSess)
	normalizeSharedWriteAttribution(emits)
	_, notesErr := applyCoverageAccept(cfg, effectiveTargets)
	drops := syncJSONOutput{syncDrops: pendingSyncDrops()}
	if notesErr != nil {
		flushFailedCoverageNotes()
		if rbErr := rollbackSessions(append([]*adapters.Session{reconciled}, sessions...)); rbErr != nil {
			fmt.Fprintf(os.Stderr, "! rollback: %v\n", rbErr)
		}
		out.addError(notesErr)
		drops.jsonOutput = out.forOutput()
		if err := writeIndentedJSON(cmd, drops); err != nil {
			return err
		}
		return notesErr
	}
	// A failed target stays on disk with the writes it made before the
	// error, so they are reported and ledgered like any other. The sweep
	// must not treat it as emitted: its earlier outputs would read as
	// orphans and go.
	var emitted []string
	for _, e := range emits {
		if e.err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: e.target, Message: e.err.Error()})
		} else {
			emitted = append(emitted, e.target)
		}
		gitignoreEntries = append(gitignoreEntries, e.recorded...)
		recordLedgerWrites(e.writes, &ledgerSession, ledgerWritten)
		recordMergedWrites(e.writes, ledgerMergedWrites)
		appendFileRecords(&out, e.target, e.writes)
	}

	if gitignoreOn {
		mainSess.StartRecording()
	}
	mainSess.StartDetailedRecording()
	if err := writeAgnosticEntryPoints(mainSess, cfg, b, effectiveTargets, false); err != nil {
		mainSess.StopDetailedRecording()
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
	} else {
		entryWrites := mainSess.StopDetailedRecording()
		recordLedgerWrites(entryWrites, &ledgerSession, ledgerWritten)
		appendFileRecords(&out, "agnostic-ai", entryWrites)
		manifestWrites, err := writeOutputManifest(mainSess, cfg, ledgerWritten, len(out.Errors) == 0 && coversAllConfiguredTargets(effectiveTargets, cfg.Targets), false)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
		}
		recordLedgerWrites(manifestWrites, &ledgerSession, ledgerWritten)
		appendFileRecords(&out, "agnostic-ai", manifestWrites)
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

	applied := shared.apply(mainSess, false)
	ledgerSession = adjustLedgerForLinks(ledgerSession, applied)
	mainSess.StartTransaction()
	ledger, kept, removed, stripped, sweepErr := sweepAndFinalizeLedger(mainSess, prev, ledgerSession, ledgerWritten, ledgerMergedWrites, emitted, cfg.Targets, false)
	for _, l := range applied {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: l.path, Action: "link"})
	}
	if gitignoreOn {
		for _, l := range applied {
			gitignoreEntries = append(gitignoreEntries, l.path)
		}

		retained := ledger.orphans
		if sweepErr != nil || !coversAllConfiguredTargets(emitted, cfg.Targets) {
			retained = ledger.outputs
		}
		for _, path := range retained {
			if !cfg.IsUnmanaged(path) {
				gitignoreEntries = append(gitignoreEntries, path)
			}
		}
		block, err := syncManagedBlock(root, cfg, b, effectiveTargets, gitignoreEntries)
		if err != nil {
			return undoSweep(mainSess, nil, err)
		}
		prior := priorIgnoreFiles(root, cfg)
		if err := updateGitignore(root, cfg, block); err != nil {
			return undoSweep(mainSess, prior, fmt.Errorf("gitignore: %w", err))
		}
		if _, err := writeWorktreeInclude(root, cfg, block); err != nil {
			return undoSweep(mainSess, widenIgnores(prior, filepath.Join(root, gitignoreRel(cfg)), block), fmt.Errorf("worktreeinclude: %w", err))
		}
	}
	mainSess.Commit()
	if sweepErr != nil {
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: sweepErr.Error()})
	}
	for _, p := range removed {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: "delete"})
	}
	for _, p := range stripped {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: "update"})
	}
	for _, p := range kept {
		out.Skipped = append(out.Skipped, fileRecord{Target: "agnostic-ai", Path: p, Action: "orphan"})
	}
	unledgered := keepUnledgered(cfg, prev, &ledger, ledgerWritten, len(out.Errors) == 0 && coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
	for _, p := range unledgered.Leftover {
		out.Skipped = append(out.Skipped, fileRecord{Target: unledgered.Target, Path: filepath.ToSlash(p), Action: "leftover"})
	}
	for _, p := range unledgered.Orphaned {
		out.Skipped = append(out.Skipped, fileRecord{Target: unledgered.Target, Path: filepath.ToSlash(p), Action: "orphan"})
	}
	for _, p := range unmanagedSkips(sessions) {
		out.Skipped = append(out.Skipped, fileRecord{Target: "agnostic-ai", Path: p, Action: "unmanaged"})
	}
	trackedIgnored := gitTrackedAndIgnored(root, trackedIgnoreCandidates(cfg, ledger.outputs))
	if untrack && len(trackedIgnored) > 0 {
		removedFromIndex, err := gitRmCached(root, trackedIgnored)
		for _, p := range removedFromIndex {
			out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: "untracked"})
		}
		if len(removedFromIndex) > 0 {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  ! %s\n", untrackPullAdvice)
		}
		trackedIgnored = removeMatching(trackedIgnored, removedFromIndex)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: fmt.Sprintf("untrack: %v", err)})
		}
	}
	for _, p := range trackedIgnored {
		out.Skipped = append(out.Skipped, fileRecord{Target: "agnostic-ai", Path: p, Action: "tracked"})
	}
	// Warnings and notes go into the JSON, not to the terminal, so keep
	// the previous digests: the next plain sync still prints them.
	ledger.specSums = prev.SpecSums
	ledger.modelAliases = prev.ModelAliases
	if len(out.Errors) == 0 && coversAllConfiguredTargets(effectiveTargets, cfg.Targets) {
		ledger.specSums = specSums(cfg, b)
	}
	ledger.specFileSums = prev.SpecFileSums
	if len(out.Errors) == 0 {
		ledger.specFileSums = syncedSpecFileSums(root, prev.SpecFileSums, b, importProvenanceTargets(emits, sessions, mainSess), cfg)
	}
	ledger.backups = syncBackups(prev.Backups, sessionPaths(sessions, (*adapters.Session).Backups))
	ledger.listed = carriedListed(prev)
	if err := writeStateFile(root, len(out.Writes), prev.WarningsDigest, prev.NotesDigest, ledger); err != nil {
		fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
	}
	drops.jsonOutput = out.forOutput()
	return writeIndentedJSON(cmd, drops)
}

// syncJSONOutput is the `sync --json` output: the shared schema plus the
// capability warnings and coverage notes a plain sync prints.
type syncJSONOutput struct {
	jsonOutput
	syncDrops
}

type syncDrops struct {
	Warnings []dropRecord `json:"warnings"`
	Notes    []dropRecord `json:"notes"`
}

// pendingSyncDrops returns the buffered capability warnings and coverage
// notes, ordered by target, without clearing them.
func pendingSyncDrops() syncDrops {
	return syncDrops{
		Warnings: dropRecords(adapters.PendingCapabilityWarnings()),
		Notes:    dropRecords(adapters.PendingCoverageNotes()),
	}
}

func dropRecords(records []adapters.DropRecord) []dropRecord {
	out := make([]dropRecord, 0, len(records))
	for _, r := range records {
		out = append(out, dropRecord{Target: r.Target, Kind: string(r.Kind), Count: r.Count, Message: r.Message})
	}
	return out
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
		_, _ = fmt.Fprintf(w, "[%s]\tadded: %d\tchanged: %d", r.Target, len(r.Missing), len(r.changed()))
		if len(r.Orphaned) > 0 {
			_, _ = fmt.Fprintf(w, "\torphaned: %d", len(r.Orphaned))
		}
		switch {
		case len(r.Leftover) > 0 && r.Unledgered:
			_, _ = fmt.Fprintf(w, "\tleftover: %d", len(r.Leftover))
		case len(r.Leftover) > 0:
			_, _ = fmt.Fprintf(w, "\tremoved: %d", len(r.Leftover))
		}
		_, _ = fmt.Fprintln(w)
	}
	_ = w.Flush()
}

// printSyncPlanJSON emits what a sync would do in the `sync --json`
// schema, with the create, update, and delete actions a real run reports,
// and kept orphans and leftovers in skipped. A dry run also lists each
// unchanged file as a skip, so every planned output appears once.
func printSyncPlanJSON(cmd *cobra.Command, command string, reports []driftReport, withCurrent bool, drops syncDrops, notesErr error) error {
	out := jsonOutput{Version: "1", Command: command}
	out.addError(notesErr)
	for _, r := range reports {
		for _, f := range r.Missing {
			out.Writes = append(out.Writes, fileRecord{Target: r.Target, Path: f.Path, Action: "create", Bytes: len(f.Content)})
		}
		for _, f := range r.changed() {
			out.Writes = append(out.Writes, fileRecord{Target: r.Target, Path: f.Path, Action: "update", Bytes: len(f.Content)})
		}
		for _, p := range r.Leftover {
			if r.Unledgered {
				out.Skipped = append(out.Skipped, fileRecord{Target: r.Target, Path: p, Action: "leftover"})
				continue
			}
			out.Writes = append(out.Writes, fileRecord{Target: r.Target, Path: p, Action: "delete"})
		}
		for _, p := range r.Orphaned {
			out.Skipped = append(out.Skipped, fileRecord{Target: r.Target, Path: p, Action: "orphan"})
		}
		if withCurrent {
			for _, f := range r.Current {
				out.Skipped = append(out.Skipped, fileRecord{Target: r.Target, Path: f.Path, Action: "skip", Bytes: len(f.Content)})
			}
		}
	}
	if err := writeIndentedJSON(cmd, syncJSONOutput{out.forOutput(), drops}); err != nil {
		return err
	}
	return notesErr
}

// printSyncCheckJSON emits a JSON result for `sync --check`. Every drifted
// file appears in writes, with the actions driftRecords assigns.
func printSyncCheckJSON(cmd *cobra.Command, reports []driftReport, drops syncDrops, notesErr error) error {
	out := jsonOutput{Version: "1", Command: "sync --check", Writes: driftRecords(reports)}
	out.addError(notesErr)
	hasDrift := len(out.Writes) > 0
	if err := writeIndentedJSON(cmd, syncJSONOutput{out.forOutput(), drops}); err != nil {
		return err
	}
	if hasDrift {
		return errors.Join(errDriftDetected(), notesErr)
	}
	return notesErr
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
	if hint := reconcileHint(reports); hint != "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), hint)
	}
	return errDriftDetected()
}

// reconcileHint names what settles the drift in reports. doctor --fix
// removes an unledgered leftover and writes the rest; a scope document
// no ledger proves sync wrote is left to the user, so it gets a manual
// step instead of a command that would leave it in place.
func reconcileHint(reports []driftReport) string {
	var syncDrift, removable, scoped bool
	var adopt []string
	for _, r := range reports {
		for _, f := range r.Unmanaged {
			if !slices.Contains(adopt, f.Target) {
				adopt = append(adopt, f.Target)
			}
		}
		switch {
		case !r.hasDrift():
		case r.against != "" && len(r.Leftover)+len(r.Orphaned) == 0:
			// The --against step, not sync, settles it.
		case len(r.Unmanaged) > 0:
		case r.Unledgered:
			removable = removable || len(r.Leftover) > 0
			scoped = scoped || len(r.Orphaned) > 0
		default:
			syncDrift = true
		}
	}
	const manual = "delete the files that look generated, listed above, by hand if stale, or list them under sync.unmanaged"
	fix := ""
	switch {
	case removable:
		fix = "doctor --fix"
	case syncDrift:
		fix = "sync"
	}
	var hint string
	switch {
	case fix == "" && !scoped:
	case fix == "":
		hint = "to reconcile, " + manual
	case scoped:
		hint = "to reconcile, run: agnostic-ai " + fix + ", then " + manual
	default:
		hint = "to reconcile, run: agnostic-ai " + fix
	}
	if len(adopt) == 0 {
		return hint
	}
	imports := make([]string, len(adopt))
	for i, t := range adopt {
		imports[i] = "agnostic-ai import " + t
	}
	move := "to move the hand-written files into .agnostic-ai/, run: " + strings.Join(imports, ", ") + ", then git rm --cached them"
	if hint == "" {
		return move
	}
	return hint + "; " + move
}

// printDriftGitHub emits one GitHub Actions error annotation per drifted file
// so drift surfaces inline on the pull request. Missing files carry no line;
// stale and edited files point at the first changed line. Returns true if any target
// drifted, so an in-sync run emits nothing and stays silent.
func printDriftGitHub(cmd *cobra.Command, reports []driftReport) bool {
	out := cmd.OutOrStdout()
	drift := false
	for _, r := range foldSharedDrift(reports) {
		missing, stale := "run agnostic-ai sync to generate it", "run agnostic-ai sync to reconcile"
		if r.against != "" {
			where, step := againstPlace(r.against)
			missing = "is not " + where + "; " + step
			stale = "does not match the specs " + where + "; " + step
		} else {
			missing, stale = "is missing; "+missing, "drifted from specs; "+stale
		}
		for _, f := range r.Missing {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s %s%s\n",
				githubProp(f.Path), githubData(filepath.ToSlash(f.Path)), missing, githubData(r.sharedNote(f.Path)))
		}
		for _, f := range r.Stale {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s,line=%d::%s %s%s\n",
				githubProp(f.Path), firstChangedLine(f.Path, f.Content), githubData(filepath.ToSlash(f.Path)), stale, githubData(r.sharedNote(f.Path)))
		}
		for _, f := range r.Edited {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s,line=%d::%s was edited since the last sync; move the edit into .agnostic-ai/, then run agnostic-ai sync%s\n",
				githubProp(f.Path), firstChangedLine(f.Path, f.Content), githubData(filepath.ToSlash(f.Path)), githubData(r.sharedNote(f.Path)))
		}
		orphanHint := "is no longer generated and its ownership could not be proven; run agnostic-ai doctor --fix to choose removal, or list it under sync.unmanaged"
		if r.Unledgered {
			orphanHint = "looks generated, with no ledger to prove sync wrote it; delete it by hand if stale, or list it under sync.unmanaged"
		}
		for _, p := range r.Orphaned {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s %s%s\n", githubProp(p), githubData(filepath.ToSlash(p)), orphanHint, githubData(r.sharedNote(p)))
		}
		for _, p := range r.Leftover {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s is no longer generated but still loaded; run agnostic-ai %s to remove it%s\n",
				githubProp(p), githubData(filepath.ToSlash(p)), r.leftoverFix(), githubData(r.sharedNote(p)))
		}
		for _, f := range r.Unmanaged {
			drift = true
			_, _ = fmt.Fprintf(out, "::error file=%s::%s is hand-written in a generated folder, so only %s reads it; adopt it with agnostic-ai import %s, then git rm --cached it\n",
				githubProp(f.Path), githubData(f.Path), githubData(f.Target), githubData(f.Target))
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
	for _, r := range foldSharedDrift(reports) {
		for _, f := range r.Missing {
			_, _ = fmt.Fprintf(out, "would create %s (%d bytes)\n", filepath.ToSlash(f.Path), len(f.Content))
		}
		for _, f := range r.changed() {
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
	slash := filepath.ToSlash(path)
	return labeledDiff(slash+" (on disk)", slash+" (agnostic-ai sync)",
		splitLines(have), splitLines(want), maxLines)
}

// labeledDiff is unifiedDiff over line slices with caller-chosen `---` and
// `+++` labels. A nil haveLines renders a file creation.
func labeledDiff(haveLabel, wantLabel string, haveLines, wantLines []string, maxLines int) string {
	p := commonPrefix(haveLines, wantLines)
	s := 0
	for s < len(haveLines)-p && s < len(wantLines)-p &&
		haveLines[len(haveLines)-1-s] == wantLines[len(wantLines)-1-s] {
		s++
	}
	removed := haveLines[p : len(haveLines)-s]
	added := wantLines[p : len(wantLines)-s]

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", haveLabel)
	fmt.Fprintf(&b, "+++ %s\n", wantLabel)
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
	if on, err := parseSwitch(flag); err == nil {
		return on
	}
	return cfg.Gitignore.Enabled
}

// validateGitignoreFlag returns nil for "", "on", or "off"; otherwise an
// error. Used by sync to fail fast on bad input rather than silently
// falling back to config.
func validateGitignoreFlag(flag string) error {
	if flag == "" {
		return nil
	}
	if _, err := parseSwitch(flag); err != nil {
		return fmt.Errorf("--gitignore: %w", err)
	}
	return nil
}
