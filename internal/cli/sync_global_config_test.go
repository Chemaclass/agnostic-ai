package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	mustWriteGlobalTest(t, config, "version: 1\ntargets: [claude]\non-unsupported: error\nsources:\n  rules: rules\n")

	_, errOut, err := runGlobalCheck("sync")
	if err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	want := fmt.Sprintf("warning: %s: global mode reads only targets; ignoring on-unsupported, sources", config)
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
