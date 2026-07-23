package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// debounceWindow coalesces editor save flurries (write-then-truncate,
// rename-into-place, multi-file save) into a single re-sync.
const debounceWindow = 50 * time.Millisecond

// agnosticOverlayDir is the project-relative directory where importers
// stash captured per-target settings so they survive a wipe of the
// native config tree between `import` and `sync`. Watched by `sync
// --watch` so hand-edits to the overlay trigger a re-emit.
const agnosticOverlayDir = ".agnostic-ai/overlays"

// watchSync runs an initial sync, then re-runs whenever a watched path
// changes. When forcePoll is false it tries fsnotify and falls back to
// polling if the watcher cannot be created.
func watchSync(ctx context.Context, pollInterval time.Duration, root string, targets []string, dryRun, backup bool, gitignoreFlag string, forcePoll bool, jobs int) error {
	if err := runSyncOnce(root, targets, dryRun, backup, gitignoreFlag, jobs); err != nil {
		return err
	}
	if !forcePoll {
		err := watchSyncFsnotify(ctx, root, targets, dryRun, backup, gitignoreFlag, jobs)
		if err == nil || ctx.Err() != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "! fsnotify unavailable (%v); falling back to polling\n", err)
	}
	return watchSyncPoll(ctx, pollInterval, root, targets, dryRun, backup, gitignoreFlag, jobs)
}

// watchSyncFsnotify watches via OS file events. Returns the first
// unrecoverable setup error so the caller can fall back to polling.
// Per-event errors during the loop are logged and the loop continues.
func watchSyncFsnotify(ctx context.Context, root string, targets []string, dryRun, backup bool, gitignoreFlag string, jobs int) error {
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()

	if err := addWatchPaths(w, watchDirs(root, cfg)); err != nil {
		return err
	}

	printWatchBanner(len(w.WatchList()), "fsnotify")

	var (
		debounce  *time.Timer
		fire      = make(chan struct{}, 1)
		changed   = map[string]struct{}{}
		lastEvent string
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
			// emit events too. fsnotify is not recursive on its own. A
			// bare directory carries no spec content, so it is not
			// recorded as a change; the child file events attribute the
			// re-sync.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = addWatchPaths(w, []string{ev.Name})
					continue
				}
			}
			lastEvent = ev.Name
			changed[filepath.Clean(ev.Name)] = struct{}{}
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
			printWatchEvent(lastEvent)
			paths := mapKeys(changed)
			changed = map[string]struct{}{}
			if err := resyncForChanges(root, targets, paths, dryRun, backup, gitignoreFlag, jobs); err != nil {
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
func watchSyncPoll(ctx context.Context, interval time.Duration, root string, targets []string, dryRun, backup bool, gitignoreFlag string, jobs int) error {
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	watched := watchDirs(root, cfg)
	snapshot := collectMtimes(watched)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	printWatchBanner(len(watched), "poll")

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			curr := collectMtimes(watched)
			if !mtimesChanged(snapshot, curr) {
				continue
			}
			changed := changedPaths(snapshot, curr)
			snapshot = curr
			printWatchEvent(firstOf(changed))
			if err := resyncForChanges(root, targets, changed, dryRun, backup, gitignoreFlag, jobs); err != nil {
				fmt.Fprintf(os.Stderr, "! sync: %v\n", err)
			}
			if newCfg, err := config.Load(root); err == nil {
				watched = watchDirs(root, newCfg)
				snapshot = collectMtimes(watched)
			}
		}
	}
}

// printWatchBanner shows a one-line header at watch start. Mode is
// "fsnotify" or "poll" so users can tell which backend is active.
func printWatchBanner(dirs int, mode string) {
	summaryf("→ watching %d path%s (%s) · Ctrl+C to exit\n", dirs, plural(dirs), mode)
}

// printWatchEvent prints a timestamped one-liner per debounced change
// burst. path is shown relative to cwd when possible to keep the line
// short.
func printWatchEvent(path string) {
	stamp := time.Now().Format("15:04:05")
	rel := path
	if cwd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	if rel == "" {
		summaryf("[%s] change detected · re-syncing\n", stamp)
		return
	}
	summaryf("[%s] change · %s\n", stamp, rel)
}

// changedPaths returns every path a poll-mode snapshot diff reports as
// added, removed, or modified, sorted for deterministic attribution and
// logging. A modified or added file appears with its current mtime; a
// removed file is present in prev but absent from curr.
func changedPaths(prev, curr map[string]time.Time) []string {
	changed := map[string]struct{}{}
	for k, t := range curr {
		if prev[k] != t {
			changed[filepath.Clean(k)] = struct{}{}
		}
	}
	for k := range prev {
		if _, ok := curr[k]; !ok {
			changed[filepath.Clean(k)] = struct{}{}
		}
	}
	return mapKeys(changed)
}

