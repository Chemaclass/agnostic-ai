package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestGuidedUpgrade_RejectsLinkedWritableSpecLayersBeforeOfferAndLock(t *testing.T) {
	for _, name := range []string{"external local", "global local", "external source", "global source", "nested global local", "nested global source"} {
		t.Run(name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			outside := t.TempDir()
			if strings.HasPrefix(name, "nested") {
				outside = filepath.Join(root, "global")
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			if strings.Contains(name, "global") {
				t.Setenv("AGNOSTIC_AI_HOME", outside)
			}
			const originalConfig = "version: 1\nrequires: 0.81.0\ntargets: [claude]\n"
			const originalAgent = "---\nname: demo\ntools: [Read]\n---\nagent\n"
			mustWriteFile(t, "agnostic-ai.yaml", originalConfig)
			target := filepath.Join(outside, "agents", "demo.md")
			mustWriteFile(t, target, originalAgent)
			alias := filepath.Join(root, ".agnostic-ai")
			if strings.HasSuffix(name, "local") {
				if err := os.Mkdir(alias, 0700); err != nil {
					t.Fatal(err)
				}
				alias = filepath.Join(alias, "local")
			}
			if err := os.Symlink(outside, alias); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			changes, _, err := planCapabilitiesAgentTools(projectMigrationScope(root))
			if err != nil {
				t.Fatal(err)
			}
			wantChanges := 0
			if strings.HasSuffix(name, "local") || strings.HasPrefix(name, "nested") {
				wantChanges = 1
			}
			if len(changes) != wantChanges {
				t.Fatalf("linked layer migration candidates=%d, want %d", len(changes), wantChanges)
			}
			deps := defaultGuidedUpgradeDeps()
			deps.check = func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil }
			deps.manualInstall = nil
			var calls []string
			deps.install = func(io.Writer, string, string) (string, error) { calls = append(calls, "install"); return "/tool", nil }
			deps.run = func(string, string, []string, io.Writer) (string, error) {
				calls = append(calls, "updated command")
				return "agnostic-ai version 0.82.0", nil
			}
			cmd := &cobra.Command{Use: "status"}
			input := &planUpgradeInput{reader: strings.NewReader("\n")}
			var out bytes.Buffer
			cmd.SetIn(input)
			cmd.SetOut(&out)
			handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
			if handled || err != nil {
				t.Errorf("unsafe offer handled=%v err=%v", handled, err)
			}
			if len(calls) != 0 || input.reads != 0 || strings.Contains(out.String(), "[Y/n]") {
				t.Errorf("unsafe upgrade calls=%v reads=%d output=%s", calls, input.reads, out.String())
			}
			if err := deps.reconcile(root, "0.82.0"); err == nil {
				t.Error("unsafe layer reconciled")
			}
			for _, path := range []string{filepath.Join(root, ".agnostic-ai", projectLockName), filepath.Join(outside, projectLockName)} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("unsafe upgrade created lock %s: %v", path, err)
				}
			}
			for path, want := range map[string]string{"agnostic-ai.yaml": originalConfig, target: originalAgent} {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != want {
					t.Errorf("%s changed: %s", path, data)
				}
			}
		})
	}
}

func TestGuidedUpgrade_InternalSpecDirectoryAliasesCanReconcile(t *testing.T) {
	root := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\nrequires: 0.81.0\n")
	source := filepath.Join(root, "sources")
	local := filepath.Join(root, "personal")
	for _, dir := range []string{source, local} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(source, filepath.Join(root, ".agnostic-ai")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(local, filepath.Join(source, "local")); err != nil {
		t.Fatal(err)
	}
	deps := defaultGuidedUpgradeDeps()
	if project, err := deps.project(); err != nil || project == "" {
		t.Fatalf("internal source aliases rejected: %s, %v", project, err)
	}
	if err := deps.reconcile(root, "0.82.0"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("agnostic-ai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "requires: 0.82.0") {
		t.Errorf("internal aliases not reconciled: %s", data)
	}
}

func TestGuidedUpgrade_RechecksRetargetedSpecDirectoryBeforeLock(t *testing.T) {
	root := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\nrequires: 0.81.0\n")
	inside := filepath.Join(root, "sources")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, ".agnostic-ai")
	if err := os.Symlink(inside, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	deps := defaultGuidedUpgradeDeps()
	if _, err := deps.project(); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, alias); err != nil {
		t.Fatal(err)
	}
	if err := deps.reconcile(root, "0.82.0"); err == nil {
		t.Error("retargeted directory reconciled")
	}
	if _, err := os.Stat(filepath.Join(outside, projectLockName)); !os.IsNotExist(err) {
		t.Errorf("outside lock created: %v", err)
	}
}
