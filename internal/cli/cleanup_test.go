package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Regression for #390: cleanup removes only the `.bak` backups sync wrote
// (emitted target files + entry-point files), never unrelated user `.bak`
// files. setupFixture configures the default targets, so CLAUDE.md and
// .claude/rules/r1.md are in the emitted set; important.bak is not.
func TestRunCleanupBackups_RemovesOnlySyncBackups(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	syncBak := []string{"CLAUDE.md.bak", ".claude/rules/r1.md.bak"}
	userBak := []string{"important.bak", "docs/notes.bak"}
	for _, p := range append(append([]string{}, syncBak...), userBak...) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := runCleanupBackups(".", false); err != nil {
		t.Fatal(err)
	}

	for _, p := range syncBak {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("expected sync backup %s removed: %v", p, err)
		}
	}
	for _, p := range userBak {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("unrelated backup %s must survive: %v", p, err)
		}
	}
}

func TestRunCleanupBackups_DryRunListsButDoesNotDelete(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	path := filepath.Join(dir, "CLAUDE.md.bak")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCleanupBackups(".", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dry-run should not delete: %v", err)
	}
}

// TestCleanupCmd_BareInvocationRemovesBakFiles regresses #219: bare
// `agnostic-ai cleanup` must run the default mode without erroring.
func TestCleanupCmd_BareInvocationRemovesBakFiles(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	path := filepath.Join(dir, "CLAUDE.md.bak")
	if err := os.WriteFile(path, []byte("backup"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"cleanup"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bare cleanup should succeed, got: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected .bak removed by bare cleanup, err=%v", err)
	}
}
