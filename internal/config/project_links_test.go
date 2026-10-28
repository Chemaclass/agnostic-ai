package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func linkProject(t *testing.T) (project, outside string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	return t.TempDir(), t.TempDir()
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestCheckBookkeepingPath_AllowsALinkedSpecFolderOnlyWhenItHoldsSpecs(t *testing.T) {
	project, outside := linkProject(t)
	shared := filepath.Join(outside, "shared-specs")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, shared, filepath.Join(project, ".agnostic-ai"))
	ledger := filepath.Join(project, ".agnostic-ai", ".sync-state")

	if err := CheckBookkeepingPath(project, ledger); err == nil {
		t.Error("the ledger in a linked folder that holds no specs must be refused")
	}
	if err := os.WriteFile(filepath.Join(shared, "AGNOSTIC_AI.md"), []byte("Be brief.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckBookkeepingPath(project, ledger); err != nil {
		t.Errorf("the ledger in a linked spec folder must stay writable: %v", err)
	}
	if err := CheckProjectLinks(project, filepath.Join(project, ".agnostic-ai", "rules", "r1.md"), "write", false); err == nil {
		t.Error("an output written through the linked spec folder must be refused")
	}
	symlinkOrSkip(t, ".agnostic-ai", filepath.Join(project, ".claude"))
	if err := CheckProjectLinks(project, filepath.Join(project, ".claude", "settings.json"), "write", false); err == nil {
		t.Error("an output folder linked into the linked spec folder must be refused")
	}
	symlinkOrSkip(t, filepath.Join(outside, "profile"), filepath.Join(shared, ".sync-state"))
	err := CheckBookkeepingPath(project, ledger)
	if err == nil || !strings.Contains(err.Error(), "is a link to "+filepath.Join(outside, "profile")) {
		t.Errorf("a ledger linked elsewhere must be refused, got %v", err)
	}
}
