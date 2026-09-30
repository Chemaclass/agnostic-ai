package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func globalConfigTestHome(t *testing.T) (string, string) {
	t.Helper()
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared instructions\n")
	return home, source
}

func assertGlobalInstructions(t *testing.T, home string, want map[string]bool) {
	t.Helper()
	for rel, exists := range want {
		_, err := os.Stat(filepath.Join(home, rel))
		if exists && err != nil {
			t.Errorf("expected %s to be written: %v", rel, err)
		}
		if !exists && !os.IsNotExist(err) {
			t.Errorf("expected %s left alone, got stat error %v", rel, err)
		}
	}
}

func TestSyncGlobal_HomeConfigLimitsSyncAndCheckToItsTargets(t *testing.T) {
	home, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex, cursor]\n")

	out, _, err := runGlobalCheck("sync")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	if !strings.Contains(out, "Synced global configuration to 3 target(s).") {
		t.Errorf("expected three targets synced, got:\n%s", out)
	}
	assertGlobalInstructions(t, home, map[string]bool{
		".claude/CLAUDE.md": true,
		".codex/AGENTS.md":  true,
		".cursor/AGENTS.md": true,
		".gemini/GEMINI.md": false,
		".qoder/AGENTS.md":  false,
	})
	if _, _, err := runGlobalCheck("sync", "--check"); err != nil {
		t.Errorf("sync --global --check must cover only the configured targets: %v", err)
	}
}

func TestSyncGlobal_LocalHomeConfigReplacesSharedTargets(t *testing.T) {
	home, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "targets: [cursor]\n")

	if _, _, err := runGlobalCheck("sync"); err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	assertGlobalInstructions(t, home, map[string]bool{
		".cursor/AGENTS.md": true,
		".claude/CLAUDE.md": false,
		".codex/AGENTS.md":  false,
	})
}

func TestSyncGlobal_FlagsWinOverHomeConfig(t *testing.T) {
	home, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex, cursor]\n")

	if _, _, err := runGlobalCheck("sync", "--only", "codex"); err != nil {
		t.Fatalf("sync --global --only codex: %v", err)
	}
	assertGlobalInstructions(t, home, map[string]bool{
		".codex/AGENTS.md":  true,
		".claude/CLAUDE.md": false,
	})

	_, _, err := runGlobalCheck("sync", "--only", "gemini")
	if err == nil || !strings.Contains(err.Error(), "gemini is not in this run's targets (claude, codex, cursor)") {
		t.Errorf("--only outside the configured targets must name them, got %v", err)
	}

	if _, _, err := runGlobalCheck("sync", "-t", "gemini"); err != nil {
		t.Fatalf("sync --global -t gemini: %v", err)
	}
	assertGlobalInstructions(t, home, map[string]bool{".gemini/GEMINI.md": true})
}

func TestSyncGlobal_HomeConfigUnknownTargetSuggestsName(t *testing.T) {
	home, source := globalConfigTestHome(t)
	config := filepath.Join(source, "local", "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "targets: [claude, curser]\n")

	_, _, err := runGlobalCheck("sync")
	if err == nil || !strings.Contains(err.Error(), config) || !strings.Contains(err.Error(), `unsupported target "curser" (did you mean cursor?)`) {
		t.Fatalf("want the config path and a suggestion, got %v", err)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": false})

	if _, _, err := runGlobalCheck("lint"); err == nil || !strings.Contains(err.Error(), "did you mean cursor?") {
		t.Errorf("lint --global must reject the same config, got %v", err)
	}
}

func TestSyncGlobal_HomeConfigWarnsOnKeysItIgnores(t *testing.T) {
	_, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "version: 1\ntargets: [claude]\nsources:\n  rules: rules\n")

	_, errOut, err := runGlobalCheck("sync")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	want := fmt.Sprintf("warning: %s: global mode reads only targets, requires, lint, on-unsupported, and models; ignoring sources", config)
	if !strings.Contains(errOut, want) {
		t.Errorf("expected %q, got:\n%s", want, errOut)
	}
}

func TestSyncGlobal_WithoutHomeConfigSyncsEveryTarget(t *testing.T) {
	_, _ = globalConfigTestHome(t)

	out, _, err := runGlobalCheck("sync")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	want := fmt.Sprintf("Synced global configuration to %d target(s).", len(globalTargetNames()))
	if !strings.Contains(out, want) {
		t.Errorf("expected %q, got:\n%s", want, out)
	}
}

func TestSyncGlobal_TargetDroppedFromHomeConfigKeepsItsFiles(t *testing.T) {
	home, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "targets: [claude, cursor]\n")
	if _, _, err := runGlobalCheck("sync"); err != nil {
		t.Fatalf("first sync --global: %v", err)
	}

	mustWriteGlobalTest(t, config, "targets: [claude]\n")
	if _, _, err := runGlobalCheck("sync"); err != nil {
		t.Fatalf("second sync --global: %v", err)
	}
	assertGlobalInstructions(t, home, map[string]bool{".cursor/AGENTS.md": true})
}

