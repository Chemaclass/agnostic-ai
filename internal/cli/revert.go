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
				if err := adapter.Emit(b, cfg, false); err != nil {
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
	return cmd
}

// revertOne restores path from path+".bak" if the backup exists, else
// removes path. Missing files at either location are ignored.
func revertOne(path string, dryRun bool) error {
	bak := path + ".bak"
	if _, err := os.Stat(bak); err == nil {
		if dryRun {
			fmt.Printf("    restore: %s ← %s\n", path, bak)
			return nil
		}
		data, err := os.ReadFile(bak)
		if err != nil {
			return fmt.Errorf("read backup %s: %w", bak, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("restore %s: %w", path, err)
		}
		if err := os.Remove(bak); err != nil {
			return fmt.Errorf("remove %s: %w", bak, err)
		}
		fmt.Printf("    restored: %s\n", path)
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", bak, err)
	}

	if dryRun {
		fmt.Printf("    remove:  %s\n", path)
		return nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove %s: %w", path, err)
	}
	fmt.Printf("    removed:  %s\n", path)
	return nil
}
