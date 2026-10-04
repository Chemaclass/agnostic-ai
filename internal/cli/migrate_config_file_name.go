package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// configFileNameMigration renames the legacy agnostic.config.yaml to
// agnostic-ai.yaml. Both load the same config, so sync writes the same
// files after it.
var configFileNameMigration = specMigration{
	ID:      "config-file-name",
	Group:   "config",
	Release: "0.79.0",
	Summary: "rename " + config.LegacyConfigFileName + " to " + config.ConfigFileName,
	Plan: func(root string) ([]migrationChange, []migrationSkip, error) {
		legacy := filepath.Join(root, config.LegacyConfigFileName)
		info, err := os.Lstat(legacy)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, []migrationSkip{{legacy, "not a regular file, such as a symlink; rename it by hand"}}, nil
		}
		body, err := os.ReadFile(legacy)
		if err != nil {
			return nil, nil, err
		}
		if gitIgnores(root, config.ConfigFileName) && !gitIgnores(root, config.LegacyConfigFileName) {
			return nil, []migrationSkip{{legacy, "git ignores " + config.ConfigFileName + ", so a commit after the migration would drop the config; unignore it, then run migrate again"}}, nil
		}
		current := filepath.Join(root, config.ConfigFileName)
		currentInfo, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return []migrationChange{{Path: legacy, NewPath: current, Before: string(body), After: string(body)}}, nil, nil
		} else if err != nil {
			return nil, nil, err
		}
		// The loader stats the new name, so a broken symlink there loads
		// the legacy file.
		if _, err := os.Stat(current); errors.Is(err, os.ErrNotExist) {
			return nil, []migrationSkip{{legacy, config.ConfigFileName + " is a broken symlink, so this file still loads; fix or remove the symlink by hand"}}, nil
		}
		// A symlink may point at the legacy file, so only a regular file
		// can stand in for it.
		if kept, err := os.ReadFile(current); err == nil && currentInfo.Mode().IsRegular() && string(kept) == string(body) {
			return []migrationChange{{Path: legacy, NewPath: current, Before: string(body), Remove: true}}, nil, nil
		}
		return nil, []migrationSkip{{legacy, config.ConfigFileName + " exists and wins; remove " + config.LegacyConfigFileName + " by hand once it holds nothing you need"}}, nil
	},
}

// gitIgnores reports whether git ignores name under root. A tracked file
// is never ignored; outside a work tree nothing is. check-ignore rejects
// runGit's --literal-pathspecs, so it runs on its own.
func gitIgnores(root, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "check-ignore", "-q", "--", name)
	cmd.Dir = root
	return cmd.Run() == nil
}
