package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
	"github.com/spf13/cobra"
)

func TestGuidedUpgrade_RejectsExternalOrGlobalConfigAliases(t *testing.T) {
	for _, name := range []string{"base config", "local config", "global config", "nested global config"} {
		t.Run(name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			outside := t.TempDir()
			if name == "nested global config" {
				outside = filepath.Join(root, "global")
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			if name == "global config" || name == "nested global config" {
				t.Setenv("AGNOSTIC_AI_HOME", outside)
			}
			const original = "version: 1\nrequires: 0.81.0\n"
			target := filepath.Join(outside, "agnostic-ai.yaml")
			if err := os.WriteFile(target, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(root, "agnostic-ai.yaml")
			if name == "local config" {
				if err := os.WriteFile(alias, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
				alias = filepath.Join(root, "agnostic-ai.local.yaml")
			}
			if err := os.Symlink(target, alias); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			deps := defaultGuidedUpgradeDeps()
			deps.check = func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil }
			deps.install = func(io.Writer, string, string) (string, error) {
				t.Error("outside config offered installation")
				return "", nil
			}
			cmd := &cobra.Command{Use: "status"}
			cmd.SetIn(strings.NewReader("\n"))
			var notes bytes.Buffer
			cmd.SetOut(&notes)
			if handled, err := runGuidedUpgrade(cmd, "0.81.0", deps); handled || err != nil {
				t.Errorf("unsafe upgrade handled=%v error=%v", handled, err)
			}
			if !strings.Contains(notes.String(), target) || strings.Contains(notes.String(), "[Y/n]") {
				t.Errorf("unsafe offer: %s", notes.String())
			}
			if _, err := deps.project(); err == nil {
				t.Error("offered project reconciliation for outside alias")
			}
			if err := deps.reconcile(root, "0.82.0"); err == nil {
				t.Error("reconciled outside alias")
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != original {
				t.Errorf("outside config changed: %s", data)
			}
		})
	}
}

func TestGuidedUpgrade_RechecksRetargetedInternalConfigAlias(t *testing.T) {
	root := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	const original = "version: 1\nrequires: 0.81.0\n"
	inside := filepath.Join(root, "settings.yaml")
	outside := filepath.Join(t.TempDir(), "other.yaml")
	for _, path := range []string{inside, outside} {
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "agnostic-ai.yaml")
	if err := os.Symlink(inside, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	deps := defaultGuidedUpgradeDeps()
	if got, err := deps.project(); err != nil || got == "" {
		t.Fatalf("internal alias rejected: %s, %v", got, err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, alias); err != nil {
		t.Fatal(err)
	}
	if err := deps.reconcile(root, "0.82.0"); err == nil {
		t.Error("retargeted alias reconciled")
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("outside config changed: %s", data)
	}
}

func TestGuidedUpgrade_LocalPackageGuidanceDoesNotRunGlobalInstaller(t *testing.T) {
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	deps := guidedUpgradeDeps{
		check:   func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil },
		project: func() (string, error) { return "/project", nil },
		manualInstall: func(string, string) string {
			return "From /project, run pnpm add -D agnostic-ai@0.82.0, then rerun this command."
		},
		install: func(io.Writer, string, string) (string, error) {
			t.Error("local installation ran global updater")
			return "", nil
		},
		run:       func(string, string, []string, io.Writer) (string, error) { return "agnostic-ai version 0.82.0", nil },
		reconcile: func(string, string) error { t.Error("local installation changed project"); return nil },
	}
	handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
	if handled || err != nil {
		t.Errorf("handled %v, error %v", handled, err)
	}
	if !strings.Contains(out.String(), "pnpm add -D") || strings.Contains(out.String(), "[Y/n]") {
		t.Errorf("output: %s", out.String())
	}
}

func TestGuidedUpgrade_InternalAliasCanReconcile(t *testing.T) {
	root := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	target := filepath.Join(root, "settings.yaml")
	if err := os.WriteFile(target, []byte("version: 1\nrequires: 0.81.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "agnostic-ai.yaml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	deps := defaultGuidedUpgradeDeps()
	if got, err := deps.project(); err != nil || got == "" {
		t.Fatalf("internal alias rejected: %s, %v", got, err)
	}
	if err := deps.reconcile(root, "0.82.0"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "requires: 0.82.0") {
		t.Errorf("internal alias not reconciled: %s", data)
	}
}

func TestNPMUpgradeHint_UsesOwningManagerForLocalAndCachedInstalls(t *testing.T) {
	for _, tc := range []struct {
		name, path, lock, want string
		global                 bool
	}{
		{name: "npm local", path: "node_modules/agnostic-ai/bin/agnostic-ai", want: "npm install -D agnostic-ai@0.82.0"},
		{name: "pnpm local", path: "node_modules/.pnpm/agnostic-ai@0.81.0/node_modules/agnostic-ai/bin/agnostic-ai", lock: "pnpm-lock.yaml", want: "pnpm add -D agnostic-ai@0.82.0"},
		{name: "npx cache", path: ".npm/_npx/cache/node_modules/agnostic-ai/bin/agnostic-ai", want: "npx agnostic-ai@0.82.0"},
		{name: "npm global", path: "lib/node_modules/agnostic-ai/bin/agnostic-ai", global: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			if err := os.WriteFile(filepath.Join(root, "agnostic-ai.yaml"), []byte("version: 1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.lock != "" {
				if err := os.WriteFile(filepath.Join(root, tc.lock), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			exe := filepath.Join(root, filepath.FromSlash(tc.path))
			sourceRoot := root
			if tc.global {
				sourceRoot = t.TempDir()
			}
			setRunningExecutable(t, exe)
			queriedGlobal := false
			hint := npmUpgradeHint(exe, sourceRoot, "0.82.0", func() (string, error) { queriedGlobal = true; return filepath.Join(root, "lib", "node_modules"), nil })
			if tc.want != "" && !strings.Contains(hint, tc.want) {
				t.Errorf("hint %q, want %s", hint, tc.want)
			}
			if tc.global && hint != "" {
				t.Errorf("verified global npm suppressed: %s", hint)
			}
			if !tc.global && queriedGlobal {
				t.Error("local/cache install queried global npm")
			}
		})
	}
}
