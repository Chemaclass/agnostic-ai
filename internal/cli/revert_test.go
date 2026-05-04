package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRevertOne_RestoresFromBak(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := revertOne(path, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Errorf("expected restored content, got %q", got)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected .bak removed, got err=%v", err)
	}
}

func TestRevertOne_RemovesWhenNoBak(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := revertOne(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed, got err=%v", err)
	}
}

func TestRevertOne_DryRunNoSideEffects(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := revertOne(path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dry-run removed the file: %v", err)
	}
}

func TestRevertOne_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	if err := revertOne(filepath.Join(dir, "ghost.md"), false); err != nil {
		t.Errorf("unexpected error reverting missing file: %v", err)
	}
}

func TestRevertCmd_RemovesGeneratedClaudeMd(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := NewRootCmd("test").Execute(); err != nil {
		// no-op execute to make sure command tree builds
		_ = err
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md after sync: %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("expected CLAUDE.md removed after revert, err=%v", err)
	}
}

func TestRevertCmd_RestoresFromBackup(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Pre-existing CLAUDE.md gets backed up by sync --backup, then a
	// follow-up revert should restore the original.
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("hand-written"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--backup"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hand-written" {
		t.Errorf("expected restored content, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md.bak")); !os.IsNotExist(err) {
		t.Errorf("expected .bak removed after restore")
	}
}

func TestRevertCmd_DryRunNoSideEffects(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Errorf("dry-run revert removed the file: %v", err)
	}
}
