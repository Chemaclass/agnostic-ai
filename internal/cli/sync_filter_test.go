package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// setupFilterFixture creates a project with targets: [claude, cursor] explicitly
// so filter tests have a controlled, small set to reason about.
func setupFilterFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ntargets: [claude, cursor]\n"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "rules", "r1.md"),
		[]byte("---\nname: r1\n---\nrule body"), 0o644))
	return dir
}

func TestSync_Only_EmitsSubset(t *testing.T) {
	dir := setupFilterFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--only", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".claude/rules/r1.md")); err != nil {
		t.Errorf("expected .claude/rules/r1.md from --only claude: %v", err)
	}
	// cursor should not have been emitted
	if _, err := os.Stat(filepath.Join(dir, ".cursor")); !os.IsNotExist(err) {
		t.Errorf("expected .cursor absent when --only claude, got: %v", err)
	}
}

func TestSync_Except_SkipsTarget(t *testing.T) {
	dir := setupFilterFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--except", "cursor"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".claude/rules/r1.md")); err != nil {
		t.Errorf("expected .claude/rules/r1.md when cursor is excepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor")); !os.IsNotExist(err) {
		t.Errorf("expected .cursor absent when --except cursor, got: %v", err)
	}
}

func TestSync_OnlyAndExcept_MutuallyExclusive(t *testing.T) {
	dir := setupFilterFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--only", "claude", "--except", "cursor"})
	if err := root.Execute(); err == nil {
		t.Error("expected error when --only and --except used together")
	}
}

func TestSync_Only_UnknownTarget_Errors(t *testing.T) {
	dir := setupFilterFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--only", "doesnotexist"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for unknown target in --only")
	}
}

func TestSync_Except_UnknownTarget_Errors(t *testing.T) {
	dir := setupFilterFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--except", "doesnotexist"})
	if err := root.Execute(); err == nil {
		t.Error("expected error for unknown target in --except")
	}
}

func TestRevert_Only_RevertsSubset(t *testing.T) {
	dir := setupFilterFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// sync both targets first
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	claudeRule := filepath.Join(dir, ".claude/rules/r1.md")
	if _, err := os.Stat(claudeRule); err != nil {
		t.Fatalf("expected claude rule after sync: %v", err)
	}

	// revert only claude (with --force so adapter-emitted rules without
	// .bak are deleted; without --force the new default preserves them)
	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "--only", "claude", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(claudeRule); !os.IsNotExist(err) {
		t.Errorf("expected claude rule removed after revert --only claude --force, err=%v", err)
	}
}

func TestRevert_OnlyAndExcept_MutuallyExclusive(t *testing.T) {
	dir := setupFilterFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"revert", "--only", "claude", "--except", "cursor"})
	if err := root.Execute(); err == nil {
		t.Error("expected error when --only and --except used together in revert")
	}
}
