package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// newCleanupCmd builds `agnostic-ai cleanup`. With no flags it removes
// `.bak` files left by `sync --backup` (currently the only cleanup
// mode). `--backups` stays as an explicit opt-in for scripts that want
// the legacy flag form; it has no effect on behavior today. When more
// cleanup modes ship in the future, a bare `cleanup` may opt into
// running every mode while the explicit `--backups` flag continues to
// scope cleanup to .bak removal.
func newCleanupCmd() *cobra.Command {
	var (
		backups bool
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove .bak backups left by `sync --backup`.",
		Long: `cleanup removes housekeeping leftovers from previous agnostic-ai runs.

Today only .bak cleanup is supported. It removes the ` + "`<path>.bak`" + ` backups
that ` + "`sync --backup`" + ` writes, scoped to the paths the configured adapters
emit plus the entry-point files (CLAUDE.md, AGENTS.md, ...). Unrelated
.bak files (vim backups, manual saves, other tools) are never touched.

Pair with --dry-run to preview the deletions without touching disk.`,
		Example: `  # Preview which .bak files would be removed
  agnostic-ai cleanup --dry-run

  # Remove the .bak backups sync --backup wrote (no flag needed)
  agnostic-ai cleanup

  # Explicit form for scripts that prefer to spell the mode out
  agnostic-ai cleanup --backups`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = backups
			return runCleanupBackups(".", dryRun)
		},
	}
	cmd.Flags().BoolVar(&backups, "backups", false, "Explicit form of the default cleanup mode (.bak removal). Kept for scripts; bare `cleanup` does the same thing today.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report intended deletions without touching disk")
	return cmd
}

// runCleanupBackups removes (or lists) the `.bak` backups `sync --backup`
// would have written: one per emitted adapter file plus per entry-point
// file. Scoping to the sync-owned set is the whole point: a blind `*.bak`
// sweep destroys unrelated user backups (#390). Backups that are absent are
// skipped silently.
func runCleanupBackups(root string, dryRun bool) error {
	cfg, b, err := loadProject(root)
	if err != nil {
		return err
	}
	baks, err := syncBackupPaths(cfg, b)
	if err != nil {
		return err
	}
	var removed, listed int
	for _, bak := range baks {
		if _, err := os.Stat(bak); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat %s: %w", bak, err)
		}
		if dryRun {
			summaryf("  would remove %s\n", bak)
			listed++
			continue
		}
		if err := os.Remove(bak); err != nil {
			return fmt.Errorf("remove %s: %w", bak, err)
		}
		verbosef("  removed %s\n", bak)
		removed++
	}
	if dryRun {
		summaryf("cleanup: %d .bak file(s) would be removed (dry-run)\n", listed)
	} else {
		summaryf("cleanup: removed %d .bak file(s)\n", removed)
	}
	return nil
}

// syncBackupPaths returns the `<path>.bak` paths `sync --backup` creates:
// one per emitted adapter file plus per entry-point file, deduplicated. The
// emitted set is captured the same way check and revert compute it, so
// cleanup only ever targets sync-written backups, never unrelated *.bak.
func syncBackupPaths(cfg *config.Config, b spec.Bundle) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		bak := path + ".bak"
		if seen[bak] {
			return
		}
		seen[bak] = true
		out = append(out, bak)
	}
	sess := adapters.NewSession()
	for _, t := range cfg.Targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		sess.StartCapture()
		// dryRun=true: capture the paths without writing anything.
		if err := adapters.EmitWithProvenance(sess, adapter, b, cfg, true); err != nil {
			sess.StopCapture()
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range sess.StopCapture() {
			add(f.Path)
		}
	}
	for _, p := range entryPointPaths(cfg, cfg.Targets) {
		add(p)
	}
	return out, nil
}