// mapKeys returns the sorted keys of a path set. Sorting keeps the
// re-sync attribution and its summary independent of map iteration order.
func mapKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// firstOf returns the first element or "" for an empty slice. Used to
// show one representative path in the per-change log line.
func firstOf(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
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
// Includes `.agnostic-ai/overlays/` so hand-edits to the captured
// per-target overlays (claude.settings.json, codex.config.toml) trigger
// a re-emit just like spec changes do.
func watchDirs(root string, cfg *config.Config) []string {
	paths := []string{}
	if cfgPath, _, err := config.ResolveConfigPath(root); err == nil {
		paths = append(paths, cfgPath)
	}
	localPath := filepath.Join(root, config.LocalOverrideFileName)
	if _, err := os.Stat(localPath); err == nil {
		paths = append(paths, localPath)
	}
	for _, src := range []string{
		cfg.Sources.Agents,
		cfg.Sources.Skills,
		cfg.Sources.Rules,
		cfg.Sources.Hooks,
		cfg.Sources.MCPs,
		cfg.Sources.Settings,
		cfg.Sources.Reviews,
		cfg.Sources.Environments,
		cfg.Sources.Ignore,
	} {
		if src != "" {
			paths = append(paths, filepath.Join(root, src))
		}
	}
	pu := filepath.Join(root, defaultProjectUser)
	if dirExists(pu) {
		paths = append(paths, pu)
	}
	overlay := filepath.Join(root, agnosticOverlayDir)
	if dirExists(overlay) {
		paths = append(paths, overlay)
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

// affectedResync describes how one debounced change burst maps onto the
// targets that must re-emit. When full is set the caller re-syncs every
// configured target (config or overlay edits, deletes, renames, or paths
// that cannot be attributed to a spec). Otherwise targets holds the
// affected subset and reason names the spec kind(s) behind it.
type affectedResync struct {
	full    bool
	reason  string
	targets []string
}

// resyncForChanges re-emits only the targets affected by a debounced set
// of changed paths, falling back to a full re-sync when the change spans
// config or overlay files or cannot be attributed to a spec. Loading the
// project here mirrors runSyncOnce so the attribution sees the same
// bundle the emission will, and it always defers the actual writing to
// runSyncOnce so the affected subset gets the identical ledger, gitignore,
// and orphan-sweep handling a partial `--only` sync already gets.
func resyncForChanges(root string, configured, changed []string, dryRun, backup bool, gitignoreFlag string, jobs int) error {
	cfg, b, err := loadProject(root)
	if err != nil {
		// Without a loadable project the change cannot be attributed;
		// re-sync every configured target so correctness is never traded
		// for a narrower emit. runSyncOnce surfaces the load error itself.
		return runSyncOnce(root, configured, dryRun, backup, gitignoreFlag, jobs)
	}
	effective := configured
	if len(effective) == 0 {
		effective = cfg.Targets
	}
	plan := planWatchResync(root, cfg, b, changed, effective)
	if plan.full {
		summaryf("  ↳ %s · full re-sync\n", plan.reason)
		return runSyncOnce(root, effective, dryRun, backup, gitignoreFlag, jobs)
	}
	if len(plan.targets) == 0 {
		summaryf("  ↳ %s change · no configured target emits it; skipped\n", plan.reason)
		return nil
	}
	summaryf("  ↳ %s change · re-syncing %d target%s: %s\n",
		plan.reason, len(plan.targets), plural(len(plan.targets)), strings.Join(plan.targets, ", "))
	return runSyncOnce(root, plan.targets, dryRun, backup, gitignoreFlag, jobs)
}

// planWatchResync maps a debounced set of changed paths onto the targets
// that must re-emit. It is pure: it reads cfg and b but touches neither
// disk nor global state, so it is unit-testable in isolation.
//
// A change forces a full re-sync when it touches the config file, the
// local override, the captured overlay tree, an unrecognized path, or a
// spec that is no longer in the bundle (a delete or rename). Otherwise
// each changed spec contributes the configured targets that emit its
// kind, narrowed by the spec's own `target:` / `targets:` frontmatter
// scoping, and the union of those targets is re-emitted.
func planWatchResync(root string, cfg *config.Config, b spec.Bundle, changed, configured []string) affectedResync {
	if len(changed) == 0 {
		return affectedResync{full: true, reason: "unattributed change"}
	}
	kinds := map[spec.Kind]struct{}{}
	affected := map[string]struct{}{}
	for _, raw := range changed {
		cp := filepath.Clean(raw)
		if isFullSyncPath(root, cp) {
			return affectedResync{full: true, reason: "config or overlay change"}
		}
		kind, ok := watchKindForPath(root, cfg, cp)
		if !ok {
			return affectedResync{full: true, reason: "unrecognized path " + filepath.ToSlash(cp)}
		}
		owner, ok := findSpecOwner(b, cp)
		if !ok {
			return affectedResync{full: true, reason: "removed or renamed spec"}
		}
		kinds[kind] = struct{}{}
		for _, t := range affectedTargetsForKind(kind, configured, owner) {
			affected[t] = struct{}{}
		}
	}
	// Emit in configured order so the summary and downstream emission are
	// deterministic regardless of map iteration order.
	var targets []string
	for _, t := range configured {
		if _, ok := affected[t]; ok {
			targets = append(targets, t)
		}
	}
	return affectedResync{reason: kindList(kinds), targets: targets}
}

// isFullSyncPath reports whether cp is a project-wide input whose change
// cannot be scoped to a single spec kind: the base or legacy config file,
// the local override, or anything under the captured overlay tree. Such
// edits re-key collisions, targets, or per-target overlays, so a full
// re-sync is the only correct response.
func isFullSyncPath(root, cp string) bool {
	for _, name := range []string{
		config.ConfigFileName,
		config.LegacyConfigFileName,
		config.LocalOverrideFileName,
	} {
		if cp == filepath.Clean(filepath.Join(root, name)) {
			return true
		}
	}
	return pathWithin(filepath.Join(root, agnosticOverlayDir), cp)
}

// watchKindForPath attributes cp to the spec kind whose source directory
// contains it, using the project layer's configured source paths. Returns
// false when cp falls outside every source directory (an unrecognized
// path the caller treats as a full re-sync).
func watchKindForPath(root string, cfg *config.Config, cp string) (spec.Kind, bool) {
	for _, sk := range []struct {
		src  string
		kind spec.Kind
	}{
		{cfg.Sources.Agents, spec.KindAgent},
		{cfg.Sources.Skills, spec.KindSkill},
		{cfg.Sources.Rules, spec.KindRule},
		{cfg.Sources.Hooks, spec.KindHook},
		{cfg.Sources.MCPs, spec.KindMCP},
		{cfg.Sources.Commands, spec.KindCommand},
		{cfg.Sources.Settings, spec.KindSettings},
		{cfg.Sources.Reviews, spec.KindReview},
		{cfg.Sources.Environments, spec.KindEnvironment},
		{cfg.Sources.Ignore, spec.KindIgnore},
	} {
		if sk.src == "" {
			continue
		}
		if pathWithin(filepath.Join(root, sk.src), cp) {
			return sk.kind, true
		}
	}
	return "", false
}

// findSpecOwner returns the bundle entry that owns cp: the spec whose
// source file is cp, or — for a folder-based skill — the SKILL.md whose
// directory contains cp so a changed asset re-emits its whole skill.
// Returns false when no entry owns cp (a delete, a rename, or a
// not-yet-loaded file), which the caller escalates to a full re-sync.
func findSpecOwner(b spec.Bundle, cp string) (spec.Entry, bool) {
	for _, e := range b.All() {
		ep := filepath.Clean(e.Path)
		if ep == cp {
			return e, true
		}
		if e.Kind == spec.KindSkill && filepath.Base(ep) == "SKILL.md" &&
			pathWithin(filepath.Dir(ep), cp) {
			return e, true
		}
	}
	return spec.Entry{}, false
}

// affectedTargetsForKind returns the configured targets that emit kind
// and are allowed by the owning spec's target scoping, in configured
// order. targetsSupportingKind is the same capability matrix the
// orphan-kind validator uses, so a rule change hits every rule-emitting
// target while a claude-scoped agent hits only claude.
func affectedTargetsForKind(kind spec.Kind, configured []string, owner spec.Entry) []string {
	emitters := targetsSupportingKind[kind]
	var out []string
	for _, t := range configured {
		if _, ok := emitters[t]; !ok {
			continue
		}
		if !owner.EmitsTo(t) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// kindList renders a set of kinds as a stable, comma-separated string for
// the re-sync summary (e.g. "agent, rule").
func kindList(kinds map[spec.Kind]struct{}) string {
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, string(k))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// pathWithin reports whether cp is dir itself or a descendant of it. Both
// operands are cleaned so a relative watched root ("./.agnostic-ai/rules")
// and an absolute event path compare on equal footing.
func pathWithin(dir, cp string) bool {
	dir = filepath.Clean(dir)
	cp = filepath.Clean(cp)
	if dir == cp {
		return true
	}
	rel, err := filepath.Rel(dir, cp)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
