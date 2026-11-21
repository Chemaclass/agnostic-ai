package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncUser_RefusesRuleWithTargetOverrideCondition(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nglobs: ['*.go']\n---\nGo only.\n")
	_, _, topLevel := runUserAgentTest("--only", "qwen")
	if topLevel == nil {
		t.Fatal("top-level globs rule synced globally")
	}
	writeFile(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nx-qwen:\n  globs: ['*.go']\n---\nGo only.\n")
	_, _, err := runUserAgentTest("--only", "qwen")
	if err == nil {
		t.Fatal("x-qwen globs rule synced globally")
	}
	if err.Error() != topLevel.Error() {
		t.Errorf("error = %q, want %q", err, topLevel)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".qwen", "QWEN.md")); !os.IsNotExist(statErr) {
		t.Errorf("QWEN.md written for a conditional rule: %v", statErr)
	}
}

func TestSyncUser_OtherTargetOverrideConditionDoesNotBlock(t *testing.T) {
	home, source := userAgentTestHome(t)
	writeFile(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nx-cursor:\n  globs: ['*.go']\n---\nGo only.\n")
	if _, w, err := runUserAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if got := readFile(t, filepath.Join(home, ".qwen", "QWEN.md")); !strings.Contains(got, "Go only.") {
		t.Errorf("QWEN.md missing rule body: %s", got)
	}
}

func TestLintUser_RuleWithTargetOverrideConditionExitsOne(t *testing.T) {
	_, source := userAgentTestHome(t)
	rule := filepath.Join(source, "rules", "go.md")
	writeFile(t, rule, "---\nname: go\nx-claude:\n  globs: ['*.go']\n---\nGo only.\n")

	for _, command := range []string{"lint", "validate"} {
		out, _, err := runUserCheck(command)
		if err == nil {
			t.Fatalf("%s --scope user must exit non-zero on a rule sync --scope user rejects:\n%s", command, out)
		}
		if !strings.Contains(out, rule) || !strings.Contains(out, "scoped or conditional") {
			t.Errorf("%s: expected the x-claude rule reported on %s, got:\n%s", command, rule, out)
		}
	}
}