func TestValidateGlobal_ChecksHookEventsAgainstConfiguredTargets(t *testing.T) {
	_, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "tool.yaml")
	mustWriteGlobalTest(t, hook, "name: tool\nevent: BeforeTool\ncommand: echo hi\n")
	if out, _, err := runGlobalCheck("validate"); err != nil {
		t.Fatalf("a gemini event is valid when gemini is a global target: %v\n%s", err, out)
	}

	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, cursor]\n")
	out, _, err := runGlobalCheck("validate")
	if err == nil {
		t.Fatalf("validate --global must reject an event the configured targets never fire:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, "for enabled targets claude, cursor") {
		t.Errorf("expected the event checked against claude and cursor on %s, got:\n%s", hook, out)
	}
}

func runProjectSync(args ...string) error {
	var out bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"sync"}, args...))
	return cmd.Execute()
}

func TestSync_RefusesToRunInTheGlobalHome(t *testing.T) {
	home, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "local", "rules", "mine.md"), "---\nname: mine\n---\nMine.\n")
	link := filepath.Join(home, "home-link")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	remedy := "run `agnostic-ai sync --global`, or unset AGNOSTIC_AI_HOME if this is a project"
	local := filepath.Join(source, "local")

	for dir, want := range map[string]string{
		source:                                source + " is the global home (AGNOSTIC_AI_HOME); " + remedy,
		link:                                  source + " is the global home (AGNOSTIC_AI_HOME); " + remedy,
		local:                                 local + " is inside the global home " + source + " (AGNOSTIC_AI_HOME); " + remedy,
		filepath.Join(link, "local", "rules"): filepath.Join(local, "rules") + " is inside the global home " + source + " (AGNOSTIC_AI_HOME); " + remedy,
	} {
		testutil.Chdir(t, dir)
		for _, args := range [][]string{{}, {"--check"}, {"--dry-run"}, {"--json"}, {"--plan"}, {"--watch"}} {
			if err := runProjectSync(args...); err == nil || err.Error() != want {
				t.Errorf("sync %v in %s:\nwant %s\ngot  %v", args, dir, want, err)
			}
		}
	}
	for _, rel := range []string{"CLAUDE.md", ".claude", ".agnostic-ai", "local/CLAUDE.md", "local/.agnostic-ai"} {
		if _, err := os.Stat(filepath.Join(source, rel)); !os.IsNotExist(err) {
			t.Errorf("project sync wrote %s into the global home: %v", rel, err)
		}
	}
}

func TestProjectWriters_RefuseToRunInTheGlobalHome(t *testing.T) {
	_, source := globalConfigTestHome(t)
	testutil.Chdir(t, source)
	for _, args := range [][]string{
		{"init", "--all"},
		{"import", "claude"},
		{"new", "rule", "x"},
		{"packs", "add", "./pack"},
		{"packs", "remove", "pack"},
		{"packs", "update"},
		{"cleanup"},
		{"revert"},
		{"install-hook"},
	} {
		var out bytes.Buffer
		cmd := NewRootCmd("test")
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), source+" is the global home (AGNOSTIC_AI_HOME)") {
			t.Errorf("%v in the global home: want the refusal, got %v", args, err)
		}
	}
	for _, rel := range []string{"agnostic-ai.yaml", ".agnostic-ai", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(source, rel)); !os.IsNotExist(err) {
			t.Errorf("a project command wrote %s into the global home: %v", rel, err)
		}
	}
}

