package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_Check_PassesWhenInSync(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check"})
	if err := root.Execute(); err != nil {
		t.Errorf("check should pass after sync, got: %v", err)
	}
}

func TestDoctor_DetectsMissing(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	err := root.Execute()
	if err == nil {
		t.Error("doctor should fail when no sync has been run")
	}
}

func TestDoctor_Fix_ReconcilesMissingAndStale(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	claudeMd := filepath.Join(dir, ".claude/rules/r1.md")
	original, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeMd, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--fix"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --fix should succeed, got: %v", err)
	}

	got, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("doctor --fix did not restore CLAUDE.md")
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Errorf("doctor should be clean after --fix, got: %v", err)
	}
}

func TestDoctor_Fix_BackupPreservesHandEdits(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	claudeMd := filepath.Join(dir, ".claude/rules/r1.md")
	handEdit := []byte("hand-edited contents\n")
	if err := os.WriteFile(claudeMd, handEdit, 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--fix", "--backup"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	bak, err := os.ReadFile(claudeMd + ".bak")
	if err != nil {
		t.Fatalf("expected .bak file, got: %v", err)
	}
	if string(bak) != string(handEdit) {
		t.Errorf("backup mismatch: got %q, want %q", bak, handEdit)
	}
}

func TestDoctor_DetectsStale(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, ".claude/rules/r1.md"),
		[]byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	if err := root.Execute(); err == nil {
		t.Error("doctor should detect stale claude rule")
	}
}
