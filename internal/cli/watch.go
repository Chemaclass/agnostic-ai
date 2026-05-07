package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// watchSync runs an initial sync then re-runs whenever a watched path changes.
// Polls every interval; Ctrl-C (or ctx cancellation) exits cleanly.
func watchSync(ctx context.Context, interval time.Duration, root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
	if err := runSyncOnce(root, targets, dryRun, backup, gitignoreFlag); err != nil {
		return err
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	watched := watchDirs(root, cfg)
	snapshot := collectMtimes(watched)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	summaryf("→ watching for changes (Ctrl+C to exit)\n")

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			curr := collectMtimes(watched)
			if !mtimesChanged(snapshot, curr) {
				continue
			}
			snapshot = curr
			summaryf("→ change detected, re-syncing\n")
			if err := runSyncOnce(root, targets, dryRun, backup, gitignoreFlag); err != nil {
				fmt.Fprintf(os.Stderr, "! sync: %v\n", err)
			}
			if newCfg, err := config.Load(root); err == nil {
				watched = watchDirs(root, newCfg)
				snapshot = collectMtimes(watched)
			}
		}
	}
}

// watchDirs returns the config file and source directories to watch.
func watchDirs(root string, cfg *config.Config) []string {
	paths := []string{filepath.Join(root, "agnostic.config.yaml")}
	for _, src := range []string{
		cfg.Sources.Agents,
		cfg.Sources.Skills,
		cfg.Sources.Rules,
		cfg.Sources.Hooks,
		cfg.Sources.MCPs,
	} {
		if src != "" {
			paths = append(paths, filepath.Join(root, src))
		}
	}
	pu := filepath.Join(root, defaultProjectUser)
	if dirExists(pu) {
		paths = append(paths, pu)
	}
	return paths
}

// collectMtimes walks paths and returns a file-path → mtime map.
func collectMtimes(paths []string) map[string]time.Time {
	mtimes := make(map[string]time.Time)
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			mtimes[p] = info.ModTime()
			continue
		}
		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			mtimes[path] = fi.ModTime()
			return nil
		})
	}
	return mtimes
}

// mtimesChanged reports whether any file was added, removed, or modified.
func mtimesChanged(prev, curr map[string]time.Time) bool {
	if len(prev) != len(curr) {
		return true
	}
	for k, t := range curr {
		if prev[k] != t {
			return true
		}
	}
	return false
}
