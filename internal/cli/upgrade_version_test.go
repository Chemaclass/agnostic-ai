package cli

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runUpgradeVersion executes `upgrade` with the given arguments against a
// stub binary install and reports what reached the installer.
func runUpgradeVersion(t *testing.T, installed string, args ...string) (out string, installedVersion string, runs int, err error) {
	t.Helper()
	deps := upgradeDeps{
		detect: func(version string) (upgradeInfo, error) {
			return upgradeInfo{
				Path: "/tmp/agnostic-ai", Method: installBinary,
				Version: version, Latest: "2.0.0",
			}, nil
		},
		run: func(io.Writer, string) error {
			runs++
			return nil
		},
		install: func(_, version, _ string, _ *http.Client) error {
			runs++
			installedVersion = version
			return nil
		},
	}
	root := &cobra.Command{Use: "agnostic-ai", Version: installed, SilenceUsage: true}
	root.AddCommand(newUpgradeCmdWithDeps(deps))
	root.SetArgs(append([]string{"upgrade"}, args...))
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	err = root.Execute()
	return buf.String(), installedVersion, runs, err
}

// A pinned project names a version the installed CLI is ahead of. The
// latest-only path reported nothing to do; naming the version installs it.
func TestUpgradeVersion_InstallsAnOlderReleaseThanInstalled(t *testing.T) {
	out, got, runs, err := runUpgradeVersion(t, "2.0.0", "--version", "v1.5.0")
	if err != nil {
		t.Fatalf("upgrade --version: %v", err)
	}
	if got != "1.5.0" {
		t.Errorf("installed version = %q, want 1.5.0 with the tag's v stripped", got)
	}
	if runs != 1 {
		t.Errorf("install calls = %d, want 1", runs)
	}
	if strings.Contains(out, "Already on latest") {
		t.Errorf("reported latest for an explicitly requested version:\n%s", out)
	}
}

func TestUpgradeVersion_AcceptsATagWithoutTheVPrefix(t *testing.T) {
	_, got, _, err := runUpgradeVersion(t, "2.0.0", "--version", "1.5.0")
	if err != nil {
		t.Fatalf("upgrade --version: %v", err)
	}
	if got != "1.5.0" {
		t.Errorf("installed version = %q, want 1.5.0", got)
	}
}

func TestUpgradeVersion_RequestingTheInstalledVersionDoesNothing(t *testing.T) {
	out, _, runs, err := runUpgradeVersion(t, "2.0.0", "--version", "v2.0.0")
	if err != nil {
		t.Fatalf("upgrade --version: %v", err)
	}
	if runs != 0 {
		t.Errorf("install calls = %d, want 0 when the request is already installed", runs)
	}
	if !strings.Contains(out, "Already on 2.0.0") {
		t.Errorf("output does not say the request is already met:\n%s", out)
	}
}

func TestUpgradeVersion_CheckReportsTheTargetWithoutInstalling(t *testing.T) {
	out, _, runs, err := runUpgradeVersion(t, "2.0.0", "--version", "v1.5.0", "--check")
	if err != nil {
		t.Fatalf("upgrade --check --version: %v", err)
	}
	if runs != 0 {
		t.Errorf("install calls = %d, want 0 under --check", runs)
	}
	if !strings.Contains(out, "1.5.0") {
		t.Errorf("--check does not name the requested version:\n%s", out)
	}
}

// The tag reaches a release URL, so a value that is not a release version
// is refused up front instead of becoming a confusing 404.
func TestUpgradeVersion_RejectsATagThatIsNotAReleaseVersion(t *testing.T) {
	for _, tag := range []string{"latest", "main", "v1.5", "1.5.0/../..", "v1.5.0 && rm -rf /", ""} {
		_, _, runs, err := runUpgradeVersion(t, "2.0.0", "--version", tag)
		if err == nil {
			t.Errorf("tag %q was accepted, want a release-version error", tag)
		}
		if runs != 0 {
			t.Errorf("tag %q reached the installer", tag)
		}
	}
}

// A package-manager install pins through its own package manager, so the
// flag says that rather than running the manager's plain upgrade command,
// which would fetch latest and silently ignore the requested version.
func TestUpgradeVersion_PackageManagerInstallIsRefusedWithItsOwnName(t *testing.T) {
	deps := upgradeDeps{
		detect: func(version string) (upgradeInfo, error) {
			return upgradeInfo{
				Path: "/opt/homebrew/bin/agnostic-ai", Method: installHomebrew,
				Version: version, Latest: "2.0.0", Command: upgradeCommandFor(installHomebrew),
			}, nil
		},
		run: func(io.Writer, string) error { t.Fatal("ran the package manager's latest-only command"); return nil },
		install: func(string, string, string, *http.Client) error {
			t.Fatal("installed a binary over a cask")
			return nil
		},
	}
	root := &cobra.Command{Use: "agnostic-ai", Version: "2.0.0", SilenceUsage: true}
	root.AddCommand(newUpgradeCmdWithDeps(deps))
	root.SetArgs([]string{"upgrade", "--version", "v1.5.0"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil {
		t.Fatal("homebrew install accepted --version")
	}
	if !strings.Contains(err.Error(), "homebrew") {
		t.Errorf("error does not name the install method: %v", err)
	}
}
