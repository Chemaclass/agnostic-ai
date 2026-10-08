package cli

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func runUpgradeRequires() error {
	version, err := installedReleaseVersion()
	if err != nil {
		return err
	}
	if err := refuseGlobalHome(".", "edit its requires and schema by hand, then run `agnostic-ai sync --global`"); err != nil {
		return err
	}
	if _, _, err := config.ResolveConfigPath("."); err != nil {
		return err
	}
	lock, err := acquireProjectLock(".", "upgrade --requires")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if _, _, err := config.LoadWithSources("."); err != nil {
		return err
	}
	changed, err := config.PersistRequires(".", version, schemaURL(version))
	if err != nil {
		return fmt.Errorf("reconcile requires: %w", err)
	}
	if err := runSyncOnce(".", nil, false, false, "", 0); err != nil {
		return fmt.Errorf("sync after reconciling requires to %s: %w", version, err)
	}
	if len(changed) == 0 {
		summaryf("%s project requires and schema already use agnostic-ai %s\n", tick(), version)
	} else {
		summaryf("%s adopted agnostic-ai %s in %s\n", tick(), version, strings.Join(changed, ", "))
	}
	if hint := pendingMigrationHint(projectMigrationScope(".")); hint != "" {
		summaryf("%s %s\n", bang(), hint)
	}
	return nil
}

func installedReleaseVersion() (string, error) {
	req, err := config.ParseRequirement(runningVersion)
	if err == nil {
		if allowed, release := req.Allows(runningVersion); allowed && release {
			return req.String(), nil
		}
	}
	return "", fmt.Errorf("upgrade --requires needs a stable installed release; this build is %q", runningVersion)
}
