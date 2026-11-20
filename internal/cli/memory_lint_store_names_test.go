package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryLint_NamesTheRepoPersonalStoreWhenNoStoreExists(t *testing.T) {
	parent := repoMemoryProject(t, false)

	out, err := runRoot(t, "memory", "lint")

	if err != nil || !strings.Contains(out, "No memory store found") {
		t.Fatalf("want a no-store notice (err %v):\n%s", err, out)
	}
	if !strings.Contains(out, filepath.ToSlash(parent)+"/") || strings.Contains(out, ".agnostic-ai/local/memory") {
		t.Errorf("notice must name the repo personal store under %s:\n%s", parent, out)
	}
}
