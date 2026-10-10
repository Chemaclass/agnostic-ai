package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const envNoUpdateCheck = "AGNOSTIC_AI_NO_UPDATE_CHECK"
const envUpgradeInProgress = "AGNOSTIC_AI_UPGRADE_IN_PROGRESS"

type upgradeOffer struct {
	Latest        string
	Guidance      string
	GuidanceError string
}

type guidedUpgradeDeps struct {
	manualInstall func(string, string) string
	check         func(string) (upgradeOffer, error)
	project       func() (string, error)
	install       func(io.Writer, string, string) (string, error)
	run           func(string, string, []string, io.Writer) (string, error)
	reconcile     func(string, string) error
}

func guidedUpgradeEachRun(cmd *cobra.Command) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if automaticUpgradeEligible(c, runningVersion) {
				handled, err := runGuidedUpgrade(c, runningVersion, defaultGuidedUpgradeDeps())
				if handled || err != nil {
					return err
				}
			}
			return run(c, args)
		}
	}
	for _, sub := range cmd.Commands() {
		guidedUpgradeEachRun(sub)
	}
}

func automaticUpgradeEligible(cmd *cobra.Command, version string) bool {
	out, ok := cmd.OutOrStdout().(*os.File)
	interactive := stdinIsTerminal(cmd.InOrStdin()) && ok && term.IsTerminal(out.Fd())
	return automaticUpgradeAllowed(cmd, version, interactive)
}

func automaticUpgradeAllowed(cmd *cobra.Command, version string, interactive bool) bool {
	if !stableRelease(version) || os.Getenv(envNoUpdateCheck) != "" || os.Getenv(envUpgradeInProgress) != "" || os.Getenv("CI") != "" || os.Getenv("AGNOSTIC_AI_TARGET") != "" {
		return false
	}
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "hook", "project", "install-hook", "lsp", "completion", "upgrade", "update", "use":
			return false
		}
	}
	for _, name := range []string{"quiet", "json", "global", "watch", "check", "dry-run", "plan", "list"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Value.String() == "true" {
			return false
		}
	}
	if f := cmd.Flags().Lookup("format"); f != nil && f.Value.String() == "json" {
		return false
	}
	return interactive
}

func defaultGuidedUpgradeDeps() guidedUpgradeDeps {
	return guidedUpgradeDeps{
		check:         cachedUpgradeOffer,
		manualInstall: guidedNPMUpgradeHint,
		project: func() (string, error) {
			if err := refuseGlobalHome(".", "upgrade global specs separately"); err != nil {
				return "", nil
			}
			path, _, err := config.ResolveConfigPath(".")
			if err != nil {
				return "", nil
			}
			root, err := filepath.Abs(filepath.Dir(path))
			if err != nil {
				return "", fmt.Errorf("resolve project root: %w", err)
			}
			if err := validateGuidedProjectConfigs(root); err != nil {
				return "", err
			}
			return root, nil
		},
		install: func(out io.Writer, current, latest string) (string, error) {
			info, err := detectUpgradeInstallation(current)
			if err != nil {
				return "", err
			}
			info.Latest = latest
			deps := defaultUpgradeDeps()
			deps.detect = func(string) (upgradeInfo, error) { return info, nil }
			if err := runUpgradeWithDeps(out, false, "", current, deps); err != nil {
				return "", err
			}
			return info.LinkPath, nil
		},
		run: runUpdatedBinary,
		reconcile: func(root, version string) error {
			if err := validateGuidedProjectConfigs(root); err != nil {
				return err
			}
			lock, err := acquireProjectLock(root, "guided upgrade")
			if err != nil {
				return err
			}
			defer func() { _ = lock.Close() }()
			if err := validateGuidedProjectConfigs(root); err != nil {
				return err
			}
			_, err = config.PersistRequires(root, version, schemaURL(version))
			if err != nil {
				return fmt.Errorf("reconcile requires and schema: %w", err)
			}
			return nil
		},
	}
}

func runUpdatedBinary(path, root string, args []string, out io.Writer) (string, error) {
	cmd := exec.Command(path, args...)
	cmd.Dir = root
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), envUpgradeInProgress+"=1", envNoUpdateCheck+"=1")
	var captured bytes.Buffer
	cmd.Stdout = io.MultiWriter(out, &captured)
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return captured.String(), fmt.Errorf("%s %s: %w", path, strings.Join(args, " "), err)
	}
	return captured.String(), nil
}

