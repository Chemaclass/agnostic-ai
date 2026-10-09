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

func TestGuidedUpgrade_RejectsEntriesAliasedIntoNestedGlobalHome(t *testing.T) {
	for _, name := range []string{"project file", "project kind", "local file", "local kind"} {
		t.Run(name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			home := filepath.Join(root, "global")
			t.Setenv("AGNOSTIC_AI_HOME", home)
			const originalConfig = "version: 1\nrequires: 0.81.0\ntargets: [claude]\n"
			const originalAgent = "---\nname: demo\ntools: [Read]\n---\nagent\n"
			mustWriteFile(t, "agnostic-ai.yaml", originalConfig)
			target := filepath.Join(home, "agents", "demo.md")
			mustWriteFile(t, target, originalAgent)
			source := filepath.Join(root, ".agnostic-ai")
			if strings.HasPrefix(name, "local") {
				source = filepath.Join(source, "local")
			}
			alias := filepath.Join(source, "agents")
			destination := filepath.Dir(target)
			if strings.HasSuffix(name, "file") {
				alias = filepath.Join(alias, "demo.md")
				destination = target
			}
			if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(destination, alias); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			changes, _, err := planCapabilitiesAgentTools(projectMigrationScope(root))
			if err != nil {
				t.Fatal(err)
			}
			if len(changes) != 1 {
				t.Fatalf("global agent migration candidates=%d, want one", len(changes))
			}
			deps := defaultGuidedUpgradeDeps()
			deps.check = func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil }
			deps.manualInstall = nil
			installed := false
			deps.install = func(io.Writer, string, string) (string, error) { installed = true; return "/tool", nil }
			deps.run = func(string, string, []string, io.Writer) (string, error) { return "agnostic-ai version 0.82.0", nil }
			cmd := &cobra.Command{Use: "status"}
			input := &planUpgradeInput{reader: strings.NewReader("\n")}
			var out bytes.Buffer
			cmd.SetIn(input)
			cmd.SetOut(&out)
			handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
			if handled || err != nil || installed || input.reads != 0 {
				t.Errorf("global entry offered upgrade: handled=%v err=%v installed=%v reads=%d", handled, err, installed, input.reads)
			}
			if !strings.Contains(out.String(), "global") || strings.Contains(out.String(), "[Y/n]") {
				t.Errorf("unsafe offer: %s", out.String())
			}
			if err := deps.reconcile(root, "0.82.0"); err == nil {
				t.Error("global entry reconciled")
			}
			if _, err := os.Stat(filepath.Join(root, ".agnostic-ai", projectLockName)); !os.IsNotExist(err) {
				t.Errorf("global entry created project lock: %v", err)
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

func TestGuidedUpgrade_NestedGlobalHomeKeepsBenignSourceAliases(t *testing.T) {
	for _, name := range []string{"internal file", "internal kind", "external custom source"} {
		t.Run(name, func(t *testing.T) {
			root := testutil.TempCwd(t)
			home := filepath.Join(root, "global")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AGNOSTIC_AI_HOME", home)
			targetRoot := filepath.Join(root, "sources", "agents")
			cfg := "version: 1\nrequires: 0.81.0\ntargets: [claude]\n"
			if name == "external custom source" {
				targetRoot = filepath.Join(t.TempDir(), "agents")
				cfg += "sources:\n  agents: '" + filepath.ToSlash(targetRoot) + "'\n"
			}
			mustWriteFile(t, "agnostic-ai.yaml", cfg)
			target := filepath.Join(targetRoot, "demo.md")
			const agent = "---\nname: demo\ntools: [Read]\n---\nagent\n"
			mustWriteFile(t, target, agent)
			if name != "external custom source" {
				alias := filepath.Join(root, ".agnostic-ai", "agents")
				destination := targetRoot
				if name == "internal file" {
					alias = filepath.Join(alias, "demo.md")
					destination = target
				}
				if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(destination, alias); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			changes, _, err := planCapabilitiesAgentTools(projectMigrationScope(root))
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if name == "external custom source" {
				want = 0
			}
			if len(changes) != want {
				t.Errorf("migration candidates=%d, want %d", len(changes), want)
			}
			deps := defaultGuidedUpgradeDeps()
			if got, err := deps.project(); err != nil || got == "" {
				t.Fatalf("benign source refused: %s, %v", got, err)
			}
			if err := deps.reconcile(root, "0.82.0"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != agent {
				t.Errorf("reconciliation changed source: %s", data)
			}
		})
	}
}

func TestGuidedUpgrade_RechecksEntryAliasIntoNestedGlobalBeforeLock(t *testing.T) {
	root := testutil.TempCwd(t)
	home := filepath.Join(root, "global")
	t.Setenv("AGNOSTIC_AI_HOME", home)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\nrequires: 0.81.0\ntargets: [claude]\n")
	const agent = "---\nname: demo\ntools: [Read]\n---\nagent\n"
	inside := filepath.Join(root, "sources", "demo.md")
	outside := filepath.Join(home, "agents", "demo.md")
	mustWriteFile(t, inside, agent)
	mustWriteFile(t, outside, agent)
	alias := filepath.Join(root, ".agnostic-ai", "agents", "demo.md")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inside, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	deps := defaultGuidedUpgradeDeps()
	if _, err := deps.project(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, alias); err != nil {
		t.Fatal(err)
	}
	if err := deps.reconcile(root, "0.82.0"); err == nil {
		t.Error("global retargeted entry reconciled")
	}
	if _, err := os.Stat(filepath.Join(root, ".agnostic-ai", projectLockName)); !os.IsNotExist(err) {
		t.Errorf("unsafe reconciliation created lock: %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != agent {
		t.Errorf("global agent changed: %s", data)
	}
}

func TestGuidedUpgrade_NestedGlobalHomeUsesPhysicalDirectoryIdentity(t *testing.T) {
	root := testutil.TempCwd(t)
	home := filepath.Join(root, "global")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	target := filepath.Join(home, "agents", "demo.md")
	mustWriteFile(t, target, "---\nname: demo\ntools: [Read]\n---\nagent\n")
	aliasHome := filepath.Join(strings.ToUpper(root), "global")
	info, err := os.Stat(aliasHome)
	originalInfo, originalErr := os.Stat(home)
	if err != nil || originalErr != nil || !os.SameFile(info, originalInfo) {
		t.Skip("filesystem does not resolve alternate directory case")
	}
	t.Setenv("AGNOSTIC_AI_HOME", aliasHome)
	alias := filepath.Join(root, ".agnostic-ai", "agents", "demo.md")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := validateGuidedProjectConfigs(root); err == nil {
		t.Error("global source with alternate project spelling accepted")
	}
}

func TestGuidedUpgrade_ShadowedGlobalEntryStillRefusesUpgrade(t *testing.T) {
	root := testutil.TempCwd(t)
	home := filepath.Join(root, "global")
	t.Setenv("AGNOSTIC_AI_HOME", home)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	target := filepath.Join(home, "agents", "demo.md")
	mustWriteFile(t, target, "---\nname: demo\ntools: [Read]\n---\nagent\n")
	mustWriteFile(t, ".agnostic-ai/local/agents/demo.md", "---\nname: demo\ncan: [read]\n---\nlocal agent\n")
	alias := filepath.Join(root, ".agnostic-ai", "agents", "demo.md")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := validateGuidedProjectConfigs(root); err == nil {
		t.Error("local override hid global-linked project entry")
	}
}

func TestGuidedUpgrade_ShadowedGlobalMCPRemainsWritableToRawPlanner(t *testing.T) {
	root := testutil.TempCwd(t)
	home := filepath.Join(root, "global")
	t.Setenv("AGNOSTIC_AI_HOME", home)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	target := filepath.Join(home, "mcps", "demo.yaml")
	const original = "name: demo\ncommand: server\nenv:\n  NODE_ENV: production\n"
	mustWriteFile(t, target, original)
	local := filepath.Join(root, ".agnostic-ai", "local", "mcps", "demo.yaml")
	mustWriteFile(t, local, "name: demo\nenv:\n  NODE_ENV: !literal development\n")
	alias := filepath.Join(root, ".agnostic-ai", "mcps", "demo.yaml")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, bundle, err := loadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.MCPs) != 1 || bundle.MCPs[0].Path != local {
		t.Fatalf("local override did not shadow global MCP: %+v", bundle.MCPs)
	}
	plan, err := planMCPLiterals(projectMigrationScope(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.changes) != 1 || plan.changes[0].Path != alias {
		t.Fatalf("raw planner did not propose global base rewrite: %+v", plan.changes)
	}
	deps := defaultGuidedUpgradeDeps()
	if _, err := deps.project(); err == nil {
		t.Error("shadowed global base MCP passed preflight")
	}
	if err := deps.reconcile(root, "0.82.0"); err == nil {
		t.Error("shadowed global base MCP reconciled")
	}
	if _, err := os.Stat(filepath.Join(root, ".agnostic-ai", projectLockName)); !os.IsNotExist(err) {
		t.Errorf("unsafe reconciliation created lock: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("global MCP changed: %s", data)
	}
}
