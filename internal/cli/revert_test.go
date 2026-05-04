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