func runGuidedUpgrade(cmd *cobra.Command, current string, deps guidedUpgradeDeps) (bool, error) {
	offer, err := deps.check(current)
	if err != nil || !newerStableRelease(offer.Latest, current) {
		return false, nil
	}
	root, err := deps.project()
	out := cmd.OutOrStdout()
	if err != nil {
		_, _ = fmt.Fprintf(out, "Automatic project upgrade skipped: %v. Continuing your requested command.\n", err)
		return false, nil
	}
	if deps.manualInstall != nil {
		if hint := deps.manualInstall(root, offer.Latest); hint != "" {
			_, _ = fmt.Fprintf(out, "Update available: %s -> %s\n%s\nContinuing your requested command.\n", strings.TrimPrefix(current, "v"), offer.Latest, hint)
			return false, nil
		}
	}
	_, _ = fmt.Fprintf(out, "Update available: %s -> %s\nRelease guidance: %s/tag/v%s\n", strings.TrimPrefix(current, "v"), offer.Latest, releasesHTMLURL, offer.Latest)
	if offer.Guidance == "" && offer.GuidanceError == "" {
		offer.GuidanceError = "release notes could not be loaded"
	}
	if offer.Guidance != "" {
		_, _ = fmt.Fprintln(out, offer.Guidance)
	}
	if offer.GuidanceError != "" {
		_, _ = fmt.Fprintf(out, "Release guidance unavailable: %s. Review the release page for manual steps.\n", offer.GuidanceError)
	}
	if root != "" {
		_, _ = fmt.Fprintf(out, "This updates the tool, sets this project's requires to exact %s (replacing any range or pin, including existing local overrides), and sets its schema to %s.\nThen it previews and applies built-in migrations, runs sync and sync --check in %s.\n", offer.Latest, schemaURL(offer.Latest), root)
	} else {
		_, _ = fmt.Fprintln(out, "This updates the tool only. No current project will be changed.")
	}
	_, _ = fmt.Fprintln(out, "The requested command will not run after an accepted upgrade. Rerun it with the updated tool.")
	_, _ = fmt.Fprint(out, "Update agnostic-ai")
	if root != "" {
		_, _ = fmt.Fprint(out, " and migrate this project")
	}
	_, _ = fmt.Fprint(out, "? [Y/n] ")
	input, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(input))
	if answer != "" && answer != "y" && answer != "yes" {
		return false, nil
	}
	path, err := deps.install(out, current, offer.Latest)
	if err != nil {
		return true, fmt.Errorf("guided upgrade installation failed; project migrations did not start. Run agnostic-ai upgrade to retry: %w", err)
	}
	actual, err := deps.run(path, "", []string{"--version"}, io.Discard)
	fields := strings.Fields(actual)
	if err != nil || len(fields) == 0 || !versionsEqual(fields[len(fields)-1], offer.Latest) {
		if err == nil {
			err = fmt.Errorf("installed version is %q, expected %s", strings.TrimSpace(actual), offer.Latest)
		}
		return true, fmt.Errorf("verify updated executable; project changes did not start. Check agnostic-ai upgrade --check: %w", err)
	}
	if root != "" {
		if err := deps.reconcile(root, offer.Latest); err != nil {
			return true, fmt.Errorf("agnostic-ai %s is installed, but this project's requires and schema could not be updated; project migrations did not start. Resolve the reported project or config problem, then run agnostic-ai upgrade --requires, agnostic-ai migrate --dry-run, agnostic-ai migrate, agnostic-ai sync, and agnostic-ai sync --check in %s: %w", offer.Latest, root, err)
		}
		for _, args := range [][]string{{"migrate", "--dry-run"}, {"migrate"}, {"sync"}, {"sync", "--check"}} {
			_, _ = fmt.Fprintf(out, "\nRunning: agnostic-ai %s\n", strings.Join(args, " "))
			if _, err := deps.run(path, root, args, out); err != nil {
				return true, fmt.Errorf("guided upgrade stopped at %s; the tool and project pins are updated. Resolve the reported problem, then run migrate, sync, and sync --check in %s: %w", strings.Join(args, " "), root, err)
			}
		}
	}
	_, _ = fmt.Fprintln(out, "\nUpgrade complete. Review the release guidance and any migration skips above for remaining manual steps. Rerun your original command.")
	return true, nil
}
