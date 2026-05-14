package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

type syncStateFile struct {
	SyncedAt     time.Time `json:"synced_at"`
	FilesChanged int       `json:"files_changed"`
}

func stateFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".agnostic-ai", ".sync-state")
}

func writeStateFile(projectRoot string, filesChanged int) error {
	p := stateFilePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	data, err := json.Marshal(syncStateFile{
		SyncedAt:     time.Now().UTC(),
		FilesChanged: filesChanged,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func runSyncOnce(root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
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
		adapters.StartCounting()
	}
	for _, t := range effectiveTargets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "! %v\n", err)
			continue
		}
		verbosef("→ emit %s\n", t)
		if err := adapter.Emit(b, cfg, dryRun); err != nil {
			if gitignoreOn {
				adapters.StopRecording()
			}
			if !dryRun {
				adapters.StopCounting()
			}
			return fmt.Errorf("%s: %w", t, err)
		}
	}
	if err := writeAgnosticEntryPoints(cfg, effectiveTargets, dryRun); err != nil {
		if gitignoreOn {
			adapters.StopRecording()
		}
		if !dryRun {
			adapters.StopCounting()
		}
		return err
	}
	filesChanged := 0
	if !dryRun {
		filesChanged = adapters.StopCounting()
	}
	if gitignoreOn {
		entries := adapters.StopRecording()
		entries = append(entries, ".agnostic-ai/.sync-state", ".agnostic-ai/AGNOSTIC_AI.md")
		if err := updateGitignore(cfg, normalizeAndSort(entries)); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
		summaryf("→ updated .gitignore\n")
	}
	if !dryRun {
		if err := writeStateFile(root, filesChanged); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}
	return nil
}

// runSyncJSON runs a real sync pass and emits a JSON result describing each
// file written, updated, or skipped per target.
func runSyncJSON(cmd *cobra.Command, root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
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
	for _, t := range effectiveTargets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		adapters.StartDetailedRecording()
		if err := adapter.Emit(b, cfg, dryRun); err != nil {
			adapters.StopDetailedRecording()
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		for _, f := range adapters.StopDetailedRecording() {
			rec := fileRecord{Target: t, Path: f.Path, Action: f.Action, Bytes: f.Bytes}
			if f.Action == "skip" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
	}

	adapters.StartDetailedRecording()
	if err := writeAgnosticEntryPoints(cfg, effectiveTargets, dryRun); err != nil {
		adapters.StopDetailedRecording()
		out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
	} else {
		for _, f := range adapters.StopDetailedRecording() {
			rec := fileRecord{Target: "agnostic-ai", Path: f.Path, Action: f.Action, Bytes: f.Bytes}
			if f.Action == "skip" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
	}

	if gitignoreOn {
		entries := adapters.StopRecording()
		entries = append(entries, ".agnostic-ai/.sync-state", ".agnostic-ai/AGNOSTIC_AI.md")
		if err := updateGitignore(cfg, normalizeAndSort(entries)); err != nil {
			return fmt.Errorf("gitignore: %w", err)
		}
	}
	if !dryRun {
		if err := writeStateFile(root, len(out.Writes)); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}
	return emitJSON(cmd, out)
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
