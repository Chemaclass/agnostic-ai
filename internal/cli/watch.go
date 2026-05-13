package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// debounceWindow coalesces editor save flurries (write-then-truncate,
// rename-into-place, multi-file save) into a single re-sync.
const debounceWindow = 50 * time.Millisecond

// watchSync runs an initial sync, then re-runs whenever a watched path
// changes. When forcePoll is false it tries fsnotify and falls back to
// polling if the watcher cannot be created.
func watchSync(ctx context.Context, pollInterval time.Duration, root string, targets []string, dryRun, backup bool, gitignoreFlag string, forcePoll bool) error {
	if err := runSyncOnce(root, targets, dryRun, backup, gitignoreFlag); err != nil {
		return err
	}
	if !forcePoll {
		err := watchSyncFsnotify(ctx, root, targets, dryRun, backup, gitignoreFlag)
		if err == nil || ctx.Err() != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "! fsnotify unavailable (%v); falling back to polling\n", err)
	}
	return watchSyncPoll(ctx, pollInterval, root, targets, dryRun, backup, gitignoreFlag)
}

// watchSyncFsnotify watches via OS file events. Returns the first
// unrecoverable setup error so the caller can fall back to polling.
// Per-event errors during the loop are logged and the loop continues.
func watchSyncFsnotify(ctx context.Context, root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := addWatchPaths(w, watchDirs(root, cfg)); err != nil {
		return err
	}

	summaryf("→ watching for changes (Ctrl+C to exit)\n")

	var (
		debounce *time.Timer
		fire     = make(chan struct{}, 1)
	)
	defer func() {
		if debounce != nil {
			debounce.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if isIgnoredEvent(ev) {
				continue
			}
			// New directory under a watched root: add it so children
			// emit events too. fsnotify is not recursive on its own.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = addWatchPaths(w, []string{ev.Name})
				}
			}
			if debounce == nil {
				debounce = time.AfterFunc(debounceWindow, func() {
					select {
					case fire <- struct{}{}:
					default:
					}
				})
			} else {
				debounce.Reset(debounceWindow)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "! watch: %v\n", err)
		case <-fire:
			summaryf("→ change detected, re-syncing\n")
			if err := runSyncOnce(root, targets, dryRun, backup, gitignoreFlag); err != nil {
				fmt.Fprintf(os.Stderr, "! sync: %v\n", err)
			}
			// Pick up source roots that may have appeared (e.g. a
			// hooks/ dir created mid-session) by re-reading config.
			if newCfg, err := config.Load(root); err == nil {
				_ = addWatchPaths(w, watchDirs(root, newCfg))
			}
		}
	}
}

// watchSyncPoll is the original mtime-poll loop. Used as a fallback
// when fsnotify fails (e.g. some network mounts) or with --watch-poll.
func watchSyncPoll(ctx context.Context, interval time.Duration, root string, targets []string, dryRun, backup bool, gitignoreFlag string) error {
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

// addWatchPaths registers a file or directory and (for directories) every
// nested subdirectory with the watcher. Already-watched paths are
// silently skipped. Missing paths are skipped without error so callers
// can pass speculative roots.
func addWatchPaths(w *fsnotify.Watcher, paths []string) error {
	existing := make(map[string]struct{}, len(w.WatchList()))
	for _, p := range w.WatchList() {
		existing[p] = struct{}{}
	}
	add := func(p string) error {
		if _, ok := existing[p]; ok {
			return nil
		}
		if err := w.Add(p); err != nil {
			return fmt.Errorf("watch %s: %w", p, err)
		}
		existing[p] = struct{}{}
		return nil
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if err := add(p); err != nil {
				return err
			}
			continue
		}
		if err := add(p); err != nil {
			return err
		}
		walkErr := filepath.WalkDir(p, func(sub string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() || sub == p {
				return nil
			}
			return add(sub)
		})
		if walkErr != nil {
			return walkErr
		}
	}
	return nil
}

// isIgnoredEvent filters out events that should never trigger a re-sync:
// chmod-only events (editor "touch" on save) and writes to the .sync-state
// file we own.
func isIgnoredEvent(ev fsnotify.Event) bool {
	if ev.Op == fsnotify.Chmod {
		return true
	}
	if filepath.Base(ev.Name) == ".sync-state" {
		return true
	}
	return false
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
