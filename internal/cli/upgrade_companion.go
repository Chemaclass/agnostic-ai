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

type upgradeCompanionDeps struct {
	installed func() (bool, error)
	current   func() (string, bool, error)
	run       func(string, string, []string, io.Writer) (string, error)
}

type companionUpgradeError struct{ cause error }

func (e *companionUpgradeError) Error() string {
	return fmt.Sprintf("CLI upgrade succeeded, but companion update failed: %v. Retry with: agnostic-ai companion install", e.cause)
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
			return companionBuiltFor(context.Background(), path), true, nil
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
	for _, path := range candidates {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		actual, err := deps.run(path, "", []string{"--version"}, io.Discard)
		fields := strings.Fields(actual)
		if err == nil {
			if len(fields) == 0 || (version != "" && !versionsEqual(fields[len(fields)-1], version)) || (version == "" && versionsEqual(fields[len(fields)-1], info.Version)) {
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
			if !found || versionsEqual(builtFor, fields[len(fields)-1]) {
				return nil
			}
		}
		_, err = deps.run(path, "", []string{"companion", "install"}, out)
		return err
	}
	return fmt.Errorf("verify updated executable: %w", errors.Join(problems...))
}
