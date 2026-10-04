package cli

import (
	"errors"
	"os"
	"path/filepath"

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
		switch _, err := os.Lstat(filepath.Join(root, config.ConfigFileName)); {
		case err == nil:
			return nil, []migrationSkip{{legacy, config.ConfigFileName + " exists and wins; remove " + config.LegacyConfigFileName + " by hand once it holds nothing you need"}}, nil
		case !errors.Is(err, os.ErrNotExist):
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, []migrationSkip{{legacy, "not a regular file, such as a symlink; rename it by hand"}}, nil
		}
		body, err := os.ReadFile(legacy)
		if err != nil {
			return nil, nil, err
		}
		return []migrationChange{{Path: legacy, NewPath: filepath.Join(root, config.ConfigFileName), Before: string(body), After: string(body)}}, nil, nil
	},
}
