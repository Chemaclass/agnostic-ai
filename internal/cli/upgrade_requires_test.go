package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func executeUpgradeRequiresForTest(t *testing.T, installed string, args ...string) error {
	t.Helper()
	setRunningVersion(t, installed)
	captureLogOut(t)
	root := NewRootCmd("9.9.9")
	root.SetArgs(append([]string{"upgrade", "--requires"}, args...))
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root.Execute()
}

func readUpgradeRequiresFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestUpgradeRequires_UsesInstalledVersionAndPreservesConfig(t *testing.T) {
	requiresProject(t, "0.76.0")
	initial := "# Team configuration.\n# yaml-language-server: $schema=" + schemaURL("0.76.0") + "\nversion: 1\ntargets: [claude]\nrequires: \"0.76.0\" # Keep CI reproducible.\noutputs:\n  claude:\n    rules-dir: .claude/team-rules\n# Keep this trailing comment.\n"
	mustWriteFile(t, "agnostic-ai.yaml", initial)

	if err := executeUpgradeRequiresForTest(t, "v0.77.0"); err != nil {
		t.Fatalf("upgrade --requires: %v", err)
	}
	want := strings.ReplaceAll(initial, "0.76.0", "0.77.0")
	if got := string(readUpgradeRequiresFile(t, "agnostic-ai.yaml")); got != want {
		t.Errorf("config changed beyond the pin and schema:\n got %s\nwant %s", got, want)
	}
	output := filepath.Join(".claude", "team-rules", "safe.md")
	if got := string(readUpgradeRequiresFile(t, output)); !strings.Contains(got, "Be safe.") {
		t.Errorf("sync output lost the rule: %s", got)
	}
	if _, _, err := runAsVersion(t, "v0.77.0", "sync", "--check"); err != nil {
		t.Errorf("reconciled project still drifts: %v", err)
	}
}

func TestUpgradeRequires_ReconcilesLocalOverrides(t *testing.T) {
	for _, value := range []string{"\"0.76.0\"", "null", "\"\""} {
		t.Run(value, func(t *testing.T) {
			requiresProject(t, "0.75.0")
			local := "# Personal budget.\n# yaml-language-server: $schema=" + schemaURL("0.76.0") + "\nrequires: " + value + " # Local override.\nlint:\n  instructions-words: 1234\n"
			mustWriteFile(t, config.LocalOverrideFileName, local)

			if err := executeUpgradeRequiresForTest(t, "v0.77.0"); err != nil {
				t.Fatalf("upgrade with local requires: %v", err)
			}
			cfg, err := config.Load(".")
			if err != nil {
				t.Fatalf("load reconciled config: %v", err)
			}
			if cfg.Requires != "0.77.0" {
				t.Errorf("effective requires = %q, want 0.77.0", cfg.Requires)
			}
			got := string(readUpgradeRequiresFile(t, config.LocalOverrideFileName))
			for _, keep := range []string{"# Personal budget.", "# Local override.", "instructions-words: 1234", schemaURL("0.77.0")} {
				if !strings.Contains(got, keep) {
					t.Errorf("local override lost %q:\n%s", keep, got)
				}
			}
			if got := string(readUpgradeRequiresFile(t, filepath.Join(".claude", "rules", "safe.md"))); !strings.Contains(got, "Be safe.") {
				t.Errorf("local override prevented sync: %s", got)
			}
		})
	}
}

func TestUpgradeRequires_LegacyConfigStillSyncs(t *testing.T) {
	requiresProject(t, "0.76.0")
	if err := os.Rename(config.ConfigFileName, config.LegacyConfigFileName); err != nil {
		t.Fatal(err)
	}
	if err := executeUpgradeRequiresForTest(t, "v0.77.0"); err != nil {
		t.Fatalf("upgrade legacy config: %v", err)
	}
	got := string(readUpgradeRequiresFile(t, config.LegacyConfigFileName))
	if !strings.Contains(got, "0.77.0") || !strings.Contains(got, schemaURL("0.77.0")) {
		t.Errorf("legacy config was not reconciled: %s", got)
	}
	assertAbsent(t, config.ConfigFileName)
	if got := string(readUpgradeRequiresFile(t, filepath.Join(".claude", "rules", "safe.md"))); !strings.Contains(got, "Be safe.") {
		t.Errorf("legacy project was not synced: %s", got)
	}
}

