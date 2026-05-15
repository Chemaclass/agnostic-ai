package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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
		Short: "Remove agnostic-ai-generated leftovers (currently: .bak files).",
		Long: `cleanup removes housekeeping leftovers from previous agnostic-ai runs.

Today only .bak cleanup is supported. It walks the project root, finds
every file ending in '.bak' (the form ` + "`sync --backup`" + ` writes), and
removes them. ` + "`.git/`" + ` and ` + "`.agnostic-ai/`" + ` are skipped.

Pair with --dry-run to preview the deletions without touching disk.`,
		Example: `  # Preview which .bak files would be removed
  agnostic-ai cleanup --dry-run

  # Remove every .bak under the project root (no flag needed)
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

// runCleanupBackups walks root and deletes (or lists) every file whose
// name ends in `.bak`. `.git/` and `.agnostic-ai/` are pruned so the
// command never reaches under VCS metadata or the managed state dir.
func runCleanupBackups(root string, dryRun bool) error {
	skipDirs := map[string]bool{".git": true, ".agnostic-ai": true}
	var removed, listed int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".bak") {
			return nil
		}
		if dryRun {
			summaryf("  would remove %s\n", path)
			listed++
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		verbosef("  removed %s\n", path)
		removed++
		return nil
	})
	if err != nil {
		return err
	}
	if dryRun {
		summaryf("cleanup: %d .bak file(s) would be removed (dry-run)\n", listed)
	} else {
		summaryf("cleanup: removed %d .bak file(s)\n", removed)
	}
	return nil
}
