package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func runSyncOnce(root string, targets []string, dryRun, backup bool, gitignoreFlag string, jobs int) error {
	gitquery.Reset()
	return runSyncPass(root, targets, syncRunOptions{dryRun: dryRun, backup: backup, gitignoreFlag: gitignoreFlag, jobs: jobs})
}

// resetSyncBuffers clears what an earlier pass left in the adapter buffers.
func resetSyncBuffers() {
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
	adapters.TakeResolvedAliases()
	adapters.TakeMemorySeeds()
}

// prepareSyncTargets runs the checks and setup both `sync` and
// `sync --json` share before emitting: it queues the personal memory seed,
// stops on errors in the specs, and notes overlay files no target reads.
func prepareSyncTargets(out io.Writer, root string, prior ledgerSnapshot, cfg *config.Config, renders *targetRenders, targets []string, opts syncRunOptions) ([]string, map[string]string, error) {
	b := renders.b
	if !opts.dryRun {
		preparePersonalMemorySync(root, cfg)
	}
	if len(targets) == 0 {
		targets = cfg.Targets
	}
	if err := stopOnSpecTypos(b, append(slices.Clone(cfg.Targets), targets...)); err != nil {
		return nil, nil, err
	}
	warnBareCapabilitiesTo(out, cfg, b, targets)
	if err := stopOnPendingImports(prior.state, cfg); err != nil {
		return nil, nil, err
	}
	if err := keepHandWrittenInstructions(cfg, b, targets, prior.state.Outputs, opts.backup, opts.keepEdits); err != nil {
		return nil, nil, err
	}
	if err := keepHandWrittenMCPFiles(renders, cfg, targets, prior.state.Outputs, opts.backup, opts.keepEdits); err != nil {
		return nil, nil, err
	}
	winners, err := detectRenderedCollisions(renders, cfg, targets, root, prior.state.Backups)
	if err != nil {
		return nil, nil, err
	}
	noteUnmanagedDoubleStar(cfg, root)
	return targets, winners, noteUnreadOverlays(root)
}

// syncRunOptions are the flags one sync pass runs with.
type syncRunOptions struct {
	dryRun, backup, keepEdits, untrack bool
	gitignoreFlag                      string
	jobs                               int
	// quietWhenClean drops the report of a pass that changed nothing,
	// such as the watch's reconcile pass right after its first sync.
	quietWhenClean bool
}

// runSyncPass is one sync pass. With keepEdits, an output edited since the
// last sync is left in place and named, and the rest is written. With
// untrack, a generated path git both tracks and ignores is removed from
// the index (not the working tree) after the sweep.
func runSyncPass(root string, targets []string, opts syncRunOptions) error {
	if err := stopOnMissingPacks(root); err != nil {
		return err
	}
	return runSyncPassWith(root, readLedger(root), nil, targets, opts)
}

// projectLoad is a project the caller loaded under root. log holds the
// verbose lines the load printed while the caller held them back.
type projectLoad struct {
	cfg    *config.Config
	bundle spec.Bundle
	log    string
}

// view returns the project with a config the pass can change without
// changing the caller's.
func (p projectLoad) view() (*config.Config, spec.Bundle) {
	cfg := *p.cfg
	return &cfg, p.bundle
}

