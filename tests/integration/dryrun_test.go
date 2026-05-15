package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestInitDryRun_NoFilesWritten(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"init", "--all", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init --dry-run failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "agnostic-ai.yaml")); err == nil {
		t.Error("agnostic-ai.yaml must not exist after init --dry-run")
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai")); err == nil {
		t.Error(".agnostic-ai/ must not exist after init --dry-run")
	}
}

func TestImportDryRun_NoSpecsWritten(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	// Sync first so there is a .claude/ tree to import from.
	syncRoot := cli.NewRootCmd("test")
	syncRoot.SetArgs([]string{"sync", "-t", "claude"})
	if err := syncRoot.Execute(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Remove rule specs so import would recreate them.
	rulesDir := filepath.Join(dir, "rules")
	if err := os.RemoveAll(rulesDir); err != nil {
		t.Fatal(err)
	}

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"import", "claude", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("import --dry-run failed: %v", err)
	}

	// Dry-run must not recreate the removed directory.
	if _, err := os.Stat(rulesDir); err == nil {
		t.Error("rules/ must not be recreated by import --dry-run")
	}
}

func TestSyncDryRun_NoFilesWritten(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --dry-run failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("CLAUDE.md must not exist after sync --dry-run")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude")); err == nil {
		t.Error(".claude/ must not exist after sync --dry-run")
	}
}
