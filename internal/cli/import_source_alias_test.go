package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestNewImportSourceAlias_SharesWritesAndKeepsTargetOnRemoval(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	target := filepath.Join(t.TempDir(), "shared guidance \u00e9 \U0001f9ed")
	mustWriteFile(t, filepath.Join(target, "rule.md"), "Original.\n")
	alias, err := newImportSourceAlias(t.TempDir(), target)
	if err != nil {
		t.Fatal(err)
	}
	if alias == target {
		t.Error("alias lost its distinct path")
	}
	aliasInfo, err := os.Stat(alias)
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(aliasInfo, targetInfo) {
		t.Error("alias has a separate physical destination")
	}
	mustWriteFile(t, filepath.Join(alias, "rule.md"), "Replacement.\n")
	if got := readFile(t, filepath.Join(target, "rule.md")); got != "Replacement.\n" {
		t.Errorf("alias write = %q", got)
	}
	if err := os.RemoveAll(alias); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(target, "rule.md")); got != "Replacement.\n" {
		t.Errorf("alias removal changed target = %q", got)
	}
}
