package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssertGoldenTree_RefreshExcludesSourceAndSyncOwnedFiles(t *testing.T) {
	outputDir := t.TempDir()
	expectedDir := filepath.Join(t.TempDir(), "kitsink")
	for path, body := range map[string]string{
		"rule.md":                    "adapter output",
		"AGENTS.md":                  "sync output",
		".agnostic-ai/rules/rule.md": "source spec",
	} {
		fullPath := filepath.Join(outputDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("UPDATE_GOLDEN", "1")
	AssertGoldenTree(t, outputDir, expectedDir, "AGENTS.md")
	got := WalkRel(t, expectedDir)
	if len(got) != 1 || got[0] != "rule.md" {
		t.Fatalf("expected only adapter output, got %v", got)
	}

	t.Setenv("UPDATE_GOLDEN", "0")
	AssertGoldenTree(t, outputDir, expectedDir, "AGENTS.md")
}