func TestSync_GlobalHomeGuardFollowsTheResolvedSourceRoot(t *testing.T) {
	home, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\n")
	unused := filepath.Join(home, ".agnostic-ai")
	mustWriteGlobalTest(t, filepath.Join(unused, "agnostic-ai.yaml"), "targets: [claude]\n")

	testutil.Chdir(t, unused)
	if err := runProjectSync(); err != nil {
		t.Errorf("~/.agnostic-ai is a plain project while AGNOSTIC_AI_HOME points elsewhere: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unused, "CLAUDE.md")); err != nil {
		t.Errorf("expected the project sync to write CLAUDE.md: %v", err)
	}

	t.Setenv("AGNOSTIC_AI_HOME", "")
	want := unused + " is the global home (AGNOSTIC_AI_HOME is unset); run `agnostic-ai sync --global`, or set AGNOSTIC_AI_HOME to another root if this is a project"
	if err := runProjectSync("--check"); err == nil || err.Error() != want {
		t.Errorf("~/.agnostic-ai is the global home once AGNOSTIC_AI_HOME is unset:\nwant %s\ngot  %v", want, err)
	}
}

func TestSyncGlobal_HomeConfigSkipsTargetsWithoutAGlobalSurface(t *testing.T) {
	home, source := globalConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	mustWriteGlobalTest(t, config, "version: 1\ntargets: [claude, aider, continue]\n")

	out, errOut, err := runGlobalCheck("sync")
	if err != nil {
		t.Fatalf("a project-shaped home config must keep sync --global working: %v", err)
	}
	if !strings.Contains(out, "Synced global configuration to 1 target(s).") {
		t.Errorf("expected only claude synced, got:\n%s", out)
	}
	want := "warning: " + config + ": sync --global cannot write aider, continue; skipping them"
	if strings.Count(errOut, "cannot write") != 1 || !strings.Contains(errOut, want) {
		t.Errorf("expected one warning %q, got:\n%s", want, errOut)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})
	for _, command := range []string{"lint", "validate", "list"} {
		if out, _, err := runGlobalCheck(command); err != nil {
			t.Errorf("%s --global with a project-shaped home config: %v\n%s", command, err, out)
		}
	}
}

func TestSyncGlobal_DuplicateHomeConfigTargetSyncsOnce(t *testing.T) {
	_, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex, claude]\n")

	out, _, err := runGlobalCheck("sync")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	if !strings.Contains(out, "Synced global configuration to 2 target(s).") {
		t.Errorf("expected the duplicate dropped, got:\n%s", out)
	}
}

func TestSyncGlobal_TargetFlagBypassesHomeConfig(t *testing.T) {
	home, source := globalConfigTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [curser]\n")

	if _, _, err := runGlobalCheck("sync", "-t", "claude"); err != nil {
		t.Fatalf("-t must not read the home config: %v", err)
	}
	assertGlobalInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})
	if out, _, err := runGlobalCheck("list"); err != nil {
		t.Errorf("list --global needs no targets: %v\n%s", err, out)
	}
	if _, _, err := runGlobalCheck("sync"); err == nil {
		t.Error("sync --global without -t must still reject the broken home config")
	}
}

func TestLintGlobal_ChecksKindsAgainstGlobalSurfaces(t *testing.T) {
	_, source := globalAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "start.yaml")
	mustWriteGlobalTest(t, hook, "name: start\nevent: SessionStart\ncommand: echo hi\n")
	agent := filepath.Join(source, "agents", "reviewer.md")
	mustWriteGlobalTest(t, agent, "---\nname: reviewer\ndescription: Review code\n---\nReview it.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [windsurf, zed]\n")

	out, _, err := runGlobalCheck("validate")
	if err == nil {
		t.Fatalf("validate --global must reject hooks no configured target writes:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, "no enabled target supports hooks. Enable one of: augment, claude, codex, cursor, gemini, qoder") {
		t.Errorf("expected the hook orphan with the global hook targets, got:\n%s", out)
	}

	out, _, _ = runGlobalCheck("lint")
	for _, want := range []string{
		hook + ": hook spec not consumed by any enabled target; targets that support hooks: augment, claude, codex, cursor, gemini, qoder",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, agent) {
		t.Errorf("windsurf writes global agents, so the agent is not dead:\n%s", out)
	}
}

func TestSync_GlobalHomeAtUserHomeGuardsOnlyTheRoot(t *testing.T) {
	home, _ := globalConfigTestHome(t)
	t.Setenv("AGNOSTIC_AI_HOME", home)
	mustWriteGlobalTest(t, filepath.Join(home, "agnostic-ai.yaml"), "targets: [claude]\n")
	project := filepath.Join(home, "proj")
	mustWriteGlobalTest(t, filepath.Join(project, "agnostic-ai.yaml"), "targets: [claude]\n")

	testutil.Chdir(t, home)
	if err := runProjectSync("--check"); err == nil || !strings.Contains(err.Error(), home+" is the global home") {
		t.Errorf("sync in HOME as the global home: want the refusal, got %v", err)
	}
	testutil.Chdir(t, project)
	if err := runProjectSync(); err != nil {
		t.Errorf("a project under HOME must sync when the global home is HOME itself: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Errorf("expected the project sync to write CLAUDE.md: %v", err)
	}
}
