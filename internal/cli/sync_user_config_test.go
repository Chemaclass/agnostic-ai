package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func userConfigTestHome(t *testing.T) (string, string) {
	t.Helper()
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared instructions\n")
	return home, source
}

func assertUserInstructions(t *testing.T, home string, want map[string]bool) {
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

func TestSyncUser_HomeConfigLimitsSyncAndCheckToItsTargets(t *testing.T) {
	home, source := userConfigTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex, cursor]\n")

	out, _, err := runUserCheck("sync")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	if !strings.Contains(out, "Synced user configuration to 3 target(s).") {
		t.Errorf("expected three targets synced, got:\n%s", out)
	}
	assertUserInstructions(t, home, map[string]bool{
		".claude/CLAUDE.md": true,
		".codex/AGENTS.md":  true,
		".cursor/AGENTS.md": true,
		".gemini/GEMINI.md": false,
		".qoder/AGENTS.md":  false,
	})
	if _, _, err := runUserCheck("sync", "--check"); err != nil {
		t.Errorf("sync --scope user --check must cover only the configured targets: %v", err)
	}
}

func TestSyncUser_LocalHomeConfigReplacesSharedTargets(t *testing.T) {
	home, source := userConfigTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex]\n")
	writeFile(t, filepath.Join(source, "local", "agnostic-ai.yaml"), "targets: [cursor]\n")

	if _, _, err := runUserCheck("sync"); err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	assertUserInstructions(t, home, map[string]bool{
		".cursor/AGENTS.md": true,
		".claude/CLAUDE.md": false,
		".codex/AGENTS.md":  false,
	})
}

func TestSyncUser_FlagsWinOverHomeConfig(t *testing.T) {
	home, source := userConfigTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex, cursor]\n")

	if _, _, err := runUserCheck("sync", "--only", "codex"); err != nil {
		t.Fatalf("sync --scope user --only codex: %v", err)
	}
	assertUserInstructions(t, home, map[string]bool{
		".codex/AGENTS.md":  true,
		".claude/CLAUDE.md": false,
	})

	_, _, err := runUserCheck("sync", "--only", "gemini")
	if err == nil || !strings.Contains(err.Error(), "gemini is not in this run's targets (claude, codex, cursor)") {
		t.Errorf("--only outside the configured targets must name them, got %v", err)
	}

	if _, _, err := runUserCheck("sync", "-t", "gemini"); err != nil {
		t.Fatalf("sync --scope user -t gemini: %v", err)
	}
	assertUserInstructions(t, home, map[string]bool{".gemini/GEMINI.md": true})
}

func TestSyncUser_HomeConfigUnknownTargetSuggestsName(t *testing.T) {
	home, source := userConfigTestHome(t)
	config := filepath.Join(source, "local", "agnostic-ai.yaml")
	writeFile(t, config, "targets: [claude, curser]\n")

	_, _, err := runUserCheck("sync")
	if err == nil || !strings.Contains(err.Error(), config) || !strings.Contains(err.Error(), `unsupported target "curser" (did you mean cursor?)`) {
		t.Fatalf("want the config path and a suggestion, got %v", err)
	}
	assertUserInstructions(t, home, map[string]bool{".claude/CLAUDE.md": false})

	if _, _, err := runUserCheck("lint"); err == nil || !strings.Contains(err.Error(), "did you mean cursor?") {
		t.Errorf("lint --scope user must reject the same config, got %v", err)
	}
}

func TestSyncUser_HomeConfigWarnsOnKeysItIgnores(t *testing.T) {
	_, source := userConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	writeFile(t, config, "version: 1\ntargets: [claude]\nsources:\n  rules: rules\n")

	_, errOut, err := runUserCheck("sync")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	want := fmt.Sprintf("warning: %s: user scope reads only targets, builtins, requires, lint, on-unsupported, models, approvals, and conversations; ignoring sources", config)
	if !strings.Contains(errOut, want) {
		t.Errorf("expected %q, got:\n%s", want, errOut)
	}
}

func TestSyncUser_WithoutHomeConfigSyncsEveryTarget(t *testing.T) {
	_, _ = userConfigTestHome(t)

	out, _, err := runUserCheck("sync")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	want := fmt.Sprintf("Synced user configuration to %d target(s).", len(userTargetNames()))
	if !strings.Contains(out, want) {
		t.Errorf("expected %q, got:\n%s", want, out)
	}
}

