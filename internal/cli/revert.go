package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func newRevertCmd() *cobra.Command {
	var targets []string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Undo a previous sync. Restores .bak files when present.",
		Long: "revert reverses the effect of `sync` for every target.\n" +
			"For each file the matching adapter would emit:\n" +
			"  - if <path>.bak exists, the .bak content is restored and the .bak removed\n" +
			"  - otherwise the file is removed (it did not exist before sync)\n" +
			"Pair with `sync --backup` so the .bak trail exists. Without backups,\n" +
			"revert simply deletes the generated files.",
		Example: `  # Preview what would be reverted
  agnostic-ai revert --dry-run

  # Restore .bak files where they exist; remove generated files otherwise
  agnostic-ai revert`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				targets = cfg.Targets
			}
			for _, t := range targets {
				adapter, ok := adapters.Get(t)
				if !ok {
					fmt.Fprintf(os.Stderr, "! unknown target: %s\n", t)
					continue
				}
				adapters.StartCapture()
				// Pass dryRun=true so any adapter that writes outside
				// emit.WriteFile (e.g. a future os.Mkdir for an empty
				// output dir) does not side-effect during revert.
				if err := adapter.Emit(b, cfg, true); err != nil {
					adapters.StopCapture()
					return fmt.Errorf("%s: %w", t, err)
				}
				files := adapters.StopCapture()
				fmt.Printf("← revert %s\n", t)
				for _, f := range files {
					if err := revertOne(f.Path, dryRun); err != nil {
						return fmt.Errorf("%s: %w", t, err)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targets, "target", "t", nil, "Targets to revert (default: all in config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report intended actions without touching disk")
	registerTargetCompletion(cmd)
	return cmd
}

// revertOne restores path from path+".bak" if the backup exists, else
// removes path. Missing files at either location are ignored.
func revertOne(path string, dryRun bool) error {
	bak := path + ".bak"
	data, err := os.ReadFile(bak)
	switch {
	case err == nil:
		if dryRun {
			fmt.Printf("    restore: %s ← %s\n", path, bak)
			return nil
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("restore %s: %w", path, err)
		}
		if err := os.Remove(bak); err != nil {
			return fmt.Errorf("remove %s: %w", bak, err)
		}
		fmt.Printf("    restored: %s\n", path)
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("read backup %s: %w", bak, err)
	}

	if dryRun {
		fmt.Printf("    remove:  %s\n", path)
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	fmt.Printf("    removed:  %s\n", path)
	return nil
}
