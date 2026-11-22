package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
)

// The companion sets this for an upgrade whose output is kept in its update log.
const envCompanionUpdate = "AGNOSTIC_AI_COMPANION_UPDATE"

type upgradeCompanionDeps struct {
	installed func() (bool, error)
	current   func() (string, bool, error)
	run       func(string, string, []string, io.Writer) (string, error)
}

type companionUpgradeError struct {
	cause error
	// cliCurrent says the CLI needed no upgrade, only the companion.
	cliCurrent bool
}

func (e *companionUpgradeError) Error() string {
	if e.cliCurrent {
		return fmt.Sprintf("companion update failed: %v. Retry with: agnostic-ai companion install", e.cause)
	}
	return fmt.Sprintf("CLI upgrade succeeded, but companion update failed: %v. Retry with: agnostic-ai companion install", e.cause)
}

// cliNotYetPublished is a package manager that upgraded nothing because
// it does not serve the new release yet.
type cliNotYetPublished struct {
	method        installMethod
	current, want string
}

func (e cliNotYetPublished) Error() string {
	want := e.want
	if want == "" {
		want = "a newer release"
	}
	source := map[installMethod]string{installHomebrew: "Homebrew", installNPM: "npm", installGoInstall: "the Go module proxy"}[e.method]
	if source == "" {
		source = "the install source"
	}
	return fmt.Sprintf("the CLI is still %s: %s does not serve %s yet. Run agnostic-ai upgrade again later", e.current, source, want)
}
func (e *companionUpgradeError) Unwrap() error { return e.cause }

func defaultUpgradeCompanionDeps() *upgradeCompanionDeps {
	return &upgradeCompanionDeps{
		installed: func() (bool, error) {
			dir, err := companionDir()
			if err != nil {
				return false, err
			}
			_, found, err := companionInstalled(dir, runtime.GOOS)
			return found, err
		},
		current: func() (string, bool, error) {
			dir, err := companionDir()
			if err != nil {
				return "", false, err
			}
			path, found, err := companionInstalled(dir, runtime.GOOS)
			if err != nil || !found {
				return "", found, err
			}
			builtFor, _ := companionBuiltFor(context.Background(), path)
			return builtFor, true, nil
		},
		run: runUpdatedBinary,
	}
}

func refreshUpgradedCompanion(out io.Writer, info upgradeInfo, version string, deps upgradeCompanionDeps) error {
	var candidates []string
	if info.Method == installHomebrew {
		for _, marker := range []string{"/Caskroom/", "/Cellar/"} {
			if prefix, _, ok := strings.Cut(filepath.ToSlash(info.Path), marker); ok {
				candidates = append(candidates, filepath.Join(filepath.FromSlash(prefix), "bin", binaryName))
				break
			}
		}
	}
	candidates = append(candidates, info.LinkPath, info.Path)
	seen := map[string]bool{}
	var problems []error
	// stillOld says a candidate reported the version this upgrade started from.
	stillOld := false
	// ranBroken says a candidate could not report its version at all.
	ranBroken := false
	for _, path := range candidates {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		reported, actual, err := reportedCLIVersion(deps.run, path)
		if err != nil {
			ranBroken = true
		}
		if err == nil && reported != "" && info.Version != "" && !versionsEqual(version, info.Version) && versionsEqual(reported, info.Version) {
			stillOld = true
		}
		// A package channel can publish a newer release after the GitHub check. The CLI it installed
		// is already in place, so the companion follows it.
		aheadOfCheck := err == nil && info.Method != installBinary && newerStableRelease(reported, version)
		if aheadOfCheck {
			_, _ = fmt.Fprintf(out, "The package manager installed %s, newer than the release checked (%s).\n", reported, version)
		}
		if err == nil && !aheadOfCheck {
			if reported == "" || (version != "" && !versionsEqual(reported, version)) || (version == "" && versionsEqual(reported, info.Version)) {
				err = fmt.Errorf("%s reports %q, expected updated CLI %s", path, strings.TrimSpace(actual), version)
			}
		}
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if deps.current != nil {
			builtFor, found, err := deps.current()
			if err != nil {
				return err
			}
			if !found || versionsEqual(builtFor, reported) {
				return nil
			}
		}
		_, err = deps.run(path, "", []string{"companion", "install"}, out)
		return err
	}
	if stillOld && !ranBroken {
		return cliNotYetPublished{method: info.Method, current: info.Version, want: version}
	}
	return fmt.Errorf("verify updated executable: %w", errors.Join(problems...))
}
