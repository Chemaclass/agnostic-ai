package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func orphanRemovalPrompt(cmd *cobra.Command) func(string) (bool, error) {
	in := cmd.InOrStdin()
	f, ok := in.(*os.File)
	interactive := ok && term.IsTerminal(f.Fd())
	reader := bufio.NewReader(in)
	return func(path string) (bool, error) {
		if !interactive {
			cmd.Printf("  ~ kept orphan %s; run `agnostic-ai doctor --fix` in a terminal to choose removal, or delete it by hand if stale or list it under sync.unmanaged\n", filepath.ToSlash(path))
			return false, nil
		}
		return confirmOrphanRemoval(cmd, reader, path)
	}
}

func confirmOrphanRemoval(cmd *cobra.Command, reader *bufio.Reader, path string) (bool, error) {
	cmd.Printf("Remove kept orphan %s? Its ownership could not be proven. [y/N] ", filepath.ToSlash(path))
	line, err := reader.ReadString('\n')
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("confirm removal of %s: %w", path, err)
	}
	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func offerOrphanRemoval(cfg *config.Config, reports []driftReport, backup bool, confirm func(string) (bool, error)) (int, error) {
	recorded := readStateFile(".").Orphans
	if orphanedCount(reports) == 0 {
		return 0, nil
	}
	bundle, err := spec.LoadLayered(resolveLayers(".", cfg))
	if err != nil {
		return 0, err
	}
	generated, unloaded, err := orphanGeneratedPaths(cfg, bundle, reports)
	if err != nil {
		return 0, err
	}
	sess := adapters.NewSession()
	sess.SetUnmanaged(cfg.Sync.Unmanaged)
	sess.SetBackup(backup)
	removed, refused := 0, 0
	pruned := map[string]bool{}
	for i := range reports {
		if reports[i].Unledgered {
			continue
		}
		var remaining []string
		for _, path := range reports[i].Orphaned {
			if slices.ContainsFunc(generated, func(generatedPath string) bool { return samePath(generatedPath, path) }) {
				continue
			}
			clean := filepath.Clean(path)
			if !slices.Contains(recorded, path) || cfg.IsUnmanaged(path) || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || underSymlinkedDir(path) {
				remaining = append(remaining, path)
				continue
			}
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return removed, fmt.Errorf("%s: %w", path, err)
			}
			if !info.Mode().IsRegular() {
				remaining = append(remaining, path)
				continue
			}
			if len(unloaded) > 0 {
				refused++
				remaining = append(remaining, path)
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return removed, fmt.Errorf("read %s: %w", path, err)
			}
			yes, err := confirm(path)
			if err != nil {
				return removed, err
			}
			if !yes {
				remaining = append(remaining, path)
				continue
			}

			if underSymlinkedDir(path) {
				remaining = append(remaining, path)
				continue
			}
			info, err = os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return removed, fmt.Errorf("%s: %w", path, err)
			}
			if !info.Mode().IsRegular() {
				remaining = append(remaining, path)
				continue
			}

			if backup {
				backupPath := path + ".bak"
				backupInfo, err := os.Lstat(backupPath)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return removed, fmt.Errorf("%s: %w", backupPath, err)
				}
				if err == nil && backupInfo.Mode()&os.ModeSymlink != 0 {
					return removed, fmt.Errorf("%s: refusing symlink backup destination", backupPath)
				}
			}
			// Confirmation authorizes these bytes only; an edit during the prompt stays.
			done, err := sess.RemoveCopy(path, adapters.ContentSum(string(data)), false)
			if err != nil {
				return removed, err
			}
			if !done {
				remaining = append(remaining, path)
				continue
			}
			removed++
			pruneAncestorDirs(path, pruned)
		}
		reports[i].Orphaned = remaining
	}
	if refused > 0 {
		keptf("  ~ kept %d orphaned file(s): target(s) %s could not be loaded, so what they generate is unknown (install or fix them, then run `agnostic-ai doctor --fix` again)\n", refused, strings.Join(unloaded, ", "))
	}
	return removed, nil
}
