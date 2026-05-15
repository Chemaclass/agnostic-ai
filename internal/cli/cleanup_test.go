package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRunCleanupBackups_RemovesBakFiles(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	for _, p := range []string{
		"CLAUDE.md.bak",
		".claude/rules/foo.md.bak",
		".claude/agents/keep.md", // not a .bak; must survive
		"sub/dir/x.bak",
	} {
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
	for _, p := range []string{"CLAUDE.md.bak", ".claude/rules/foo.md.bak", "sub/dir/x.bak"} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("expected %s removed: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/agents/keep.md")); err != nil {
		t.Errorf("unrelated file lost: %v", err)
	}
}

func TestRunCleanupBackups_DryRunListsButDoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	path := filepath.Join(dir, "a.bak")
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

func TestRunCleanupBackups_SkipsGitAndAgnosticDirs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	for _, p := range []string{
		".git/x.bak",
		".agnostic-ai/y.bak",
		"top.bak",
	} {
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
	if _, err := os.Stat(filepath.Join(dir, "top.bak")); !os.IsNotExist(err) {
		t.Errorf("top.bak should be removed")
	}
	for _, p := range []string{".git/x.bak", ".agnostic-ai/y.bak"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("protected dir lost a file: %s err=%v", p, err)
		}
	}
}
