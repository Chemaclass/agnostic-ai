package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func printUpgradeRequiresHint(out io.Writer, version string) {
	if err := refuseGlobalHome(".", ""); err != nil {
		return
	}
	cfg, sources, err := config.LoadWithSources(".")
	if err != nil {
		return
	}
	req, err := config.ParseRequirement(cfg.Requires)
	if err != nil || !req.Above(version) {
		return
	}
	_, _ = fmt.Fprintf(out, "\nProject config (%s) still requires agnostic-ai %s.\n", strings.Join(sources, " + "), req)
	_, _ = fmt.Fprintf(out, "Run `agnostic-ai upgrade --requires` to use installed agnostic-ai %s in this project.\n", strings.TrimPrefix(version, "v"))
	_, _ = fmt.Fprintln(out, "This updates requires and the schema URL, then runs sync.")
}

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
	noteCollapsedRequires(changed)
	if cfg, _, err := loadValidConfig("."); err == nil && noTargetsKept(cfg) {
		warnEmptyTargets(os.Stderr)
	} else if err := runSyncOnce(".", nil, false, false, "", 0); err != nil {
		return fmt.Errorf("sync after reconciling requires to %s: %w", version, err)
	}
	if len(changed) == 0 {
		summaryf("%s project requires and schema already use agnostic-ai %s\n", tick(), version)
	} else {
		paths := make([]string, 0, len(changed))
		for _, c := range changed {
			paths = append(paths, c.Path)
		}
		summaryf("%s adopted agnostic-ai %s in %s\n", tick(), version, strings.Join(paths, ", "))
	}
	if hint := pendingMigrationHint(projectMigrationScope(".")); hint != "" {
		summaryf("%s %s\n", bang(), hint)
	}
	return nil
}

// noteCollapsedRequires names each range that adopting a release turned
// into a minimum, since the user wrote its upper bound on purpose.
func noteCollapsedRequires(changed []config.RequiresChange) {
	for _, c := range changed {
		if c.Collapsed() {
			summaryf("%s %s: requires %q became %q, a minimum without an upper bound\n", bang(), c.Path, c.From, c.To)
		}
	}
}

func installedReleaseVersion() (string, error) {
	if release, ok := stableReleaseVersion(runningVersion); ok {
		return release, nil
	}
	return "", fmt.Errorf("upgrade --requires needs a stable installed release; this build is %q", runningVersion)
}
