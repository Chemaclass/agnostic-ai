package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

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
	// absent from the current write set is swept via
	// emit.RemoveGenerated. The header-guarded sweep skips
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

func runSyncOnce(root string, targets []string, dryRun, backup bool, gitignoreFlag string) (retErr error) {
	start := time.Now()
	sess := adapters.NewSession()
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
	if backup {
		sess.SetBackup(true)
		defer sess.SetBackup(false)
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
	if gitignoreOn {
		sess.StartRecording()
		// Safety net for the early returns below: the success path
		// consumes the recorded paths for the .gitignore block, after
		// which this deferred call no-ops (StopRecording is idempotent).
		defer sess.StopRecording()
	}
	if !dryRun {
		sess.StartTransaction()
		defer func() {
			if retErr != nil {
				fmt.Fprintf(os.Stderr, "! sync failed; rolling back partial writes\n")
				if rbErr := sess.Rollback(); rbErr != nil {
					fmt.Fprintf(os.Stderr, "! rollback: %v\n", rbErr)
				}
			} else {
				sess.Commit()
			}
		}()
	}
	shared.reconcile(prev.Outputs, dryRun)
	verbose := verbosity >= levelVerbose
	filesChanged := 0
	var ledgerSession []string
	for _, t := range effectiveTargets {
		writes, resolved, err := emitTarget(sess, t, b, cfg, dryRun)
		if err != nil {
			if !resolved {
				fmt.Fprintf(os.Stderr, "! %v\n", err)
				continue
			}
			return fmt.Errorf("%s: %w", t, err)
		}
		if dryRun {
			continue
		}
		recordLedgerWrites(writes, &ledgerSession)
		created, updated, skipped := classifyDetailedWrites(writes)
		filesChanged += created + updated
		if verbose {
			verbosef("→ %s: %d created, %d updated, %d unchanged\n", t, created, updated, skipped)
		}
	}
	sess.StartDetailedRecording()
	if err := writeAgnosticEntryPoints(sess, cfg, b, effectiveTargets, dryRun); err != nil {
		sess.StopDetailedRecording()
		return err
	}
	entryWrites := sess.StopDetailedRecording()
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
	applied := shared.apply(sess, dryRun)
	ledgerSession = adjustLedgerForLinks(ledgerSession, applied)
	if gitignoreOn {
		entries := sess.StopRecording()
		for _, l := range applied {
			entries = append(entries, l.path)
		}
		entries = append(entries, gitignoreHintsForTargets(cfg, effectiveTargets)...)
		block := buildManagedBlock(cfg, entries)
		if err := updateGitignore(root, cfg, block); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
		summaryf("→ updated .gitignore\n")
	}
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
	removed, sweepErr := sweepLedgerOrphans(sess, prev.Outputs, ledger, dryRun)
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
func runSyncJSON(cmd *cobra.Command, root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
	sess := adapters.NewSession()
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
	if backup {
		sess.SetBackup(true)
		defer sess.SetBackup(false)
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
	if gitignoreOn {
		sess.StartRecording()
		defer sess.StopRecording()
	}
	shared.reconcile(prev.Outputs, dryRun)

	out := jsonOutput{Version: "1", Command: "sync"}
	var ledgerSession []string
	for _, t := range effectiveTargets {
		writes, _, err := emitTarget(sess, t, b, cfg, dryRun)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		recordLedgerWrites(writes, &ledgerSession)
		appendFileRecords(&out, t, writes)
	}

	sess.StartDetailedRecording()
	if err := writeAgnosticEntryPoints(sess, cfg, b, effectiveTargets, dryRun); err != nil {
		sess.StopDetailedRecording()
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
	} else {
		entryWrites := sess.StopDetailedRecording()
		recordLedgerWrites(entryWrites, &ledgerSession)
		appendFileRecords(&out, "agnostic-ai", entryWrites)
		// See runSyncOnce: AGNOSTIC_AI.md is read on subsequent
		// syncs without going through emit, so register it for the
		// ledger by hand.
		if _, err := os.Stat(adapters.AgnosticEntryPointPath); err == nil {
			ledgerSession = append(ledgerSession, adapters.AgnosticEntryPointPath)
		}
	}

	applied := shared.apply(sess, dryRun)
	ledgerSession = adjustLedgerForLinks(ledgerSession, applied)
	for _, l := range applied {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: l.path, Action: "link"})
	}
	if gitignoreOn {
		entries := sess.StopRecording()
		for _, l := range applied {
			entries = append(entries, l.path)
		}
		entries = append(entries, gitignoreHintsForTargets(cfg, effectiveTargets)...)
		block := buildManagedBlock(cfg, entries)
		if err := updateGitignore(root, cfg, block); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
	}
	ledger := finalizeLedger(ledgerSession)
	ledger = reconcilePartialLedger(ledger, prev.Outputs, coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
	removed, sweepErr := sweepLedgerOrphans(sess, prev.Outputs, ledger, dryRun)
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
		return fmt.Errorf("drift detected")
	}
	return nil
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