func TestUpgradeRequires_DoesNotDetectOrInstall(t *testing.T) {
	requiresProject(t, "0.76.0")
	setRunningVersion(t, "v0.77.0")
	captureLogOut(t)
	deps := upgradeDeps{
		detect: func(string) (upgradeInfo, error) {
			t.Error("reconciliation detected an installation")
			return upgradeInfo{}, fmt.Errorf("unexpected installation detection")
		},
		run: func(io.Writer, string) error {
			t.Error("reconciliation ran an installer command")
			return fmt.Errorf("unexpected installer command")
		},
		install: func(string, string, string, *http.Client) error {
			t.Error("reconciliation downloaded a release")
			return fmt.Errorf("unexpected release download")
		},
	}
	root := &cobra.Command{Use: "agnostic-ai", Version: "9.9.9", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newUpgradeCmdWithDeps(deps))
	root.SetArgs([]string{"upgrade", "--requires"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("config-only reconciliation: %v", err)
	}
	if got := string(readUpgradeRequiresFile(t, filepath.Join(".claude", "rules", "safe.md"))); !strings.Contains(got, "Be safe.") {
		t.Errorf("config-only operation did not sync: %s", got)
	}
}

func TestUpgradeRequires_RefusesNonReleaseBeforeWrites(t *testing.T) {
	for _, version := range []string{"(devel)", "v0.77.1-0.20261002123456-123456789abc", "v0.78.0-rc.1"} {
		t.Run(version, func(t *testing.T) {
			requiresProject(t, "0.76.0")
			before := readUpgradeRequiresFile(t, config.ConfigFileName)
			err := executeUpgradeRequiresForTest(t, version)
			if err == nil || !strings.Contains(err.Error(), "stable") {
				t.Errorf("nonrelease %q: got %v, want stable-release error", version, err)
			}
			if after := readUpgradeRequiresFile(t, config.ConfigFileName); !bytes.Equal(before, after) {
				t.Error("nonrelease changed configuration")
			}
			assertAbsent(t, filepath.Join(".agnostic-ai", projectLockName), filepath.Join(".claude", "rules", "safe.md"))
		})
	}
}

func TestUpgradeRequires_RejectsExplicitInstallFlags(t *testing.T) {
	for _, flag := range []string{"--version=", "--check=false", "--run=false"} {
		t.Run(flag, func(t *testing.T) {
			requiresProject(t, "0.76.0")
			before := readUpgradeRequiresFile(t, config.ConfigFileName)
			err := executeUpgradeRequiresForTest(t, "v0.77.0", flag)
			if errs.CodeOf(err) != errs.CodeFlagConflict {
				t.Errorf("explicit %s: got %v, want flag conflict", flag, err)
			}
			if after := readUpgradeRequiresFile(t, config.ConfigFileName); !bytes.Equal(before, after) {
				t.Error("conflicting flags changed configuration")
			}
			assertAbsent(t, filepath.Join(".agnostic-ai", projectLockName), filepath.Join(".claude", "rules", "safe.md"))
		})
	}
}

func TestUpgradeRequires_RefusesHeldProjectLock(t *testing.T) {
	requiresProject(t, "0.76.0")
	lock, err := acquireProjectLock(".", "sync --watch")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	before := readUpgradeRequiresFile(t, config.ConfigFileName)
	err = executeUpgradeRequiresForTest(t, "v0.77.0")
	if err == nil || !strings.Contains(err.Error(), "project is locked by agnostic-ai sync --watch") {
		t.Errorf("held project lock: got %v", err)
	}
	if after := readUpgradeRequiresFile(t, config.ConfigFileName); !bytes.Equal(before, after) {
		t.Error("contending reconciliation changed config")
	}
	assertAbsent(t, filepath.Join(".claude", "rules", "safe.md"))
}

func TestUpgradeRequires_KeepsNewPinsWhenSyncFails(t *testing.T) {
	requiresProject(t, "0.76.0")
	initial := "Keep this hand-written instruction.\n"
	mustWriteFile(t, "CLAUDE.md", initial)
	err := executeUpgradeRequiresForTest(t, "v0.77.0")
	if err == nil || !strings.Contains(err.Error(), "CLAUDE.md") {
		t.Errorf("sync conflict: got %v, want the protected instruction path", err)
	}
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatalf("load config after failed sync: %v", err)
	}
	if cfg.Requires != "0.77.0" || !strings.Contains(string(readUpgradeRequiresFile(t, config.ConfigFileName)), schemaURL("0.77.0")) {
		t.Errorf("failed sync restored obsolete pins: requires = %q", cfg.Requires)
	}
	if got := string(readUpgradeRequiresFile(t, "CLAUDE.md")); got != initial {
		t.Errorf("failed sync overwrote hand-written instructions: %s", got)
	}
}

func TestUpgradeRequires_RefusesGlobalHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", home)
	testutil.Chdir(t, home)
	mustWriteFile(t, config.ConfigFileName, "requires: \"0.76.0\"\n")
	before := readUpgradeRequiresFile(t, config.ConfigFileName)
	err := executeUpgradeRequiresForTest(t, "v0.77.0")
	if err == nil || !strings.Contains(err.Error(), "global home") {
		t.Errorf("global-home reconciliation: got %v", err)
	}
	if after := readUpgradeRequiresFile(t, config.ConfigFileName); !bytes.Equal(before, after) {
		t.Error("project reconciliation changed global config")
	}
	assertAbsent(t, filepath.Join(".agnostic-ai", projectLockName))
}
