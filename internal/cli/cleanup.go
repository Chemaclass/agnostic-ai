package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// newCleanupCmd builds `agnostic-ai cleanup`, currently scoped to
// removing the `.bak` files `sync --backup` leaves on disk. Reserved
// for future cleanup modes (orphan generated files, stale sync-state,
// etc.); pass `--backups` explicitly so a future invocation without
// flags can either bundle every mode or stay a no-op.
func newCleanupCmd() *cobra.Command {
	var (
		backups bool
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove agnostic-ai-generated leftovers (currently: .bak files).",
		Long: `cleanup removes housekeeping leftovers from previous agnostic-ai runs.

Today only --backups is supported. It walks the project root, finds every
file ending in '.bak' (the form ` + "`sync --backup`" + ` writes), and removes
them. ` + "`.git/`" + ` and ` + "`.agnostic-ai/`" + ` are skipped.

Pair with --dry-run to preview the deletions without touching disk.`,
		Example: `  # Preview which .bak files would be removed
  agnostic-ai cleanup --backups --dry-run

  # Remove every .bak under the project root
  agnostic-ai cleanup --backups`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !backups {
				return fmt.Errorf("nothing to do: pass --backups (other modes not yet implemented)")
			}
			return runCleanupBackups(".", dryRun)
		},
	}
	cmd.Flags().BoolVar(&backups, "backups", false, "Remove *.bak files left by `sync --backup`")
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
