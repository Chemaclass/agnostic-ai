package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// The memory built-in ships a rule for Kiro alone: its include lines name
// project files, so they mean nothing in a home file every project reads.
func TestSyncGlobal_MemoryBuiltinLeavesTheKiroIncludeToProjects(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [kiro, codex]\nbuiltins: [memory]\n")

	if _, warnings, err := runGlobalAgentTest(); err != nil {
		t.Fatalf("sync --global with the memory built-in: %v\n%s", err, warnings)
	}
	for _, path := range []string{
		filepath.Join(home, ".kiro", "steering", "AGENTS.md"),
		filepath.Join(home, ".codex", "AGENTS.md"),
	} {
		got := readGlobalTest(t, path)
		if !strings.Contains(got, "shared-memory-policy") {
			t.Errorf("%s lacks the shared memory policy:\n%s", path, got)
		}
		if strings.Contains(got, "#[[file:") || strings.Contains(got, "kiro-memory-index") {
			t.Errorf("%s holds the Kiro include:\n%s", path, got)
		}
	}
	if _, warnings, err := runGlobalAgentTest("--check"); err != nil {
		t.Errorf("sync --global --check after sync: %v\n%s", err, warnings)
	}
}

func TestLintGlobal_MemoryBuiltinRulesAreNotConditionalRules(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [kiro]\nbuiltins: [memory]\n")

	for _, command := range []string{"lint", "validate"} {
		out, _, err := runGlobalCheck(command)
		if err != nil || strings.Contains(out, "unconditionally") || strings.Contains(out, "LINT010") {
			t.Errorf("%s --global with the memory built-in: %v\n%s", command, err, out)
		}
	}
}
