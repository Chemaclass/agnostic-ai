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
	cmd.PrintErrf("Remove kept orphan %s? Its ownership could not be proven. [y/N] ", filepath.ToSlash(path))
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
	state := readStateFile(".")
	recorded, merged := state.Orphans, state.Merged
	if orphanedCount(reports) == 0 {
		return 0, nil
	}
	bundle, err := spec.LoadLayered(resolveLayers(".", cfg))
	if err != nil {
		return 0, err
	}
	bundle.ApplyModelTiers(cfg.Models)
	generated, unloaded, renderErr := orphanGeneratedPaths(cfg, bundle, reports)
	sess := adapters.NewSession()
	sess.SetUnmanaged(cfg.Sync.Unmanaged)
	sess.SetBackup(backup)
	removed, refused := 0, 0
	released := map[string]*mergedOutput{}
	defer func() {
		if err := recordMergedRelease(".", released); err != nil {
			fmt.Fprintf(os.Stderr, "! state file: %v\n", err)
		}
	}()
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
			if len(unloaded) > 0 || renderErr != nil {
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
			// A merged file also holds the user's keys: the confirmation
			// releases sync's keys, the edited ones included, not the file.
			if m, ok := merged[path]; ok && !m.Unrecorded {
				// Confirmation authorizes these bytes only; an edit during the prompt stays.
				if now, err := os.ReadFile(path); err != nil || string(now) != string(data) {
					remaining = append(remaining, path)
					continue
				}
				result, _, err := sess.ReleaseMerged(path, m.Keys, m.Created, true, false)
				if err != nil {
					return removed, err
				}
				if result == adapters.MergedKept {
					remaining = append(remaining, path)
					continue
				}
				if result == adapters.MergedRemoved {
					pruneAncestorDirs(path, pruned)
				}
				released[path] = nil
				removed++
				continue
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
		var causes, fixes []string
		if len(unloaded) > 0 {
			causes = append(causes, "target(s) "+strings.Join(unloaded, ", ")+" could not be loaded")
			fixes = append(fixes, "install or fix them")
		}
		// The drift check already printed renderErr itself.
		if renderErr != nil {
			causes = append(causes, "entry points could not be rendered")
			fixes = append(fixes, "fix the render error")
		}
		keptf("  ~ kept %d orphaned file(s): %s, so what they generate is unknown (%s, then run `agnostic-ai doctor --fix` again)\n", refused, strings.Join(causes, " and "), strings.Join(fixes, " and "))
	}
	return removed, nil
}