// runSyncPassWith is runSyncPass with the state file the caller read
// under root, and the project it loaded, or nil to load it here.
// Nothing writes the state file before the pass ends.
func runSyncPassWith(root string, prior ledgerSnapshot, project *projectLoad, targets []string, opts syncRunOptions) (retErr error) {
	if err := prior.refuseNewer(); err != nil {
		return err
	}
	start := time.Now()
	originalLog := logOut
	var earlyMessages strings.Builder
	if verbosity == levelVerbose && !opts.dryRun {
		logOut = &earlyMessages
		defer func() {
			logOut = originalLog
			_, _ = fmt.Fprint(originalLog, earlyMessages.String())
		}()
	}
	resetSyncBuffers()
	cfg, b, err := project.orLoad(root)
	if err != nil {
		return err
	}
	cfg.Gitignore.Enabled = resolveGitignore(cfg, opts.gitignoreFlag)
	prev := prior.state
	release := holdPriorState(root, prev)
	defer release()
	renders := newTargetRenders(cfg, b)
	effectiveTargets, winners, err := prepareSyncTargets(logOut, root, prior, cfg, renders, targets, opts)
	if err != nil {
		return err
	}
	warnOwnSpecs(cfg, b, effectiveTargets)
	shared, err := planSharedSkills(cfg, renders, effectiveTargets)
	if err != nil {
		return err
	}
	prior.warn()
	edits := editGuardFor(opts.keepEdits, prev).backingUpMCPFiles(unledgeredMCPFiles(renders, cfg, effectiveTargets, prev.Outputs))

	// Each concurrent target owns its own session; sessions collects every
	// session with a rollback log in write order, so the deferred rollback
	// undoes the latest phase first.
	mainSess := newMainSession(cfg, opts.backup, edits)
	var sessions []*adapters.Session
	// No session logs the ignore files, so the rollback restores them.
	var ignoreFiles []priorFile
	// writesCompleted marks the point past which a returned error (the
	// --untrack index cleanup, below) is a real, already-written sync
	// reported as failed, never grounds to undo the writes themselves.
	var writesCompleted bool
	if !opts.dryRun {
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
	gitignoreOn := !opts.dryRun && resolveGitignore(cfg, opts.gitignoreFlag)

	reconciled, err := shared.reconcile(prev.Outputs, prior.workdir(root).state, opts.dryRun)
	sessions = append(sessions, reconciled) // written first (link removals), rolled back last
	if err != nil {
		return err
	}

	// Emit every target concurrently (bounded by jobs) on its own session,
	// collecting per-target results in stable order. The first emit error
	// cancels the group and trips the rollback above.
	// A dry run records its writes too, so it can tell which merged
	// files the sweep it skips would release.
	recordWrites := gitignoreOn || opts.dryRun
	emits, targetSessions, emitErr := emitTargetsConcurrent(effectiveTargets, b, cfg, opts.dryRun, opts.backup, recordWrites, true, opts.jobs, edits, winners)
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

	w := collectEmits(emits, opts.dryRun)
	if err := w.writeEntryPoints(mainSess, cfg, b, effectiveTargets, prev, opts, recordWrites); err != nil {
		return err
	}
	applied := shared.apply(mainSess, opts.dryRun)
	w.ledgerSession = adjustLedgerForLinks(w.ledgerSession, applied)
	ledger, kept, removed, stripped, sweepErr := sweepAndFinalizeLedger(mainSess, prev, w.ledgerSession, w.written, w.merged, w.emitted, cfg.Targets, opts.dryRun)
	// No session writes after the sweep, so one read of the backups serves
	// the ignore files and the ledger.
	var backups map[string]string
	if !opts.dryRun {
		backups = syncBackups(prev.Backups, sessionPaths(sessions, (*adapters.Session).Backups))
	}
	if gitignoreOn {
		ignores, err := writeSyncIgnores(root, cfg, b, effectiveTargets, w.gitignore, applied, ledger, sweepErr, w.emitted, prior.missing(), backups)
		ignoreFiles = ignores.prior
		if err != nil {
			return err
		}
		w.report.updated = append(w.report.updated, ignores.changed...)
	}

	if !opts.dryRun {
		runMemorySeeds()
	}

	// Concurrent emission appends capability warnings / coverage notes in
	// completion order; re-sort them to the target sequence so the flushed
	// output below is byte-identical regardless of --jobs.
	adapters.OrderBufferedDropsByTarget(effectiveTargets)
	drops := newPassDrops(cfg, prev, accepted)
	drops.quietUnchanged = opts.quietWhenClean
	if verbosity != levelVerbose {
		drops.flush()
	}
	holdKept := verbosity == levelVerbose && !opts.dryRun
	var deferredMessages strings.Builder
	printKept := func(format string, args ...any) {
		if holdKept {
			_, _ = fmt.Fprintf(&deferredMessages, format, args...)
		} else {
			keptf(format, args...)
		}
	}
	if sweepErr != nil {
		fmt.Fprintf(os.Stderr, "! orphan sweep: %v\n", sweepErr)
	}
	if len(w.unresolved) > 0 && !opts.dryRun {
		printKept("  ~ skipped removing stale files: %s did not load; a sync that loads every target removes them\n", strings.Join(w.unresolved, ", "))
	}
	w.reportSweep(printKept, removed, stripped, kept, ledger.merged, prev.OutputSums)
	allTargets := w.complete && coversAllConfiguredTargets(effectiveTargets, cfg.Targets)
	if !opts.dryRun {
		printUnledgered(printKept, keepUnledgered(cfg, prev, prior.workdir(root).missing(), &ledger, w.written, allTargets))
	}
	printSessionEdits(printKept, sessions, cfg, effectiveTargets, holdKept)
	if opts.dryRun {
		if verbosity == levelVerbose {
			drops.flush()
		}
		if !opts.quietWhenClean {
			summaryf("%s would sync %d target%s · %s\n", tick(), len(w.emitted), plural(len(w.emitted)), shortDuration(time.Since(start)))
		}
		return nil
	}
	// Every write this sync makes is done; an --untrack failure below
	// reports as a failed run but never undoes them (see writesCompleted
	// above).
	writesCompleted = true
	base := specSums(cfg, b, cfg.Targets)
	w.report.specs = diffSpecSums(specBaseline{prev.SpecSums, prev.SpecTargets}, base, bundleSpecKeys(b))
	// A partial run leaves other targets on older specs; keep the old
	// baseline so the run that updates them still names the change.
	ledger.specSums, ledger.specTargets = prev.SpecSums, prev.SpecTargets
	if coversAllConfiguredTargets(w.emitted, cfg.Targets) {
		ledger.specSums, ledger.specTargets = base.sums, base.targets
	}
	ledger.specFileSums = syncedSpecFileSums(root, prev.SpecFileSums, b, importProvenanceTargets(emits, targetSessions, mainSess), cfg)
	trackedIgnored, untracked, untrackErr := untrackIgnored(root, cfg, ledger.outputs, opts.untrack)
	w.report.untracked = untracked
	if len(untracked) > 0 && verbosity < levelDefault {
		keptf("  %s %s\n", bang(), untrackPullAdvice)
	}
	if untrackErr != nil {
		untrackErr = fmt.Errorf("untrack: %w", untrackErr)
	}
	w.report.trackedIgnored = trackedIgnored
	if verbosity >= levelDefault {
		w.report.pending = removeMatching(gitPending(root, w.report.changedPaths()), w.report.trackedIgnored)
	}
	recordModelAliases(&ledger, prev.ModelAliases, w.emitted, printKept)
	ledger.backups = backups
	digest, notesDigest := drops.record(&ledger, w.emitted, !allTargets)
	toList := unlistedTargets(prev, intersect(effectiveTargets, w.emitted))
	ledger.listed = carriedListed(prev)
	if verbosity >= levelDefault && len(toList) > 0 {
		ledger.listed = append(slices.Clone(ledger.listed), toList...)
	}
	release()
	stateErr := writeStateFile(root, w.report.filesChanged(), digest, notesDigest, ledger)
	if stateErr != nil {
		stateErr = fmt.Errorf("sync wrote files but could not record them: %w", stateErr)
	}
	if verbosity == levelVerbose {
		logOut = originalLog
	}
	if verbosity >= levelDefault && (!opts.quietWhenClean || w.report.filesChanged() > 0 || len(toList) > 0) {
		w.render(cfg, b, toList, time.Since(start))
	}
	if verbosity == levelVerbose {
		_, _ = fmt.Fprint(logOut, earlyMessages.String())
		earlyMessages.Reset()
		_, _ = fmt.Fprint(logOut, deferredMessages.String())
		drops.flush()
	}
	return errors.Join(untrackErr, stateErr)
}

// orLoad returns the project's config and bundle, and loads the project
// under root when the caller has not.
func (p *projectLoad) orLoad(root string) (*config.Config, spec.Bundle, error) {
	if p == nil {
		return loadProject(root)
	}
	_, _ = fmt.Fprint(logOut, p.log)
	cfg, b := p.view()
	return cfg, b, nil
}

// warnOwnSpecs warns about mistakes in the project's own specs that still
// parse and emit. Pack specs are not the user's to edit; lint still
// reports them.
func warnOwnSpecs(cfg *config.Config, b spec.Bundle, targets []string) {
	// A mistyped key parses and emits, so without this line nothing in a
	// sync says the rule or agent lost its setting.
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
	for _, c := range globalNameClashes(b, targets) {
		if c.silencedBy(cfg.Sync) == "" {
			summaryf("%s %s\n", bang(), c)
		}
	}
}

// passWrites gathers what the targets and entry points of one text pass
// wrote, for the report, the ledger, and the ignore files.
type passWrites struct {
	report        syncReport
	ledgerSession []string
	written       map[string]string
	merged        map[string]mergedOutput
	gitignore     []string
	// complete is false when a target did not load.
	complete            bool
	emitted, unresolved []string
	rows                []emitRow
}

// collectEmits records each target's writes. A target that did not load
// is named on stderr and left out of the emitted targets.
func collectEmits(emits []targetEmit, dryRun bool) passWrites {
	w := passWrites{written: map[string]string{}, merged: map[string]mergedOutput{}, complete: true}
	for _, e := range emits {
		if e.err != nil && !e.resolved {
			fmt.Fprintf(os.Stderr, "! %v\n", e.err)
			w.complete = false
			w.unresolved = append(w.unresolved, e.target)
			continue
		}
		w.emitted = append(w.emitted, e.target)
		w.gitignore = append(w.gitignore, e.recorded...)
		if dryRun {
			continue
		}
		recordLedgerWrites(e.writes, &w.ledgerSession, w.written)
		recordMergedWrites(e.writes, w.merged)
		w.report.addWrites(e.target, e.writes)
		if verbosity >= levelVerbose {
			created, updated, skipped := classifyDetailedWrites(e.writes)
			w.rows = append(w.rows, emitRow{target: e.target, created: created, updated: updated, skipped: skipped, dur: e.dur})
		}
	}
	return w
}

// writeEntryPoints writes the entry points serially through sess, after
// every target, so their ordering, dedupe, and gitignore capture stay
// stable regardless of --jobs, then the output manifest.
func (w *passWrites) writeEntryPoints(sess *adapters.Session, cfg *config.Config, b spec.Bundle, targets []string, prev syncStateFile, opts syncRunOptions, record bool) error {
	if record {
		sess.StartRecording()
	}
	sess.StartDetailedRecording()
	entryFiles, err := writeEntryPointFiles(sess, cfg, b, targets, opts.dryRun)
	if err != nil {
		sess.StopDetailedRecording()
		if record {
			sess.StopRecording()
		}
		return err
	}
	entryWrites := sess.StopDetailedRecording()
	if record {
		w.gitignore = append(w.gitignore, sess.StopRecording()...)
	}
	if opts.dryRun {
		// Like the sweep, release merged files only when every configured
		// target emitted; otherwise another target may still own them.
		if coversAllConfiguredTargets(w.emitted, cfg.Targets) {
			return printMergedRewrites(prev, w.gitignore, opts.keepEdits, cfg.Sync.Unmanaged)
		}
		return nil
	}
	recordLedgerWrites(entryWrites, &w.ledgerSession, w.written)
	w.report.addWrites("", entryWrites)
	creditEntryPointReaders(cfg, &w.report, w.rows, w.emitted, entryWrites, entryFiles)
	manifestWrites, err := writeOutputManifest(sess, cfg, w.written, w.complete && coversAllConfiguredTargets(targets, cfg.Targets), false)
	if err != nil {
		return err
	}
	recordLedgerWrites(manifestWrites, &w.ledgerSession, w.written)
	w.report.addWrites("", manifestWrites)
	w.ledgerSession = withAgnosticEntryPoint(w.ledgerSession)
	return nil
}

// render prints the pass report, the per-target rows under -v, and, on a
// target's first sync, what each tool now reads.
func (w *passWrites) render(cfg *config.Config, b spec.Bundle, toList []string, took time.Duration) {
	if verbosity == levelVerbose {
		w.report.renderCompact(logOut, len(w.emitted), took)
	} else {
		w.report.render(logOut, len(w.emitted), took, verbosity > levelVerbose)
	}
	if verbosity >= levelVerbose && len(w.rows) > 0 {
		renderTargetRows(logOut, w.rows, verbosity > levelVerbose)
	}
	// The first sync is where the setup pays off, so it shows what
	// each tool now reads from the one source.
	if len(toList) > 0 {
		printToolReads(logOut, cfg, b, toList)
	}
}

// reportSweep adds the files the sweep removed or stripped to the report,
// and names the orphans it kept.
func (w *passWrites) reportSweep(printKept func(string, ...any), removed, stripped, kept []string, merged map[string]mergedOutput, priorSums map[string]string) {
	for _, p := range removed {
		w.report.removed = append(w.report.removed, filepath.ToSlash(p))
	}
	for _, p := range stripped {
		w.report.updated = append(w.report.updated, filepath.ToSlash(p))
	}
	for _, p := range kept {
		reason := keptOrphanReason(merged, priorSums, p)
		printKept("  ~ kept orphan %s (%s; run `agnostic-ai doctor --fix` to choose removal, or list it under sync.unmanaged)\n", p, reason)
	}
}

// printUnledgered names the leftovers no ledger proves sync wrote.
func printUnledgered(printKept func(string, ...any), unledgered driftReport) {
	for _, p := range unledgered.Leftover {
		printKept("  ~ kept leftover %s (no ledger proves sync wrote it; run `agnostic-ai doctor --fix` to remove it)\n", filepath.ToSlash(p))
	}
	for _, p := range unledgered.Orphaned {
		printKept("  ~ kept leftover %s (looks generated, with no ledger to prove sync wrote it; delete it by hand if stale, or list it under sync.unmanaged)\n", filepath.ToSlash(p))
	}
}

// printSessionEdits names the hand edits the sessions kept, removed,
// overwrote, or could not back up, then the unmanaged paths they skipped.
// It runs after the sweep, so refused orphan removals are reported too.
// holdSkips sends the skips through printKept with the other held lines.
func printSessionEdits(printKept func(string, ...any), sessions []*adapters.Session, cfg *config.Config, targets []string, holdSkips bool) {
	for _, p := range sessionPaths(sessions, (*adapters.Session).KeptEdits) {
		printKept("  ~ kept %s (edited since the last sync; move the edit into .agnostic-ai/, then run `agnostic-ai sync`)\n", p)
	}
	for _, p := range sessionPaths(sessions, (*adapters.Session).RemovedEdits) {
		printKept("%s removed a hand edit to %s (saved as %s.bak); move the edit into .agnostic-ai/\n", bang(), p, p)
	}
	for _, p := range sessionPaths(sessions, (*adapters.Session).OverwroteEdits) {
		printKept("%s overwrote a hand edit to %s (saved as %s.bak); %s\n", bang(), p, p, overwrittenEditAdvice(cfg, targets, p))
	}
	for _, p := range sessionPaths(sessions, (*adapters.Session).BackupBlockedEdits) {
		printKept("%s kept a hand edit to %s: %s.bak already holds an earlier one; move both into .agnostic-ai/, then delete the .bak\n", bang(), p, p)
	}
	for _, p := range unmanagedSkips(sessions) {
		if holdSkips {
			printKept("  ~ skip (unmanaged) %s\n", p)
		} else {
			summaryf("  ~ skip (unmanaged) %s\n", p)
		}
	}
}

// recordModelAliases records the model aliases this run resolved. --quiet
// prints no notes, so it records only new aliases and leaves a moved one
// for the next sync that does to report.
func recordModelAliases(ledger *syncLedger, prev map[string]map[string]string, emitted []string, printKept func(string, ...any)) {
	resolved := adapters.TakeResolvedAliases()
	if verbosity < levelDefault {
		ledger.modelAliases = addedModelAliases(prev, resolved, emitted)
		return
	}
	for _, note := range movedAliasNotes(prev, resolved) {
		printKept("  note: %s\n", note)
	}
	ledger.modelAliases = nextModelAliases(prev, resolved, emitted)
}

// passDrops are the capability warnings and coverage notes one pass
// buffered, with whether each set matches what the last sync printed.
type passDrops struct {
	cfg                               *config.Config
	prev                              syncStateFile
	accepted                          []adapters.AcceptedNote
	digest, notesDigest               string
	warningRecords, noteRecords       []string
	warningsUnchanged, notesUnchanged bool
	// quietUnchanged drops the count of unchanged drops, which a watch's
	// reconcile pass would repeat from the pass before it.
	quietUnchanged bool
}

func newPassDrops(cfg *config.Config, prev syncStateFile, accepted []adapters.AcceptedNote) passDrops {
	d := passDrops{
		cfg:            cfg,
		prev:           prev,
		accepted:       accepted,
		digest:         adapters.CapabilityWarningsDigest(),
		notesDigest:    adapters.CoverageNotesDigest(),
		warningRecords: dropFingerprints(adapters.PendingCapabilityWarnings()),
		noteRecords:    dropFingerprints(adapters.PendingCoverageNotes()),
	}
	reshow := verbosity > levelVerbose && showRepeatedDrops
	d.warningsUnchanged = !reshow && d.digest != "" && d.digest == prev.WarningsDigest && (verbosity != levelVerbose || len(prev.WarningRecords) == 0)
	d.notesUnchanged = !reshow && d.notesDigest != "" && d.notesDigest == prev.NotesDigest && (verbosity != levelVerbose || len(prev.NoteRecords) == 0)
	return d
}

// flush prints the buffered drops, folding the ones the last sync already
// printed into one count line.
func (d passDrops) flush() {
	var hidden []string
	if verbosity == levelVerbose && !d.warningsUnchanged && len(d.prev.WarningRecords) > 0 {
		if n := adapters.SuppressKnownCapabilityWarnings(fingerprintSet(d.prev.WarningRecords)); n > 0 {
			hidden = append(hidden, fmt.Sprintf("%d capability warning%s", n, plural(n)))
		}
	}
	if verbosity == levelVerbose && !d.notesUnchanged && len(d.prev.NoteRecords) > 0 {
		if n := adapters.SuppressKnownCoverageNotes(fingerprintSet(d.prev.NoteRecords)); n > 0 {
			hidden = append(hidden, fmt.Sprintf("%d coverage note%s", n, plural(n)))
		}
	}
	if d.warningsUnchanged {
		n := adapters.PendingCapabilityWarningsCount()
		hidden = append(hidden, fmt.Sprintf("%d capability warning%s", n, plural(n)))
		adapters.ResetCapabilityWarnings()
	}
	if d.notesUnchanged {
		n := adapters.PendingCoverageNotesCount()
		hidden = append(hidden, fmt.Sprintf("%d coverage note%s", n, plural(n)))
		adapters.DiscardCoverageNotes()
	}
	if d.cfg.Sync.DroppedSummary && verbosity >= levelDefault {
		adapters.RenderDroppedSummary(logOut)
	}
	adapters.FlushCapabilityWarnings()
	adapters.FlushCoverageNotes()
	if len(hidden) > 0 && !d.quietUnchanged {
		summaryf("  (%s unchanged since last sync; -vv shows them)\n", strings.Join(hidden, " and "))
	}
	if verbosity > levelVerbose {
		adapters.PrintAcceptedNotes(d.accepted)
	}
}

// record sets the drop records the ledger keeps and returns the digests
// the state file stores. A partial run keeps the records of the targets
// it left out; --quiet printed nothing, so it keeps the last sync's.
func (d passDrops) record(ledger *syncLedger, emitted []string, partial bool) (digest, notesDigest string) {
	ledger.warningRecords, ledger.noteRecords = d.warningRecords, d.noteRecords
	if partial {
		ledger.warningRecords = mergePartialDropRecords(d.prev.WarningRecords, d.warningRecords, emitted)
		ledger.noteRecords = mergePartialDropRecords(d.prev.NoteRecords, d.noteRecords, emitted)
	}
	if verbosity < levelDefault {
		ledger.warningRecords, ledger.noteRecords = d.prev.WarningRecords, d.prev.NoteRecords
		return d.prev.WarningsDigest, d.prev.NotesDigest
	}
	return d.digest, d.notesDigest
}

// newMainSession returns the session that drives the serial writes after
// the targets emit: entry points, shared-skill links, and the orphan sweep.
func newMainSession(cfg *config.Config, backup bool, edits editGuard) *adapters.Session {
	sess := adapters.NewSession()
	sess.SetProjectRoot(".")
	sess.SetUnmanaged(cfg.Sync.Unmanaged)
	if backup {
		sess.SetBackup(true)
	}
	edits.apply(sess)
	return sess
}

// withAgnosticEntryPoint lists AGNOSTIC_AI.md in the ledger session. A
// later sync reads it from disk and skips the rewrite, so detailed
// recording never captures it, and the ledger would treat it as an orphan.
func withAgnosticEntryPoint(session []string) []string {
	if _, err := os.Stat(adapters.AgnosticEntryPointPath); err == nil {
		return append(session, adapters.AgnosticEntryPointPath)
	}
	return session
}

// ignoreStep is the last step writeSyncIgnores reached, so each pass can
// keep its own rollback when one fails.
type ignoreStep int

const (
	ignorePlan ignoreStep = iota
	ignoreGitignore
	ignoreExcludes
	ignoreWorktree
)

// syncIgnores holds the ignore files as they were before the write, the
// managed block, and the files the write changed.
type syncIgnores struct {
	prior   []priorFile
	block   []string
	changed []string
	step    ignoreStep
}

// writeSyncIgnores writes the managed .gitignore block, the personal
// excludes, and the worktree include after the sweep. recorded is what the
// targets and entry points wrote; the links and the outputs the sweep
// kept join it.
func writeSyncIgnores(root string, cfg *config.Config, b spec.Bundle, targets, recorded []string, links []skillLink, ledger syncLedger, sweepErr error, emitted []string, noLedger bool, backups map[string]string) (syncIgnores, error) {
	entries := recorded
	for _, l := range links {
		entries = append(entries, l.path)
	}
	retained := ledger.orphans
	if sweepErr != nil || !coversAllConfiguredTargets(emitted, cfg.Targets) {
		retained = ledger.outputs
	}
	for _, path := range retained {
		if !cfg.IsUnmanaged(path) {
			entries = append(entries, path)
		}
	}
	var w syncIgnores
	// Some adapters choose output paths from what is on disk, so the
	// planners render after the emit, never from the renders before it.
	block, personal, err := personalSplitBlock(root, cfg, newTargetRenders(cfg, b), targets, entries, noLedger)
	if err != nil {
		return w, err
	}
	w.block = block
	w.prior = priorIgnoreFiles(root, cfg)
	w.step = ignoreGitignore
	rel, changed, err := writeGitignoreBlock(root, cfg, block)
	if err != nil {
		return w, fmt.Errorf("gitignore: %w", err)
	}
	w.step = ignoreExcludes
	if err := writePersonalExcludes(root, withLeftoverBackups(personal, block, backups)); err != nil {
		return w, fmt.Errorf("git exclude: %w", err)
	}
	if changed {
		w.changed = append(w.changed, filepath.ToSlash(rel))
	}
	w.step = ignoreWorktree
	if changed, err = writeWorktreeInclude(root, cfg, block); err != nil {
		return w, fmt.Errorf("worktreeinclude: %w", err)
	}
	if changed {
		w.changed = append(w.changed, worktreeIncludeFile)
	}
	return w, nil
}

// planIgnoreFiles is writeSyncIgnores for `sync --plan`: a report of the
// ignore files the sync would change, with the content it would write.
// The outputs the reports plan stand in for the paths a sync records.
func planIgnoreFiles(root string, cfg *config.Config, b spec.Bundle, targets []string, reports []driftReport, ledger ledgerSnapshot) (driftReport, error) {
	plan := driftReport{Target: "agnostic-ai"}
	var recorded []string
	for _, r := range reports {
		for _, files := range [][]adapters.CapturedFile{r.Current, r.Missing, r.Stale, r.Edited} {
			for _, f := range files {
				recorded = append(recorded, f.Path)
			}
		}
	}
	retained := ledger.state.Orphans
	if !coversAllConfiguredTargets(targets, cfg.Targets) {
		retained = ledger.state.Outputs
	}
	for _, p := range retained {
		if !cfg.IsUnmanaged(p) {
			recorded = append(recorded, p)
		}
	}
	renders := newTargetRenders(cfg, b)
	shared, err := planSharedSkills(cfg, renders, targets)
	if err != nil {
		return plan, err
	}
	for _, l := range shared.links {
		recorded = append(recorded, l.path)
	}
	block, _, err := plannedSplitBlock(root, cfg, renders, targets, recorded, ledger.missing())
	if err != nil {
		return plan, err
	}
	files := []struct {
		rel    string
		render func(string) string
	}{
		{gitignoreRel(cfg), func(existing string) string {
			return replaceManagedBlock(stripLooseFixedDuplicates(existing), block)
		}},
		{worktreeIncludeFile, func(existing string) string {
			return renderWorktreeInclude(existing, cfg, block)
		}},
	}
	for _, f := range files {
		existing, updated, err := plannedIgnoreFile(filepath.Join(root, f.rel), f.render)
		if err != nil {
			return plan, err
		}
		switch {
		case updated == existing:
		case strings.TrimSpace(updated) == "" && f.rel == worktreeIncludeFile:
			plan.Leftover = append(plan.Leftover, f.rel)
		case existing == "" && !fileExists(filepath.Join(root, f.rel)):
			plan.Missing = append(plan.Missing, adapters.CapturedFile{Path: f.rel, Content: updated})
		default:
			plan.Stale = append(plan.Stale, adapters.CapturedFile{Path: f.rel, Content: updated})
		}
	}
	return plan, nil
}

// withIgnorePlan folds the ignore file changes into the agnostic-ai
// report, which lists the other files sync writes for every target.
func withIgnorePlan(reports []driftReport, ignores driftReport) []driftReport {
	if !ignores.hasDrift() {
		return reports
	}
	for i := range reports {
		if reports[i].Target == ignores.Target {
			reports[i].Missing = append(reports[i].Missing, ignores.Missing...)
			reports[i].Stale = append(reports[i].Stale, ignores.Stale...)
			reports[i].Leftover = append(reports[i].Leftover, ignores.Leftover...)
			return reports
		}
	}
	return append(reports, ignores)
}

// untrackIgnored lists the outputs Git both tracks and ignores. With
// untrack, it removes them from the index, never the working tree, and
// returns the ones still tracked and the ones it removed.
func untrackIgnored(root string, cfg *config.Config, outputs []string, untrack bool) (stillTracked, removedFromIndex []string, err error) {
	tracked := gitTrackedAndIgnored(root, trackedIgnoreCandidates(cfg, outputs))
	if !untrack || len(tracked) == 0 {
		return tracked, nil, nil
	}
	removedFromIndex, err = gitRmCached(root, tracked)
	return removeMatching(tracked, removedFromIndex), removedFromIndex, err
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
	keep    bool
	sums    map[string]string
	backups map[string]string
}

// editGuardFor compares outputs against the sums the last sync recorded.
// Under --keep-edits a path the ledger has no sum for is compared with
// the version Git committed (#1397); with no ledger at all, the default
// backup has no proof of an edit and stays off.
func editGuardFor(keepEdits bool, prev syncStateFile) editGuard {
	if keepEdits && prev.OutputSums == nil {
		return editGuard{keep: true, sums: map[string]string{}}
	}
	return editGuard{keep: keepEdits, sums: prev.OutputSums, backups: prev.Backups}
}

func (g editGuard) apply(sess *adapters.Session) {
	switch {
	case g.keep:
		sess.KeepEditsSince(g.sums)
		sess.SetCommittedSum(committedSum)
	case g.sums != nil:
		sess.BackUpEditsSince(g.sums)
		sess.TrustBackupsSince(g.backups)
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

// overwrittenEditAdvice says where a hand edit that sync saved as a
// backup belongs. A tool's own memory can save a fact to an entry point,
// so with shared memory on, that advice names the memory folder too.
func overwrittenEditAdvice(cfg *config.Config, targets []string, path string) string {
	advice := "move the edit into .agnostic-ai/"
	if !slices.Contains(cfg.Builtins, memoryBuiltin) {
		return advice
	}
	for _, p := range entryPointPaths(cfg, targets) {
		if filepath.ToSlash(p) == path {
			return advice + ", or save a fact in .agnostic-ai/memory/"
		}
	}
	return advice
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

// emitRow is one target's verbose summary line, held until the entry-point
// files are written so it can count the ones that target reads.
type emitRow struct {
	target                    string
	created, updated, skipped int
	dur                       time.Duration
}

// creditEntryPointReaders counts each entry-point file for every synced
// target that reads it: in the report's target total, and in that target's
// verbose row.
func creditEntryPointReaders(cfg *config.Config, report *syncReport, rows []emitRow, targets []string, writes []adapters.WrittenFile, files []entryPointFile) {
	for _, target := range targets {
		created, updated, skipped := classifyDetailedWrites(entryWritesRead(cfg, target, writes, files))
		if created+updated > 0 {
			report.markTarget(target)
		}
		for i := range rows {
			if rows[i].target == target {
				rows[i].created += created
				rows[i].updated += updated
				rows[i].skipped += skipped
			}
		}
	}
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
// file written, updated, or skipped per target. prior is the state file
// the caller read under root, and project the project it loaded there.
func runSyncJSON(cmd *cobra.Command, root string, prior ledgerSnapshot, project projectLoad, targets []string, opts syncRunOptions) error {
	if err := prior.refuseNewer(); err != nil {
		return err
	}
	resetSyncBuffers()
	defer resetSyncBuffers()
	cfg, b := project.view()
	cfg.Gitignore.Enabled = resolveGitignore(cfg, opts.gitignoreFlag)
	prev := prior.state
	release := holdPriorState(root, prev)
	defer release()
	renders := newTargetRenders(cfg, b)
	effectiveTargets, winners, err := prepareSyncTargets(cmd.ErrOrStderr(), root, prior, cfg, renders, targets, syncRunOptions{backup: opts.backup, keepEdits: opts.keepEdits})
	if err != nil {
		return err
	}
	shared, err := planSharedSkills(cfg, renders, effectiveTargets)
	if err != nil {
		return err
	}
	prior.warn()
	edits := editGuardFor(opts.keepEdits, prev).backingUpMCPFiles(unledgeredMCPFiles(renders, cfg, effectiveTargets, prev.Outputs))

	// JSON retains partial target output except when coverage acceptance rejects the run.
	mainSess := newMainSession(cfg, opts.backup, edits)
	gitignoreOn := resolveGitignore(cfg, opts.gitignoreFlag)
	reconciled, err := shared.reconcile(prev.Outputs, prior.workdir(root).state, false)
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
	emits, sessions, _ := emitTargetsConcurrent(effectiveTargets, b, cfg, false, opts.backup, gitignoreOn, false, opts.jobs, edits, winners)
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
	if err := writeAgnosticEntryPoints(mainSess, cfg, b, effectiveTargets); err != nil {
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
		ledgerSession = withAgnosticEntryPoint(ledgerSession)
	}
	if gitignoreOn {
		gitignoreEntries = append(gitignoreEntries, mainSess.StopRecording()...)
	}

	applied := shared.apply(mainSess, false)
	ledgerSession = adjustLedgerForLinks(ledgerSession, applied)
	mainSess.StartTransaction()
	ledger, kept, removed, stripped, sweepErr := sweepAndFinalizeLedger(mainSess, prev, ledgerSession, ledgerWritten, ledgerMergedWrites, emitted, cfg.Targets, false)
	backups := syncBackups(prev.Backups, sessionPaths(sessions, (*adapters.Session).Backups))
	for _, l := range applied {
		if l.changed {
			out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: l.path, Action: "link"})
		} else {
			out.Skipped = append(out.Skipped, fileRecord{Target: "agnostic-ai", Path: l.path, Action: "skip"})
		}
	}
	if gitignoreOn {
		ignores, err := writeSyncIgnores(root, cfg, b, effectiveTargets, gitignoreEntries, applied, ledger, sweepErr, emitted, prior.missing(), backups)
		if err != nil {
			restore := ignores.prior
			if ignores.step == ignoreWorktree {
				restore = widenIgnores(restore, filepath.Join(root, gitignoreRel(cfg)), ignores.block)
			}
			return undoSweep(mainSess, restore, err)
		}
	}
	mainSess.Commit()
	runMemorySeeds()
	if sweepErr != nil {
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: sweepErr.Error()})
	}
	for _, p := range removed {
		rec := fileRecord{Target: "agnostic-ai", Path: p, Action: "delete"}
		if slices.Contains(mainSess.RemovedEdits(), p) {
			rec.Backup = p + ".bak"
		}
		out.Writes = append(out.Writes, rec)
	}
	for _, p := range stripped {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: "update"})
	}
	for _, p := range kept {
		out.Skipped = append(out.Skipped, fileRecord{Target: "agnostic-ai", Path: p, Action: "orphan"})
	}
	unledgered := keepUnledgered(cfg, prev, prior.workdir(root).missing(), &ledger, ledgerWritten, len(out.Errors) == 0 && coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
	for _, p := range unledgered.Leftover {
		out.Skipped = append(out.Skipped, fileRecord{Target: unledgered.Target, Path: filepath.ToSlash(p), Action: "leftover"})
	}
	for _, p := range unledgered.Orphaned {
		out.Skipped = append(out.Skipped, fileRecord{Target: unledgered.Target, Path: filepath.ToSlash(p), Action: "orphan"})
	}
	for _, p := range unmanagedSkips(sessions) {
		out.Skipped = append(out.Skipped, fileRecord{Target: "agnostic-ai", Path: p, Action: "unmanaged"})
	}
	trackedIgnored, removedFromIndex, untrackErr := untrackIgnored(root, cfg, ledger.outputs, opts.untrack)
	for _, p := range removedFromIndex {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: "untracked"})
	}
	if len(removedFromIndex) > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  ! %s\n", untrackPullAdvice)
	}
	if untrackErr != nil {
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: fmt.Sprintf("untrack: %v", untrackErr)})
	}
	for _, p := range trackedIgnored {
		out.Skipped = append(out.Skipped, fileRecord{Target: "agnostic-ai", Path: p, Action: "tracked"})
	}
	// Warnings and notes go into the JSON, not to the terminal, so keep
	// the previous digests: the next plain sync still prints them.
	ledger.specSums, ledger.specTargets = prev.SpecSums, prev.SpecTargets
	ledger.modelAliases = prev.ModelAliases
	if len(out.Errors) == 0 && coversAllConfiguredTargets(effectiveTargets, cfg.Targets) {
		base := specSums(cfg, b, cfg.Targets)
		ledger.specSums, ledger.specTargets = base.sums, base.targets
	}
	ledger.specFileSums = prev.SpecFileSums
	if len(out.Errors) == 0 {
		ledger.specFileSums = syncedSpecFileSums(root, prev.SpecFileSums, b, importProvenanceTargets(emits, sessions, mainSess), cfg)
	}
	ledger.backups = backups
	ledger.warningRecords, ledger.noteRecords = prev.WarningRecords, prev.NoteRecords
	ledger.listed = carriedListed(prev)
	release()
	stateErr := writeStateFile(root, len(out.Writes), prev.WarningsDigest, prev.NotesDigest, ledger)
	if stateErr != nil {
		stateErr = fmt.Errorf("sync wrote files but could not record them: %w", stateErr)
		out.addError(stateErr)
	}
	drops.syncDrops = pendingSyncDrops()
	drops.jsonOutput = out.forOutput()
	return errors.Join(writeIndentedJSON(cmd, drops), stateErr)
}

func dropFingerprints(records []adapters.DropRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.Target+"\x00"+adapters.DropRecordFingerprint(record))
	}
	slices.Sort(out)
	return out
}

func fingerprintSet(records []string) map[string]bool {
	out := make(map[string]bool, len(records))
	for _, record := range records {
		_, fingerprint, tagged := strings.Cut(record, "\x00")
		if !tagged {
			fingerprint = record
		}
		out[fingerprint] = true
	}
	return out
}

func mergePartialDropRecords(previous, current, emitted []string) []string {
	out := make([]string, 0, len(current))
	for _, record := range previous {
		target, _, tagged := strings.Cut(record, "\x00")
		if tagged && target != "agnostic-ai" && !slices.Contains(emitted, target) {
			out = append(out, record)
		}
	}
	for _, record := range current {
		target, _, tagged := strings.Cut(record, "\x00")
		if tagged && (target == "agnostic-ai" || slices.Contains(emitted, target)) {
			out = append(out, record)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
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
