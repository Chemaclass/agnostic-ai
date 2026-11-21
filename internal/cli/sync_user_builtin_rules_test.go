package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// The memory built-in ships a rule for Kiro alone: its include lines name
// project files, so they mean nothing in a home file every project reads.
func TestSyncUser_MemoryBuiltinLeavesTheKiroIncludeToProjects(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [kiro, codex]\nbuiltins: [memory]\n")

	if _, warnings, err := runUserAgentTest(); err != nil {
		t.Fatalf("sync --scope user with the memory built-in: %v\n%s", err, warnings)
	}
	for _, path := range []string{
		filepath.Join(home, ".kiro", "steering", "AGENTS.md"),
		filepath.Join(home, ".codex", "AGENTS.md"),
	} {
		got := readFile(t, path)
		if !strings.Contains(got, "shared-memory-policy") {
			t.Errorf("%s lacks the shared memory policy:\n%s", path, got)
		}
		if strings.Contains(got, "#[[file:") || strings.Contains(got, "kiro-memory-index") {
			t.Errorf("%s holds the Kiro include:\n%s", path, got)
		}
	}
	if _, warnings, err := runUserAgentTest("--check"); err != nil {
		t.Errorf("sync --scope user --check after sync: %v\n%s", err, warnings)
	}
}

func TestLintUser_MemoryBuiltinRulesAreNotConditionalRules(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [kiro]\nbuiltins: [memory]\n")

	for _, command := range []string{"lint", "validate"} {
		out, _, err := runUserCheck(command)
		if err != nil || strings.Contains(out, "unconditionally") || strings.Contains(out, "LINT010") {
			t.Errorf("%s --scope user with the memory built-in: %v\n%s", command, err, out)
		}
	}
}

func TestCheckScopeUser_SkipsInactiveBuiltinHooksAndKeepsUserHooks(t *testing.T) {
	_, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [kiro]\nbuiltins: [memory]\n")
	writeFile(t, filepath.Join(source, "hooks", "user-subagent.yaml"), "name: user-subagent\ntargets: [claude]\nevent: SubagentStart\ncommand: echo user-hook\n")

	scope, err := loadCheckScope(true)
	if err != nil {
		t.Fatal(err)
	}
	foundUser := false
	for _, hook := range scope.bundle.Hooks {
		if hook.Name == "memory-subagent-start" {
			t.Error("Claude-only builtin hook reaches the Kiro check scope")
		}
		if hook.Name == "user-subagent" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Error("user hook was removed from global validation")
	}
}