func TestSyncUser_TargetDroppedFromHomeConfigKeepsItsFiles(t *testing.T) {
	home, source := userConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	writeFile(t, config, "targets: [claude, cursor]\n")
	if _, _, err := runUserCheck("sync"); err != nil {
		t.Fatalf("first sync --scope user: %v", err)
	}

	writeFile(t, config, "targets: [claude]\n")
	if _, _, err := runUserCheck("sync"); err != nil {
		t.Fatalf("second sync --scope user: %v", err)
	}
	assertUserInstructions(t, home, map[string]bool{".cursor/AGENTS.md": true})
}

func TestValidateUser_ChecksHookEventsAgainstConfiguredTargets(t *testing.T) {
	_, source := userAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "tool.yaml")
	writeFile(t, hook, "name: tool\nevent: BeforeTool\ncommand: echo hi\n")
	if out, _, err := runUserCheck("validate"); err != nil {
		t.Fatalf("a gemini event is valid when gemini is a user target: %v\n%s", err, out)
	}

	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, cursor]\n")
	out, _, err := runUserCheck("validate")
	if err == nil {
		t.Fatalf("validate --scope user must reject an event the configured targets never fire:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, "for targets claude, cursor") {
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

func TestSync_RefusesToRunInTheUserHome(t *testing.T) {
	home, source := userConfigTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\n")
	writeFile(t, filepath.Join(source, "local", "rules", "mine.md"), "---\nname: mine\n---\nMine.\n")
	link := filepath.Join(home, "home-link")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	remedy := "run `agnostic-ai sync --scope user`, or unset AGNOSTIC_AI_HOME if this is a project"
	local := filepath.Join(source, "local")

	for dir, want := range map[string]string{
		source:                                source + " is the user source (AGNOSTIC_AI_HOME); " + remedy,
		link:                                  source + " is the user source (AGNOSTIC_AI_HOME); " + remedy,
		local:                                 local + " is inside the user source " + source + " (AGNOSTIC_AI_HOME); " + remedy,
		filepath.Join(link, "local", "rules"): filepath.Join(local, "rules") + " is inside the user source " + source + " (AGNOSTIC_AI_HOME); " + remedy,
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
			t.Errorf("project sync wrote %s into the user source: %v", rel, err)
		}
	}
}

func TestProjectWriters_RefuseToRunInTheUserHome(t *testing.T) {
	_, source := userConfigTestHome(t)
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
		{"migrate"},
	} {
		var out bytes.Buffer
		cmd := NewRootCmd("test")
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetIn(strings.NewReader(""))
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), source+" is the user source (AGNOSTIC_AI_HOME)") {
			t.Errorf("%v in the user source: want the refusal, got %v", args, err)
		}
	}
	for _, rel := range []string{"agnostic-ai.yaml", ".agnostic-ai", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(source, rel)); !os.IsNotExist(err) {
			t.Errorf("a project command wrote %s into the user source: %v", rel, err)
		}
	}
}

func TestSync_UserHomeGuardFollowsTheResolvedSourceRoot(t *testing.T) {
	home, source := userConfigTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\n")
	unused := filepath.Join(home, ".agnostic-ai")
	writeFile(t, filepath.Join(unused, "agnostic-ai.yaml"), "targets: [claude]\n")

	testutil.Chdir(t, unused)
	if err := runProjectSync(); err != nil {
		t.Errorf("~/.agnostic-ai is a plain project while AGNOSTIC_AI_HOME points elsewhere: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unused, "CLAUDE.md")); err != nil {
		t.Errorf("expected the project sync to write CLAUDE.md: %v", err)
	}

	t.Setenv("AGNOSTIC_AI_HOME", "")
	want := unused + " is the user source (AGNOSTIC_AI_HOME is unset); run `agnostic-ai sync --scope user`, or set AGNOSTIC_AI_HOME to another root if this is a project"
	if err := runProjectSync("--check"); err == nil || err.Error() != want {
		t.Errorf("~/.agnostic-ai is the user source once AGNOSTIC_AI_HOME is unset:\nwant %s\ngot  %v", want, err)
	}
}

func TestSyncUser_HomeConfigSkipsTargetsWithoutAUserSurface(t *testing.T) {
	home, source := userConfigTestHome(t)
	config := filepath.Join(source, "agnostic-ai.yaml")
	writeFile(t, config, "version: 1\ntargets: [claude, aider, continue]\n")

	out, errOut, err := runUserCheck("sync")
	if err != nil {
		t.Fatalf("a project-shaped home config must keep sync --scope user working: %v", err)
	}
	if !strings.Contains(out, "Synced user configuration to 1 target(s).") {
		t.Errorf("expected only claude synced, got:\n%s", out)
	}
	want := "warning: " + config + ": sync --scope user cannot write aider, continue; skipping them"
	if strings.Count(errOut, "cannot write") != 1 || !strings.Contains(errOut, want) {
		t.Errorf("expected one warning %q, got:\n%s", want, errOut)
	}
	assertUserInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})
	for _, command := range []string{"lint", "validate", "list"} {
		if out, _, err := runUserCheck(command); err != nil {
			t.Errorf("%s --scope user with a project-shaped home config: %v\n%s", command, err, out)
		}
	}
}

