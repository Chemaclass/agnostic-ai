package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func newRevertCmd() *cobra.Command {
	var targets, only, except []string
	var dryRun, jsonOut, force bool

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Undo a previous sync. Restores .bak files when present.",
		Long: "revert reverses the effect of `sync` for every target.\n" +
			"For each file the matching adapter would emit, plus the entry-point\n" +
			"files sync distributes (CLAUDE.md, AGENTS.md, GEMINI.md, ...):\n" +
			"  - if <path>.bak exists, the .bak content is restored and the .bak removed\n" +
			"  - otherwise the file is left in place to protect user-authored content\n" +
			"    that happens to share a path with an adapter-emitted file (e.g. a\n" +
			"    helper script next to SKILL.md). Pass --force to delete unbacked files.\n" +
			"Pair with `sync --backup` so the .bak trail exists for the files you want\n" +
			"restored to their pre-sync content. An unfiltered revert also restores\n" +
			"intact recorded backups of outputs sync removed.",
		Example: `  # Preview what would be reverted
  agnostic-ai revert --dry-run

  # Restore .bak files where they exist; leave unbacked files alone
  agnostic-ai revert

  # Also remove files that have no .bak (old default behavior)
  agnostic-ai revert --force

  # Revert only Claude and Cursor
  agnostic-ai revert --only claude,cursor

  # Revert everything except Codex
  agnostic-ai revert --except codex

  # Machine-readable output for CI dashboards and editor extensions
  agnostic-ai revert --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(only) > 0 && len(except) > 0 {
				return fmt.Errorf("--only and --except are mutually exclusive")
			}
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			if dryRun {
				readLedger(".").warnNewer()
			} else {
				// Not acquireLedgerLock: revert leaves .gitignore alone.
				lock, err := openProjectLock(".", "revert")
				if err != nil {
					return err
				}
				defer func() { _ = lock.Close() }()
				if err := readLedger(".").refuseNewer(); err != nil {
					return err
				}
			}
			base := targets
			if len(base) == 0 {
				base = cfg.Targets
			}
			effective, err := filterTargets(base, only, except)
			if err != nil {
				return err
			}
			cfg = cfg.WithAdditionalTargets(effective...)

			if jsonOut {
				return runRevertJSON(cmd, effective, dryRun, force, len(targets) == 0 && len(only) == 0 && len(except) == 0)
			}

			seen := map[string]bool{}
			inSource := sourceTree(cfg)
			sess := adapters.NewSession()
			for _, t := range effective {
				adapter, err := adapters.Resolve(t)
				if err != nil {
					fmt.Fprintf(os.Stderr, "! %v\n", err)
					continue
				}
				sess.StartCapture()
				// Pass dryRun=true so any adapter that writes outside
				// WriteFile (e.g. a future os.Mkdir for an empty
				// output dir) does not side-effect during revert.
				if err := emitAdapter(sess, adapter, b, cfg, true); err != nil {
					sess.StopCapture()
					return fmt.Errorf("%s: %w", t, err)
				}
				files := sess.StopCapture()
				summaryf("← revert %s\n", t)
				var tally revertTally
				for _, f := range files {
					if alreadyHandled(seen, f.Path) {
						continue
					}
					action, err := revertOutput(inSource, f.Path, dryRun, force)
					if err != nil {
						return fmt.Errorf("%s: %w", t, err)
					}
					tally.add(action)
				}
				tally.report(dryRun)
			}

			// sync writes the entry-point files (CLAUDE.md, AGENTS.md, ...)
			// outside adapter emission, so revert them on the same terms.
			summaryf("← revert entry-points\n")
			var tally revertTally
			for _, p := range entryPointPaths(cfg, effective) {
				if alreadyHandled(seen, p) {
					continue
				}
				action, err := revertOutput(inSource, p, dryRun, force)
				if err != nil {
					return fmt.Errorf("entry-point: %w", err)
				}
				tally.add(action)
			}
			companions, err := adapters.AgentsCompanionPaths(cfg, b, effective)
			if err != nil {
				return err
			}
			for _, p := range companions {
				if alreadyHandled(seen, p) {
					continue
				}
				action, err := revertOutput(inSource, p, dryRun, false)
				if err != nil {
					return fmt.Errorf("claude: %w", err)
				}
				tally.addRestore(action)
			}
			if len(targets) == 0 && len(only) == 0 && len(except) == 0 {
				for _, p := range missingRecordedBackups(cfg) {
					if alreadyHandled(seen, p) {
						continue
					}
					action, err := revertOutput(inSource, p, dryRun, false)
					if err != nil {
						return fmt.Errorf("recorded backup: %w", err)
					}
					tally.addRestore(action)
				}
			}
			tally.report(dryRun)
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to revert (default: all in config)")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Revert only these targets (comma-separated); mutually exclusive with --except")
	cmd.Flags().StringSliceVar(&except, "except", nil, "Revert all configured targets except these (comma-separated); mutually exclusive with --only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report intended actions without touching disk")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&force, "force", false, "Also delete adapter-emitted files that lack a .bak (use with care: removes user-authored files that share a path with adapter output)")
	registerTargetCompletion(cmd)
	return cmd
}

// sourceTree reports whether path resolves into the source tree: the
// .agnostic-ai folder or a configured spec folder. Those files are what
// the user writes, so revert never restores or deletes them, whatever
// output path a config points there. Lexical containment covers `..`
// segments; os.SameFile on each existing parent covers letter case on a
// case-insensitive disk and links.
func sourceTree(cfg *config.Config) func(path string) bool {
	var roots []string
	var infos []os.FileInfo
	for _, d := range append([]string{config.SourceBaseDir}, sourceFolders(cfg)...) {
		abs, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		roots = append(roots, abs)
		if info, err := os.Stat(abs); err == nil {
			infos = append(infos, info)
		}
	}
	return func(path string) bool {
		abs, err := filepath.Abs(path)
		if err != nil {
			return true
		}
		for _, r := range roots {
			if rel, err := filepath.Rel(r, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true
			}
		}
		for dir := filepath.Dir(abs); ; dir = filepath.Dir(dir) {
			if info, err := os.Stat(dir); err == nil && slices.ContainsFunc(infos, func(r os.FileInfo) bool { return os.SameFile(r, info) }) {
				return true
			}
			if next := filepath.Dir(dir); next == dir {
				return false
			}
		}
	}
}

// sourceFolders lists the spec folders the config sets.
func sourceFolders(cfg *config.Config) []string {
	var out []string
	for _, d := range []string{
		cfg.Sources.Agents, cfg.Sources.Skills, cfg.Sources.Rules,
		cfg.Sources.Hooks, cfg.Sources.MCPs, cfg.Sources.Commands,
		cfg.Sources.Settings, cfg.Sources.Reviews, cfg.Sources.Environments,
		cfg.Sources.Ignore,
	} {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// revertOutput is revertOne for a path a config can point anywhere: a
// path in the source tree is kept.
func revertOutput(inSource func(string) bool, path string, dryRun, force bool) (string, error) {
	if inSource(path) {
		verbosef("    keep:    %s (source file)\n", path)
		return "skip", nil
	}
	return revertOne(path, dryRun, force)
}

// revertTally counts what revert did to one group of files.
type revertTally struct{ restored, removed, preserved int }

func (t *revertTally) add(action string) {
	switch action {
	case "restore":
		t.restored++
	case "remove":
		t.removed++
	case "preserve":
		t.preserved++
	}
}

// addRestore counts only a restore, for files revert never deletes, so
// --force does not report them as kept for lack of --force.
func (t *revertTally) addRestore(action string) {
	if action == "restore" {
		t.restored++
	}
}

func (t revertTally) report(dryRun bool) {
	verb := ""
	if dryRun {
		verb = "would be "
	}
	if t.restored > 0 {
		summaryf("    %d file(s) %srestored from .bak\n", t.restored, verb)
	}
	if t.removed > 0 {
		summaryf("    %d file(s) %sremoved\n", t.removed, verb)
	}
	if t.preserved > 0 {
		summaryf("    %d file(s) preserved (no .bak; pass --force to delete)\n", t.preserved)
	}
}

// restorePerm sets path to bak's mode before the bytes land, so private
// bytes are never readable through a wider file. It never widens path's
// read and write bits, since older versions wrote every backup at 0644,
// and keeps path's owner write and executable bits so the write succeeds
// and a hook still runs.
func restorePerm(path, bak string) (os.FileMode, error) {
	info, err := os.Stat(bak)
	if err != nil {
		return 0, err
	}
	cur, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	perm := info.Mode().Perm()&cur.Mode().Perm()&0o666 | cur.Mode().Perm()&0o311
	return perm, os.Chmod(path, perm)
}

// revertOne restores path from path+".bak" when the backup exists. When
// there is no .bak the default is to leave path alone ("preserve") so a
// user-authored file that happens to share a path with adapter output
// (e.g. a helper script propagated into a skill folder) is not silently
// deleted. Pass force=true to delete unbacked files (old behavior).
// Missing source files are reported as "skip".
//
// Returns the action taken: "restore", "remove", "preserve", or "skip".
func revertOne(path string, dryRun, force bool) (string, error) {
	bak := path + ".bak"
	data, err := os.ReadFile(bak)
	switch {
	case err == nil:
		if dryRun {
			verbosef("    restore: %s ← %s\n", path, bak)
			return "restore", nil
		}
		if _, statErr := os.Lstat(path); errors.Is(statErr, fs.ErrNotExist) {
			info, err := os.Stat(bak)
			if err != nil {
				return "", fmt.Errorf("stat backup %s: %w", bak, err)
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
			if err != nil {
				return "", fmt.Errorf("restore %s: %w", path, err)
			}
			_, err = f.Write(data)
			if err == nil {
				err = f.Chmod(info.Mode().Perm())
			}
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				return "", fmt.Errorf("restore %s: %w", path, err)
			}
		} else if perm, err := restorePerm(path, bak); err != nil {
			return "", fmt.Errorf("restore %s: %w", path, err)
		} else if err := os.WriteFile(path, data, perm); err != nil {
			return "", fmt.Errorf("restore %s: %w", path, err)
		}
		if err := os.Remove(bak); err != nil {
			return "", fmt.Errorf("remove %s: %w", bak, err)
		}
		verbosef("    restored: %s\n", path)
		return "restore", nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("read backup %s: %w", bak, err)
	}

	if !force {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			return "skip", nil
		}
		verbosef("    preserve: %s (no .bak; pass --force to delete)\n", path)
		return "preserve", nil
	}

	if dryRun {
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			return "skip", nil
		}
		verbosef("    remove:  %s\n", path)
		return "remove", nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "skip", nil
		}
		return "", fmt.Errorf("remove %s: %w", path, err)
	}
	verbosef("    removed:  %s\n", path)
	return "remove", nil
}

// runRevertJSON performs the revert and emits a JSON result describing each
// file restored, removed, preserved, or skipped per target.
func runRevertJSON(cmd *cobra.Command, targets []string, dryRun, force, all bool) error {
	cfg, b, err := loadProject(".")
	if err != nil {
		return err
	}
	cfg = cfg.WithAdditionalTargets(targets...)

	out := jsonOutput{Version: "1", Command: "revert"}
	seen := map[string]bool{}
	inSource := sourceTree(cfg)
	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		sess.StartCapture()
		if err := emitAdapter(sess, adapter, b, cfg, true); err != nil {
			sess.StopCapture()
			out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
			continue
		}
		files := sess.StopCapture()
		for _, f := range files {
			if alreadyHandled(seen, f.Path) {
				continue
			}
			action, err := revertOutput(inSource, f.Path, dryRun, force)
			if err != nil {
				out.Errors = append(out.Errors, errorRecord{Target: t, Message: err.Error()})
				continue
			}
			rec := fileRecord{Target: t, Path: f.Path, Action: action, Bytes: 0}
			if action == "skip" || action == "preserve" {
				out.Skipped = append(out.Skipped, rec)
			} else {
				out.Writes = append(out.Writes, rec)
			}
		}
	}

	// Entry-point files are written outside adapter emission; revert them
	// too so the JSON report matches the text path.
	for _, p := range entryPointPaths(cfg, targets) {
		if alreadyHandled(seen, p) {
			continue
		}
		action, err := revertOutput(inSource, p, dryRun, force)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
			continue
		}
		rec := fileRecord{Target: "agnostic-ai", Path: p, Action: action, Bytes: 0}
		if action == "skip" || action == "preserve" {
			out.Skipped = append(out.Skipped, rec)
		} else {
			out.Writes = append(out.Writes, rec)
		}
	}
	companions, err := adapters.AgentsCompanionPaths(cfg, b, targets)
	if err != nil {
		out.Errors = append(out.Errors, errorRecord{Target: "claude", Message: err.Error()})
	}
	for _, p := range companions {
		if alreadyHandled(seen, p) {
			continue
		}
		action, err := revertOutput(inSource, p, dryRun, false)
		if err != nil {
			out.Errors = append(out.Errors, errorRecord{Target: "claude", Message: err.Error()})
			continue
		}
		if action == "restore" {
			out.Writes = append(out.Writes, fileRecord{Target: "claude", Path: p, Action: action})
		}
	}
	if all {
		for _, p := range missingRecordedBackups(cfg) {
			if alreadyHandled(seen, p) {
				continue
			}
			action, err := revertOutput(inSource, p, dryRun, false)
			if err != nil {
				out.Errors = append(out.Errors, errorRecord{Target: "agnostic-ai", Message: err.Error()})
				continue
			}
			if action == "restore" {
				out.Writes = append(out.Writes, fileRecord{Target: "agnostic-ai", Path: p, Action: action})
			}
		}
	}
	return emitJSON(cmd, out)
}

// alreadyHandled reports whether revert already handled path, and marks it
// handled. A second pass over a restored file finds no backup, so with
// --force it would delete the bytes the first pass put back.
func alreadyHandled(seen map[string]bool, path string) bool {
	key := filepath.Clean(path)
	if seen[key] {
		return true
	}
	seen[key] = true
	return false
}

// missingRecordedBackups finds intact sync backups whose output no longer exists.
func missingRecordedBackups(cfg *config.Config) []string {
	var paths []string
	for bak, sum := range readStateFile(".").Backups {
		if !strings.HasSuffix(bak, ".bak") || filepath.IsAbs(bak) {
			continue
		}
		p := strings.TrimSuffix(bak, ".bak")
		clean := filepath.Clean(p)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || cfg.IsUnmanaged(p) || underSymlinkedDir(p) {
			continue
		}
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if sum == "" || fileSum(bak) != sum {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
