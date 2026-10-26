package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_RefusesRuleWithTargetOverrideCondition(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nglobs: ['*.go']\n---\nGo only.\n")
	_, _, topLevel := runGlobalAgentTest("--only", "qwen")
	if topLevel == nil {
		t.Fatal("top-level globs rule synced globally")
	}
	mustWriteGlobalTest(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nx-qwen:\n  globs: ['*.go']\n---\nGo only.\n")
	_, _, err := runGlobalAgentTest("--only", "qwen")
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

func TestSyncGlobal_OtherTargetOverrideConditionDoesNotBlock(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "rules/go.md"), "---\nname: go\nx-cursor:\n  globs: ['*.go']\n---\nGo only.\n")
	if _, w, err := runGlobalAgentTest("--only", "qwen"); err != nil {
		t.Fatalf("sync: %v\n%s", err, w)
	}
	if got := readGlobalTest(t, filepath.Join(home, ".qwen", "QWEN.md")); !strings.Contains(got, "Go only.") {
		t.Errorf("QWEN.md missing rule body: %s", got)
	}
}
