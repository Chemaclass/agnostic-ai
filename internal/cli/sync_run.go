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

func readStateFile(projectRoot string) syncStateFile {
	var s syncStateFile
	data, err := os.ReadFile(stateFilePath(projectRoot))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

func writeStateFile(projectRoot string, filesChanged int, warningsDigest string, outputs []string) error {
	p := stateFilePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	data, err := json.Marshal(syncStateFile{
		Version:        syncStateVersion,
		SyncedAt:       time.Now().UTC(),
		FilesChanged:   filesChanged,
		WarningsDigest: warningsDigest,
		Outputs:        outputs,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func runSyncOnce(root string, targets []string, dryRun, backup bool, gitignoreFlag string) (retErr error) {
	start := time.Now()
	adapters.ResetCapabilityWarnings()
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
	if backup {
		adapters.SetBackup(true)
		defer adapters.SetBackup(false)
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
	if gitignoreOn {
		adapters.StartRecording()
	}
	if !dryRun {
		adapters.StartTransaction()
		defer func() {
			if retErr != nil {
				fmt.Fprintf(os.Stderr, "! sync failed; rolling back partial writes\n")
				if rbErr := adapters.Rollback(); rbErr != nil {
					fmt.Fprintf(os.Stderr, "! rollback: %v\n", rbErr)
				}
			} else {
				adapters.Commit()
			}
		}()
	}
	verbose := verbosity >= levelVerbose
	filesChanged := 0
	var ledgerSession []string
	if !dryRun {
		for _, t := range effectiveTargets {
			adapter, err := adapters.Resolve(t)
			if err != nil {
				fmt.Fprintf(os.Stderr, "! %v\n", err)
				continue
			}
			adapters.StartDetailedRecording()
			if err := adapters.EmitWithProvenance(adapter, b, cfg, dryRun); err != nil {
				adapters.StopDetailedRecording()
				if gitignoreOn {
					adapters.StopRecording()
				}
				return fmt.Errorf("%s: %w", t, err)
			}
			writes := adapters.StopDetailedRecording()
			recordLedgerWrites(writes, &ledgerSession)
			created, updated, skipped := classifyDetailedWrites(writes)
			filesChanged += created + updated
			if verbose {
				verbosef("→ %s: %d created, %d updated, %d unchanged\n", t, created, updated, skipped)
			}
		}
		adapters.StartDetailedRecording()
		if err := writeAgnosticEntryPoints(cfg, b, effectiveTargets, dryRun); err != nil {
			adapters.StopDetailedRecording()
			if gitignoreOn {
				adapters.StopRecording()
			}
			return err
		}
		writes := adapters.StopDetailedRecording()
		recordLedgerWrites(writes, &ledgerSession)
		created, updated, _ := classifyDetailedWrites(writes)
		filesChanged += created + updated
		// resolveAgnosticBody reads AGNOSTIC_AI.md from disk on
		// subsequent syncs and skips the re-write, so detailed
		// recording never captures the path. Add it explicitly so
		// the ledger does not treat the source body as an orphan.
		if _, err := os.Stat(adapters.AgnosticEntryPointPath); err == nil {
			ledgerSession = append(ledgerSession, adapters.AgnosticEntryPointPath)
		}
	} else {
		for _, t := range effectiveTargets {
			adapter, err := adapters.Resolve(t)
			if err != nil {
				fmt.Fprintf(os.Stderr, "! %v\n", err)
				continue
			}
			if err := adapters.EmitWithProvenance(adapter, b, cfg, dryRun); err != nil {
				if gitignoreOn {
					adapters.StopRecording()
				}
				return fmt.Errorf("%s: %w", t, err)
			}
		}
		if err := writeAgnosticEntryPoints(cfg, b, effectiveTargets, dryRun); err != nil {
			if gitignoreOn {
				adapters.StopRecording()
			}
			return err
		}
	}
	if gitignoreOn {
		entries := adapters.StopRecording()
		block := buildManagedBlock(cfg, entries)
		if err := updateGitignore(root, cfg, block); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
		summaryf("→ updated .gitignore\n")
	}
	digest := adapters.CapabilityWarningsDigest()
	prev := readStateFile(root)
	if digest != "" && digest == prev.WarningsDigest {
		n := adapters.PendingCapabilityWarningsCount()
		summaryf("  (%d capability warning%s unchanged since last sync; delete %s to re-show)\n",
			n, plural(n), stateFilePath(root))
		adapters.ResetCapabilityWarnings()
	} else {
		adapters.FlushCapabilityWarnings()
	}
	ledger := finalizeLedger(ledgerSession)
	ledger = reconcilePartialLedger(ledger, prev.Outputs, coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
	removed, sweepErr := sweepLedgerOrphans(prev.Outputs, ledger, dryRun)
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
		if err := writeStateFile(root, filesChanged, digest, ledger); err != nil {
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

// runSyncJSON runs a real sync pass and emits a JSON result describing each
// file written, updated, or skipped per target.
func runSyncJSON(cmd *cobra.Command, root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
	adapters.ResetCapabilityWarnings()
	defer adapters.ResetCapabilityWarnings()
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
	if backup {
		adapters.SetBackup(true)
		defer adapters.SetBackup(false)
	}
	gitignoreOn := !dryRun && resolveGitignore(cfg, gitignoreFlag)
	if gitignoreOn {
		adapters.StartRecording()
	}

	out := jsonOutput{Version: "1", Command: "sync"}
	var ledgerSession []string
	for _, t := range effectiveTargets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		adapters.StartDetailedRecording()
		if err := adapters.EmitWithProvenance(adapter, b, cfg, dryRun); err != nil {
			adapters.StopDetailedRecording()
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		writes := adapters.StopDetailedRecording()
		recordLedgerWrites(writes, &ledgerSession)
		for _, f := range writes {
			rec := fileRecord{Target: t, Path: f.Path, Action: f.Action, Bytes: f.Bytes}
			if f.Action == "skip" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
	}

	adapters.StartDetailedRecording()
	if err := writeAgnosticEntryPoints(cfg, b, effectiveTargets, dryRun); err != nil {
		adapters.StopDetailedRecording()
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
	} else {
		writes := adapters.StopDetailedRecording()
		recordLedgerWrites(writes, &ledgerSession)
		for _, f := range writes {
			rec := fileRecord{Target: "agnostic-ai", Path: f.Path, Action: f.Action, Bytes: f.Bytes}
			if f.Action == "skip" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
		// See runSyncOnce: AGNOSTIC_AI.md is read on subsequent
		// syncs without going through emit, so register it for the
		// ledger by hand.
		if _, err := os.Stat(adapters.AgnosticEntryPointPath); err == nil {
			ledgerSession = append(ledgerSession, adapters.AgnosticEntryPointPath)
		}
	}

	if gitignoreOn {
		entries := adapters.StopRecording()
		block := buildManagedBlock(cfg, entries)
		if err := updateGitignore(root, cfg, block); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
	}
	prev := readStateFile(root)
	ledger := finalizeLedger(ledgerSession)
	ledger = reconcilePartialLedger(ledger, prev.Outputs, coversAllConfiguredTargets(effectiveTargets, cfg.Targets))
	removed, sweepErr := sweepLedgerOrphans(prev.Outputs, ledger, dryRun)
	if sweepErr != nil {
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: sweepErr.Error()})
	}
	for _, p := range removed {
		out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: "delete"})
	}
	if !dryRun {
		// JSON path does not print warnings, so preserve the previous
		// digest so the next non-JSON run can still sticky-suppress.
		if err := writeStateFile(root, len(out.Writes), prev.WarningsDigest, ledger); err != nil {
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