func TestSyncUser_DuplicateHomeConfigTargetSyncsOnce(t *testing.T) {
	_, source := userConfigTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex, claude]\n")

	out, _, err := runUserCheck("sync")
	if err != nil {
		t.Fatalf("sync --scope user: %v", err)
	}
	if !strings.Contains(out, "Synced user configuration to 2 target(s).") {
		t.Errorf("expected the duplicate dropped, got:\n%s", out)
	}
}

func TestSyncUser_TargetFlagBypassesHomeConfig(t *testing.T) {
	home, source := userConfigTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [curser]\n")

	if _, _, err := runUserCheck("sync", "-t", "claude"); err != nil {
		t.Fatalf("-t must not read the home config: %v", err)
	}
	assertUserInstructions(t, home, map[string]bool{".claude/CLAUDE.md": true})
	if out, _, err := runUserCheck("list"); err != nil {
		t.Errorf("list --scope user needs no targets: %v\n%s", err, out)
	}
	if _, _, err := runUserCheck("sync"); err == nil {
		t.Error("sync --scope user without -t must still reject the broken home config")
	}
}

func TestLintUser_ChecksKindsAgainstUserSurfaces(t *testing.T) {
	checkHookAdvice := func(out, marker string) {
		t.Helper()
		_, advice, ok := strings.Cut(out, marker)
		if !ok {
			t.Errorf("missing user hook advice in:\n%s", out)
			return
		}
		names := strings.Split(strings.SplitN(advice, "\n", 2)[0], ", ")
		for _, target := range []string{"claude", "codebuddy", "grok"} {
			if !slices.Contains(names, target) {
				t.Errorf("user hook advice omits %s: %v", target, names)
			}
		}
		for _, target := range []string{"windsurf", "zed"} {
			if slices.Contains(names, target) {
				t.Errorf("user hook advice lists project-only hooks for %s", target)
			}
		}
	}
	_, source := userAgentTestHome(t)
	hook := filepath.Join(source, "hooks", "start.yaml")
	writeFile(t, hook, "name: start\nevent: SessionStart\ncommand: echo hi\n")
	agent := filepath.Join(source, "agents", "reviewer.md")
	writeFile(t, agent, "---\nname: reviewer\ndescription: Review code\n---\nReview it.\n")
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [windsurf, zed]\n")

	out, _, err := runUserCheck("validate")
	if err == nil {
		t.Fatalf("validate --scope user must reject hooks no configured target writes:\n%s", out)
	}
	if !strings.Contains(out, hook) || !strings.Contains(out, "no enabled target supports hooks.") {
		t.Errorf("expected the hook orphan with the user hook targets, got:\n%s", out)
	}
	checkHookAdvice(out, "Enable one of: ")

	out, _, _ = runUserCheck("lint")
	if !strings.Contains(out, hook+": hook spec not consumed by any enabled target;") {
		t.Errorf("expected the user hook orphan finding, got:\n%s", out)
	}
	checkHookAdvice(out, "targets that support hooks: ")
	if strings.Contains(out, agent) {
		t.Errorf("windsurf writes user agents, so the agent is not dead:\n%s", out)
	}
}

func TestSync_UserSourceAtHomeDirGuardsOnlyTheRoot(t *testing.T) {
	home, _ := userConfigTestHome(t)
	t.Setenv("AGNOSTIC_AI_HOME", home)
	writeFile(t, filepath.Join(home, "agnostic-ai.yaml"), "targets: [claude]\n")
	project := filepath.Join(home, "proj")
	writeFile(t, filepath.Join(project, "agnostic-ai.yaml"), "targets: [claude]\n")

	testutil.Chdir(t, home)
	if err := runProjectSync("--check"); err == nil || !strings.Contains(err.Error(), home+" is the user source") {
		t.Errorf("sync in HOME as the user source: want the refusal, got %v", err)
	}
	testutil.Chdir(t, project)
	if err := runProjectSync(); err != nil {
		t.Errorf("a project under HOME must sync when the user source is HOME itself: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Errorf("expected the project sync to write CLAUDE.md: %v", err)
	}
}
